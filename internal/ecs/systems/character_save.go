package systems

import (
	"context"
	"encoding/json"
	"math"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	snapshotChannelSize = 1000
	batchSize           = 100
	batchTimeout        = 500 * time.Millisecond
)

// InventorySnapshot represents a serialized inventory container for database storage
type InventorySnapshot struct {
	CharacterID  int64
	Kind         int16
	InventoryKey int16
	Data         json.RawMessage
	Version      int
}

// InventorySaverInterface defines the contract for inventory serialization
// This interface breaks circular dependencies between game and systems packages
type InventorySaverInterface interface {
	// SerializeInventories extracts and serializes all inventory containers for a character
	SerializeInventories(world interface{}, characterID types.EntityID, handle types.Handle) []InventorySnapshot
}

type CharacterSaveSystem struct {
	ecs.BaseSystem
	saver        *CharacterSaver
	saveInterval time.Duration
	logger       *zap.Logger
}

func NewCharacterSaveSystem(saver *CharacterSaver, saveInterval time.Duration, logger *zap.Logger) *CharacterSaveSystem {
	return &CharacterSaveSystem{
		BaseSystem:   ecs.NewBaseSystem("CharacterSaveSystem", 500),
		saver:        saver,
		saveInterval: saveInterval,
		logger:       logger,
	}
}

func (s *CharacterSaveSystem) Update(w *ecs.World, dt float64) {
	now := ecs.GetResource[ecs.TimeState](w).Now
	charEntities := ecs.GetResource[ecs.CharacterEntities](w)

	for entityID, charEntity := range charEntities.Map {
		if now.After(charEntity.NextSaveAt) {
			if !w.Alive(charEntity.Handle) {
				charEntities.Remove(entityID)
				s.logger.Warn("Character entity no longer alive, removed from save tracking",
					zap.Uint64("entity_id", uint64(entityID)))
				continue
			}

			s.saver.Save(w, entityID, charEntity.Handle)

			// Deterministic jitter based on entityID to spread saves (0-10% of interval)
			jitter := time.Duration(entityID%100) * s.saveInterval / 100
			nextSaveAt := now.Add(s.saveInterval + jitter)
			charEntities.UpdateSaveTime(entityID, now, nextSaveAt)
		}
	}
}

func (s *CharacterSaveSystem) Stop() {
	s.saver.Stop()
}

type CharacterSnapshot struct {
	CharacterID int64
	X           int
	Y           int
	Heading     int16
	Stamina     int16
	SHP         int16
	HHP         int16
	Inventories []InventorySnapshot
}

type CharacterSaver struct {
	db              *persistence.Postgres
	snapshotChannel chan CharacterSnapshot
	numWorkers      int
	logger          *zap.Logger
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	inventorySaver  InventorySaverInterface
}

func NewCharacterSaver(db *persistence.Postgres, numWorkers int, inventorySaver InventorySaverInterface, logger *zap.Logger) *CharacterSaver {
	ctx, cancel := context.WithCancel(context.Background())

	cs := &CharacterSaver{
		db:              db,
		snapshotChannel: make(chan CharacterSnapshot, snapshotChannelSize),
		numWorkers:      numWorkers,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
		inventorySaver:  inventorySaver,
	}

	for i := 0; i < numWorkers; i++ {
		cs.wg.Add(1)
		go cs.saveWorker(i)
	}

	return cs
}

func (s *CharacterSaver) Save(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	if !hasTransform {
		s.logger.Warn("Character entity missing Transform component",
			zap.Uint64("entity_id", uint64(entityID)))
		return
	}

	inventories := s.inventorySaver.SerializeInventories(w, entityID, handle)

	snapshot := CharacterSnapshot{
		CharacterID: int64(entityID),
		X:           int(transform.X),
		Y:           int(transform.Y),
		Heading:     int16(math.Mod(transform.Direction*180/math.Pi+360, 360)),
		Stamina:     100, // TODO
		SHP:         100, // TODO
		HHP:         100, // TODO
		Inventories: inventories,
	}

	select {
	case s.snapshotChannel <- snapshot:
	default:
		s.logger.Warn("Character save channel full, dropping snapshot",
			zap.Int64("character_id", snapshot.CharacterID),
			zap.Int("channel_size", snapshotChannelSize))
	}
}

