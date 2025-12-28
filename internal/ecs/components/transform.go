package components

import "origin/internal/ecs"

// Position represents an entity's position in world coordinates
type Position struct {
	X float64
	Y float64
}

// Velocity represents an entity's movement vector per tick
type Velocity struct {
	X float64
	Y float64
}

// Speed represents maximum movement speed (units per second)
type Speed struct {
	Value float64
}

// MovementTarget represents a destination for pathfinding/movement
type MovementTarget struct {
	X            float64
	Y            float64
	TargetHandle ecs.Handle // Optional: handle of entity to follow (0 if none)
	Interact     bool       // Whether to interact when reaching target
	Attack       bool       // Whether to attack when reaching target
}

// Component IDs - registered on init
var (
	PositionID       ecs.ComponentID
	VelocityID       ecs.ComponentID
	SpeedID          ecs.ComponentID
	MovementTargetID ecs.ComponentID
)

func init() {
	PositionID = ecs.GetComponentID[Position]()
	VelocityID = ecs.GetComponentID[Velocity]()
	SpeedID = ecs.GetComponentID[Speed]()
	MovementTargetID = ecs.GetComponentID[MovementTarget]()
}
