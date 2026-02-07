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

// DropDecaySystem periodically sweeps DroppedItem entities and despawns expired ones.
type DropDecaySystem struct {
	ecs.BaseSystem
	query   *ecs.PreparedQuery
	deleter DroppedObjectDeleter
	logger  *zap.Logger
}

func NewDropDecaySystem(deleter DroppedObjectDeleter, logger *zap.Logger) *DropDecaySystem {
	return &DropDecaySystem{
		BaseSystem: ecs.NewBaseSystemWithInterval("DropDecay", 900, 60),
		deleter:    deleter,
		logger:     logger,
	}
}

func (s *DropDecaySystem) Update(w *ecs.World, dt float64) {
	if s.query == nil {
		s.query = w.Query().With(components.DroppedItemComponentID).Prepare()
	}

	nowUnix := ecs.GetResource[ecs.TimeState](w).Now.Unix()

	type expiredEntry struct {
		entityID types.EntityID
		handle   types.Handle
		region   int
	}

	var expired []expiredEntry

	s.query.ForEach(func(h types.Handle) {
		dropped, ok := ecs.GetComponent[components.DroppedItem](w, h)
		if !ok {
			return
		}

		if dropped.DropTime+constt.DroppedDespawnSeconds <= nowUnix {
			extID, hasExt := ecs.GetComponent[ecs.ExternalID](w, h)
			if !hasExt {
				return
			}
			info, hasInfo := ecs.GetComponent[components.EntityInfo](w, h)
			region := 0
			if hasInfo {
				region = info.Region
			}
			expired = append(expired, expiredEntry{entityID: extID.ID, handle: h, region: region})
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
