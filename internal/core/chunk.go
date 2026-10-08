package core

import (
	"context"
	"errors"
	"fmt"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"sync"
	"time"

	"go.uber.org/zap"
)

type ChunkManager interface {
	ActiveChunks() []*Chunk
	GetChunk(coord types.ChunkCoord) *Chunk
	GetChunkFast(coord types.ChunkCoord) *Chunk
	UpdateEntityPosition(entityID types.EntityID, newCenter types.ChunkCoord)
}

// Chunk represents a game chunk with all its data and functionality
type Chunk struct {
	Coord    types.ChunkCoord
	Region   int
	Layer    int
	State    types.ChunkState
	Tiles    []byte
	LastTick uint64
	Version  uint32 // версия чанка (инкрементируется при изменении тайлов)

	tilesDirty bool
	chunkSize  int

	isPassable  []uint64
	isSwimmable []uint64

	// Failed builds remain raw even while other objects in the chunk are active.
	rawObjects []*repository.Object
	// Absolute positions survive prefix consumption without reindexing the tail.
	rawObjectIndex map[types.EntityID]int
	rawObjectBase  int
	// The next retry starts beyond the last visited span, so retained failures
	// cannot prevent later durable records from reaching activation.
	rawActivationCursor int
	// Freeze each cycle's upper bound: continuous appends must not postpone
	// retries of the retained head indefinitely.
	rawActivationEnd int
	// SQL rows have unique IDs; preserve legacy handling of malformed duplicate
	// cache inputs without making the ordinary insertion path scan the backlog.
	rawDuplicateIDs map[types.EntityID]struct{}
	// rawInventoriesByOwner includes inventories for failed builds awaiting a retry.
	rawInventoriesByOwner map[types.EntityID][]repository.Inventory
	// rawDirtyObjectIDs tracks pending writes for raw or restored objects.
	// It survives activation and deactivation until those objects are saved.
	rawDirtyObjectIDs map[types.EntityID]struct{}
	// deletedObjectIDs tracks runtime-despawned object ids that must be soft-deleted in DB
	// on the next save, even though they no longer exist in the active ECS chunk handles.
	deletedObjectIDs map[types.EntityID]struct{}
	// A load may finish after a committed replacement was installed by the shard.
	// Revision and removals fence that older read without discarding other rows.
	cacheRevision       uint64
	cacheLoadInFlight   bool
	committedRemovedIDs map[types.EntityID]struct{}
	rawDataDirty        bool
	spatial             *SpatialHashGrid

	mu sync.RWMutex
}

func NewChunk(coord types.ChunkCoord, region int, layer int, chunkSize int) *Chunk {
	cellSize := 16
	totalTiles := chunkSize * chunkSize
	bitsetSize := (totalTiles + 63) / 64

	return &Chunk{
		Coord:                 coord,
		Region:                region,
		Layer:                 layer,
		State:                 types.ChunkStateUnloaded,
		chunkSize:             chunkSize,
		Tiles:                 make([]byte, totalTiles),
		isPassable:            make([]uint64, bitsetSize),
		isSwimmable:           make([]uint64, bitsetSize),
		rawInventoriesByOwner: make(map[types.EntityID][]repository.Inventory, 8),
		rawObjectIndex:        make(map[types.EntityID]int, 8),
		rawDuplicateIDs:       make(map[types.EntityID]struct{}),
		rawDirtyObjectIDs:     make(map[types.EntityID]struct{}, 8),
		deletedObjectIDs:      make(map[types.EntityID]struct{}, 8),
		committedRemovedIDs:   make(map[types.EntityID]struct{}),
		spatial:               NewSpatialHashGrid(cellSize),
	}
}

func (c *Chunk) SetState(state types.ChunkState) {
	c.mu.Lock()
	c.State = state
	c.mu.Unlock()
}

func (c *Chunk) GetState() types.ChunkState {
	c.mu.RLock()
	state := c.State
	c.mu.RUnlock()
	return state
}

func (c *Chunk) SetRawObjects(objects []*repository.Object) {
	c.mu.Lock()
	c.rawObjects = objects
	c.rebuildRawObjectIndexLocked()
	c.rawDirtyObjectIDs = make(map[types.EntityID]struct{}, 8)
	c.mu.Unlock()
}

// GetRawObjects returns a read-only cache view. Durable removals can leave nil
// slots until bounded activation consumes them; callers must skip those slots.
func (c *Chunk) GetRawObjects() []*repository.Object {
	c.mu.RLock()
	objects := c.rawObjects
	c.mu.RUnlock()
	return objects
}

