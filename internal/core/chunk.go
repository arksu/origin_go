package core

import (
	"context"
	"database/sql"
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

	isPassable  []uint64
	isSwimmable []uint64

	rawObjects []*repository.Object
	// rawInventoriesByOwner keeps preloaded/inactive object-owned inventories by owner entity id.
	rawInventoriesByOwner map[types.EntityID][]repository.Inventory
	// rawDirtyObjectIDs tracks object IDs changed while chunk was active and then deactivated.
	// It allows delta persistence for inactive chunks without rewriting all raw objects.
	rawDirtyObjectIDs map[types.EntityID]struct{}
	// deletedObjectIDs tracks runtime-despawned object ids that must be soft-deleted in DB
	// on the next save, even though they no longer exist in the active ECS chunk handles.
	deletedObjectIDs map[types.EntityID]struct{}
	rawDataDirty     bool
	spatial          *SpatialHashGrid

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
		Tiles:                 make([]byte, totalTiles),
		isPassable:            make([]uint64, bitsetSize),
		isSwimmable:           make([]uint64, bitsetSize),
		rawInventoriesByOwner: make(map[types.EntityID][]repository.Inventory, 8),
		rawDirtyObjectIDs:     make(map[types.EntityID]struct{}, 8),
		deletedObjectIDs:      make(map[types.EntityID]struct{}, 8),
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
	c.rawDirtyObjectIDs = make(map[types.EntityID]struct{}, 8)
	c.mu.Unlock()
}

func (c *Chunk) GetRawObjects() []*repository.Object {
	c.mu.RLock()
	objects := c.rawObjects
	c.mu.RUnlock()
	return objects
}

