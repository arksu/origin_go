package components

import "origin/internal/ecs"

// ChunkRef tracks which chunk an entity currently belongs to
type ChunkRef struct {
	CurrentChunkX int
	CurrentChunkY int
	PrevChunkX    int
	PrevChunkY    int
	IsMigrating   bool
}

const ChunkRefComponentID ecs.ComponentID = 11

func init() {
	ecs.RegisterComponent[ChunkRef](ChunkRefComponentID)
}