// GetRawActivationObjects returns the next suffix to visit and its cache offset.
// Advancing through suffixes preserves fair retries without moving the backlog.
// The caller holds the owning shard and chunk persistence gate.
func (c *Chunk) GetRawActivationObjects() (int, []*repository.Object) {
	c.mu.Lock()
	defer c.mu.Unlock()
	end := min(c.rawActivationEnd, c.rawObjectBase+len(c.rawObjects))
	if c.rawActivationCursor < c.rawObjectBase || c.rawActivationCursor >= end {
		c.rawActivationCursor = c.rawObjectBase
		end = c.rawObjectBase + len(c.rawObjects)
		c.rawActivationEnd = end
	}
	start := c.rawActivationCursor - c.rawObjectBase
	return start, c.rawObjects[start : end-c.rawObjectBase]
}

func (c *Chunk) rebuildRawObjectIndexLocked() {
	clear(c.rawObjectIndex)
	clear(c.rawDuplicateIDs)
	c.rawObjectBase = 0
	c.rawActivationCursor = 0
	c.rawActivationEnd = len(c.rawObjects)
	for i, object := range c.rawObjects {
		if object == nil {
			continue
		}
		id := types.EntityID(object.ID)
		if _, exists := c.rawObjectIndex[id]; exists {
			c.rawDuplicateIDs[id] = struct{}{}
		} else {
			c.rawObjectIndex[id] = i
		}
	}
}

func (c *Chunk) rawObjectPositionLocked(id types.EntityID) (int, bool) {
	if _, duplicate := c.rawDuplicateIDs[id]; duplicate {
		for i, object := range c.rawObjects {
			if object != nil && types.EntityID(object.ID) == id {
				return i, true
			}
		}
		return 0, false
	}
	absolute, found := c.rawObjectIndex[id]
	return absolute - c.rawObjectBase, found
}

// RetainRawActivationPrefix consumes a visited prefix. See RetainRawActivationRange.
func (c *Chunk) RetainRawActivationPrefix(visited int, retained []*repository.Object) {
	c.RetainRawActivationRange(0, visited, retained)
}

// RetainRawActivationRange consumes only a visited span. Failures are packed
// backwards within that span; neither preceding records nor the unvisited tail
// are copied or read. The next retry starts after the span, before revisiting
// failures at the head. retained must be an ordered subset with owned storage.
// The caller holds the owning shard and chunk persistence gate.
func (c *Chunk) RetainRawActivationRange(start, visited int, retained []*repository.Object) {
	if visited == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	retainedIndex := 0
	end := start + visited
	for _, object := range c.rawObjects[start:end] {
		if retainedIndex < len(retained) && object == retained[retainedIndex] {
			retainedIndex++
			continue
		}
		if object != nil {
			id := types.EntityID(object.ID)
			if _, duplicate := c.rawDuplicateIDs[id]; !duplicate {
				delete(c.rawObjectIndex, id)
				delete(c.rawInventoriesByOwner, id)
			}
		}
	}
	consumed := visited - len(retained)
	retainedStart := start + consumed
	c.rawActivationCursor = c.rawObjectBase + end
	if start > 0 && end == len(c.rawObjects) {
		// At the tail, reclaim successful slots within this span immediately.
		retainedStart = start
		copy(c.rawObjects[start:end], retained)
		clear(c.rawObjects[start+len(retained) : end])
		c.rawObjects = c.rawObjects[:start+len(retained)]
	} else {
		copy(c.rawObjects[retainedStart:end], retained)
		clear(c.rawObjects[start:retainedStart])
	}
	retainedAbsolute := c.rawObjectBase + retainedStart
	if start == 0 {
		c.rawObjects = c.rawObjects[consumed:]
		c.rawObjectBase += consumed
	}
	for i, object := range retained {
		c.rawObjectIndex[types.EntityID(object.ID)] = retainedAbsolute + i
	}
	if len(c.rawDuplicateIDs) > 0 {
		// Duplicate IDs only arise in malformed/manual cache inputs. Keep their
		// old semantics; production rows retain the bounded prefix path above.
		c.rebuildRawObjectIndexLocked()
		for id := range c.rawInventoriesByOwner {
			if _, remains := c.rawObjectIndex[id]; !remains {
				delete(c.rawInventoriesByOwner, id)
			}
		}
	}
	if len(c.rawObjects) == 0 {
		c.rawObjects = nil
		c.rawObjectBase = 0
		c.rawActivationCursor = 0
		c.rawActivationEnd = 0
	}
}

