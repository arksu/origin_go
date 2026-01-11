package core

import (
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"sync"
)

// Chunk extends ChunkData with game-specific functionality
type Chunk struct {
	*types.ChunkData

	isPassable  []uint64
	isSwimmable []uint64

	rawObjects []*repository.Object
	spatial    *SpatialHashGrid

	mu sync.RWMutex
}

func NewChunk(coord types.ChunkCoord, layer int32, chunkSize int) *Chunk {
	cellSize := 16.0
	totalTiles := chunkSize * chunkSize
	bitsetSize := (totalTiles + 63) / 64

	return &Chunk{
		ChunkData:   types.NewChunkData(coord, layer, chunkSize),
		isPassable:  make([]uint64, bitsetSize),
		isSwimmable: make([]uint64, bitsetSize),
		spatial:     NewSpatialHashGrid(cellSize),
	}
}

func (c *Chunk) SetState(state types.ChunkState) {
	c.mu.Lock()
	c.ChunkData.State = state
	c.mu.Unlock()
}

func (c *Chunk) GetState() types.ChunkState {
	c.mu.RLock()
	state := c.ChunkData.State
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
	c.ChunkData.Tiles = Tiles
	c.ChunkData.LastTick = lastTick
	c.mu.Unlock()
}

func (c *Chunk) populateTileBitsets() {
	for i, tileID := range c.ChunkData.Tiles {
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
	if index >= len(c.ChunkData.Tiles) {
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
	if index >= len(c.ChunkData.Tiles) {
		return false
	}
	return c.getBit(c.isSwimmable, index)
}
