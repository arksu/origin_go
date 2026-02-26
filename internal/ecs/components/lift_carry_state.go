package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// LiftCarryState tracks the world object currently carried by a player.
type LiftCarryState struct {
	ObjectEntityID types.EntityID
	ObjectHandle   types.Handle
	StartedAtUnixMs int64
}

const LiftCarryStateComponentID ecs.ComponentID = 30

func init() {
	ecs.RegisterComponent[LiftCarryState](LiftCarryStateComponentID)
}

