package systems

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

// DroppedObjectDeleter handles DB deletion of dropped objects (object + inventory).
type DroppedObjectDeleter interface {
	DeleteDroppedObject(region int, entityID types.EntityID) error
}

// DroppedItemSpatialRemover removes dropped item entities from chunk spatial hash.
type DroppedItemSpatialRemover interface {
	RemoveStaticFromChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int)
}

// DropDecaySystem periodically sweeps DroppedItem entities and despawns expired ones.
type DropDecaySystem struct {
	ecs.BaseSystem
	query          *ecs.PreparedQuery
	deleter        DroppedObjectDeleter
	spatialRemover DroppedItemSpatialRemover
	logger         *zap.Logger
}

func NewDropDecaySystem(
	deleter DroppedObjectDeleter,
	spatialRemover DroppedItemSpatialRemover,
	logger *zap.Logger,
) *DropDecaySystem {
	return &DropDecaySystem{
		BaseSystem:     ecs.NewBaseSystemWithInterval("DropDecay", 900, 60),
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

		if dropped.DropTime+constt.DroppedDespawnSeconds <= nowRuntimeSeconds {
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

		// Remove inventory container from ECS
		refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
		containerHandle, found := refIndex.Lookup(constt.InventoryDroppedItem, e.entityID, 0)
		if found {
			refIndex.Remove(constt.InventoryDroppedItem, e.entityID, 0)
			w.Despawn(containerHandle)
		}
		if s.spatialRemover != nil {
			s.spatialRemover.RemoveStaticFromChunkSpatial(e.handle, e.chunkX, e.chunkY, e.x, e.y)
		}
		w.Despawn(e.handle)

		// Soft-delete from DB (object + inventory)
		if s.deleter != nil && e.region > 0 {
			if err := s.deleter.DeleteDroppedObject(e.region, e.entityID); err != nil {
				s.logger.Error("Failed to delete expired dropped object from DB",
					zap.Uint64("entity_id", uint64(e.entityID)),
					zap.Error(err))
			}
		}
	}
}
