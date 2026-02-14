package systems

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// ContextAction describes one selectable action for an entity context menu.
type ContextAction = types.ContextAction

// ContextActionResolver computes available actions for target entity state/behaviors.
type ContextActionResolver interface {
	ComputeActions(
		w *ecs.World,
		playerID types.EntityID,
		playerHandle types.Handle,
		targetID types.EntityID,
		targetHandle types.Handle,
	) []ContextAction

	// ExecuteAction performs validation + action execution.
	// Returns false when action is no longer available and should be silently ignored.
	ExecuteAction(
		w *ecs.World,
		playerID types.EntityID,
		playerHandle types.Handle,
		targetID types.EntityID,
		targetHandle types.Handle,
		actionID string,
	) bool
}
