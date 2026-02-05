package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type PlacementService struct{}

func NewPlacementService() *PlacementService {
	return &PlacementService{}
}

type PlacementResult struct {
	Success bool
	X       uint8
	Y       uint8

	// For swap operations
	SwapItem      *components.InvItem
	SwapItemIndex int

	// For merge operations
	MergedQuantity   uint32
	RemainingInSrc   uint32
	MergeTargetIndex int
}

func (ps *PlacementService) CheckGridPlacement(
	container *components.InventoryContainer,
	item *components.InvItem,
	dstX, dstY uint8,
	allowSwapOrMerge bool,
) *PlacementResult {
	if container.Kind != constt.InventoryGrid {
		return &PlacementResult{Success: false}
	}

	// Check bounds
	if int(dstX)+int(item.W) > int(container.Width) ||
		int(dstY)+int(item.H) > int(container.Height) {
		return &PlacementResult{Success: false}
	}

	// Check for collisions with existing items
	var collidingItem *components.InvItem
	collidingIndex := -1

	for i := range container.Items {
		existing := &container.Items[i]
		if existing.ItemID == item.ItemID {
			// Skip self
			continue
		}

		if ps.itemsOverlap(dstX, dstY, item.W, item.H, existing) {
			if collidingItem != nil {
				// Multiple collisions - cannot place
				return &PlacementResult{Success: false}
			}
			collidingItem = existing
			collidingIndex = i
		}
	}

	// No collision - can place directly
	if collidingItem == nil {
		return &PlacementResult{
			Success: true,
			X:       dstX,
			Y:       dstY,
		}
	}

	// Collision exists - check if swap or merge is allowed
	if !allowSwapOrMerge {
		return &PlacementResult{Success: false}
	}

	// Try merge if same type and stackable
	if collidingItem.TypeID == item.TypeID {
		if mergeResult := ps.tryMerge(item, collidingItem, collidingIndex); mergeResult != nil {
			return mergeResult
		}
	}

	// Try swap - check if colliding item can fit in source position
	// For swap, we need the source container info which we don't have here
	// So we just return the swap candidate and let the caller handle it
	return &PlacementResult{
		Success:       true,
		X:             dstX,
		Y:             dstY,
		SwapItem:      collidingItem,
		SwapItemIndex: collidingIndex,
	}
}

func (ps *PlacementService) itemsOverlap(
	x1, y1, w1, h1 uint8,
	item2 *components.InvItem,
) bool {
	x2, y2 := item2.X, item2.Y
	w2, h2 := item2.W, item2.H

	// Check if rectangles overlap
	return x1 < x2+w2 && x1+w1 > x2 && y1 < y2+h2 && y1+h1 > y2
}

func (ps *PlacementService) tryMerge(
	srcItem *components.InvItem,
	dstItem *components.InvItem,
	dstIndex int,
) *PlacementResult {
	itemDef, ok := itemdefs.Global().GetByID(int(srcItem.TypeID))
	if !ok {
		return nil
	}

	// Check if stackable
	if itemDef.Stack == nil || itemDef.Stack.Mode != itemdefs.StackModeStack {
		return nil
	}

	maxStack := uint32(itemDef.Stack.Max)
	if dstItem.Quantity >= maxStack {
		// Destination stack is full
		return nil
	}

	// Calculate merge amounts
	spaceInDst := maxStack - dstItem.Quantity
	toTransfer := srcItem.Quantity
	if toTransfer > spaceInDst {
		toTransfer = spaceInDst
	}

	return &PlacementResult{
		Success:          true,
		X:                dstItem.X,
		Y:                dstItem.Y,
		MergedQuantity:   toTransfer,
		RemainingInSrc:   srcItem.Quantity - toTransfer,
		MergeTargetIndex: dstIndex,
	}
}

func (ps *PlacementService) CheckHandPlacement(
	container *components.InventoryContainer,
	item *components.InvItem,
	allowSwapOrMerge bool,
) *PlacementResult {
	if container.Kind != constt.InventoryHand {
		return &PlacementResult{Success: false}
	}

	// Hand can hold only one item
	if len(container.Items) == 0 {
		return &PlacementResult{Success: true}
	}

	if !allowSwapOrMerge {
		return &PlacementResult{Success: false}
	}

	// Swap with existing item
	existingItem := &container.Items[0]

	// Try merge first if same type
	if existingItem.TypeID == item.TypeID {
		if mergeResult := ps.tryMerge(item, existingItem, 0); mergeResult != nil {
			return mergeResult
		}
	}

	return &PlacementResult{
		Success:       true,
		SwapItem:      existingItem,
		SwapItemIndex: 0,
	}
}

func (ps *PlacementService) CheckEquipmentPlacement(
	container *components.InventoryContainer,
	item *components.InvItem,
	slot netproto.EquipSlot,
	allowSwapOrMerge bool,
) *PlacementResult {
	if container.Kind != constt.InventoryEquipment {
		return &PlacementResult{Success: false}
	}

	// Find if slot is already occupied
	for i := range container.Items {
		existing := &container.Items[i]
		if existing.EquipSlot == slot {
			if !allowSwapOrMerge {
				return &PlacementResult{Success: false}
			}
			return &PlacementResult{
				Success:       true,
				SwapItem:      existing,
				SwapItemIndex: i,
			}
		}
	}

	return &PlacementResult{Success: true}
}

func (ps *PlacementService) FindFreeSpace(
	container *components.InventoryContainer,
	itemW, itemH uint8,
) (bool, uint8, uint8) {
	if container.Kind != constt.InventoryGrid {
		return false, 0, 0
	}

	// Simple first-fit algorithm
	for y := uint8(0); y <= container.Height-itemH; y++ {
		for x := uint8(0); x <= container.Width-itemW; x++ {
			if ps.canPlaceAt(container, x, y, itemW, itemH, types.EntityID(0)) {
				return true, x, y
			}
		}
	}

	return false, 0, 0
}

func (ps *PlacementService) canPlaceAt(
	container *components.InventoryContainer,
	x, y, w, h uint8,
	excludeItemID types.EntityID,
) bool {
	for i := range container.Items {
		item := &container.Items[i]
		if item.ItemID == excludeItemID {
			continue
		}
		if ps.itemsOverlap(x, y, w, h, item) {
			return false
		}
	}
	return true
}

func (ps *PlacementService) ValidateSwap(
	srcContainer *components.InventoryContainer,
	srcItem *components.InvItem,
	dstContainer *components.InventoryContainer,
	swapItem *components.InvItem,
	srcX, srcY uint8,
) bool {
	// Check if swap item can fit in source position
	switch srcContainer.Kind {
	case constt.InventoryGrid:
		// Check if swap item fits at source position
		if int(srcX)+int(swapItem.W) > int(srcContainer.Width) ||
			int(srcY)+int(swapItem.H) > int(srcContainer.Height) {
			return false
		}
		// Check for collisions (excluding the original item)
		return ps.canPlaceAt(srcContainer, srcX, srcY, swapItem.W, swapItem.H, srcItem.ItemID)

	case constt.InventoryHand:
		// Hand can always accept one item
		return true

	case constt.InventoryEquipment:
		// Equipment swap needs slot validation - handled by caller
		return true
	}

	return false
}
