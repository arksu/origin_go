package components

import "origin/internal/ecs"

// ObjectInternalState tracks runtime state and dirty flag for world objects.
// Added to all non-player entities on chunk activation; used to skip
// unchanged objects during persistence.
type ObjectInternalState struct {
	Flags   []string
	State   any
	IsDirty bool
}

const ObjectInternalStateComponentID ecs.ComponentID = 23

func init() {
	ecs.RegisterComponent[ObjectInternalState](ObjectInternalStateComponentID)
}
