package components

import "origin/internal/ecs"

// CollisionResult represents the result of a collision check
type CollisionResult struct {
	Blocked  bool
	BlockedX int
	BlockedY int
}

const CollisionResultComponentID ecs.ComponentID = 15

func init() {
	ecs.RegisterComponent[CollisionResult](CollisionResultComponentID)
}
