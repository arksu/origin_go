package game

import (
	"math"
	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/playerstate"
	"origin/internal/types"
)

func (service *ActionService) isDirectionAction(id string) bool {
	definition, exists := service.definitions.Get(id)
	return exists && definition.Target.Kind == actiondefs.TargetDirection
}

// Point movement keeps ordinary armed/timed actions under their existing rules.
// A directed action must be canceled when movement is accepted, even if blocked.
func (service *ActionService) CancelForPointMovement(world *ecs.World, playerID types.EntityID, player types.Handle) {
	if service == nil || world != service.world || !world.Alive(player) {
		return
	}
	if active, exists := ecs.GetComponent[components.ActiveGameAction](world, player); exists && service.isDirectionAction(active.ActionID) {
		service.Cancel(world, playerID, player)
	}
}

func normalizeActionAim(angle *float32) (float64, bool) {
	if angle == nil || math.IsNaN(float64(*angle)) || math.IsInf(float64(*angle), 0) {
		return 0, false
	}
	normalized := math.Mod(float64(*angle), 2*math.Pi)
	if normalized < 0 {
		normalized += 2 * math.Pi
	}
	// Adding a tiny negative remainder can round up to a full turn.
	if normalized == 2*math.Pi {
		normalized = 0
	}
	return normalized, true
}

func directionActionUnavailable(world *ecs.World, player types.Handle) bool {
	movement, exists := ecs.GetComponent[components.Movement](world, player)
	return playerstate.IsIncapacitated(world, player) || exists && movement.State == constt.StateStunned
}

func validActionAim(angle float64) bool {
	return !math.IsNaN(angle) && !math.IsInf(angle, 0) && angle >= 0 && angle < 2*math.Pi
}
