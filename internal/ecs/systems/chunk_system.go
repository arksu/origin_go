package systems

import (
	"math"

	_const "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

type chunkMigrationManager interface {
	core.ChunkManager
	IsWithinWorldBounds(coord types.ChunkCoord) bool
}

type ChunkSystem struct {
	ecs.BaseSystem
	chunkManager      chunkMigrationManager
	logger            *zap.Logger
	pendingMigrations map[types.Handle]uint64
	updateTick        uint64
}

func NewChunkSystem(chunkManager chunkMigrationManager, logger *zap.Logger) *ChunkSystem {
	return &ChunkSystem{
		BaseSystem:        ecs.NewBaseSystem("ChunkSystem", 400),
		chunkManager:      chunkManager,
		logger:            logger,
		pendingMigrations: make(map[types.Handle]uint64),
	}
}

func (s *ChunkSystem) Update(w *ecs.World, dt float64) {
	s.updateTick++
	movedEntities := ecs.GetResource[ecs.MovedEntities](w)
	for i := 0; i < movedEntities.Count; i++ {
		s.tryMigrateEntity(w, movedEntities.Handles[i])
	}

	// A stopped entity no longer appears in MovedEntities, so keep retrying
	// an aborted migration until its destination becomes active.
	for h, lastAttemptTick := range s.pendingMigrations {
		if lastAttemptTick != s.updateTick {
			s.tryMigrateEntity(w, h)
		}
	}
}

func (s *ChunkSystem) tryMigrateEntity(w *ecs.World, h types.Handle) {
	if !w.Alive(h) {
		delete(s.pendingMigrations, h)
		return
	}
	chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, h)
	transform, hasTransform := ecs.GetComponent[components.Transform](w, h)
	if !hasChunkRef || !hasTransform {
		delete(s.pendingMigrations, h)
		return
	}

	// TransformUpdateSystem has already committed the collision-adjusted
	// position; movement intent may differ near a border.
	// Floor keeps negative fractional positions on the negative side of a border.
	newChunkX := int(math.Floor(transform.X / float64(_const.ChunkWorldSize)))
	newChunkY := int(math.Floor(transform.Y / float64(_const.ChunkWorldSize)))
	if newChunkX == chunkRef.CurrentChunkX && newChunkY == chunkRef.CurrentChunkY {
		delete(s.pendingMigrations, h)
		return
	}
	s.migrateEntity(w, h, chunkRef, transform, newChunkX, newChunkY)
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
	if !s.chunkManager.IsWithinWorldBounds(newChunkCoord) {
		delete(s.pendingMigrations, h)
		s.logger.Error("Entity migration target is outside world bounds",
			zap.Uint64("handle", uint64(h)),
			zap.Int("chunk_x", newChunkX),
			zap.Int("chunk_y", newChunkY),
		)
		return
	}
	newChunk := s.chunkManager.GetChunk(newChunkCoord)
	if newChunk == nil || newChunk.State != types.ChunkStateActive {
		if _, alreadyPending := s.pendingMigrations[h]; !alreadyPending {
			s.logger.Warn("Target chunk not found or not active for entity migration",
				zap.Uint64("handle", uint64(h)),
				zap.Int("chunk_x", newChunkX),
				zap.Int("chunk_y", newChunkY),
				zap.String("chunk_state", func() string {
					if newChunk == nil {
						return "nil"
					}
					return string(newChunk.State)
				}()))
		}
		s.pendingMigrations[h] = s.updateTick
		return
	}

	entityID, hasEntityID := w.GetExternalID(h)
	if !hasEntityID {
		delete(s.pendingMigrations, h)
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
	delete(s.pendingMigrations, h)
}
