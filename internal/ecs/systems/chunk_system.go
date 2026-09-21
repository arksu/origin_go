package systems

import (
	_const "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

type ChunkSystem struct {
	ecs.BaseSystem
	chunkManager core.ChunkManager
	logger       *zap.Logger
}

func NewChunkSystem(chunkManager core.ChunkManager, logger *zap.Logger) *ChunkSystem {
	return &ChunkSystem{
		BaseSystem:   ecs.NewBaseSystem("ChunkSystem", 400),
		chunkManager: chunkManager,
		logger:       logger,
	}
}

func (s *ChunkSystem) Update(w *ecs.World, dt float64) {
	movedEntities := ecs.GetResource[ecs.MovedEntities](w)
	// Process only entities that moved this frame
	for i := 0; i < movedEntities.Count; i++ {
		h := movedEntities.Handles[i]

		if !w.Alive(h) {
			continue
		}

		chunkRef, ok := ecs.GetComponent[components.ChunkRef](w, h)
		if !ok {
			continue
		}

		// ChunkSystem runs after TransformUpdateSystem, which already
		// committed the collision-adjusted final position to the transform.
		// Migrate on where the entity IS, not where it asked to go: intent
		// and final diverge whenever collision alters the path near a border.
		transform, ok := ecs.GetComponent[components.Transform](w, h)
		if !ok {
			continue
		}
		newChunkX := floorDiv(int(transform.X), _const.ChunkWorldSize)
		newChunkY := floorDiv(int(transform.Y), _const.ChunkWorldSize)

		// Check if entity needs to migrate to different chunk
		if newChunkX != chunkRef.CurrentChunkX || newChunkY != chunkRef.CurrentChunkY {
			s.migrateEntity(w, h, chunkRef, transform, newChunkX, newChunkY)
		}
	}
}

func (s *ChunkSystem) migrateEntity(
	w *ecs.World,
	h types.Handle,
	chunkRef components.ChunkRef,
	transform components.Transform,
	newChunkX, newChunkY int,
) {
	// Validate everything before mutating: aborting midway would leave the
	// spatial registration and ChunkRef inconsistent.
	newChunkCoord := types.ChunkCoord{X: newChunkX, Y: newChunkY}
	newChunk := s.chunkManager.GetChunk(newChunkCoord)
	if newChunk == nil || newChunk.State != types.ChunkStateActive {
		s.logger.Error("Target chunk not found or not active for entity migration",
			zap.Uint64("handle", uint64(h)),
			zap.Int("chunk_x", newChunkX),
			zap.Int("chunk_y", newChunkY),
			zap.String("chunk_state", func() string {
				if newChunk == nil {
					return "nil"
				}
				return string(newChunk.State)
			}()))
		return
	}

	entityID, hasEntityID := w.GetExternalID(h)
	if !hasEntityID {
		s.logger.Error("Entity missing external ID for chunk migration",
			zap.Uint64("handle", uint64(h)),
			zap.Int("new_chunk_x", newChunkX),
			zap.Int("new_chunk_y", newChunkY),
		)
		return
	}

	entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](w, h)
	isStatic := hasEntityInfo && entityInfo.IsStatic

	// Remove from the old chunk's grid using the final position:
	// TransformUpdateSystem already moved the entry to this cell (the grid
	// update there must never be skipped, even on cross-border moves).
	oldChunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
	oldChunk := s.chunkManager.GetChunk(oldChunkCoord)
	if oldChunk != nil && oldChunk.State == types.ChunkStateActive {
		if isStatic {
			oldChunk.Spatial().RemoveStatic(h, int(transform.X), int(transform.Y))
		} else {
			oldChunk.Spatial().RemoveDynamic(h, int(transform.X), int(transform.Y))
		}
	}

	// Add entity to new chunk spatial hash
	if isStatic {
		newChunk.Spatial().AddStatic(h, int(transform.X), int(transform.Y))
	} else {
		newChunk.Spatial().AddDynamic(h, int(transform.X), int(transform.Y))
	}

	// Update ChunkRef component
	ecs.WithComponent(w, h, func(cr *components.ChunkRef) {
		cr.PrevChunkX = cr.CurrentChunkX
		cr.PrevChunkY = cr.CurrentChunkY
		cr.CurrentChunkX = newChunkX
		cr.CurrentChunkY = newChunkY
	})

	// Update entity position in chunk manager (drives AOI and chunk streaming)
	s.chunkManager.UpdateEntityPosition(entityID, newChunkCoord)
}