func (s *CharacterSaver) saveWorker(workerID int) {
	defer s.wg.Done()

	batch := make([]CharacterSnapshot, 0, batchSize)
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			if len(batch) > 0 {
				// Create a new context for final save to avoid context cancellation
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
				defer cancel()

				s.flushBatchWithContext(ctx, batch)
			}
			return

		case snapshot := <-s.snapshotChannel:
			batch = append(batch, snapshot)
			if len(batch) >= batchSize {
				s.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *CharacterSaver) flushBatch(batch []CharacterSnapshot) {
	s.flushBatchWithContext(s.ctx, batch)
}

func (s *CharacterSaver) flushBatchWithContext(ctx context.Context, batch []CharacterSnapshot) {
	if len(batch) == 0 {
		return
	}

	// Convert batch to arrays for batch update
	ids := make([]int, len(batch))
	xs := make([]float64, len(batch))
	ys := make([]float64, len(batch))
	headings := make([]float64, len(batch))
	staminas := make([]int, len(batch))
	shps := make([]int, len(batch))
	hhps := make([]int, len(batch))

	for i, snapshot := range batch {
		ids[i] = int(snapshot.CharacterID)
		xs[i] = float64(snapshot.X)
		ys[i] = float64(snapshot.Y)
		headings[i] = float64(snapshot.Heading)
		staminas[i] = int(snapshot.Stamina)
		shps[i] = int(snapshot.SHP)
		hhps[i] = int(snapshot.HHP)
	}

	params := repository.UpdateCharactersParams{
		Ids:      ids,
		Xs:       xs,
		Ys:       ys,
		Headings: headings,
		Staminas: staminas,
		Shps:     shps,
		Hhps:     hhps,
	}

	err := s.db.Queries().UpdateCharacters(ctx, params)
	if err != nil {
		s.logger.Error("Failed to execute batch update",
			zap.Int("batch_size", len(batch)),
			zap.Any("params", params),
			zap.Error(err))
		return
	}

	// Batch upsert all inventories in a single query
	totalInv := 0
	for _, snapshot := range batch {
		totalInv += len(snapshot.Inventories)
	}

	if totalInv > 0 {
		ownerIDs := make([]int64, 0, totalInv)
		kinds := make([]int, 0, totalInv)
		inventoryKeys := make([]int, 0, totalInv)
		datas := make([]string, 0, totalInv)
		versions := make([]int, 0, totalInv)

		for _, snapshot := range batch {
			for _, inv := range snapshot.Inventories {
				ownerIDs = append(ownerIDs, inv.CharacterID)
				kinds = append(kinds, int(inv.Kind))
				inventoryKeys = append(inventoryKeys, int(inv.InventoryKey))
				datas = append(datas, string(inv.Data))
				versions = append(versions, inv.Version)
			}
		}

		err = s.db.Queries().UpsertInventories(ctx, repository.UpsertInventoriesParams{
			OwnerIds:      ownerIDs,
			Kinds:         kinds,
			InventoryKeys: inventoryKeys,
			Datas:         datas,
			Versions:      versions,
		})
		if err != nil {
			s.logger.Error("Failed to batch upsert inventories",
				zap.Int("batch_size", len(batch)),
				zap.Int("inventory_count", totalInv),
				zap.Error(err))
		}
	}
}

// SaveAll saves all characters from CharacterEntities
func (s *CharacterSaver) SaveAll(w *ecs.World) {
	characterEntities := ecs.GetResource[ecs.CharacterEntities](w)
	entityIDs := characterEntities.GetAll()

	s.logger.Info("Saving all characters", zap.Int("count", len(entityIDs)))

	for _, entityID := range entityIDs {
		if charEntity, exists := characterEntities.Map[entityID]; exists {
			s.Save(w, entityID, charEntity.Handle)
		}
	}

	s.logger.Info("All characters saved")
}

func (s *CharacterSaver) Stop() {
	// Process any remaining snapshots in the channel
	remaining := make([]CharacterSnapshot, 0)
	for {
		select {
		case snapshot := <-s.snapshotChannel:
			remaining = append(remaining, snapshot)
		default:
			// No more snapshots
			goto processRemaining
		}
	}

processRemaining:
	if len(remaining) > 0 {
		// Create a new context for final save to avoid context cancellation
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		defer cancel()

		s.flushBatchWithContext(ctx, remaining)
	}

	// Signal workers to stop and wait for them
	s.cancel()
	s.wg.Wait()
	close(s.snapshotChannel)
	s.logger.Info("CharacterSaver stopped")
}
