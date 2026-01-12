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

// Chunk represents a game chunk with all its data and functionality
type Chunk struct {
	Coord    types.ChunkCoord
	Region   int
	Layer    int
	State    types.ChunkState
	Tiles    []byte
	LastTick uint64

	isPassable  []uint64
	isSwimmable []uint64

	rawObjects []*repository.Object
	spatial    *SpatialHashGrid

	mu sync.RWMutex
}

func NewChunk(coord types.ChunkCoord, region int, layer int, chunkSize int) *Chunk {
	cellSize := 16
	totalTiles := chunkSize * chunkSize
	bitsetSize := (totalTiles + 63) / 64

	return &Chunk{
		Coord:       coord,
		Region:      region,
		Layer:       layer,
		State:       types.ChunkStateUnloaded,
		Tiles:       make([]byte, totalTiles),
		isPassable:  make([]uint64, bitsetSize),
		isSwimmable: make([]uint64, bitsetSize),
		spatial:     NewSpatialHashGrid(cellSize),
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
	c.mu.Unlock()
}

func (c *Chunk) ClearRawObjects() {
	c.mu.Lock()
	c.rawObjects = nil
	c.mu.Unlock()
}

func (c *Chunk) GetHandles() []types.Handle {
	return c.spatial.GetAllHandles()
}

func (c *Chunk) GetDynamicHandles() []types.Handle {
	return c.spatial.GetDynamicHandles()
}

func (c *Chunk) ClearHandles() {
	c.mu.Lock()
	c.spatial.ClearDynamic()
	c.spatial.ClearStatic()
	c.mu.Unlock()
}

func (c *Chunk) Spatial() *SpatialHashGrid {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.spatial
}

func (c *Chunk) SetTiles(Tiles []byte, lastTick uint64) {
	c.mu.Lock()
	c.Tiles = Tiles
	c.LastTick = lastTick
	c.populateTileBitsets()
	c.mu.Unlock()
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

// SaveToDB persists the chunk and its entities to the database
func (c *Chunk) SaveToDB(db *persistence.Postgres, world *ecs.World, objectFactory interface {
	Serialize(world *ecs.World, h types.Handle, objectType components.ObjectType) (*repository.Object, error)
}, logger *zap.Logger) {
	if db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coord := c.Coord

	c.mu.RLock()
	tiles := make([]byte, len(c.Tiles))
	copy(tiles, c.Tiles)
	lastTick := c.LastTick
	totalHandles := c.GetHandles()
	rawObjects := c.GetRawObjects()
	c.mu.RUnlock()

	// Determine entity count and objects to save
	var objectsToSave []*repository.Object
	var entityCount int

	if len(totalHandles) > 0 {
		// Chunk is active - serialize entities from handles
		entityCount = len(totalHandles)
		for _, h := range totalHandles {
			if !world.Alive(h) {
				continue
			}

			info, ok := ecs.GetComponent[components.EntityInfo](world, h)
			if !ok {
				continue
			}

			obj, err := objectFactory.Serialize(world, h, info.ObjectType)
			if err != nil {
				logger.Error("failed to serialize object",
					zap.Error(err),
				)
				continue
			}
			objectsToSave = append(objectsToSave, obj)
		}
	} else {
		// Chunk is inactive - use raw objects directly
		objectsToSave = rawObjects
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
	}

	// Save objects
	for _, obj := range objectsToSave {
		if obj == nil {
			continue
		}

		obj.LastTick = int64(lastTick)
		err = db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
			ID:         obj.ID,
			ObjectType: obj.ObjectType,
			Region:     obj.Region,
			X:          obj.X,
			Y:          obj.Y,
			Layer:      obj.Layer,
			ChunkX:     obj.ChunkX,
			ChunkY:     obj.ChunkY,
			Heading:    obj.Heading,
			Quality:    obj.Quality,
			HpCurrent:  obj.HpCurrent,
			HpMax:      obj.HpMax,
			IsStatic:   obj.IsStatic,
			OwnerID:    obj.OwnerID,
			DataJsonb:  obj.DataJsonb,
			CreateTick: obj.CreateTick,
			LastTick:   obj.LastTick,
		})
		if err != nil {
			logger.Error("failed to save object",
				zap.Int64("object_id", obj.ID),
				zap.Error(err),
			)
		}
	}
	logger.Debug("saved chunk", zap.Any("coord", c.Coord), zap.Int("count", len(objectsToSave)))
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
	for i := range objects {
		rawObjects[i] = &objects[i]
	}
	c.SetRawObjects(rawObjects)

	c.SetState(types.ChunkStatePreloaded)
	return nil
}
