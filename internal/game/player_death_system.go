package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

const PlayerDeathSystemPriority = 470

type PlayerDeathHandler interface {
	HandlePlayerPermanentDeath(w *ecs.World, playerID types.EntityID, playerHandle types.Handle)
}

// PlayerDeathSystem converts a live character into a permanent-death corpse
// when legacy hard HP reaches zero.
type PlayerDeathSystem struct {
	ecs.BaseSystem
	handler PlayerDeathHandler
}

func NewPlayerDeathSystem(handler PlayerDeathHandler) *PlayerDeathSystem {
	return &PlayerDeathSystem{
		BaseSystem: ecs.NewBaseSystem("PlayerDeathSystem", PlayerDeathSystemPriority),
		handler:    handler,
	}
}

func (s *PlayerDeathSystem) Update(w *ecs.World, dt float64) {
	_ = dt
	if s == nil || w == nil || s.handler == nil {
		return
	}

	characters := ecs.GetResource[ecs.CharacterEntities](w)
	for entityID, tracked := range characters.Map {
		handle := tracked.Handle
		if handle == types.InvalidHandle || !w.Alive(handle) {
			characters.Remove(entityID)
			continue
		}

		health, hasHealth := ecs.GetComponent[components.EntityHealth](w, handle)
		if !hasHealth || health.HHP > 0 {
			continue
		}

		// Remove first so the flow triggers exactly once.
		characters.Remove(entityID)
		s.handler.HandlePlayerPermanentDeath(w, entityID, handle)
	}
}
