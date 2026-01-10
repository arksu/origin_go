package components

import "origin/internal/ecs"

// Collider represents an entity's collision box dimensions
type Collider struct {
	HalfWidth  int
	HalfHeight int
}

const ColliderComponentID ecs.ComponentID = 14

func init() {
	ecs.RegisterComponent[Collider](ColliderComponentID)
}
