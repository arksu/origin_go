package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

const (
	// ChunkInterestRadius defines how many chunks around a player are active
	// 3 means a 7x7 grid (player chunk ± 3 in each direction)
	ChunkInterestRadius = 3

	// ChunkSize is the size of a chunk in world coordinate units
	// Must match game.CHUNK_SIZE * game.COORD_PER_TILE
	ChunkSize = 128 * 12 // 1536 world units
)

// ActiveChunksSystem collects active chunks based on player positions
// Runs at priority 10 (very early, before other systems)
type ActiveChunksSystem struct {
	ecs.BaseSystem
}

// NewActiveChunksSystem creates a new active chunks system
func NewActiveChunksSystem() *ActiveChunksSystem {
	return &ActiveChunksSystem{
		BaseSystem: ecs.NewBaseSystem("ActiveChunksSystem", 10),
	}
}

// Update collects active chunks from all players
func (s *ActiveChunksSystem) Update(w *ecs.World, dt float64) {
	activeLists := ecs.GetActiveLists(w)
	if activeLists == nil {
		activeLists = ecs.NewActiveLists()
		ecs.SetActiveLists(w, activeLists)
	}

	// Clear previous tick's active chunks
	activeLists.Clear()

	// Get storages
	posStorage := ecs.GetOrCreateStorage[components.Position](w)
	chunkRefStorage := ecs.GetOrCreateStorage[components.ChunkRef](w)

	// Query all players (observers that define interest)
	playerQuery := w.Query().
		With(components.PlayerID).
		With(components.PositionID)

	for _, h := range playerQuery.Handles() {
		pos, ok := posStorage.Get(h)
		if !ok {
			continue
		}

		// Get player's chunk coordinates
		var region, layer int32 = 0, 0 // Default region/layer
		if chunkRef, ok := chunkRefStorage.Get(h); ok {
			region = chunkRef.Region
			layer = chunkRef.Layer
		}

		// Calculate chunk coordinates from world position
		chunkX := int32(pos.X) / ChunkSize
		chunkY := int32(pos.Y) / ChunkSize

		// Handle negative coordinates
		if pos.X < 0 {
			chunkX--
		}
		if pos.Y < 0 {
			chunkY--
		}

		// Add chunks in interest radius
		for dx := -ChunkInterestRadius; dx <= ChunkInterestRadius; dx++ {
			for dy := -ChunkInterestRadius; dy <= ChunkInterestRadius; dy++ {
				key := components.ChunkKeyFromCoords(region, layer, chunkX+int32(dx), chunkY+int32(dy))
				activeLists.AddActiveChunk(key)
			}
		}
	}
}

// WorldToChunkCoords converts world coordinates to chunk coordinates
func WorldToChunkCoords(worldX, worldY float64) (chunkX, chunkY int32) {
	// Use floor division for correct negative coordinate handling
	chunkX = int32(floorDiv(worldX, float64(ChunkSize)))
	chunkY = int32(floorDiv(worldY, float64(ChunkSize)))
	return
}

// floorDiv performs floor division (rounds towards negative infinity)
func floorDiv(a, b float64) float64 {
	if a >= 0 {
		return float64(int64(a / b))
	}
	// For negative numbers, we need floor behavior
	q := int64(a / b)
	if float64(q)*b != a {
		q--
	}
	return float64(q)
}

// ChunkRefFromPosition creates a ChunkRef from world position
func ChunkRefFromPosition(region, layer int32, worldX, worldY float64) components.ChunkRef {
	chunkX, chunkY := WorldToChunkCoords(worldX, worldY)
	return components.ChunkRef{
		Region: region,
		Layer:  layer,
		ChunkX: chunkX,
		ChunkY: chunkY,
	}
}
