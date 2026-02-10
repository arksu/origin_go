package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

// ensureNestedContainer finds or creates a nested InventoryContainer for a container item
// (e.g. seed_bag). If the container already exists in RefIndex (e.g. preserved from a drop),
// it attaches it to the player's InventoryOwner. Otherwise creates a new empty one.
// Returns the nested container handle, or 0 if the item is not a container type.
func ensureNestedContainer(
	w *ecs.World,
	playerHandle types.Handle,
	item *components.InvItem,
	itemDef *itemdefs.ItemDef,
) types.Handle {
	if itemDef.Container == nil {
		return 0
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

	// Check if nested container already exists (e.g. preserved from drop or loaded from DB)
	if existingHandle, found := refIndex.Lookup(constt.InventoryGrid, item.ItemID, 0); found {
		// Already in RefIndex — just ensure it's linked to the player's InventoryOwner
		addNestedOwnerLink(w, playerHandle, item.ItemID, existingHandle)
		return existingHandle
	}

	// Create new empty nested container
	nestedContainer := components.InventoryContainer{
		OwnerID: item.ItemID,
		Kind:    constt.InventoryGrid,
		Key:     0,
		Version: 1,
		Width:   uint8(itemDef.Container.Size.W),
		Height:  uint8(itemDef.Container.Size.H),
		Items:   []components.InvItem{},
	}

	nestedHandle := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, nestedHandle, nestedContainer)

	refIndex.Add(constt.InventoryGrid, item.ItemID, 0, nestedHandle)
	addNestedOwnerLink(w, playerHandle, item.ItemID, nestedHandle)

	return nestedHandle
}

// addNestedOwnerLink adds an InventoryLink for a nested container to the player's InventoryOwner,
// but only if such a link doesn't already exist.
func addNestedOwnerLink(w *ecs.World, playerHandle types.Handle, itemID types.EntityID, nestedHandle types.Handle) {
	ecs.MutateComponent[components.InventoryOwner](w, playerHandle, func(o *components.InventoryOwner) bool {
		for _, link := range o.Inventories {
			if link.Kind == constt.InventoryGrid && link.OwnerID == itemID && link.Key == 0 {
				return false // already linked
			}
		}
		o.Inventories = append(o.Inventories, components.InventoryLink{
			Kind:    constt.InventoryGrid,
			Key:     0,
			OwnerID: itemID,
			Handle:  nestedHandle,
		})
		return true
	})
}

// detachNestedContainer removes the nested container link from the player's InventoryOwner.
// The container entity and its RefIndex entry are preserved so items are not lost.
func detachNestedContainer(w *ecs.World, playerHandle types.Handle, itemID types.EntityID) {
	ecs.MutateComponent[components.InventoryOwner](w, playerHandle, func(o *components.InventoryOwner) bool {
		for i, link := range o.Inventories {
			if link.Kind == constt.InventoryGrid && link.OwnerID == itemID && link.Key == 0 {
				o.Inventories = append(o.Inventories[:i], o.Inventories[i+1:]...)
				return true
			}
		}
		return false
	})
}

// serializeNestedForDrop serializes a nested container's items into InventoryDataV1
// for DB persistence when dropping a container item.
func serializeNestedForDrop(w *ecs.World, itemID types.EntityID) *InventoryDataV1 {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	nestedHandle, found := refIndex.Lookup(constt.InventoryGrid, itemID, 0)
	if !found {
		return nil
	}
	container, ok := ecs.GetComponent[components.InventoryContainer](w, nestedHandle)
	if !ok {
		return nil
	}

	items := make([]InventoryItemV1, 0, len(container.Items))
	for _, invItem := range container.Items {
		items = append(items, InventoryItemV1{
			ItemID:   uint64(invItem.ItemID),
			TypeID:   invItem.TypeID,
			Quality:  invItem.Quality,
			Quantity: invItem.Quantity,
			X:        invItem.X,
			Y:        invItem.Y,
		})
	}

	return &InventoryDataV1{
		Kind:    uint8(constt.InventoryGrid),
		Key:     0,
		Width:   container.Width,
		Height:  container.Height,
		Version: int(container.Version),
		Items:   items,
	}
}

func appendClosedNestedRefIfPresent(w *ecs.World, result *OperationResult, itemID types.EntityID) {
	if result == nil || itemID == 0 {
		return
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	if _, found := refIndex.Lookup(constt.InventoryGrid, itemID, 0); !found {
		return
	}

	for _, existing := range result.ClosedContainerRefs {
		if existing == nil {
			continue
		}
		if existing.Kind == netproto.InventoryKind_INVENTORY_KIND_GRID &&
			existing.OwnerId == uint64(itemID) &&
			existing.InventoryKey == 0 {
			return
		}
	}

	result.ClosedContainerRefs = append(result.ClosedContainerRefs, &netproto.InventoryRef{
		Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
		OwnerId:      uint64(itemID),
		InventoryKey: 0,
	})
}

func reconcileNestedContainerOwnerLink(
	w *ecs.World,
	owner *components.InventoryOwner,
	playerHandle types.Handle,
	itemID types.EntityID,
	dstContainerHandle types.Handle,
) {
	if itemID == 0 || playerHandle == types.InvalidHandle {
		return
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	nestedHandle, found := refIndex.Lookup(constt.InventoryGrid, itemID, 0)
	if !found || !w.Alive(nestedHandle) {
		return
	}

	if containerHandleBelongsToOwner(owner, dstContainerHandle) {
		addNestedOwnerLink(w, playerHandle, itemID, nestedHandle)
		return
	}
	detachNestedContainer(w, playerHandle, itemID)
}

func containerHandleBelongsToOwner(owner *components.InventoryOwner, handle types.Handle) bool {
	if owner == nil || handle == types.InvalidHandle {
		return false
	}
	for _, link := range owner.Inventories {
		if link.Handle == handle {
			return true
		}
	}
	return false
}

// deleteDroppedEntityFromECS removes a dropped item entity from ECS and InventoryRefIndex.
func deleteDroppedEntityFromECS(w *ecs.World, entityID types.EntityID, handle types.Handle, logger *zap.Logger) {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	containerHandle, found := refIndex.Lookup(constt.InventoryDroppedItem, entityID, 0)
	if found {
		refIndex.Remove(constt.InventoryDroppedItem, entityID, 0)
		w.Despawn(containerHandle)
	}
	if !w.Despawn(handle) {
		logger.Warn("deleteDroppedEntityFromECS: entity already despawned",
			zap.Uint64("entity_id", uint64(entityID)))
	}
}
