package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

const LiftCarryFollowSystemPriority = 305

type LiftCarryFollowCoordinator interface {
	SyncLiftCarryFollow(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, carry components.LiftCarryState) *ecs.MoveBatchEntry
}

type LiftCarryFollowSystem struct {
	ecs.BaseSystem
	logger   *zap.Logger
	service  LiftCarryFollowCoordinator
	query    *ecs.PreparedQuery
	eventBus *eventbus.EventBus
	moves    []ecs.MoveBatchEntry
}

func NewLiftCarryFollowSystem(world *ecs.World, service LiftCarryFollowCoordinator, eventBus *eventbus.EventBus, logger *zap.Logger) *LiftCarryFollowSystem {
	if logger == nil {
		logger = zap.NewNop()
	}
	query := ecs.NewPreparedQuery(
		world,
		0|(1<<components.LiftCarryStateComponentID),
		0,
	)
	return &LiftCarryFollowSystem{
		BaseSystem: ecs.NewBaseSystem("LiftCarryFollowSystem", LiftCarryFollowSystemPriority),
		logger:     logger,
		service:    service,
		query:      query,
		eventBus:   eventBus,
	}
}

func (s *LiftCarryFollowSystem) Update(w *ecs.World, dt float64) {
	if s == nil || w == nil || s.service == nil {
		return
	}
	s.moves = s.moves[:0]
	s.query.ForEach(func(h types.Handle) {
		playerID, hasExternalID := w.GetExternalID(h)
		if !hasExternalID {
			return
		}
		carry, hasCarry := ecs.GetComponent[components.LiftCarryState](w, h)
		if !hasCarry {
			return
		}
		if entry := s.service.SyncLiftCarryFollow(w, playerID, h, carry); entry != nil {
			s.moves = append(s.moves, *entry)
		}
	})
	if len(s.moves) > 0 && s.eventBus != nil {
		s.eventBus.PublishAsync(ecs.NewObjectMoveBatchEvent(w.Layer, s.moves), eventbus.PriorityMedium)
	}
}