// InsertCommittedObject installs already-durable data without creating dirty intent.
// The caller transfers ownership of the object and inventory payloads.
func (c *Chunk) InsertCommittedObject(obj *repository.Object, rows []repository.Inventory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := types.EntityID(obj.ID)
	if index, found := c.rawObjectPositionLocked(id); found {
		c.rawObjects[index] = obj
	} else {
		c.rawObjectIndex[id] = c.rawObjectBase + len(c.rawObjects)
		c.rawObjects = append(c.rawObjects, obj)
	}
	c.rawInventoriesByOwner[id] = rows
	delete(c.rawDirtyObjectIDs, id)
	delete(c.deletedObjectIDs, id)
	delete(c.committedRemovedIDs, id)
	c.cacheRevision++
}

// RemoveCommittedObject removes caches and pending delete intent after a durable delete.
func (c *Chunk) RemoveCommittedObject(id types.EntityID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, duplicate := c.rawDuplicateIDs[id]; duplicate {
		kept := c.rawObjects[:0]
		for _, object := range c.rawObjects {
			if object == nil || types.EntityID(object.ID) != id {
				kept = append(kept, object)
			}
		}
		clear(c.rawObjects[len(kept):])
		c.rawObjects = kept
		c.rebuildRawObjectIndexLocked()
	} else if index, found := c.rawObjectPositionLocked(id); found {
		c.rawObjects[index] = nil
		delete(c.rawObjectIndex, id)
		// One edge slot can be discarded immediately in constant time. Interior
		// holes are consumed under activation's existing bounded visit budget.
		if index == 0 {
			c.rawObjects = c.rawObjects[1:]
			c.rawObjectBase++
		} else if index == len(c.rawObjects)-1 {
			c.rawObjects = c.rawObjects[:index]
		}
		if len(c.rawObjects) == 0 {
			c.rawObjects = nil
			c.rawObjectBase = 0
		}
	}
	delete(c.rawInventoriesByOwner, id)
	delete(c.rawDirtyObjectIDs, id)
	delete(c.deletedObjectIDs, id)
	if c.cacheLoadInFlight {
		c.committedRemovedIDs[id] = struct{}{}
	}
	c.cacheRevision++
}

// beginCacheLoad starts the one protected DB read allowed by the persistence
// gate. Removal fences only need to live until that read completes or fails.
func (c *Chunk) beginCacheLoad() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheLoadInFlight = true
	clear(c.committedRemovedIDs)
	return c.cacheRevision
}

func (c *Chunk) finishCacheLoad() {
	c.mu.Lock()
	c.cacheLoadInFlight = false
	clear(c.committedRemovedIDs)
	c.mu.Unlock()
}

// installLoadedObjects merges committed rows installed after the DB read began.
func (c *Chunk) installLoadedObjects(revision uint64, objects []*repository.Object, inventories map[types.EntityID][]repository.Inventory) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if revision != c.cacheRevision {
		current := make(map[types.EntityID]*repository.Object, len(c.rawObjects))
		for _, object := range c.rawObjects {
			if object != nil {
				current[types.EntityID(object.ID)] = object
			}
		}
		merged := make([]*repository.Object, 0, len(objects)+len(current))
		for _, object := range objects {
			id := types.EntityID(object.ID)
			if _, removed := c.committedRemovedIDs[id]; removed {
				delete(inventories, id)
				continue
			}
			if newer, exists := current[id]; exists {
				merged = append(merged, newer)
				delete(current, id)
			} else {
				merged = append(merged, object)
			}
		}
		// Preserve insertion order for rows absent from the old DB result.
		for _, object := range c.rawObjects {
			if object != nil {
				if _, exists := current[types.EntityID(object.ID)]; exists {
					merged = append(merged, object)
				}
			}
		}
		for id, rows := range c.rawInventoriesByOwner {
			inventories[id] = rows
		}
		objects = merged
	} else {
		clear(c.committedRemovedIDs)
	}
	c.rawObjects = objects
	c.rebuildRawObjectIndexLocked()
	c.rawInventoriesByOwner = inventories
	c.cacheLoadInFlight = false
	clear(c.committedRemovedIDs)
}

func (c *Chunk) AddRawObject(obj *repository.Object) {
	c.mu.Lock()
	c.rawObjects = append(c.rawObjects, obj)
	if obj != nil {
		id := types.EntityID(obj.ID)
		if _, exists := c.rawObjectIndex[id]; exists {
			c.rawDuplicateIDs[id] = struct{}{}
		} else {
			c.rawObjectIndex[id] = c.rawObjectBase + len(c.rawObjects) - 1
		}
		c.rawDirtyObjectIDs[types.EntityID(obj.ID)] = struct{}{}
		delete(c.deletedObjectIDs, types.EntityID(obj.ID))
	}
	c.rawDataDirty = true
	c.mu.Unlock()
}

