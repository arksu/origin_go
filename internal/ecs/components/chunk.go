package components

import "origin/internal/ecs"

// ChunkRef stores the chunk coordinates for an entity
// Used for spatial partitioning and active chunk filtering
type ChunkRef struct {
	Region int32
	Layer  int32
	ChunkX int32
	ChunkY int32
}

// ChunkKey generates a unique key for chunk lookup
// Packs region, layer, x, y into a single uint64
func (c ChunkRef) Key() uint64 {
	// Pack: region (16 bits) | layer (8 bits) | chunkX (20 bits) | chunkY (20 bits)
	return uint64(c.Region&0xFFFF)<<48 |
		uint64(c.Layer&0xFF)<<40 |
		uint64(uint32(c.ChunkX)&0xFFFFF)<<20 |
		uint64(uint32(c.ChunkY)&0xFFFFF)
}

// ChunkKeyFromCoords creates a chunk key from coordinates
func ChunkKeyFromCoords(region, layer, chunkX, chunkY int32) uint64 {
	return uint64(region&0xFFFF)<<48 |
		uint64(layer&0xFF)<<40 |
		uint64(uint32(chunkX)&0xFFFFF)<<20 |
		uint64(uint32(chunkY)&0xFFFFF)
}

// UnpackChunkKey extracts coordinates from a chunk key
func UnpackChunkKey(key uint64) (region, layer, chunkX, chunkY int32) {
	region = int32((key >> 48) & 0xFFFF)
	layer = int32((key >> 40) & 0xFF)
	chunkX = int32((key >> 20) & 0xFFFFF)
	chunkY = int32(key & 0xFFFFF)
	return
}

// Component ID
var ChunkRefID ecs.ComponentID

func init() {
	ChunkRefID = ecs.GetComponentID[ChunkRef]()
}
