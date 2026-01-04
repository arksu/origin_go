package components

import "origin/internal/ecs"

// Transform represents an entity's position and orientation in 2D space
type Transform struct {
	X         float32
	Y         float32
	Direction float32
}

const TransformComponentID ecs.ComponentID = 10

func init() {
	ecs.RegisterComponent[Transform](TransformComponentID)
}
