package systems

import (
	"context"
	"fmt"
	"sync"
	"time"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"

	"go.uber.org/zap"
)

const (
	batchSize                    = 100
	batchTimeout                 = 500 * time.Millisecond
	characterSaveTimeout         = 10 * time.Second
	characterSaveShutdownTimeout = 50 * time.Second
)

type characterSaveQueue struct {
	mu      sync.Mutex
	pending map[int64]*CharacterSnapshot
	closed  bool
	wake    chan struct{}

	// Capture, persistence and acknowledgement share this lock with SaveSync.
	// A queued older snapshot must never write after a synchronous final save.
	writeMu sync.Mutex
}

type CharacterSaver struct {
	queues         []characterSaveQueue
	numWorkers     int
	logger         *zap.Logger
	wg             sync.WaitGroup
	stop           chan struct{}
	stopOnce       sync.Once
	inventorySaver InventorySaverInterface
	persistBatch   func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error
}

func NewCharacterSaver(db *persistence.Postgres, numWorkers int, inventorySaver InventorySaverInterface, logger *zap.Logger) *CharacterSaver {
	return newCharacterSaver(numWorkers, inventorySaver, logger, func(ctx context.Context, characters repository.UpdateCharactersParams, inventories repository.UpsertInventoriesParams) error {
		return db.WithTx(ctx, func(queries *repository.Queries) error {
			if err := queries.UpdateCharacters(ctx, characters); err != nil {
				return fmt.Errorf("update characters: %w", err)
			}
			if len(inventories.OwnerIds) > 0 {
				if err := queries.UpsertInventories(ctx, inventories); err != nil {
					return fmt.Errorf("upsert inventories: %w", err)
				}
			}
			return nil
		})
	})
}

func newCharacterSaver(numWorkers int, inventorySaver InventorySaverInterface, logger *zap.Logger, persistBatch func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error) *CharacterSaver {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &CharacterSaver{
		queues:         make([]characterSaveQueue, max(1, numWorkers)),
		numWorkers:     numWorkers,
		logger:         logger,
		stop:           make(chan struct{}),
		inventorySaver: inventorySaver,
		persistBatch:   persistBatch,
	}
	for i := range s.queues {
		s.queues[i].pending = make(map[int64]*CharacterSnapshot)
		s.queues[i].wake = make(chan struct{}, 1)
	}
	for i := 0; i < numWorkers; i++ {
		s.wg.Add(1)
		go s.saveWorker(&s.queues[i])
	}
	return s
}

func (s *CharacterSaver) queueForCharacter(characterID int64) *characterSaveQueue {
	return &s.queues[uint64(characterID)%uint64(len(s.queues))]
}

func (s *CharacterSaver) enqueueSnapshot(snapshot CharacterSnapshot) bool {
	queue := s.queueForCharacter(snapshot.CharacterID)
	queue.mu.Lock()
	if queue.closed {
		queue.mu.Unlock()
		s.logger.Error("Cannot enqueue character snapshot after saver stopped", zap.Int64("character_id", snapshot.CharacterID))
		return false
	}
	// Keep one complete snapshot per unsaved character, including after despawn.
	// A full channel used to silently lose the final snapshot during disconnect bursts.
	queue.pending[snapshot.CharacterID] = &snapshot
	ready := len(queue.pending) >= batchSize
	queue.mu.Unlock()
	if ready {
		queue.notify()
	}
	return true
}

func (q *characterSaveQueue) notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *characterSaveQueue) pendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (s *CharacterSaver) saveWorker(queue *characterSaveQueue) {
	defer s.wg.Done()
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()
	var retryAfter time.Time
	for {
		select {
		case <-s.stop:
			s.drainQueue(queue)
			return
		case <-queue.wake:
		case <-ticker.C:
		}
		if time.Now().Before(retryAfter) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), characterSaveTimeout)
		err := s.flushPending(ctx, queue, 0)
		cancel()
		if err != nil {
			s.logger.Error("Character save failed; snapshots retained for retry", zap.Int("pending_characters", queue.pendingCount()), zap.Error(err))
			// New arrivals must not turn an unavailable database into a tight retry loop.
			retryAfter = time.Now().Add(batchTimeout)
		} else if queue.pendingCount() >= batchSize {
			queue.notify()
		}
	}
}

