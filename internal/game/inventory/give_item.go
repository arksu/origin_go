package inventory

import (
	"fmt"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"
)

type givePlacementPolicy uint8

const (
	givePlacementRootNestedHand givePlacementPolicy = iota + 1
	givePlacementNestedRootHand
)

const defaultGivePlacementPolicy = givePlacementRootNestedHand

// GiveItemResult represents the outcome of a GiveItem operation.
// Used by admin commands, crafting, machines, and other mechanics that create items.
type GiveItemResult struct {
	Success bool
	Message string
	// GrantedCount is how many items were actually placed.
	GrantedCount uint32
	// PlacedInHand reports whether the final successful placement used hand fallback.
	PlacedInHand bool

	// UpdatedContainers holds containers modified during the operation (for client sync).
	UpdatedContainers []*ContainerInfo

	// SpawnedDroppedEntityID is kept for backward compatibility with callers.
	// GiveItem no longer drops to world, so this field is always nil.
	SpawnedDroppedEntityID *types.EntityID
}

// GiveItem creates new items and places them using the universal placement policy:
// root player grids -> eligible nested grids -> hand fallback. It never drops to world.
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
	requestedCount := count

	if s.idAllocator == nil {
		return &GiveItemResult{Success: false, Message: "id allocator not configured"}
	}

	resource := itemDef.ResolveResource(false)
	var allUpdatedContainers []*ContainerInfo
	grantedCount := uint32(0)
	placedInHand := false

	for i := uint32(0); i < count; i++ {
		owner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
		if !hasOwner {
			break
		}

		newItem := components.InvItem{
			TypeID:   uint32(itemDef.DefID),
			Resource: resource,
			Quality:  quality,
			Quantity: 1,
			W:        uint8(itemDef.Size.W),
			H:        uint8(itemDef.Size.H),
		}

		updated := s.tryAddToEligibleGrid(w, playerID, playerHandle, &owner, &newItem, itemDef)
		if len(updated) > 0 {
			grantedCount++
			allUpdatedContainers = mergeUpdatedContainerInfos(allUpdatedContainers, updated)
			continue
		}

		updated = s.tryAddToHand(w, playerID, playerHandle, &owner, &newItem, itemDef)
		if len(updated) > 0 {
			grantedCount++
			placedInHand = true
			allUpdatedContainers = mergeUpdatedContainerInfos(allUpdatedContainers, updated)
			break
		}

		break
	}

	if grantedCount == 0 {
		return &GiveItemResult{
			Success:      false,
			Message:      fmt.Sprintf("granted 0/%d items: no free space", requestedCount),
			GrantedCount: 0,
		}
	}

	message := fmt.Sprintf("granted %d/%d items", grantedCount, requestedCount)
	if placedInHand {
		message = fmt.Sprintf("%s; last item placed in hand", message)
	} else if grantedCount < requestedCount {
		message = fmt.Sprintf("%s; no more free space", message)
	}

	return &GiveItemResult{
		Success:           true,
		Message:           message,
		GrantedCount:      grantedCount,
		PlacedInHand:      placedInHand,
		UpdatedContainers: allUpdatedContainers,
	}
}

func (s *InventoryOperationService) tryAddToEligibleGrid(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	owner *components.InventoryOwner,
	item *components.InvItem,
	itemDef *itemdefs.ItemDef,
) []*ContainerInfo {
	if owner == nil {
		return nil
	}
	gridLinks := orderedGridLinks(owner.Inventories, playerID, defaultGivePlacementPolicy)
	for _, link := range gridLinks {
		if !w.Alive(link.Handle) {
			continue
		}
		container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !hasContainer {
			continue
		}
		if !s.canPlaceInContainer(w, item, owner, link.Handle, &container) {
			continue
		}
		found, x, y := s.placementService.FindFreeSpace(&container, item.W, item.H)
		if !found {
			continue
		}
		return s.placeNewItemInContainer(w, playerHandle, link.Handle, item, itemDef, x, y)
	}
	return nil
}

