package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// ChunkUpdateSystem updates ChunkRef components when entities move between chunks
// Runs at priority 250 (after collision, before visibility)
type ChunkUpdateSystem struct {
	ecs.BaseSystem
}

// NewChunkUpdateSystem creates a new chunk update system
func NewChunkUpdateSystem() *ChunkUpdateSystem {
	return &ChunkUpdateSystem{
		BaseSystem: ecs.NewBaseSystem("ChunkUpdateSystem", 250),
	}
}

// Update checks all dynamic entities and updates their ChunkRef if needed
func (s *ChunkUpdateSystem) Update(w *ecs.World, dt float64) {
	chunkIndex := ecs.GetChunkIndex(w)
	if chunkIndex == nil {
		return
	}

	posStorage := ecs.GetOrCreateStorage[components.Position](w)
	chunkRefStorage := ecs.GetOrCreateStorage[components.ChunkRef](w)

	// Use active lists if available, otherwise query all entities with Position + ChunkRef
	activeLists := ecs.GetActiveLists(w)

	var handles []ecs.Handle
	if activeLists != nil && len(activeLists.Dynamic) > 0 {
		// Only check dynamic entities (they're the ones that move)
		handles = activeLists.Dynamic
	} else {
		// Fallback: query all entities with Position and ChunkRef
		query := w.Query().
			With(components.PositionID).
			With(components.ChunkRefID)
		handles = query.Handles()
	}

	for _, h := range handles {
		pos, ok := posStorage.Get(h)
		if !ok {
			continue
		}

		chunkRef := chunkRefStorage.GetPtr(h)
		if chunkRef == nil {
			continue
		}

		// Calculate current chunk from position
		newChunkX, newChunkY := WorldToChunkCoords(pos.X, pos.Y)

		// Check if chunk changed
		if newChunkX != chunkRef.ChunkX || newChunkY != chunkRef.ChunkY {
			// Update ChunkRef component
			chunkRef.ChunkX = newChunkX
			chunkRef.ChunkY = newChunkY

			// Update chunk index
			newKey := chunkRef.Key()
			chunkIndex.UpdateChunk(h, newKey)
		}
	}
}

// InitializeEntityChunk sets up ChunkRef for a new entity and adds it to the index
// Should be called when spawning entities with Position
func InitializeEntityChunk(w *ecs.World, h ecs.Handle, region, layer int32, worldX, worldY float64) {
	chunkRef := ChunkRefFromPosition(region, layer, worldX, worldY)
	ecs.AddComponent(w, h, chunkRef)

	chunkIndex := ecs.GetChunkIndex(w)
	if chunkIndex != nil {
		chunkIndex.Add(h, chunkRef.Key())
	}
}

// RemoveEntityFromChunkIndex removes an entity from the chunk index
// Should be called when despawning entities
func RemoveEntityFromChunkIndex(w *ecs.World, h ecs.Handle) {
	chunkIndex := ecs.GetChunkIndex(w)
	if chunkIndex != nil {
		chunkIndex.Remove(h)
	}
}
