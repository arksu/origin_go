package behaviors

import (
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/types"
)

// ConsumePlayerLongActionStamina applies long-action stamina cost and propagates
// movement mode / stats dirty flags consistently across gameplay systems.
func ConsumePlayerLongActionStamina(
	world *ecs.World,
	playerHandle types.Handle,
	cost float64,
) bool {
	if world == nil || playerHandle == types.InvalidHandle || !world.Alive(playerHandle) {
		return false
	}

	stats, hasStats := ecs.GetComponent[components.EntityStats](world, playerHandle)
	if !hasStats {
		return true
	}

	con := characterattrs.DefaultValue
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](world, playerHandle); hasProfile {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	maxStamina := entitystats.MaxStaminaFromCon(con)
	currentStamina := entitystats.ClampStamina(stats.Stamina, maxStamina)
	statsChanged := currentStamina != stats.Stamina
	currentEnergy := stats.Energy
	if currentEnergy < 0 {
		currentEnergy = 0
		statsChanged = true
	}

	if !entitystats.CanConsumeLongActionStamina(currentStamina, maxStamina, cost) {
		if statsChanged {
			ecs.WithComponent(world, playerHandle, func(entityStats *components.EntityStats) {
				entityStats.Stamina = currentStamina
				entityStats.Energy = currentEnergy
			})
			ecs.MarkPlayerStatsDirtyByHandle(world, playerHandle, ecs.ResolvePlayerStatsTTLms(world))
		}
		ecs.UpdateEntityStatsRegenSchedule(world, playerHandle, currentStamina, currentEnergy, maxStamina)
		return false
	}

	nextStamina := entitystats.ClampStamina(currentStamina-cost, maxStamina)
	statsChanged = statsChanged || nextStamina != stats.Stamina
	if statsChanged {
		ecs.WithComponent(world, playerHandle, func(entityStats *components.EntityStats) {
			entityStats.Stamina = nextStamina
			entityStats.Energy = currentEnergy
		})
	}
	_, isCarrying := ecs.GetComponent[components.LiftCarryState](world, playerHandle)

	modeMutated := ecs.MutateComponent[components.Movement](world, playerHandle, func(m *components.Movement) bool {
		mode, canMove := entitystats.ResolveAllowedMoveModeWithCarry(
			m.Mode,
			nextStamina,
			maxStamina,
			currentEnergy,
			isCarrying,
		)
		changed := mode != m.Mode
		if changed {
			m.Mode = mode
		}
		if !canMove && m.State == constt.StateMoving {
			m.ClearTarget()
			changed = true
		}
		return changed
	})
	if modeMutated {
		ecs.MarkMovementModeDirtyByHandle(world, playerHandle)
	}

	if statsChanged {
		ecs.MarkPlayerStatsDirtyByHandle(world, playerHandle, ecs.ResolvePlayerStatsTTLms(world))
	}
	ecs.UpdateEntityStatsRegenSchedule(world, playerHandle, nextStamina, currentEnergy, maxStamina)
	return true
}
