package systems

import (
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/types"
)

const EntityStatsRegenSystemPriority = 480

// EntityStatsRegenSystem applies scheduled stamina regeneration without ECS full scans.
type EntityStatsRegenSystem struct {
	ecs.BaseSystem
	due []types.Handle
}

func NewEntityStatsRegenSystem() *EntityStatsRegenSystem {
	return &EntityStatsRegenSystem{
		BaseSystem: ecs.NewBaseSystem("EntityStatsRegenSystem", EntityStatsRegenSystemPriority),
		due:        make([]types.Handle, 0, 256),
	}
}

func (s *EntityStatsRegenSystem) Update(w *ecs.World, dt float64) {
	_ = dt

	timeState := ecs.GetResource[ecs.TimeState](w)
	updateState := ecs.GetResource[ecs.EntityStatsUpdateState](w)
	s.due = updateState.PopDueRegen(timeState.Tick, s.due[:0])

	for _, handle := range s.due {
		if handle == types.InvalidHandle || !w.Alive(handle) {
			continue
		}

		stats, hasStats := ecs.GetComponent[components.EntityStats](w, handle)
		if !hasStats {
			continue
		}

		attributes := characterattrs.Default()
		if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
			attributes = characterattrs.Normalize(profile.Attributes)
		}
		maxStamina := entitystats.MaxStaminaFromAttributes(attributes)
		nextStamina, nextEnergy, changed := entitystats.RegenerateStamina(stats.Stamina, stats.Energy, maxStamina)
		if changed {
			ecs.WithComponent(w, handle, func(entityStats *components.EntityStats) {
				entityStats.Stamina = nextStamina
				entityStats.Energy = nextEnergy
			})
			ecs.MarkPlayerStatsDirtyByHandle(w, handle, ecs.ResolvePlayerStatsTTLms(w))
		}

		ecs.UpdateEntityStatsRegenSchedule(w, handle, nextStamina, nextEnergy, maxStamina)
	}
}
