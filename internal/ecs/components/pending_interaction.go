package components

import (
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// PendingInteraction stores the intent to auto-interact with a target entity
// once the player reaches interaction range. Set by MoveToEntity (autoInteract=true)
// and cleared on any new MoveTo/MoveToEntity or after execution by AutoInteractSystem.
type PendingInteraction struct {
	TargetEntityID types.EntityID
	TargetHandle   types.Handle
	Type           netproto.InteractionType
	Range          float64
}

const PendingInteractionComponentID ecs.ComponentID = 22

func init() {
	ecs.RegisterComponent[PendingInteraction](PendingInteractionComponentID)
}