// characterID zero flushes a bounded batch; SaveSync targets only its own character.
func (s *CharacterSaver) flushPending(ctx context.Context, queue *characterSaveQueue, characterID int64) error {
	queue.writeMu.Lock()
	defer queue.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	queue.mu.Lock()
	batch := make([]*CharacterSnapshot, 0, min(batchSize, len(queue.pending)))
	if characterID != 0 {
		if snapshot := queue.pending[characterID]; snapshot != nil {
			batch = append(batch, snapshot)
		}
	} else {
		for _, snapshot := range queue.pending {
			batch = append(batch, snapshot)
			if len(batch) == batchSize {
				break
			}
		}
	}
	queue.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	characters, inventories := characterSaveParams(batch)
	if err := s.persistBatch(ctx, characters, inventories); err != nil {
		return err
	}
	queue.mu.Lock()
	for _, snapshot := range batch {
		// Enqueue can replace this snapshot while the database write is in flight.
		if queue.pending[snapshot.CharacterID] == snapshot {
			delete(queue.pending, snapshot.CharacterID)
		}
	}
	queue.mu.Unlock()
	return nil
}

func characterSaveParams(batch []*CharacterSnapshot) (repository.UpdateCharactersParams, repository.UpsertInventoriesParams) {
	characters := repository.UpdateCharactersParams{}
	inventories := repository.UpsertInventoriesParams{}
	type inventoryKey struct {
		ownerID int64
		kind    int16
		key     int16
	}
	inventoryIndexes := make(map[inventoryKey]int)
	for _, snapshot := range batch {
		characters.Ids = append(characters.Ids, int(snapshot.CharacterID))
		characters.Xs = append(characters.Xs, float64(snapshot.X))
		characters.Ys = append(characters.Ys, float64(snapshot.Y))
		characters.Headings = append(characters.Headings, float64(snapshot.Heading))
		characters.Staminas = append(characters.Staminas, snapshot.Stamina)
		characters.Energies = append(characters.Energies, snapshot.Energy)
		characters.Shps = append(characters.Shps, int(snapshot.SHP))
		characters.Hhps = append(characters.Hhps, int(snapshot.HHP))
		characters.IsLyings = append(characters.IsLyings, snapshot.IsLying)
		characters.Attributes = append(characters.Attributes, snapshot.Attributes)
		characters.Exps = append(characters.Exps, snapshot.Exp)
		characters.Skills = append(characters.Skills, snapshot.Skills)
		characters.Discovery = append(characters.Discovery, snapshot.Discovery)
		cooldowns := snapshot.ActionCooldowns
		if cooldowns == "" {
			cooldowns = "{}"
		}
		characters.ActionCooldowns = append(characters.ActionCooldowns, cooldowns)
		for _, inventory := range snapshot.Inventories {
			key := inventoryKey{inventory.CharacterID, inventory.Kind, inventory.InventoryKey}
			if index, exists := inventoryIndexes[key]; exists {
				if inventory.Version >= inventories.Versions[index] {
					inventories.Datas[index] = string(inventory.Data)
					inventories.Versions[index] = inventory.Version
				}
				continue
			}
			inventoryIndexes[key] = len(inventories.OwnerIds)
			inventories.OwnerIds = append(inventories.OwnerIds, inventory.CharacterID)
			inventories.Kinds = append(inventories.Kinds, int(inventory.Kind))
			inventories.InventoryKeys = append(inventories.InventoryKeys, int(inventory.InventoryKey))
			inventories.Datas = append(inventories.Datas, string(inventory.Data))
			inventories.Versions = append(inventories.Versions, inventory.Version)
		}
	}
	return characters, inventories
}

func (s *CharacterSaver) drainQueue(queue *characterSaveQueue) {
	ctx, cancel := context.WithTimeout(context.Background(), characterSaveShutdownTimeout)
	defer cancel()
	for queue.pendingCount() > 0 {
		if err := s.flushPending(ctx, queue, 0); err != nil {
			if ctx.Err() != nil {
				s.logger.Error("Character saver stopped with unsaved snapshots", zap.Int("pending_characters", queue.pendingCount()), zap.Error(err))
				return
			}
			s.logger.Error("Final character save failed; retrying", zap.Int("pending_characters", queue.pendingCount()), zap.Error(err))
			select {
			case <-time.After(batchTimeout):
			case <-ctx.Done():
			}
		}
	}
}

func (s *CharacterSaver) Stop() {
	s.stopOnce.Do(func() {
		for i := range s.queues {
			queue := &s.queues[i]
			queue.mu.Lock()
			queue.closed = true
			queue.mu.Unlock()
		}
		close(s.stop)
		if s.numWorkers <= 0 {
			s.drainQueue(&s.queues[0])
		}
		s.wg.Wait()
		s.logger.Info("CharacterSaver stopped")
	})
}
