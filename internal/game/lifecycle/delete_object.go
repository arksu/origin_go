package lifecycle

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
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

// DeleteOwnedInventoryContainers removes all ECS containers owned by an object and
// recursively removes nested containers owned by its contained items.
func DeleteOwnedInventoryContainers(w *ecs.World, ownerID types.EntityID) {
	if w == nil || ownerID == 0 {
		return
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	pendingOwners := []types.EntityID{ownerID}
	seenOwners := make(map[types.EntityID]struct{}, 4)
	for len(pendingOwners) > 0 {
		currentOwnerID := pendingOwners[0]
		pendingOwners = pendingOwners[1:]
		if _, seen := seenOwners[currentOwnerID]; seen {
			continue
		}
		seenOwners[currentOwnerID] = struct{}{}

		for _, containerHandle := range refIndex.RemoveAllByOwner(currentOwnerID) {
			container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
			if hasContainer {
				for _, item := range container.Items {
					if item.ItemID != 0 {
						pendingOwners = append(pendingOwners, item.ItemID)
					}
				}
			}
			if w.Alive(containerHandle) {
				w.Despawn(containerHandle)
			}
		}
	}
}
