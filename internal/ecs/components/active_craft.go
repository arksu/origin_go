package components

import "origin/internal/ecs"

// ActiveCraft stores synthetic crafting runtime state while a cyclic action is active.
type ActiveCraft struct {
	CraftKey        string
	RequestedCycles uint32
	RemainingCycles uint32

	// StopAfterCurrentCycle allows cycle handler to finish current outputs, then stop.
	StopAfterCurrentCycle bool
}

const ActiveCraftComponentID ecs.ComponentID = 28

func init() {
	ecs.RegisterComponent[ActiveCraft](ActiveCraftComponentID)
}