func (c *Chunk) RemoveRawObjectByID(id types.EntityID) {
	if id == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i < len(c.rawObjects); i++ {
		obj := c.rawObjects[i]
		if obj == nil || types.EntityID(obj.ID) != id {
			continue
		}
		c.rawObjects = append(c.rawObjects[:i], c.rawObjects[i+1:]...)
		i--
	}
	delete(c.rawDirtyObjectIDs, id)
	c.rebuildRawObjectIndexLocked()
	c.rawDataDirty = true
}

func (c *Chunk) UpsertRawObject(obj *repository.Object) {
	if obj == nil {
		return
	}
	id := types.EntityID(obj.ID)
	if id == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if index, found := c.rawObjectPositionLocked(id); found {
		c.rawObjects[index] = obj
	} else {
		c.rawObjectIndex[id] = c.rawObjectBase + len(c.rawObjects)
		c.rawObjects = append(c.rawObjects, obj)
	}
	c.rawDirtyObjectIDs[id] = struct{}{}
	delete(c.deletedObjectIDs, id)
	c.rawDataDirty = true
}

func (c *Chunk) ClearRawObjects() {
	c.mu.Lock()
	c.rawObjects = nil
	clear(c.rawObjectIndex)
	clear(c.rawDuplicateIDs)
	c.rawObjectBase = 0
	c.rawActivationCursor = 0
	c.rawActivationEnd = 0
	c.rawDirtyObjectIDs = make(map[types.EntityID]struct{}, 8)
	c.mu.Unlock()
}

func (c *Chunk) SetRawInventoriesByOwner(inventories map[types.EntityID][]repository.Inventory) {
	c.mu.Lock()
	c.rawInventoriesByOwner = inventories
	c.mu.Unlock()
}

func (c *Chunk) GetRawInventoriesByOwner() map[types.EntityID][]repository.Inventory {
	c.mu.RLock()
	inventories := c.rawInventoriesByOwner
	c.mu.RUnlock()
	return inventories
}

func (c *Chunk) ClearRawInventoriesByOwner() {
	c.mu.Lock()
	c.rawInventoriesByOwner = make(map[types.EntityID][]repository.Inventory, 8)
	c.mu.Unlock()
}

// RemoveRawInventoriesByOwner mutates the inactive/preloaded raw-inventory cache and
// intentionally marks rawDataDirty. Callers use this for transfer/cache repair paths,
// not only gameplay deletes, so the dirtying side effect is part of the contract.
func (c *Chunk) RemoveRawInventoriesByOwner(ownerID types.EntityID) {
	if ownerID == 0 {
		return
	}
	c.mu.Lock()
	delete(c.rawInventoriesByOwner, ownerID)
	delete(c.rawDirtyObjectIDs, ownerID)
	c.rawDataDirty = true
	c.mu.Unlock()
}

// SetRawInventoriesForOwner replaces the inactive/preloaded raw-inventory cache rows for
// an owner and intentionally marks rawDataDirty/rawDirtyObjectIDs so future saves preserve
// cache repairs (e.g. cross-shard transfer moves).
func (c *Chunk) SetRawInventoriesForOwner(ownerID types.EntityID, rows []repository.Inventory) {
	if ownerID == 0 {
		return
	}
	c.mu.Lock()
	if len(rows) == 0 {
		delete(c.rawInventoriesByOwner, ownerID)
	} else {
		cloned := make([]repository.Inventory, len(rows))
		copy(cloned, rows)
		c.rawInventoriesByOwner[ownerID] = cloned
	}
	c.rawDirtyObjectIDs[ownerID] = struct{}{}
	c.rawDataDirty = true
	c.mu.Unlock()
}

func (c *Chunk) SetRawDirtyObjectIDs(ids map[types.EntityID]struct{}) {
	c.mu.Lock()
	c.rawDirtyObjectIDs = make(map[types.EntityID]struct{}, len(ids))
	for id := range ids {
		c.rawDirtyObjectIDs[id] = struct{}{}
	}
	c.mu.Unlock()
}

func (c *Chunk) GetRawDirtyObjectIDs() map[types.EntityID]struct{} {
	c.mu.RLock()
	ids := make(map[types.EntityID]struct{}, len(c.rawDirtyObjectIDs))
	for id := range c.rawDirtyObjectIDs {
		ids[id] = struct{}{}
	}
	c.mu.RUnlock()
	return ids
}

func (c *Chunk) ClearRawDirtyObjectIDs() {
	c.mu.Lock()
	c.rawDirtyObjectIDs = make(map[types.EntityID]struct{}, 8)
	c.mu.Unlock()
}

