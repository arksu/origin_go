package lifecycle

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// DeleteObjectOptions controls which runtime data is removed together with an object.
type DeleteObjectOptions struct {
	// DeleteOwnedInventories also removes every inventory container whose owner_id is
	// the object ID. For dropped items, the item ID and object ID are identical, so
	// this also removes an attached nested inventory on expiry.
	DeleteOwnedInventories bool
}

// DeleteObject removes one persistent-world object's ECS entity and optionally its
// owned inventory containers. Spatial membership and database persistence are owned
// by callers because those dependencies differ by lifecycle context.
func DeleteObject(
	w *ecs.World,
	entityID types.EntityID,
	handle types.Handle,
	options DeleteObjectOptions,
) bool {
	if w == nil || entityID == 0 || handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}

	externalID, hasExternalID := ecs.GetComponent[ecs.ExternalID](w, handle)
	if !hasExternalID || externalID.ID != entityID {
		return false
	}

	if options.DeleteOwnedInventories {
		DeleteOwnedInventoryContainers(w, entityID)
	}

	return w.Despawn(handle)
}

// DeleteOwnedInventoryContainers removes all ECS containers registered to ownerID.
// A container's nested data is represented by containers indexed under item IDs; for
// a dropped item the root item ID equals ownerID, so all of its runtime inventory data
// is removed in the same pass.
func DeleteOwnedInventoryContainers(w *ecs.World, ownerID types.EntityID) {
	if w == nil || ownerID == 0 {
		return
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, containerHandle := range refIndex.RemoveAllByOwner(ownerID) {
		if w.Alive(containerHandle) {
			w.Despawn(containerHandle)
		}
	}
}
