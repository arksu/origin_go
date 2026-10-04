package game

import (
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/playerstate"
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
// Independent timed KO on SHP<=0, permanent death on HHP<=0.
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

	before, _ := ecs.GetComponent[components.EntityHealth](w, handle)
	ecs.WithComponent(w, handle, func(health *components.EntityHealth) {
		health.SHP, health.HHP = entityhealth.ClampHealth(health.SHP, health.HHP, mhp)
		if health.HHP > 0 && nowTick > 0 && nowTick%s.cfg.StarvationDamageIntervalTicks == 0 && energy < 500 {
			health.SHP, health.HHP, _, _ = entityhealth.ApplyDamage(health.SHP, health.HHP, mhp, s.cfg.StarvationSoftDamagePerInterval, 0)
		}
		// Depletion must be observed before regeneration, but all incoming damage
		// takes priority over the one-time completion grant.
		playerstate.ResolveKnockout(health, ecs.GetResource[ecs.TimeState](w).UnixMs)
		if health.HHP > 0 && nowTick > 0 && nowTick%s.cfg.ShpRegenIntervalTicks == 0 {
			health.SHP += entityhealth.ResolveSHPRegenPerInterval(mhp, energy)
		}
		health.SHP, health.HHP = entityhealth.ClampHealth(health.SHP, health.HHP, mhp)
	})
	after, _ := ecs.GetComponent[components.EntityHealth](w, handle)
	if before.IsLying != after.IsLying {
		ecs.MarkCharacterVisualDirty(w, playerID)
	}
	if after.HHP <= 0 {
		if s.handler != nil {
			s.handler.HandlePlayerPermanentDeath(w, playerID, handle)
		}
		return true
	}
	if runtime, ok := s.handler.(interface {
		HandlePlayerItemsLocked(*ecs.World, types.EntityID, types.Handle)
		ApplyPendingStandUp(*ecs.World, types.EntityID, types.Handle)
	}); ok {
		if playerstate.ItemsLocked(w, handle) {
			runtime.HandlePlayerItemsLocked(w, playerID, handle)
		}
		runtime.ApplyPendingStandUp(w, playerID, handle)
	}
	last, sent := ecs.GetResource[ecs.EntityStatsUpdateState](w).GetLastSentPlayerStats(playerID)
	if before.KOUntilUnixMs != after.KOUntilUnixMs || before.IsLying != after.IsLying ||
		(sent && last.CanStandUp != playerstate.CanStandUp(w, handle)) {
		ecs.MarkPlayerStatsDirty(w, playerID, 0)
	} else if before.SHP != after.SHP || before.HHP != after.HHP {
		ecs.MarkPlayerStatsDirty(w, playerID, ecs.ResolvePlayerStatsTTLms(w))
	}
	return false
}

func resolveMaxHHPForHandle(w *ecs.World, handle types.Handle, lifeDeathFactor float64) float64 {
	con := characterattrs.DefaultValue
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	return entityhealth.MaxHHPFromCon(con, lifeDeathFactor)
}