func (s *InventoryOperationService) tryAddToHand(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	owner *components.InventoryOwner,
	item *components.InvItem,
	itemDef *itemdefs.ItemDef,
) []*ContainerInfo {
	if owner == nil {
		return nil
	}

	for _, link := range owner.Inventories {
		if link.Kind != constt.InventoryHand || link.OwnerID != playerID {
			continue
		}
		if !w.Alive(link.Handle) {
			continue
		}
		container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !hasContainer {
			continue
		}
		if len(container.Items) > 0 {
			continue
		}
		if !s.canPlaceInContainer(w, item, owner, link.Handle, &container) {
			continue
		}
		return s.placeNewItemInContainer(w, playerHandle, link.Handle, item, itemDef, 0, 0)
	}
	return nil
}

func (s *InventoryOperationService) canPlaceInContainer(
	w *ecs.World,
	item *components.InvItem,
	owner *components.InventoryOwner,
	handle types.Handle,
	container *components.InventoryContainer,
) bool {
	if owner == nil || container == nil {
		return false
	}
	dstInfo := &ContainerInfo{
		Handle:    handle,
		Container: container,
		Owner:     owner,
	}
	return s.validator.ValidateItemAllowedInContainer(w, item, dstInfo, 0) == nil
}

func (s *InventoryOperationService) placeNewItemInContainer(
	w *ecs.World,
	playerHandle types.Handle,
	containerHandle types.Handle,
	item *components.InvItem,
	itemDef *itemdefs.ItemDef,
	x, y uint8,
) []*ContainerInfo {
	if item == nil || s.idAllocator == nil {
		return nil
	}
	item.ItemID = s.idAllocator.GetFreeID()
	item.X = x
	item.Y = y

	ecs.MutateComponent[components.InventoryContainer](w, containerHandle, func(c *components.InventoryContainer) bool {
		c.Items = append(c.Items, *item)
		c.Version++
		return true
	})

	nestedHandle := ensureNestedContainer(w, playerHandle, item, itemDef)
	updatedOwner, _ := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	updatedContainer, _ := ecs.GetComponent[components.InventoryContainer](w, containerHandle)

	updatedContainers := []*ContainerInfo{
		{
			Handle:    containerHandle,
			Container: &updatedContainer,
			Owner:     &updatedOwner,
		},
	}
	if nestedHandle != 0 {
		nestedContainer, _ := ecs.GetComponent[components.InventoryContainer](w, nestedHandle)
		updatedContainers = append(updatedContainers, &ContainerInfo{
			Handle:    nestedHandle,
			Container: &nestedContainer,
			Owner:     &updatedOwner,
		})
	}

	return updatedContainers
}

func orderedGridLinks(
	links []components.InventoryLink,
	playerID types.EntityID,
	policy givePlacementPolicy,
) []components.InventoryLink {
	root := make([]components.InventoryLink, 0, len(links))
	nested := make([]components.InventoryLink, 0, len(links))
	for _, link := range links {
		if link.Kind != constt.InventoryGrid {
			continue
		}
		if link.OwnerID == playerID {
			root = append(root, link)
			continue
		}
		nested = append(nested, link)
	}

	switch policy {
	case givePlacementNestedRootHand:
		return append(nested, root...)
	default:
		return append(root, nested...)
	}
}

func mergeUpdatedContainerInfos(
	existing []*ContainerInfo,
	updated []*ContainerInfo,
) []*ContainerInfo {
	if len(updated) == 0 {
		return existing
	}
	indexByHandle := make(map[types.Handle]int, len(existing)+len(updated))
	for idx, info := range existing {
		if info == nil {
			continue
		}
		indexByHandle[info.Handle] = idx
	}

	for _, info := range updated {
		if info == nil {
			continue
		}
		if idx, exists := indexByHandle[info.Handle]; exists {
			existing[idx] = info
			continue
		}
		indexByHandle[info.Handle] = len(existing)
		existing = append(existing, info)
	}
	return existing
}