func (c *Chunk) MarkDeletedObjectID(id types.EntityID) {
	if id == 0 {
		return
	}
	c.mu.Lock()
	c.deletedObjectIDs[id] = struct{}{}
	c.rawDataDirty = true
	c.mu.Unlock()
}

func (c *Chunk) GetDeletedObjectIDs() map[types.EntityID]struct{} {
	c.mu.RLock()
	ids := make(map[types.EntityID]struct{}, len(c.deletedObjectIDs))
	for id := range c.deletedObjectIDs {
		ids[id] = struct{}{}
	}
	c.mu.RUnlock()
	return ids
}

func (c *Chunk) ClearDeletedObjectIDs() {
	c.mu.Lock()
	c.deletedObjectIDs = make(map[types.EntityID]struct{}, 8)
	c.mu.Unlock()
}

func (c *Chunk) MarkRawDataDirty() {
	c.mu.Lock()
	c.rawDataDirty = true
	c.mu.Unlock()
}

func (c *Chunk) ClearRawDataDirty() {
	c.mu.Lock()
	c.rawDataDirty = false
	c.mu.Unlock()
}

func (c *Chunk) GetHandles() []types.Handle {
	return c.spatial.GetAllHandles()
}

func (c *Chunk) ClearHandles() {
	c.spatial.ClearDynamic()
	c.spatial.ClearStatic()
}

func (c *Chunk) Spatial() *SpatialHashGrid {
	return c.spatial
}

// RestoreTiles installs persisted content without creating a new edit/version.
func (c *Chunk) RestoreTiles(tiles []byte, lastTick uint64, version uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(tiles) != c.chunkSize*c.chunkSize {
		return fmt.Errorf("chunk %v: expected %d tiles, got %d", c.Coord, c.chunkSize*c.chunkSize, len(tiles))
	}
	c.Tiles = append([]byte(nil), tiles...)
	c.LastTick = lastTick
	c.Version = version
	c.tilesDirty = false
	c.populateTileBitsets()
	return nil
}

// SetTile is the runtime content mutation point. False means no tile was changed.
func (c *Chunk) SetTile(localX, localY int, tileID byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if localX < 0 || localY < 0 || localX >= c.chunkSize || localY >= c.chunkSize {
		return false
	}
	index := localY*c.chunkSize + localX
	if index >= len(c.Tiles) || c.Tiles[index] == tileID {
		return false
	}
	c.Tiles[index] = tileID
	c.writeBit(c.isPassable, index, types.IsTilePassable(tileID))
	c.writeBit(c.isSwimmable, index, types.IsTileSwimmable(tileID))
	c.Version++
	c.tilesDirty = true
	return true
}

type TileSnapshot struct {
	Tiles   []byte
	Version uint32
}

func (c *Chunk) SnapshotTiles() TileSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return TileSnapshot{Tiles: append([]byte(nil), c.Tiles...), Version: c.Version}
}

func (c *Chunk) TilesDirty() bool {
	c.mu.RLock()
	d := c.tilesDirty
	c.mu.RUnlock()
	return d
}

func (c *Chunk) ClearTilesDirty() {
	c.mu.Lock()
	c.tilesDirty = false
	c.mu.Unlock()
}

// IsDirty returns true if tiles, raw data, or any active object has changed.
func (c *Chunk) IsDirty(world *ecs.World) bool {
	if c.TilesDirty() {
		return true
	}
	c.mu.RLock()
	rawDirty := c.rawDataDirty
	c.mu.RUnlock()
	if rawDirty {
		return true
	}

	for _, h := range c.GetHandles() {
		if !world.Alive(h) {
			continue
		}
		state, ok := ecs.GetComponent[components.ObjectInternalState](world, h)
		if ok && state.IsDirty {
			return true
		}
	}

	return false
}

func (c *Chunk) populateTileBitsets() {
	clear(c.isPassable)
	clear(c.isSwimmable)
	for i, tileID := range c.Tiles {
		if types.IsTilePassable(tileID) {
			c.setBit(c.isPassable, i)
		}
		if types.IsTileSwimmable(tileID) {
			c.setBit(c.isSwimmable, i)
		}
	}
}

func (c *Chunk) writeBit(bitset []uint64, index int, value bool) {
	mask := uint64(1) << uint(index%64)
	bitset[index/64] &^= mask
	if value {
		bitset[index/64] |= mask
	}
}

func (c *Chunk) setBit(bitset []uint64, index int) {
	wordIndex := index / 64
	bitIndex := uint(index % 64)
	bitset[wordIndex] |= 1 << bitIndex
}

func (c *Chunk) getBit(bitset []uint64, index int) bool {
	wordIndex := index / 64
	bitIndex := uint(index % 64)
	return (bitset[wordIndex] & (1 << bitIndex)) != 0
}

