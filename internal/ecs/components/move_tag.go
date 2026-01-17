package components

import "origin/internal/ecs"

// MoveTag indicates that an entity has moved during the current frame
// Used by systems that need to react to actual movement (not just movement intent)
type MoveTag struct {
	// Could add movement metadata here if needed (e.g., distance moved, speed, etc.)
	// For now, the presence of this component is sufficient
}

const MoveTagComponentID ecs.ComponentID = 16

func init() {
	ecs.RegisterComponent[MoveTag](MoveTagComponentID)
}
