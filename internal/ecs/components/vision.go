package components

import "origin/internal/ecs"

// Vision represents an entity's vision capabilities
type Vision struct {
	Radius float64 // Vision radius in world units
	Power  float64 // Vision power/accuracy multiplier
}

const VisionComponentID ecs.ComponentID = 16

func init() {
	ecs.RegisterComponent[Vision](VisionComponentID)
}
