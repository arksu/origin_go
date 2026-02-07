package inventory

import (
	"encoding/json"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

// GiveItemResult represents the outcome of a GiveItem operation.
// Used by admin commands, crafting, machines, and other mechanics that create items.
type GiveItemResult struct {
	Success bool
	Message string

	// UpdatedContainers holds containers modified during the operation (for client sync).
	UpdatedContainers []*ContainerInfo

	// SpawnedDroppedEntityID is set when the item was dropped to world (inventory full).
	SpawnedDroppedEntityID *types.EntityID
}

// GiveItem creates a new item and places it into the player's grid inventory.
// If the inventory is full, the item is spawned as a dropped item at the player's position.
// This is the primary entry point for programmatic item creation (admin commands, crafting, loot, etc.).
func (s *InventoryOperationService) GiveItem(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	itemKey string,
	count uint32,
	quality uint32,
) *GiveItemResult {
	// 1. Lookup item definition
	itemDef, ok := itemdefs.Global().GetByKey(itemKey)
	if !ok {
		return &GiveItemResult{Success: false, Message: "unknown item key: " + itemKey}
	}

	if count == 0 {
		count = 1
	}

	// 2. Allocate a unique ID for the new item instance
	if s.idAllocator == nil {
		return &GiveItemResult{Success: false, Message: "id allocator not configured"}
	}
	itemID := s.idAllocator.GetFreeID()

	resource := itemDef.ResolveResource(false)

	newItem := components.InvItem{
		ItemID:   itemID,
		TypeID:   uint32(itemDef.DefID),
		Resource: resource,
		Quality:  quality,
		Quantity: count,
		W:        uint8(itemDef.Size.W),
		H:        uint8(itemDef.Size.H),
	}

	// 3. Try to place into player's grid inventory
	result := s.tryAddToGrid(w, playerID, playerHandle, &newItem)
	if result != nil {
		return result
	}

	// 4. Inventory full — drop to world at player's position
	return s.dropNewItem(w, playerID, playerHandle, &newItem, itemDef)
}

// tryAddToGrid attempts to place an item into the player's first grid container with free space.
func (s *InventoryOperationService) tryAddToGrid(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	item *components.InvItem,
) *GiveItemResult {
	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return nil // no owner — will fall through to drop
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)

	for _, link := range owner.Inventories {
		if link.Kind != constt.InventoryGrid {
			continue
		}
		// Only consider player-owned grids (not nested containers)
		if link.OwnerID != playerID {
			continue
		}

		containerHandle, found := refIndex.Lookup(constt.InventoryGrid, link.OwnerID, link.Key)
		if !found || !w.Alive(containerHandle) {
			continue
		}

		container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
		if !hasContainer {
			continue
		}

		found2, x, y := s.placementService.FindFreeSpace(&container, item.W, item.H)
		if !found2 {
			continue
		}

		// Place item
		item.X = x
		item.Y = y

		ecs.MutateComponent[components.InventoryContainer](w, containerHandle, func(c *components.InventoryContainer) bool {
			c.Items = append(c.Items, *item)
			c.Version++
			return true
		})

		updatedContainer, _ := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
		info := &ContainerInfo{
			Handle:    containerHandle,
			Container: &updatedContainer,
			Owner:     &owner,
		}

		return &GiveItemResult{
			Success:           true,
			Message:           "item added to inventory",
			UpdatedContainers: []*ContainerInfo{info},
		}
	}

	return nil // no free space in any grid
}

// dropNewItem spawns a dropped item entity at the player's position.
func (s *InventoryOperationService) dropNewItem(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	item *components.InvItem,
	itemDef *itemdefs.ItemDef,
) *GiveItemResult {
	if s.persister == nil {
		return &GiveItemResult{Success: false, Message: "drop persister not configured"}
	}

	playerTransform, hasTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	if !hasTransform {
		return &GiveItemResult{Success: false, Message: "player has no transform"}
	}

	playerInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, playerHandle)
	if !hasInfo {
		return &GiveItemResult{Success: false, Message: "player has no entity info"}
	}

	playerChunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, playerHandle)
	if !hasChunkRef {
		return &GiveItemResult{Success: false, Message: "player has no chunk ref"}
	}

	droppedEntityID := item.ItemID
	nowUnix := ecs.GetResource[ecs.TimeState](w).Now.Unix()
	resource := itemDef.ResolveResource(false)
	dropX := int(playerTransform.X)
	dropY := int(playerTransform.Y)

	droppedHandle := w.Spawn(droppedEntityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CreateTransform(dropX, dropY, 0))

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:   constt.DroppedItemTypeID,
			IsStatic: true,
			Region:   playerInfo.Region,
			Layer:    playerInfo.Layer,
		})

		ecs.AddComponent(w, h, components.ChunkRef{
			CurrentChunkX: playerChunkRef.CurrentChunkX,
			CurrentChunkY: playerChunkRef.CurrentChunkY,
		})

		ecs.AddComponent(w, h, components.Appearance{
			Resource: resource,
		})

		ecs.AddComponent(w, h, components.DroppedItem{
			DropTime:        nowUnix,
			DropperID:       playerID,
			ContainedItemID: item.ItemID,
		})

		container := components.InventoryContainer{
			OwnerID: droppedEntityID,
			Kind:    constt.InventoryDroppedItem,
			Key:     0,
			Version: 1,
			Items: []components.InvItem{
				{
					ItemID:   item.ItemID,
					TypeID:   item.TypeID,
					Resource: resource,
					Quality:  item.Quality,
					Quantity: item.Quantity,
					W:        item.W,
					H:        item.H,
				},
			},
		}
		containerHandle := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, containerHandle, container)

		refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
		refIndex.Add(constt.InventoryDroppedItem, droppedEntityID, 0, containerHandle)
	})

	if droppedHandle == types.InvalidHandle {
		return &GiveItemResult{Success: false, Message: "failed to spawn dropped entity"}
	}

	// Persist to DB
	droppedData := droppedItemData{
		HasInventory:    true,
		ContainedItemID: uint64(item.ItemID),
		DropTime:        nowUnix,
		DropperID:       uint64(playerID),
	}
	objectJSON, _ := json.Marshal(droppedData)

	invData := InventoryDataV1{
		Kind:    uint8(constt.InventoryDroppedItem),
		Key:     0,
		Version: 1,
		Items: []InventoryItemV1{
			{
				ItemID:   uint64(item.ItemID),
				TypeID:   item.TypeID,
				Quality:  item.Quality,
				Quantity: item.Quantity,
			},
		},
	}
	inventoryJSON, _ := json.Marshal(invData)

	if err := s.persister.PersistDroppedObject(
		droppedEntityID, constt.DroppedItemTypeID,
		playerInfo.Region, dropX, dropY, playerInfo.Layer,
		playerChunkRef.CurrentChunkX, playerChunkRef.CurrentChunkY,
		objectJSON, inventoryJSON,
	); err != nil {
		s.logger.Error("Failed to persist dropped object from GiveItem",
			zap.Uint64("entity_id", uint64(droppedEntityID)),
			zap.Error(err))
	}

	return &GiveItemResult{
		Success:                true,
		Message:                "inventory full, item dropped to world",
		SpawnedDroppedEntityID: &droppedEntityID,
	}
}
