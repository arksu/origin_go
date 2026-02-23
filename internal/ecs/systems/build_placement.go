package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

const BuildPlacementSystemPriority = 250

type BuildPlacementFinalizer interface {
	FinalizePendingBuildPlacement(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, pending components.PendingBuildPlacement)
	CancelPendingBuildPlacement(w *ecs.World, playerID types.EntityID, playerHandle types.Handle)
}

type BuildPlacementSystem struct {
	ecs.BaseSystem
	logger  *zap.Logger
	service BuildPlacementFinalizer
	query   *ecs.PreparedQuery
}

func NewBuildPlacementSystem(world *ecs.World, service BuildPlacementFinalizer, logger *zap.Logger) *BuildPlacementSystem {
	if logger == nil {
		logger = zap.NewNop()
	}
	query := ecs.NewPreparedQuery(
		world,
		0|(1<<components.PendingBuildPlacementComponentID),
		0,
	)
	return &BuildPlacementSystem{
		BaseSystem: ecs.NewBaseSystem("BuildPlacementSystem", BuildPlacementSystemPriority),
		logger:     logger,
		service:    service,
		query:      query,
	}
}

func (s *BuildPlacementSystem) Update(w *ecs.World, dt float64) {
	if s == nil || w == nil {
		return
	}
	nowUnixMs := ecs.GetResource[ecs.TimeState](w).UnixMs
	s.query.ForEach(func(h types.Handle) {
		playerID, hasExternalID := w.GetExternalID(h)
		if !hasExternalID {
			return
		}
		pending, hasPending := ecs.GetComponent[components.PendingBuildPlacement](w, h)
		if !hasPending {
			return
		}
		if pending.ExpireAtUnixMs > 0 && nowUnixMs >= pending.ExpireAtUnixMs {
			if s.service != nil {
				s.service.CancelPendingBuildPlacement(w, playerID, h)
			}
			return
		}
		cr, hasCollision := ecs.GetComponent[components.CollisionResult](w, h)
		if !hasCollision || !cr.IsPhantom {
			return
		}
		if s.service != nil {
			s.service.FinalizePendingBuildPlacement(w, playerID, h, pending)
		}
	})
}
