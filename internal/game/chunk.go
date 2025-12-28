package game

import (
	"context"
	"fmt"
	"origin/internal/db"
	"origin/internal/ecs"
	"origin/internal/persistence"
	"sync"
	"time"
)

// Doc: chunks stored in database with binary data - array of tiles: byte (tile type), 2 bytes uint16 (extra data, not used yet)

// Chunk represents a CHUNK_SIZE * CHUNK_SIZE area of the world
type Chunk struct {
	Region     int
	X          int
	Y          int
	Layer      int
	Tiles      [][]Tile
	Entities   map[ecs.EntityID]struct{} // Set of entity IDs in this chunk
	LastAccess time.Time
	Dirty      bool // flag that tiles were modified and need to store data into db
	mu         sync.RWMutex
}

// ChunkManager manages chunk loading/unloading
type ChunkManager struct {
	db     *persistence.Postgres
	chunks map[string]*Chunk
	mu     sync.RWMutex
}

func NewChunkManager(db *persistence.Postgres) *ChunkManager {
	return &ChunkManager{
		db:     db,
		chunks: make(map[string]*Chunk),
	}
}

// LoadChunk loads a chunk from the database, parses binary data and builds a Chunk struct
func (cm *ChunkManager) LoadChunk(ctx context.Context, queries *db.Queries, region, x, y, layer int) (*Chunk, error) {
	dbChunk, err := queries.GetChunk(ctx, db.GetChunkParams{
		Region: int32(region),
		X:      int32(x),
		Y:      int32(y),
		Layer:  int32(layer),
	})
	if err != nil {
		return nil, err
	}

	chunk := &Chunk{
		Region:     region,
		X:          x,
		Y:          y,
		Layer:      layer,
		Tiles:      make([][]Tile, CHUNK_SIZE),
		Entities:   make(map[ecs.EntityID]struct{}),
		LastAccess: time.Now(),
		Dirty:      false,
	}

	// Parse binary data: each tile is 3 bytes (1 byte type + 2 bytes uint16 data)
	expectedSize := CHUNK_SIZE * CHUNK_SIZE * 3
	if len(dbChunk.Data) != expectedSize {
		return nil, fmt.Errorf("invalid chunk data size: expected %d, got %d", expectedSize, len(dbChunk.Data))
	}

	offset := 0
	for i := 0; i < CHUNK_SIZE; i++ {
		chunk.Tiles[i] = make([]Tile, CHUNK_SIZE)
		for j := 0; j < CHUNK_SIZE; j++ {
			tileType := TileType(dbChunk.Data[offset])
			extraData := uint16(dbChunk.Data[offset+1])<<8 | uint16(dbChunk.Data[offset+2])

			chunk.Tiles[i][j] = Tile{
				tileType: tileType,
				walkable: isWalkable(tileType),
				data:     extraData,
			}
			offset += 3
		}
	}

	return chunk, nil
}

// isWalkable determines if a tile type is walkable
func isWalkable(tileType TileType) bool {
	switch tileType {
	// TODO swimming tiles
	case TileWaterDeep, TileWater:
		return false
	default:
		return true
	}
}

// chunkKey generates a unique key for chunk lookup
func chunkKey(region, x, y, layer int) string {
	return fmt.Sprintf("%d:%d:%d:%d", region, x, y, layer)
}

// GetOrLoadChunk returns a cached chunk or loads it from the database
func (cm *ChunkManager) GetOrLoadChunk(ctx context.Context, queries *db.Queries, region, x, y, layer int) (*Chunk, error) {
	key := chunkKey(region, x, y, layer)

	cm.mu.RLock()
	chunk, ok := cm.chunks[key]
	cm.mu.RUnlock()
	if ok {
		chunk.mu.Lock()
		chunk.LastAccess = time.Now()
		chunk.mu.Unlock()
		return chunk, nil
	}

	// Load from database
	chunk, err := cm.LoadChunk(ctx, queries, region, x, y, layer)
	if err != nil {
		return nil, err
	}

	cm.mu.Lock()
	cm.chunks[key] = chunk
	cm.mu.Unlock()

	return chunk, nil
}

// IsPositionWalkable checks if a world coordinate position is walkable
// x, y are in world coordinates (not tile coordinates)
func (c *Chunk) IsPositionWalkable(worldX, worldY int) bool {
	// Convert world coordinates to tile coordinates within chunk
	// World coords are in COORD_PER_TILE units, chunk tiles are indexed 0 to CHUNK_SIZE-1
	chunkWorldX := c.X * CHUNK_SIZE * COORD_PER_TILE
	chunkWorldY := c.Y * CHUNK_SIZE * COORD_PER_TILE

	// Get relative position within chunk
	relX := worldX - chunkWorldX
	relY := worldY - chunkWorldY

	// Convert to tile index
	tileX := relX / COORD_PER_TILE
	tileY := relY / COORD_PER_TILE

	// Bounds check
	if tileX < 0 || tileX >= CHUNK_SIZE || tileY < 0 || tileY >= CHUNK_SIZE {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Tiles[tileY][tileX].walkable
}
