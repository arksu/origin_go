package ecs

import "origin/internal/types"

// ObjectDestructionState identifies prepared and quarantined exact generations.
// Access belongs to the owning shard; it is a resource, not a new ECS component.
type ObjectDestructionState struct {
	Prepared map[types.Handle]bool
	Pending  map[types.Handle]bool
}

// ObjectDestructionPending is false in worlds without destruction support.
func ObjectDestructionPending(w *World, target types.Handle) bool {
	if w == nil || target == types.InvalidHandle {
		return false
	}
	state, ok := TryGetResource[ObjectDestructionState](w)
	return ok && state.Pending[target]
}

// ObjectDestructionOwnerPending resolves an inventory's direct world owner.
// Nested authorization uses its currently opened root or actual personal parent.
func ObjectDestructionOwnerPending(w *World, owner types.EntityID) bool {
	return w != nil && owner != 0 && ObjectDestructionPending(w, w.GetHandleByEntityID(owner))
}
