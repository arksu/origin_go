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

// IsDirectionAction lets ingress enforce session ownership even when the client
// omits every optional directional field.
func (service *ActionService) IsDirectionAction(id string) bool {
	return service.isDirectionAction(id)
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

func normalizeActionHeading(angle float64) (float64, bool) {
	if math.IsNaN(angle) || math.IsInf(angle, 0) {
		return 0, false
	}
	normalized := math.Mod(angle, 2*math.Pi)
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
