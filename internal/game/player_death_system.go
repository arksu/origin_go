package game

import (
	"origin/internal/characterattrs"
	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/types"
)

const PlayerDeathSystemPriority = 470

const defaultStarvationSoftDamagePerInterval = 10.0

type PlayerDeathSystemConfig struct {
	LifeDeathFactor                 float64
	ShpRegenIntervalTicks           uint64
	StarvationDamageIntervalTicks   uint64
	StarvationSoftDamagePerInterval float64
}

type PlayerDeathHandler interface {
	HandlePlayerPermanentDeath(w *ecs.World, playerID types.EntityID, playerHandle types.Handle)
}

// PlayerDeathSystem executes SHP/HHP runtime transitions:
// KO on SHP<=0 (persistent until SHP recovers), permanent death on HHP<=0.
type PlayerDeathSystem struct {
	ecs.BaseSystem
	handler PlayerDeathHandler
	cfg     PlayerDeathSystemConfig
}

func NewPlayerDeathSystem(handler PlayerDeathHandler, cfg PlayerDeathSystemConfig) *PlayerDeathSystem {
	return &PlayerDeathSystem{
		BaseSystem: ecs.NewBaseSystem("PlayerDeathSystem", PlayerDeathSystemPriority),
		handler:    handler,
		cfg:        normalizePlayerDeathSystemConfig(cfg),
	}
}

func (s *PlayerDeathSystem) Update(w *ecs.World, dt float64) {
	_ = dt
	if s == nil || w == nil {
		return
	}

	characters := ecs.GetResource[ecs.CharacterEntities](w)
	nowTick := ecs.GetResource[ecs.TimeState](w).Tick
	for entityID, tracked := range characters.Map {
		handle := tracked.Handle
		if handle == types.InvalidHandle || !w.Alive(handle) {
			characters.Remove(entityID)
			continue
		}

		if _, hasHealth := ecs.GetComponent[components.EntityHealth](w, handle); !hasHealth {
			continue
		}

		if s.processPlayerHealth(w, entityID, handle, nowTick) {
			// Remove immediately to guarantee one-time permanent death processing.
			characters.Remove(entityID)
		}
	}
}

func normalizePlayerDeathSystemConfig(cfg PlayerDeathSystemConfig) PlayerDeathSystemConfig {
	if cfg.LifeDeathFactor <= 0 {
		cfg.LifeDeathFactor = 1
	}
	if cfg.ShpRegenIntervalTicks == 0 {
		cfg.ShpRegenIntervalTicks = 100
	}
	if cfg.StarvationDamageIntervalTicks == 0 {
		cfg.StarvationDamageIntervalTicks = 432000
	}
	if cfg.StarvationSoftDamagePerInterval <= 0 {
		cfg.StarvationSoftDamagePerInterval = defaultStarvationSoftDamagePerInterval
	}
	return cfg
}

