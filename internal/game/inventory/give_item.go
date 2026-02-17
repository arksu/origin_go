package inventory

import (
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

	if s.idAllocator == nil {
		return &GiveItemResult{Success: false, Message: "id allocator not configured"}
	}

	resource := itemDef.ResolveResource(false)

	// Create count separate items (each with Quantity=1)
	var allUpdatedContainers []*ContainerInfo
	var lastDroppedID *types.EntityID
	successCount := uint32(0)

	for i := uint32(0); i < count; i++ {
		// 2. Allocate a unique ID for each item instance
		itemID := s.idAllocator.GetFreeID()

		newItem := components.InvItem{
			ItemID:   itemID,
			TypeID:   uint32(itemDef.DefID),
			Resource: resource,
			Quality:  quality,
			Quantity: 1,
			W:        uint8(itemDef.Size.W),
			H:        uint8(itemDef.Size.H),
		}

		// 3. Try to place into player's grid inventory
		result := s.tryAddToGrid(w, playerID, playerHandle, &newItem, itemDef)
		if result != nil {
			successCount++
			allUpdatedContainers = append(allUpdatedContainers, result.UpdatedContainers...)
			continue
		}

		// 4. Inventory full — drop to world at player's position
		dropResult := s.dropNewItem(w, playerID, playerHandle, &newItem, itemDef)
		if dropResult.Success {
			successCount++
			lastDroppedID = dropResult.SpawnedDroppedEntityID
		}
	}

	if successCount == 0 {
		return &GiveItemResult{Success: false, Message: "failed to give any items"}
	}

	message := "item added to inventory"
	if successCount < count {
		message = "some items added, some dropped or failed"
	} else if lastDroppedID != nil {
		message = "inventory full, items dropped to world"
	}

	return &GiveItemResult{
		Success:                true,
		Message:                message,
		UpdatedContainers:      allUpdatedContainers,
		SpawnedDroppedEntityID: lastDroppedID,
	}
}

// tryAddToGrid attempts to place an item into the player's first grid container with free space.
func (s *InventoryOperationService) tryAddToGrid(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	item *components.InvItem,
	itemDef *itemdefs.ItemDef,
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

		// Create nested container for container items (e.g. seed_bag)
		nestedHandle := ensureNestedContainer(w, playerHandle, item, itemDef)

		updatedOwner, _ := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
		updatedContainer, _ := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
		info := &ContainerInfo{
			Handle:    containerHandle,
			Container: &updatedContainer,
			Owner:     &updatedOwner,
		}

		updatedContainers := []*ContainerInfo{info}

		// Include nested container in update so client receives it
		if nestedHandle != 0 {
			nestedContainer, _ := ecs.GetComponent[components.InventoryContainer](w, nestedHandle)
			updatedContainers = append(updatedContainers, &ContainerInfo{
				Handle:    nestedHandle,
				Container: &nestedContainer,
				Owner:     &updatedOwner,
			})
		}

		return &GiveItemResult{
			Success:           true,
			Message:           "item added to inventory",
			UpdatedContainers: updatedContainers,
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
	nowRuntimeSeconds := ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal
	resource := itemDef.ResolveResource(false)
	dropX := int(playerTransform.X)
	dropY := int(playerTransform.Y)

	dropParams := SpawnDroppedEntityParams{
		DroppedEntityID: droppedEntityID,
		ItemID:          item.ItemID,
		TypeID:          item.TypeID,
		Resource:        resource,
		Quality:         item.Quality,
		Quantity:        item.Quantity,
		W:               item.W,
		H:               item.H,
		DropX:           dropX,
		DropY:           dropY,
		Region:          playerInfo.Region,
		Layer:           playerInfo.Layer,
		ChunkX:          playerChunkRef.CurrentChunkX,
		ChunkY:          playerChunkRef.CurrentChunkY,
		DropperID:       playerID,
		NowUnix:         nowRuntimeSeconds,
	}

	if _, ok := SpawnDroppedEntity(w, dropParams); !ok {
		return &GiveItemResult{Success: false, Message: "failed to spawn dropped entity"}
	}

	if err := PersistDroppedEntity(s.persister, dropParams, nil); err != nil {
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
