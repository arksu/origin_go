package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

type GameActionPhase string

const (
	GameActionSelecting   GameActionPhase = "selecting"
	GameActionApproaching GameActionPhase = "approaching"
	GameActionExecuting   GameActionPhase = "executing"
)

// ActiveGameAction is transient player state; it is never saved with the character.
type ActiveGameAction struct {
	ActionID       string
	Phase          GameActionPhase
	Generation     uint64
	TargetID       types.EntityID
	TargetHandle   types.Handle
	TargetX        float64
	TargetY        float64
	MovementOwned  bool
	ExpireAtUnixMs int64
}

const ActiveGameActionComponentID ecs.ComponentID = 35

func init() {
	ecs.RegisterComponent[ActiveGameAction](ActiveGameActionComponentID)
}