func (s *PlayerDeathSystem) processPlayerHealth(w *ecs.World, playerID types.EntityID, handle types.Handle, nowTick uint64) bool {
	mhp := resolveMaxHHPForHandle(w, handle, s.cfg.LifeDeathFactor)
	if mhp <= 0 {
		mhp = 1
	}

	stats, hasStats := ecs.GetComponent[components.EntityStats](w, handle)
	energy := 0.0
	if hasStats {
		energy = stats.Energy
	}

	dirty := false
	koStateChanged := false
	triggerPermanentDeath := false

	ecs.WithComponent(w, handle, func(health *components.EntityHealth) {
		wasKnockedOut := health.KOUntilTick > 0

		nextSHP, nextHHP := entityhealth.ClampHealth(health.SHP, health.HHP, mhp)
		if nextSHP != health.SHP || nextHHP != health.HHP {
			health.SHP = nextSHP
			health.HHP = nextHHP
			dirty = true
		}

		if health.HHP <= 0 {
			if health.KOUntilTick != 0 {
				health.KOUntilTick = 0
				dirty = true
			}
			koStateChanged = wasKnockedOut
			triggerPermanentDeath = true
			return
		}

		if s.cfg.StarvationDamageIntervalTicks > 0 &&
			nowTick > 0 &&
			nowTick%s.cfg.StarvationDamageIntervalTicks == 0 &&
			energy < 500 {
			starvedSHP, starvedHHP, _, _ := entityhealth.ApplyDamage(
				health.SHP,
				health.HHP,
				mhp,
				s.cfg.StarvationSoftDamagePerInterval,
				0,
			)
			if starvedSHP != health.SHP || starvedHHP != health.HHP {
				health.SHP = starvedSHP
				health.HHP = starvedHHP
				dirty = true
			}
		}

		if health.HHP > 0 &&
			s.cfg.ShpRegenIntervalTicks > 0 &&
			nowTick > 0 &&
			nowTick%s.cfg.ShpRegenIntervalTicks == 0 {
			regen := entityhealth.ResolveSHPRegenPerInterval(mhp, energy)
			if regen > 0 {
				health.SHP += regen
				dirty = true
			}
		}

		nextSHP, nextHHP = entityhealth.ClampHealth(health.SHP, health.HHP, mhp)
		if nextSHP != health.SHP || nextHHP != health.HHP {
			health.SHP = nextSHP
			health.HHP = nextHHP
			dirty = true
		}

		if health.HHP <= 0 {
			if health.KOUntilTick != 0 {
				health.KOUntilTick = 0
				dirty = true
			}
			koStateChanged = wasKnockedOut
			triggerPermanentDeath = true
			return
		} else if health.SHP <= 0 {
			if health.KOUntilTick == 0 {
				knockedOutAtTick := nowTick
				if knockedOutAtTick == 0 {
					knockedOutAtTick = 1
				}
				health.KOUntilTick = knockedOutAtTick
				dirty = true
			}
			if applyStunnedStateAndClearActions(w, handle) {
				dirty = true
			}
		} else {
			if health.KOUntilTick != 0 {
				health.KOUntilTick = 0
				dirty = true
			}
			if clearStunnedState(w, handle) {
				dirty = true
			}
		}

		isKnockedOut := health.KOUntilTick > 0
		if isKnockedOut != wasKnockedOut {
			koStateChanged = true
		}
	})

	if dirty || koStateChanged {
		ecs.MarkPlayerStatsDirty(w, playerID, ecs.ResolvePlayerStatsTTLms(w))
	}

	if !triggerPermanentDeath {
		return false
	}
	if s.handler != nil {
		s.handler.HandlePlayerPermanentDeath(w, playerID, handle)
	}
	return true
}

func applyStunnedStateAndClearActions(w *ecs.World, handle types.Handle) bool {
	changed := false
	ecs.WithComponent(w, handle, func(movement *components.Movement) {
		if movement.State != _const.StateStunned {
			movement.State = _const.StateStunned
			changed = true
		}
		if movement.TargetType != _const.TargetNone ||
			movement.TargetHandle != types.InvalidHandle ||
			movement.VelocityX != 0 ||
			movement.VelocityY != 0 {
			movement.ClearTarget()
			changed = true
		}
	})

	ecs.RemoveComponent[components.PendingInteraction](w, handle)
	ecs.RemoveComponent[components.PendingContextAction](w, handle)
	ecs.RemoveComponent[components.PendingBuildPlacement](w, handle)
	ecs.RemoveComponent[components.PendingLiftTransition](w, handle)
	ecs.RemoveComponent[components.ActiveCyclicAction](w, handle)
	ecs.RemoveComponent[components.ActiveCraft](w, handle)
	return changed
}

func clearStunnedState(w *ecs.World, handle types.Handle) bool {
	changed := false
	ecs.WithComponent(w, handle, func(movement *components.Movement) {
		if movement.State == _const.StateStunned {
			movement.State = _const.StateIdle
			movement.VelocityX = 0
			movement.VelocityY = 0
			changed = true
		}
	})
	return changed
}

func resolveMaxHHPForHandle(w *ecs.World, handle types.Handle, lifeDeathFactor float64) float64 {
	con := characterattrs.DefaultValue
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	return entityhealth.MaxHHPFromCon(con, lifeDeathFactor)
}
