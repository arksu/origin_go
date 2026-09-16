package components

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/types"
)

// DroppedItem marks an entity as a dropped item in the world.
// Contains metadata for despawn timing and origin tracking.
type DroppedItem struct {
	// DropTime is the persisted server runtime in seconds when the item was dropped.
	// Server runtime advances only while the server is running, so offline time does not
	// consume the drop lifetime.
	DropTime        int64
	DropperID       types.EntityID // Entity that dropped the item
	ContainedItemID types.EntityID // The item instance ID contained in this dropped entity
}

// IsDroppedItemExpired reports whether a dropped item has exhausted its lifetime in
// persisted server-runtime seconds. A clock rollback must not expire live objects.
func IsDroppedItemExpired(dropTime, nowRuntimeSeconds int64) bool {
	if dropTime < 0 || nowRuntimeSeconds < dropTime {
		return false
	}
	return nowRuntimeSeconds-dropTime >= int64(constt.DroppedDespawnSeconds)
}

const DroppedItemComponentID ecs.ComponentID = 21

func init() {
	ecs.RegisterComponent[DroppedItem](DroppedItemComponentID)
}
