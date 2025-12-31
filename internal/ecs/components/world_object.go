package components

import "origin/internal/ecs"

// WorldObject stores persistent object data from the database
// Used for static world objects (trees, rocks, buildings, etc.)
type WorldObject struct {
	ObjectType int32  // Type of object (tree, rock, etc.)
	Quality    int16  // Quality/variant of the object
	HP         int32  // Current health points
	Heading    int16  // Rotation angle
	CreateTick int64  // Tick when object was created
	LastTick   int64  // Last update tick
	Data       string // Extra data (hex encoded)
}

// Component ID
var WorldObjectID ecs.ComponentID

func init() {
	WorldObjectID = ecs.GetComponentID[WorldObject]()
}