func (c *Chunk) IsTilePassable(localTileX, localTileY, chunkSize int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if localTileX < 0 || localTileX >= chunkSize || localTileY < 0 || localTileY >= chunkSize {
		return false
	}

	index := localTileY*chunkSize + localTileX
	if index >= len(c.Tiles) {
		return false
	}
	return c.getBit(c.isPassable, index)
}

func (c *Chunk) IsTileSwimmable(localTileX, localTileY, chunkSize int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if localTileX < 0 || localTileX >= chunkSize || localTileY < 0 || localTileY >= chunkSize {
		return false
	}

	index := localTileY*chunkSize + localTileX
	if index >= len(c.Tiles) {
		return false
	}
	return c.getBit(c.isSwimmable, index)
}

func (c *Chunk) TileID(localTileX, localTileY, chunkSize int) (byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if localTileX < 0 || localTileX >= chunkSize || localTileY < 0 || localTileY >= chunkSize {
		return 0, false
	}

	index := localTileY*chunkSize + localTileX
	if index < 0 || index >= len(c.Tiles) {
		return 0, false
	}
	return c.Tiles[index], true
}

// SaveToDB persists only changed chunk data to the database.
// Tiles are saved only when tilesDirty is set.
// Active objects and retained raw objects are serialized only when dirty.
// Capture and write failures are returned together. Other valid objects are
// still saved, and persistence intent is retained for retry after any failure.
func (c *Chunk) SaveToDB(db *persistence.Postgres, world *ecs.World, objectFactory interface {
	Serialize(world *ecs.World, h types.Handle) (*repository.Object, error)
	SerializeObjectInventories(world *ecs.World, h types.Handle) ([]repository.Inventory, error)
	HasPersistentInventories(typeID uint32, behaviors []string) bool
}, logger *zap.Logger) error {
	if db == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coord := c.Coord

	saveTiles := c.TilesDirty()
	totalHandles := c.GetHandles()
	rawObjects := c.GetRawObjects()
	rawInventoriesByOwner := c.GetRawInventoriesByOwner()
	rawDirtyObjectIDs := c.GetRawDirtyObjectIDs()
	pendingDeletedObjectIDs := c.GetDeletedObjectIDs()

	// Determine dirty objects to save
	var objectsToSave []*repository.Object
	inventoriesToSave := make([]repository.Inventory, 0, 16)
	deletedObjectIDs := make([]int64, 0, 8)
	var dirtyHandles []types.Handle
	activeObjectIDs := make(map[types.EntityID]struct{}, len(totalHandles))
	var saveErr error

	// Active entities are authoritative when a raw cache entry has the same ID.
	for _, h := range totalHandles {
		if !world.Alive(h) {
			continue
		}
		extID, hasExtID := ecs.GetComponent[ecs.ExternalID](world, h)
		pendingRawWrite := false
		if hasExtID {
			activeObjectIDs[extID.ID] = struct{}{}
			_, pendingRawWrite = rawDirtyObjectIDs[extID.ID]
		}

		state, hasState := ecs.GetComponent[components.ObjectInternalState](world, h)
		if hasState && !state.IsDirty && !pendingRawWrite {
			continue
		}

		info, ok := ecs.GetComponent[components.EntityInfo](world, h)
		if !ok {
			logger.Error("cannot save object without entity info", zap.Uint64("object_id", uint64(extID.ID)))
			saveErr = errors.Join(saveErr, fmt.Errorf("object handle %v has no entity info", h))
			continue
		}

		obj, err := objectFactory.Serialize(world, h)
		if err != nil {
			logger.Error("failed to serialize object",
				zap.Uint32("type_id", info.TypeID),
				zap.Error(err),
			)
			saveErr = errors.Join(saveErr, err)
			continue
		}
		if obj == nil {
			// Skip players and other non-persistent entities. If a previously persisted
			// object now resolves to nil (e.g. empty transient build site), mark it for delete.
			if extID, hasExtID := ecs.GetComponent[ecs.ExternalID](world, h); hasExtID {
				deletedObjectIDs = append(deletedObjectIDs, int64(extID.ID))
				dirtyHandles = append(dirtyHandles, h)
			}
			continue
		}
		objectsToSave = append(objectsToSave, obj)

		if objectFactory.HasPersistentInventories(info.TypeID, info.Behaviors) {
			inventories, invErr := objectFactory.SerializeObjectInventories(world, h)
			if invErr != nil {
				logger.Error("failed to serialize object inventories",
					zap.Int64("object_id", obj.ID),
					zap.Error(invErr),
				)
				saveErr = errors.Join(saveErr, invErr)
			} else if len(inventories) > 0 {
				inventoriesToSave = append(inventoriesToSave, inventories...)
			}
		}
		dirtyHandles = append(dirtyHandles, h)
	}

	// Failed builds remain raw while the chunk is active, so persist their
	// pending changes even when other objects already have ECS handles.
	for _, rawObj := range rawObjects {
		if rawObj == nil {
			continue
		}
		objectID := types.EntityID(rawObj.ID)
		if _, active := activeObjectIDs[objectID]; active {
			continue
		}
		if _, deleted := pendingDeletedObjectIDs[objectID]; deleted {
			continue
		}
		if _, dirty := rawDirtyObjectIDs[objectID]; dirty {
			objectsToSave = append(objectsToSave, rawObj)
		}
	}
	for ownerID, rows := range rawInventoriesByOwner {
		if _, active := activeObjectIDs[ownerID]; active {
			continue
		}
		if _, deleted := pendingDeletedObjectIDs[ownerID]; deleted {
			continue
		}
		if _, dirty := rawDirtyObjectIDs[ownerID]; dirty {
			inventoriesToSave = append(inventoriesToSave, rows...)
		}
	}

	if saveTiles {
		entityCount := len(totalHandles)
		for _, rawObj := range rawObjects {
			if rawObj == nil {
				continue
			}
			objectID := types.EntityID(rawObj.ID)
			if _, active := activeObjectIDs[objectID]; active {
				continue
			}
			if _, deleted := pendingDeletedObjectIDs[objectID]; deleted {
				continue
			}
			entityCount++
		}
		if err := c.saveTiles(ctx, db.Queries(), entityCount); err != nil {
			logger.Error("failed to save chunk tiles",
				zap.Int("chunk_x", coord.X),
				zap.Int("chunk_y", coord.Y),
				zap.Error(err),
			)
			saveErr = errors.Join(saveErr, err)
		}
	}

	for deletedID := range pendingDeletedObjectIDs {
		deletedObjectIDs = append(deletedObjectIDs, int64(deletedID))
	}

	// Delete objects that became non-persistent (serialize -> nil) before upserts.
	if len(deletedObjectIDs) > 0 {
		for _, objectID := range deletedObjectIDs {
			if err := db.Queries().DeleteObject(ctx, objectID); err != nil {
				logger.Error("failed to delete object during chunk save",
					zap.Int64("object_id", objectID),
					zap.Any("coord", c.Coord),
					zap.Error(err),
				)
				saveErr = errors.Join(saveErr, err)
			}
		}
	}

	// Save objects batch
	// Earlier capture or delete errors do not prevent saving other valid objects.
	if len(objectsToSave) > 0 {
		nonNilObjects := make([]*repository.Object, 0, len(objectsToSave))
		for _, obj := range objectsToSave {
			if obj == nil {
				continue
			}
			timeState := ecs.GetResource[ecs.TimeState](world)
			obj.LastTick = int64(timeState.Tick)
			nonNilObjects = append(nonNilObjects, obj)
		}
		if len(nonNilObjects) > 0 {
			if err := upsertObjectsBatch(ctx, db, nonNilObjects); err != nil {
				logger.Error("failed to batch save objects",
					zap.Any("coord", c.Coord),
					zap.Error(err),
				)
				saveErr = errors.Join(saveErr, err)
			}
		}
	}

	// Save inventories batch
	if len(inventoriesToSave) > 0 {
		if err := upsertInventoriesBatch(ctx, db, inventoriesToSave); err != nil {
			logger.Error("failed to batch save inventories",
				zap.Any("coord", c.Coord),
				zap.Error(err),
			)
			saveErr = errors.Join(saveErr, err)
		}
	}

	// Clear dirty flags only after successful save; otherwise keep for retry.
	if saveErr == nil {
		for _, h := range dirtyHandles {
			ecs.WithComponent(world, h, func(s *components.ObjectInternalState) {
				s.IsDirty = false
			})
		}
		c.ClearRawDataDirty()
		c.ClearRawDirtyObjectIDs()
		c.ClearDeletedObjectIDs()
	}

	savedTiles := 0
	if saveTiles {
		savedTiles = 1
	}
	logger.Debug("saved chunk",
		zap.Any("coord", c.Coord),
		zap.Int("saved_objects", len(objectsToSave)),
		zap.Int("saved_inventories", len(inventoriesToSave)),
		zap.Int("tiles_saved", savedTiles),
	)
	return saveErr
}

