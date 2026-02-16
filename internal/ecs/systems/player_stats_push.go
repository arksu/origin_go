package systems

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

const PlayerStatsPushSystemPriority = 490

// PlayerStatsPushSender sends throttled player stats updates to clients.
type PlayerStatsPushSender interface {
	SendPlayerStatsDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool
}

// PlayerStatsPushSystem flushes due player stats updates from EntityStatsUpdateState.
type PlayerStatsPushSystem struct {
	ecs.BaseSystem
	sender PlayerStatsPushSender
	due    []types.EntityID
}

func NewPlayerStatsPushSystem(sender PlayerStatsPushSender) *PlayerStatsPushSystem {
	return &PlayerStatsPushSystem{
		BaseSystem: ecs.NewBaseSystem("PlayerStatsPushSystem", PlayerStatsPushSystemPriority),
		sender:     sender,
		due:        make([]types.EntityID, 0, 256),
	}
}

func (s *PlayerStatsPushSystem) Update(w *ecs.World, dt float64) {
	_ = dt
	if s.sender == nil {
		return
	}

	timeState := ecs.GetResource[ecs.TimeState](w)
	updateState := ecs.GetResource[ecs.EntityStatsUpdateState](w)
	s.due = updateState.PopDuePlayerStatsPush(timeState.UnixMs, s.due[:0])
	for _, entityID := range s.due {
		handle := w.GetHandleByEntityID(entityID)
		if handle == types.InvalidHandle || !w.Alive(handle) {
			updateState.ForgetPlayer(entityID)
			continue
		}
		s.sender.SendPlayerStatsDeltaIfChanged(w, entityID, handle)
	}
}
