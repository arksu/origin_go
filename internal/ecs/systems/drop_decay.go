package systems

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/world"
	"origin/internal/types"

	"go.uber.org/zap"
)

// DropDecaySystem periodically sweeps DroppedItem entities and despawns expired ones.
type DropDecaySystem struct {
	ecs.BaseSystem
	query     *ecs.PreparedQuery
	persister *world.DroppedItemPersisterDB
	logger    *zap.Logger
}

func NewDropDecaySystem(persister *world.DroppedItemPersisterDB, logger *zap.Logger) *DropDecaySystem {
	return &DropDecaySystem{
		BaseSystem: ecs.NewBaseSystemWithInterval("DropDecay", 900, 60),
		persister:  persister,
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
	}

	var expired []expiredEntry

	s.query.ForEach(func(h types.Handle) {
		dropped, ok := ecs.GetComponent[components.DroppedItem](w, h)
		if !ok {
			return
		}

		if dropped.DropTime+constt.DroppedDespawnSeconds <= nowUnix {
			extID, hasExt := ecs.GetComponent[ecs.ExternalID](w, h)
			if hasExt {
				expired = append(expired, expiredEntry{entityID: extID.ID, handle: h})
			}
		}
	})

	for _, e := range expired {
		s.logger.Debug("Despawning expired dropped item",
			zap.Uint64("entity_id", uint64(e.entityID)))
		world.DeleteDroppedObjectFull(w, e.entityID, e.handle, s.persister, s.logger)
	}
}