// LoadFromDB loads chunk data and objects from the database
func (c *Chunk) LoadFromDB(db *persistence.Postgres, region int, layer int, logger *zap.Logger) error {
	c.SetState(types.ChunkStateLoading)
	revision := c.beginCacheLoad()
	defer c.finishCacheLoad()

	if db == nil {
		c.SetState(types.ChunkStatePreloaded)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tilesData, err := db.Queries().GetChunk(ctx, repository.GetChunkParams{
		Region: region,
		X:      c.Coord.X,
		Y:      c.Coord.Y,
		Layer:  layer,
	})
	if err != nil {
		return fmt.Errorf("load chunk %v: %w", c.Coord, err)
	}
	if tilesData.Version < 0 {
		return fmt.Errorf("chunk %v has negative version %d", c.Coord, tilesData.Version)
	}
	if err := c.RestoreTiles(tilesData.TilesData, uint64(tilesData.LastTick), uint32(tilesData.Version)); err != nil {
		return err
	}

	objects, err := db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{
		Region: region,
		ChunkX: c.Coord.X,
		ChunkY: c.Coord.Y,
		Layer:  layer,
	})
	if err != nil {
		logger.Error("failed to load objects",
			zap.Int("chunk_x", c.Coord.X),
			zap.Int("chunk_y", c.Coord.Y),
			zap.Error(err),
		)
		return fmt.Errorf("load objects for chunk %v: %w", c.Coord, err)
	}
	logger.Debug("loaded objects", zap.Any("coord", c.Coord), zap.Int("count", len(objects)))

	rawObjects := make([]*repository.Object, len(objects))
	ownerIDs := make([]int64, 0, len(objects))
	for i := range objects {
		rawObjects[i] = &objects[i]
		ownerIDs = append(ownerIDs, objects[i].ID)
	}

	rawInventoriesByOwner := make(map[types.EntityID][]repository.Inventory, len(ownerIDs))
	if len(ownerIDs) > 0 {
		inventories, invErr := loadInventoriesByOwners(ctx, db, ownerIDs)
		if invErr != nil {
			logger.Error("failed to load object inventories",
				zap.Int("chunk_x", c.Coord.X),
				zap.Int("chunk_y", c.Coord.Y),
				zap.Error(invErr),
			)
			return fmt.Errorf("load inventories for chunk %v: %w", c.Coord, invErr)
		} else {
			for _, inv := range inventories {
				ownerID := types.EntityID(inv.OwnerID)
				rawInventoriesByOwner[ownerID] = append(rawInventoriesByOwner[ownerID], inv)
			}
		}
	}
	c.installLoadedObjects(revision, rawObjects, rawInventoriesByOwner)

	c.SetState(types.ChunkStatePreloaded)
	return nil
}

