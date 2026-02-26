package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

const LiftPlacementSystemPriority = 251

type LiftPlacementFinalizer interface {
	FinalizePendingLiftTransition(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, pending components.PendingLiftTransition)
	CancelPendingLiftTransition(w *ecs.World, playerID types.EntityID, playerHandle types.Handle)
}

type LiftPlacementSystem struct {
	ecs.BaseSystem
	logger  *zap.Logger
	service LiftPlacementFinalizer
	query   *ecs.PreparedQuery
}

func NewLiftPlacementSystem(world *ecs.World, service LiftPlacementFinalizer, logger *zap.Logger) *LiftPlacementSystem {
	if logger == nil {
		logger = zap.NewNop()
	}
	query := ecs.NewPreparedQuery(
		world,
		0|(1<<components.PendingLiftTransitionComponentID),
		0,
	)
	return &LiftPlacementSystem{
		BaseSystem: ecs.NewBaseSystem("LiftPlacementSystem", LiftPlacementSystemPriority),
		logger:     logger,
		service:    service,
		query:      query,
	}
}

func (s *LiftPlacementSystem) Update(w *ecs.World, dt float64) {
	if s == nil || w == nil {
		return
	}
	nowUnixMs := ecs.GetResource[ecs.TimeState](w).UnixMs
	s.query.ForEach(func(h types.Handle) {
		playerID, hasExternalID := w.GetExternalID(h)
		if !hasExternalID {
			return
		}
		pending, hasPending := ecs.GetComponent[components.PendingLiftTransition](w, h)
		if !hasPending {
			return
		}
		if pending.ExpireAtUnixMs > 0 && nowUnixMs >= pending.ExpireAtUnixMs {
			if s.service != nil {
				s.service.CancelPendingLiftTransition(w, playerID, h)
			}
			return
		}
		cr, hasCollision := ecs.GetComponent[components.CollisionResult](w, h)
		if !hasCollision || !cr.IsPhantom {
			return
		}
		if s.service != nil {
			s.service.FinalizePendingLiftTransition(w, playerID, h, pending)
		}
	})
}

