package systems

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

const PlayerStatsPushSystemPriority = 490

// PlayerStatsPushSender sends throttled player stats updates to clients.
type PlayerStatsPushSender interface {
	SendPlayerStatsDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool
	SendMovementModeDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool
}

// PlayerStatsPushSystem flushes due player stats updates from EntityStatsUpdateState.
type PlayerStatsPushSystem struct {
	ecs.BaseSystem
	sender   PlayerStatsPushSender
	dueStats []types.EntityID
	dueModes []types.EntityID
}

func NewPlayerStatsPushSystem(sender PlayerStatsPushSender) *PlayerStatsPushSystem {
	return &PlayerStatsPushSystem{
		BaseSystem: ecs.NewBaseSystem("PlayerStatsPushSystem", PlayerStatsPushSystemPriority),
		sender:     sender,
		dueStats:   make([]types.EntityID, 0, 256),
		dueModes:   make([]types.EntityID, 0, 256),
	}
}

func (s *PlayerStatsPushSystem) Update(w *ecs.World, dt float64) {
	_ = dt
	if s.sender == nil {
		return
	}

	timeState := ecs.GetResource[ecs.TimeState](w)
	updateState := ecs.GetResource[ecs.EntityStatsUpdateState](w)
	s.dueStats = updateState.PopDuePlayerStatsPush(timeState.UnixMs, s.dueStats[:0])
	for _, entityID := range s.dueStats {
		handle := w.GetHandleByEntityID(entityID)
		if handle == types.InvalidHandle || !w.Alive(handle) {
			updateState.ForgetPlayer(entityID)
			continue
		}
		s.sender.SendPlayerStatsDeltaIfChanged(w, entityID, handle)
	}

	s.dueModes = updateState.PopDueMovementModePush(s.dueModes[:0])
	for _, entityID := range s.dueModes {
		handle := w.GetHandleByEntityID(entityID)
		if handle == types.InvalidHandle || !w.Alive(handle) {
			updateState.ForgetPlayer(entityID)
			continue
		}
		s.sender.SendMovementModeDeltaIfChanged(w, entityID, handle)
	}
}
