package components

import "origin/internal/ecs"

// Collider represents collision AABB bounds for an entity
type Collider struct {
	Width  float64 // For AABB colliders
	Height float64 // For AABB colliders
	Layer  uint8   // Collision layer for filtering
	Mask   uint8   // Which layers this collider interacts with
}

// CollisionEvent is generated when two entities collide
type CollisionEvent struct {
	HandleA  ecs.Handle
	HandleB  ecs.Handle
	OverlapX float64
	OverlapY float64
}

// Static marks an entity as immovable (walls, trees, etc.)
type Static struct{}

// Component IDs
var (
	ColliderID ecs.ComponentID
	StaticID   ecs.ComponentID
)

func init() {
	ColliderID = ecs.GetComponentID[Collider]()
	StaticID = ecs.GetComponentID[Static]()
}
