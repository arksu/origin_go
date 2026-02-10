package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// PendingContextAction stores a selected context action that must be executed
// once the player links with the target object.
type PendingContextAction struct {
	TargetEntityID types.EntityID
	TargetHandle   types.Handle
	ActionID       string

	// Unix milliseconds deadline after which the pending action is discarded.
	ExpireAtUnixMs int64
}

const PendingContextActionComponentID ecs.ComponentID = 24

func init() {
	ecs.RegisterComponent[PendingContextAction](PendingContextActionComponentID)
}
