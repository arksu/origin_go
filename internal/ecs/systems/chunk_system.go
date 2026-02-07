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
		newX := movedEntities.IntentX[i]
		newY := movedEntities.IntentY[i]

		if !w.Alive(h) {
			continue
		}

		chunkRef, ok := ecs.GetComponent[components.ChunkRef](w, h)
		if !ok {
			continue
		}

		// Calculate current chunk from new position
		newChunkX := int(newX) / _const.ChunkWorldSize
		newChunkY := int(newY) / _const.ChunkWorldSize

		// Check if entity needs to migrate to different chunk
		if newChunkX != chunkRef.CurrentChunkX || newChunkY != chunkRef.CurrentChunkY {
			s.migrateEntity(w, h, chunkRef, newChunkX, newChunkY)
		}
	}
}

func (s *ChunkSystem) migrateEntity(w *ecs.World, h types.Handle, chunkRef components.ChunkRef, newChunkX, newChunkY int) {
	// Get old chunk
	oldChunkCoord := types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}
	oldChunk := s.chunkManager.GetChunk(oldChunkCoord)
	if oldChunk != nil && oldChunk.State == types.ChunkStateActive {
		// Remove entity from old chunk spatial hash
		transform, ok := ecs.GetComponent[components.Transform](w, h)
		if ok {
			entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](w, h)
			if hasEntityInfo && entityInfo.IsStatic {
				oldChunk.Spatial().RemoveStatic(h, int(transform.X), int(transform.Y))
			} else {
				oldChunk.Spatial().RemoveDynamic(h, int(transform.X), int(transform.Y))
			}
		}
	}

	// Get new chunk
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

	// Add entity to new chunk spatial hash
	transform, ok := ecs.GetComponent[components.Transform](w, h)
	if ok {
		entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](w, h)
		if hasEntityInfo && entityInfo.IsStatic {
			newChunk.Spatial().AddStatic(h, int(transform.X), int(transform.Y))
		} else {
			newChunk.Spatial().AddDynamic(h, int(transform.X), int(transform.Y))
		}
	}

	// Get entity ID for UpdateEntityPosition call
	entityID, hasEntityID := w.GetExternalID(h)
	if !hasEntityID {
		s.logger.Error("Entity missing external ID for chunk migration",
			zap.Uint64("handle", uint64(h)),
			zap.Int("new_chunk_x", newChunkX),
			zap.Int("new_chunk_y", newChunkY),
		)
		return
	}

	// Update ChunkRef component
	ecs.WithComponent(w, h, func(cr *components.ChunkRef) {
		cr.PrevChunkX = cr.CurrentChunkX
		cr.PrevChunkY = cr.CurrentChunkY
		cr.CurrentChunkX = newChunkX
		cr.CurrentChunkY = newChunkY
	})

	// Update entity position in chunk manager
	s.chunkManager.UpdateEntityPosition(entityID, newChunkCoord)

	//s.logger.Debug("Entity migrated between chunks",
	//	zap.Uint64("handle", uint64(h)),
	//	zap.Int("from_chunk_x", chunkRef.CurrentChunkX),
	//	zap.Int("from_chunk_y", chunkRef.CurrentChunkY),
	//	zap.Int("to_chunk_x", newChunkX),
	//	zap.Int("to_chunk_y", newChunkY),
	//)
}