func upsertInventoriesBatch(ctx context.Context, db *persistence.Postgres, inventories []repository.Inventory) error {
	if len(inventories) == 0 {
		return nil
	}

	ownerIDs := make([]int64, 0, len(inventories))
	kinds := make([]int, 0, len(inventories))
	keys := make([]int, 0, len(inventories))
	datas := make([]string, 0, len(inventories))
	versions := make([]int, 0, len(inventories))

	for _, inv := range inventories {
		ownerIDs = append(ownerIDs, inv.OwnerID)
		kinds = append(kinds, int(inv.Kind))
		keys = append(keys, int(inv.InventoryKey))
		datas = append(datas, string(inv.Data))
		versions = append(versions, inv.Version)
	}

	return db.Queries().UpsertInventories(ctx, repository.UpsertInventoriesParams{
		OwnerIds:      ownerIDs,
		Kinds:         kinds,
		InventoryKeys: keys,
		Datas:         datas,
		Versions:      versions,
	})
}

func upsertObjectsBatch(ctx context.Context, db *persistence.Postgres, objects []*repository.Object) error {
	if len(objects) == 0 {
		return nil
	}
	var saveErr error
	for _, obj := range objects {
		if obj == nil {
			continue
		}
		if err := db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
			ID:         obj.ID,
			TypeID:     obj.TypeID,
			Region:     obj.Region,
			X:          obj.X,
			Y:          obj.Y,
			Layer:      obj.Layer,
			ChunkX:     obj.ChunkX,
			ChunkY:     obj.ChunkY,
			Heading:    obj.Heading,
			Quality:    obj.Quality,
			Hp:         obj.Hp,
			OwnerID:    obj.OwnerID,
			Data:       obj.Data,
			CreateTick: obj.CreateTick,
			LastTick:   obj.LastTick,
		}); err != nil {
			saveErr = errors.Join(saveErr, err)
		}
	}
	return saveErr
}

func loadInventoriesByOwners(ctx context.Context, db *persistence.Postgres, ownerIDs []int64) ([]repository.Inventory, error) {
	if len(ownerIDs) == 0 {
		return nil, nil
	}
	return db.Queries().GetInventoriesByOwners(ctx, ownerIDs)
}