func (c *Chunk) AddRawObject(obj *repository.Object) {
	c.mu.Lock()
	c.rawObjects = append(c.rawObjects, obj)
	if obj != nil {
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

	replaced := false
	for i := range c.rawObjects {
		if c.rawObjects[i] == nil || types.EntityID(c.rawObjects[i].ID) != id {
			continue
		}
		c.rawObjects[i] = obj
		replaced = true
		break
	}
	if !replaced {
		c.rawObjects = append(c.rawObjects, obj)
	}
	c.rawDirtyObjectIDs[id] = struct{}{}
	delete(c.deletedObjectIDs, id)
	c.rawDataDirty = true
}

func (c *Chunk) ClearRawObjects() {
	c.mu.Lock()
	c.rawObjects = nil
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

func (c *Chunk) SetTiles(tiles []byte, lastTick uint64) {
	c.mu.Lock()
	c.Tiles = tiles
	c.LastTick = lastTick
	c.Version++ // инкрементируем версию при изменении тайлов
	c.tilesDirty = true
	c.populateTileBitsets()
	c.mu.Unlock()
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

// IsDirty returns true if tiles have been modified or any active object is dirty.
// For inactive/preloaded chunks with raw objects, only tilesDirty is checked
// since raw objects are never mutated in-memory.
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
	for i, tileID := range c.Tiles {
		if types.IsTilePassable(tileID) {
			c.setBit(c.isPassable, i)
		}
		if types.IsTileSwimmable(tileID) {
			c.setBit(c.isSwimmable, i)
		}
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
// For active chunks, only objects with ObjectInternalState.IsDirty are serialized.
func (c *Chunk) SaveToDB(db *persistence.Postgres, world *ecs.World, objectFactory interface {
	Serialize(world *ecs.World, h types.Handle) (*repository.Object, error)
	SerializeObjectInventories(world *ecs.World, h types.Handle) ([]repository.Inventory, error)
	HasPersistentInventories(typeID uint32, behaviors []string) bool
}, logger *zap.Logger) {
	if db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coord := c.Coord

	c.mu.RLock()
	saveTiles := c.tilesDirty
	var tiles []byte
	if saveTiles {
		tiles = make([]byte, len(c.Tiles))
		copy(tiles, c.Tiles)
	}
	lastTick := c.LastTick
	totalHandles := c.GetHandles()
	rawObjects := c.GetRawObjects()
	rawInventoriesByOwner := c.GetRawInventoriesByOwner()
	rawDirtyObjectIDs := c.GetRawDirtyObjectIDs()
	pendingDeletedObjectIDs := c.GetDeletedObjectIDs()
	c.mu.RUnlock()

	// Determine dirty objects to save
	var objectsToSave []*repository.Object
	inventoriesToSave := make([]repository.Inventory, 0, 16)
	deletedObjectIDs := make([]int64, 0, 8)
	var dirtyHandles []types.Handle

	if len(totalHandles) > 0 {
		// Chunk is active - serialize only dirty entities
		for _, h := range totalHandles {
			if !world.Alive(h) {
				continue
			}

			state, hasState := ecs.GetComponent[components.ObjectInternalState](world, h)
			if hasState && !state.IsDirty {
				continue
			}

			info, ok := ecs.GetComponent[components.EntityInfo](world, h)
			if !ok {
				continue
			}

			obj, err := objectFactory.Serialize(world, h)
			if err != nil {
				logger.Error("failed to serialize object",
					zap.Uint32("type_id", info.TypeID),
					zap.Error(err),
				)
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
				} else if len(inventories) > 0 {
					inventoriesToSave = append(inventoriesToSave, inventories...)
				}
			}

			dirtyHandles = append(dirtyHandles, h)
		}
	} else {
		// Chunk is inactive/preloaded - persist only dirty raw objects/inventories.
		if len(rawDirtyObjectIDs) > 0 {
			for _, rawObj := range rawObjects {
				if rawObj == nil {
					continue
				}
				if _, dirty := rawDirtyObjectIDs[types.EntityID(rawObj.ID)]; dirty {
					objectsToSave = append(objectsToSave, rawObj)
				}
			}
			for ownerID, rows := range rawInventoriesByOwner {
				if _, dirty := rawDirtyObjectIDs[ownerID]; !dirty {
					continue
				}
				inventoriesToSave = append(inventoriesToSave, rows...)
			}
		}
	}

	if saveTiles {
		entityCount := len(totalHandles)
		if entityCount == 0 {
			entityCount = len(rawObjects)
		}
		err := db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{
			Region:      c.Region,
			X:           coord.X,
			Y:           coord.Y,
			Layer:       c.Layer,
			TilesData:   tiles,
			LastTick:    int64(lastTick),
			EntityCount: sql.NullInt32{Int32: int32(entityCount), Valid: true},
		})
		if err != nil {
			logger.Error("failed to save chunk tiles",
				zap.Int("chunk_x", coord.X),
				zap.Int("chunk_y", coord.Y),
				zap.Error(err),
			)
		} else {
			c.ClearTilesDirty()
		}
	}

	for deletedID := range pendingDeletedObjectIDs {
		deletedObjectIDs = append(deletedObjectIDs, int64(deletedID))
	}

	// Delete objects that became non-persistent (serialize -> nil) before upserts.
	saveFailed := false
	if len(deletedObjectIDs) > 0 {
		for _, objectID := range deletedObjectIDs {
			if err := db.Queries().DeleteObject(ctx, objectID); err != nil {
				logger.Error("failed to delete object during chunk save",
					zap.Int64("object_id", objectID),
					zap.Any("coord", c.Coord),
					zap.Error(err),
				)
				saveFailed = true
			}
		}
	}

	// Save objects batch
	// (saveFailed may already be set by delete pass above)
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
				saveFailed = true
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
			saveFailed = true
		}
	}

	// Clear dirty flags only after successful save; otherwise keep for retry.
	if !saveFailed {
		for _, h := range dirtyHandles {
			ecs.WithComponent(world, h, func(s *components.ObjectInternalState) {
				s.IsDirty = false
			})
		}
	}
	if !saveFailed && len(totalHandles) == 0 {
		c.ClearRawDataDirty()
		c.ClearRawDirtyObjectIDs()
	}
	if !saveFailed {
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
}

// LoadFromDB loads chunk data and objects from the database
func (c *Chunk) LoadFromDB(db *persistence.Postgres, region int, layer int, logger *zap.Logger) error {
	c.SetState(types.ChunkStateLoading)

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
	if err == nil {
		c.SetTiles(tilesData.TilesData, uint64(tilesData.LastTick))
		c.ClearTilesDirty()
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
		objects = nil
	}
	logger.Debug("loaded objects", zap.Any("coord", c.Coord), zap.Int("count", len(objects)))

	rawObjects := make([]*repository.Object, len(objects))
	ownerIDs := make([]int64, 0, len(objects))
	for i := range objects {
		rawObjects[i] = &objects[i]
		ownerIDs = append(ownerIDs, objects[i].ID)
	}
	c.SetRawObjects(rawObjects)

	rawInventoriesByOwner := make(map[types.EntityID][]repository.Inventory, len(ownerIDs))
	if len(ownerIDs) > 0 {
		inventories, invErr := loadGridInventoriesByOwners(ctx, db, ownerIDs)
		if invErr != nil {
			logger.Error("failed to load object inventories",
				zap.Int("chunk_x", c.Coord.X),
				zap.Int("chunk_y", c.Coord.Y),
				zap.Error(invErr),
			)
		} else {
			for _, inv := range inventories {
				ownerID := types.EntityID(inv.OwnerID)
				rawInventoriesByOwner[ownerID] = append(rawInventoriesByOwner[ownerID], inv)
			}
		}
	}
	c.SetRawInventoriesByOwner(rawInventoriesByOwner)

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
			return err
		}
	}
	return nil
}

func loadGridInventoriesByOwners(ctx context.Context, db *persistence.Postgres, ownerIDs []int64) ([]repository.Inventory, error) {
	if len(ownerIDs) == 0 {
		return nil, nil
	}
	return db.Queries().GetGridInventoriesByOwners(ctx, ownerIDs)
}
