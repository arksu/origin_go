package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

const LiftCarryFollowSystemPriority = 305

type LiftCarryFollowCoordinator interface {
	SyncLiftCarryFollow(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, carry components.LiftCarryState)
}

type LiftCarryFollowSystem struct {
	ecs.BaseSystem
	logger  *zap.Logger
	service LiftCarryFollowCoordinator
	query   *ecs.PreparedQuery
}

func NewLiftCarryFollowSystem(world *ecs.World, service LiftCarryFollowCoordinator, logger *zap.Logger) *LiftCarryFollowSystem {
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
	}
}

func (s *LiftCarryFollowSystem) Update(w *ecs.World, dt float64) {
	if s == nil || w == nil || s.service == nil {
		return
	}
	s.query.ForEach(func(h types.Handle) {
		playerID, hasExternalID := w.GetExternalID(h)
		if !hasExternalID {
			return
		}
		carry, hasCarry := ecs.GetComponent[components.LiftCarryState](w, h)
		if !hasCarry {
			return
		}
		s.service.SyncLiftCarryFollow(w, playerID, h, carry)
	})
}

