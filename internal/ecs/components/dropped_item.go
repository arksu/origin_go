package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// DroppedItem marks an entity as a dropped item in the world.
// Contains metadata for despawn timing and origin tracking.
type DroppedItem struct {
	DropTime        int64          // Unix timestamp (seconds) when the item was dropped
	DropperID       types.EntityID // Entity that dropped the item
	ContainedItemID types.EntityID // The item instance ID contained in this dropped entity
}

const DroppedItemComponentID ecs.ComponentID = 21

func init() {
	ecs.RegisterComponent[DroppedItem](DroppedItemComponentID)
}
