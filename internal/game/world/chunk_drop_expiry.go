package world

import (
	"errors"
	"sync"
	"time"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	chunkDropExpiryCapacity         = 512
	chunkDropExpiryCompletionBudget = 32
)

var errDropExpiryUnavailable = errors.New("dropped item expiry: persistence unavailable")

type chunkDropExpiryPhase uint8

const (
	chunkDropExpiryIdle chunkDropExpiryPhase = iota
	chunkDropExpiryQueued
	chunkDropExpiryRunning
	chunkDropExpiryReady
)

// The fixed slots retain both admission and completion when existing save
// workers are busy. Tick execution only queues work and applies durable results.
type chunkDropExpiryCoordinator struct {
	manager    *ChunkManager
	operations [chunkDropExpiryCapacity]chunkDropExpiryOperation
	byID       map[types.EntityID]*chunkDropExpiryOperation
}

type chunkDropExpiryOperation struct {
	mu          sync.Mutex
	coordinator *chunkDropExpiryCoordinator
	phase       chunkDropExpiryPhase
	chunk       *core.Chunk
	coord       types.ChunkCoord
	id          types.EntityID
	region      int
	due         time.Time
	backoff     time.Duration
	err         error
}

func newChunkDropExpiryCoordinator(cm *ChunkManager) *chunkDropExpiryCoordinator {
	coordinator := &chunkDropExpiryCoordinator{manager: cm, byID: make(map[types.EntityID]*chunkDropExpiryOperation, chunkDropExpiryCapacity)}
	for i := range coordinator.operations {
		coordinator.operations[i].coordinator = coordinator
	}
	return coordinator
}

// queueExpiredDropRaw receives BuildForChunk's expiry result with the activation
// gate held. The caller retains raw data even when admission or I/O cannot run.
func (cm *ChunkManager) queueExpiredDropRaw(chunk *core.Chunk, raw *repository.Object) {
	if cm.dropExpiry == nil {
		cm.dropExpiry = newChunkDropExpiryCoordinator(cm)
	}
	cm.dropExpiry.admit(chunk, raw)
}

// updateExpiredDrops applies synchronized worker results under the shard lock.
func (cm *ChunkManager) updateExpiredDrops() {
	if cm.dropExpiry != nil {
		cm.UpdateExpiredDrops(ecs.GetResource[ecs.TimeState](cm.world).Now)
	}
}

// PendingExpiredDrops returns retained expiry work. Call under the owning shard
// lock, including shutdown; worker callbacks never change the registration map.
func (cm *ChunkManager) PendingExpiredDrops() int {
	if cm == nil || cm.dropExpiry == nil {
		return 0
	}
	return len(cm.dropExpiry.byID)
}

// UpdateExpiredDrops queues due retries and applies durable worker results.
// The caller must hold the owning shard lock. An explicit clock lets shutdown
// continue draining after regular ticks have stopped.
func (cm *ChunkManager) UpdateExpiredDrops(now time.Time) {
	if cm != nil && cm.dropExpiry != nil {
		cm.dropExpiry.update(now)
	}
}

func (c *chunkDropExpiryCoordinator) admit(chunk *core.Chunk, raw *repository.Object) {
	id := types.EntityID(raw.ID)
	if id == 0 || chunk == nil || c.byID[id] != nil || len(c.byID) == len(c.operations) {
		return
	}
	var op *chunkDropExpiryOperation
	for i := range c.operations {
		candidate := &c.operations[i]
		candidate.mu.Lock()
		if candidate.phase == chunkDropExpiryIdle {
			op = candidate
			break
		}
		candidate.mu.Unlock()
	}
	if op == nil {
		return
	}
	op.chunk, op.coord, op.id, op.region = chunk, chunk.Coord, id, raw.Region
	op.due = ecs.GetResource[ecs.TimeState](c.manager.world).Now
	op.backoff = 0
	op.err = nil
	op.phase = chunkDropExpiryQueued
	op.mu.Unlock()
	c.byID[id] = op
	// activateChunkInternal already owns this gate; PinPersistence would attempt
	// to lock it again. Retention prevents save/deactivation until durable delete.
	c.manager.persistenceFor(op.coord).pins.Add(1)
	c.submit(op)
}

func (c *chunkDropExpiryCoordinator) submit(op *chunkDropExpiryOperation) {
	op.mu.Lock()
	op.phase = chunkDropExpiryRunning
	op.mu.Unlock()
	if !c.manager.SubmitPersistenceJob(op) {
		op.mu.Lock()
		op.phase = chunkDropExpiryQueued
		op.mu.Unlock()
	}
}

func (c *chunkDropExpiryCoordinator) update(now time.Time) {
	if len(c.byID) == 0 {
		return
	}
	budget := chunkDropExpiryCompletionBudget
	for i := range c.operations {
		op := &c.operations[i]
		op.mu.Lock()
		if op.phase == chunkDropExpiryReady {
			if op.err != nil {
				if op.backoff == 0 {
					op.backoff = time.Second
				} else {
					op.backoff *= 2
					if op.backoff > 60*time.Second {
						op.backoff = 60 * time.Second
					}
				}
				op.due = now.Add(op.backoff)
				op.phase = chunkDropExpiryQueued
			} else if budget > 0 {
				op.chunk.RemoveCommittedObject(op.id)
				delete(c.byID, op.id)
				c.manager.UnpinPersistence(op.coord)
				op.chunk = nil
				op.err = nil
				op.phase = chunkDropExpiryIdle
				budget--
			}
		}
		if op.phase == chunkDropExpiryQueued && !now.Before(op.due) {
			op.mu.Unlock()
			c.submit(op)
			continue
		}
		op.mu.Unlock()
	}
}

// Run is invoked exclusively by a chunk persistence worker. No ECS access is
// retained here; the gate orders deletion against loads, saves and replacements.
func (op *chunkDropExpiryOperation) Run() error {
	cm := op.coordinator.manager
	if cm.objectFactory == nil || cm.objectFactory.objectDeleter == nil {
		return errDropExpiryUnavailable
	}
	return cm.WithPersistence([]types.ChunkCoord{op.coord}, func() error {
		return cm.objectFactory.objectDeleter.DeleteObject(op.region, op.id)
	})
}

func (op *chunkDropExpiryOperation) Complete(err error) {
	op.mu.Lock()
	op.err = err
	op.phase = chunkDropExpiryReady
	id, coord, region := op.id, op.coord, op.region
	logger := op.coordinator.manager.logger
	op.mu.Unlock()
	if err != nil && logger != nil {
		logger.Error("Failed to expire dropped item",
			zap.Error(err), zap.Uint64("entity_id", uint64(id)), zap.Int("region", region),
			zap.Any("chunk", coord))
	}
}
