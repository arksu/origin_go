package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/lifecycle"
	"origin/internal/types"

	"go.uber.org/zap"
)

const dropDecaySweepIntervalTicks = 10

// ObjectDeleter handles DB deletion of a persistent object and its owned inventories.
type ObjectDeleter interface {
	DeleteObject(region int, entityID types.EntityID) error
}

// DroppedItemSpatialRemover removes dropped item entities from chunk spatial hash.
type DroppedItemSpatialRemover interface {
	RemoveStaticFromChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int)
}

// DropDecaySystem periodically sweeps DroppedItem entities and despawns expired ones.
type DropDecaySystem struct {
	ecs.BaseSystem
	query          *ecs.PreparedQuery
	deleter        ObjectDeleter
	spatialRemover DroppedItemSpatialRemover
	logger         *zap.Logger
}

func NewDropDecaySystem(
	deleter ObjectDeleter,
	spatialRemover DroppedItemSpatialRemover,
	logger *zap.Logger,
) *DropDecaySystem {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DropDecaySystem{
		BaseSystem:     ecs.NewBaseSystemWithInterval("DropDecay", 900, dropDecaySweepIntervalTicks),
		deleter:        deleter,
		spatialRemover: spatialRemover,
		logger:         logger,
	}
}

func (s *DropDecaySystem) Update(w *ecs.World, dt float64) {
	if s.query == nil {
		s.query = w.Query().With(components.DroppedItemComponentID).Prepare()
	}

	nowRuntimeSeconds := ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal

	type expiredEntry struct {
		entityID types.EntityID
		handle   types.Handle
		region   int
		chunkX   int
		chunkY   int
		x        int
		y        int
	}

	var expired []expiredEntry

	s.query.ForEach(func(h types.Handle) {
		dropped, ok := ecs.GetComponent[components.DroppedItem](w, h)
		if !ok {
			return
		}

		if components.IsDroppedItemExpired(dropped.DropTime, nowRuntimeSeconds) {
			extID, hasExt := ecs.GetComponent[ecs.ExternalID](w, h)
			if !hasExt {
				return
			}
			info, hasInfo := ecs.GetComponent[components.EntityInfo](w, h)
			region := 0
			if hasInfo {
				region = info.Region
			}
			chunkX, chunkY := 0, 0
			if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, h); hasChunkRef {
				chunkX = chunkRef.CurrentChunkX
				chunkY = chunkRef.CurrentChunkY
			}
			x, y := 0, 0
			if transform, hasTransform := ecs.GetComponent[components.Transform](w, h); hasTransform {
				x = int(transform.X)
				y = int(transform.Y)
			}
			expired = append(expired, expiredEntry{
				entityID: extID.ID,
				handle:   h,
				region:   region,
				chunkX:   chunkX,
				chunkY:   chunkY,
				x:        x,
				y:        y,
			})
		}
	})

	for _, e := range expired {
		s.logger.Debug("Despawning expired dropped item",
			zap.Uint64("entity_id", uint64(e.entityID)))

		// Commit the durable deletion before changing ECS so a transient DB outage
		// leaves the item available for a later retry instead of orphaning it.
		if s.deleter == nil {
			s.logger.Error("Cannot delete expired dropped object without persistence",
				zap.Uint64("entity_id", uint64(e.entityID)))
			continue
		}
		if err := s.deleter.DeleteObject(e.region, e.entityID); err != nil {
			s.logger.Error("Failed to delete expired dropped object from DB",
				zap.Uint64("entity_id", uint64(e.entityID)),
				zap.Error(err))
			continue
		}
		if s.spatialRemover != nil {
			s.spatialRemover.RemoveStaticFromChunkSpatial(e.handle, e.chunkX, e.chunkY, e.x, e.y)
		}
		if !lifecycle.DeleteObject(w, e.entityID, e.handle, lifecycle.DeleteObjectOptions{DeleteOwnedInventories: true}) {
			s.logger.Warn("Failed to delete expired dropped item from ECS",
				zap.Uint64("entity_id", uint64(e.entityID)))
		}
	}
}
