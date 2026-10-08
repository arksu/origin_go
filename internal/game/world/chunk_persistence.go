package world

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

var (
	ErrChunkPersistenceBusy   = errors.New("chunk persistence is busy")
	ErrChunkOutsideWorld      = errors.New("chunk is outside world bounds")
	ErrInvalidCommittedObject = errors.New("invalid committed object")
)

const committedDroppedActivationBudget = 64

// PersistenceJob runs on the existing chunk save workers. Complete may only
// update the job's synchronized state; it must never access the ECS World.
type PersistenceJob interface {
	Run() error
	Complete(error)
}

type chunkPersistence struct {
	ioMu sync.Mutex
	pins atomic.Int32
}

func (cm *ChunkManager) persistenceFor(coord types.ChunkCoord) *chunkPersistence {
	cm.persistenceMu.Lock()
	defer cm.persistenceMu.Unlock()
	if cm.persistence == nil {
		cm.persistence = make(map[types.ChunkCoord]*chunkPersistence)
	}
	gate := cm.persistence[coord]
	if gate == nil {
		gate = &chunkPersistence{}
		cm.persistence[coord] = gate
	}
	return gate
}

// SubmitPersistenceJob admits work without waiting for a worker or database.
func (cm *ChunkManager) SubmitPersistenceJob(job PersistenceJob) bool {
	if cm == nil || job == nil || atomic.LoadInt32(&cm.stopped) != 0 {
		return false
	}
	select {
	case cm.saveQueue <- saveRequest{job: job}:
		return true
	default:
		return false
	}
}

// PinPersistence is called under the shard lock before accepting a destruction.
// It never waits for an in-flight database operation.
func (cm *ChunkManager) PinPersistence(coord types.ChunkCoord) error {
	if cm == nil || !cm.IsWithinWorldBounds(coord) {
		return ErrChunkOutsideWorld
	}
	gate := cm.persistenceFor(coord)
	if !gate.ioMu.TryLock() {
		return ErrChunkPersistenceBusy
	}
	gate.pins.Add(1)
	gate.ioMu.Unlock()
	return nil
}

// UnpinPersistence releases one accepted operation's retention of a chunk.
func (cm *ChunkManager) UnpinPersistence(coord types.ChunkCoord) {
	gate := cm.persistenceFor(coord)
	if gate.pins.Add(-1) < 0 {
		panic("unbalanced chunk persistence pin")
	}
	cm.markPersistenceReady(coord)
}

func (cm *ChunkManager) markPersistenceReady(coord types.ChunkCoord) {
	cm.readyMu.Lock()
	cm.readyChunks[coord] = struct{}{}
	cm.readyMu.Unlock()
	// A pinned chunk may have outlived its last AOI. Keep a reconciliation entry
	// so the next owning-shard update can deactivate it after the final release.
	cm.interestMu.Lock()
	if _, exists := cm.chunkInterests[coord]; !exists {
		cm.chunkInterests[coord] = newChunkInterest()
	}
	cm.interestMu.Unlock()
}

// WithPersistence serializes a worker transaction with loads and old saves for
// every affected chunk. Canonical lock order avoids cross-chunk deadlocks.
// The owning shard must never call this method while holding its lock.
func (cm *ChunkManager) WithPersistence(coords []types.ChunkCoord, fn func() error) error {
	ordered := append([]types.ChunkCoord(nil), coords...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].X != ordered[j].X {
			return ordered[i].X < ordered[j].X
		}
		return ordered[i].Y < ordered[j].Y
	})
	gates := make([]*chunkPersistence, 0, len(ordered))
	for i, coord := range ordered {
		if !cm.IsWithinWorldBounds(coord) {
			return ErrChunkOutsideWorld
		}
		if i != 0 && coord == ordered[i-1] {
			continue
		}
		gates = append(gates, cm.persistenceFor(coord))
	}
	for _, gate := range gates {
		gate.ioMu.Lock()
	}
	defer func() {
		for i := len(gates) - 1; i >= 0; i-- {
			gates[i].ioMu.Unlock()
		}
	}()
	return fn()
}

// InsertCommittedDropped installs one owned, durable record in the raw cache.
// Existing activation handles publication and capacity retries under shard lock.
func (cm *ChunkManager) InsertCommittedDropped(raw *repository.Object, inventoryRoot *repository.Inventory) error {
	if raw == nil || inventoryRoot == nil || raw.ID <= 0 || raw.TypeID != constt.DroppedItemTypeID ||
		inventoryRoot.OwnerID != raw.ID || inventoryRoot.Kind != int16(constt.InventoryDroppedItem) ||
		inventoryRoot.InventoryKey != 0 || raw.Region != cm.region || raw.Layer != cm.layer {
		return ErrInvalidCommittedObject
	}
	coord := types.ChunkCoord{X: raw.ChunkX, Y: raw.ChunkY}
	if !cm.IsWithinWorldBounds(coord) {
		return ErrChunkOutsideWorld
	}
	gate := cm.persistenceFor(coord)
	if !gate.ioMu.TryLock() {
		return ErrChunkPersistenceBusy
	}
	defer gate.ioMu.Unlock()
	cm.chunksMu.Lock()
	chunk := cm.chunks[coord]
	if chunk == nil {
		chunk = core.NewChunk(coord, cm.region, cm.layer, constt.ChunkSize)
		cm.chunks[coord] = chunk
	}
	cm.chunksMu.Unlock()
	objectCopy := *raw
	objectCopy.Data.RawMessage = append([]byte(nil), raw.Data.RawMessage...)
	inventoryCopy := *inventoryRoot
	inventoryCopy.Data = append([]byte(nil), inventoryRoot.Data...)
	chunk.InsertCommittedObject(&objectCopy, []repository.Inventory{inventoryCopy})
	cm.markPersistenceReady(coord)
	if chunk.GetState() == types.ChunkStateUnloaded {
		_ = cm.requestLoad(coord)
	}
	return nil
}

// RemoveCommittedSource reconciles stale raw caches after an atomic replacement.
// ECS removal remains the destruction service's responsibility.
func (cm *ChunkManager) RemoveCommittedSource(coord types.ChunkCoord, id types.EntityID) {
	if chunk := cm.GetChunkFast(coord); chunk != nil {
		chunk.RemoveCommittedObject(id)
	}
}

// WorldBounds returns the half-open bounds in world coordinates.
func (cm *ChunkManager) WorldBounds() (minX, minY, maxX, maxY int) {
	size := constt.ChunkSize * constt.CoordPerTile
	minX = cm.cfg.Game.WorldMinXChunks * size
	minY = cm.cfg.Game.WorldMinYChunks * size
	return minX, minY, minX + cm.cfg.Game.WorldWidthChunks*size, minY + cm.cfg.Game.WorldHeightChunks*size
}
