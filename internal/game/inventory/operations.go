package inventory

import (
	"encoding/json"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

// droppedItemData is the JSON structure stored in object.data for dropped items.
type droppedItemData struct {
	HasInventory    bool   `json:"has_inventory"`
	ContainedItemID uint64 `json:"contained_item_id"`
	DropTime        int64  `json:"drop_time"`
	DropperID       uint64 `json:"dropper_id"`
}

type OperationResult struct {
	Success   bool
	ErrorCode netproto.ErrorCode
	Message   string

	// Updated containers (for sending to client)
	UpdatedContainers []*ContainerInfo

	// ClosedContainerRefs lists nested container refs that should be closed on the client
	// (e.g. when a container item is picked up into the hand).
	ClosedContainerRefs []*netproto.InventoryRef

	// For drop_to_world operations
	SpawnedDroppedEntityID *types.EntityID

	// For pickup_from_world operations
	DespawnedDroppedEntityID *types.EntityID
}

// EntityIDAllocator provides unique entity IDs for new dropped items.
type EntityIDAllocator interface {
	GetFreeID() types.EntityID
}

// DroppedItemPersister handles DB persistence for dropped item objects and their inventory.
type DroppedItemPersister interface {
	PersistDroppedObject(entityID types.EntityID, typeID int, region, x, y, layer, chunkX, chunkY int, objectData json.RawMessage, inventoryData json.RawMessage) error
	DeleteDroppedObject(region int, entityID types.EntityID) error
}

type InventoryOperationService struct {
	validator        *Validator
	placementService *PlacementService
	idAllocator      EntityIDAllocator
	persister        DroppedItemPersister
	logger           *zap.Logger
}

func NewInventoryOperationService(
	logger *zap.Logger,
	idAlloc EntityIDAllocator,
	persister DroppedItemPersister,
) *InventoryOperationService {
	return &InventoryOperationService{
		validator:        NewValidator(),
		placementService: NewPlacementService(),
		idAllocator:      idAlloc,
		persister:        persister,
		logger:           logger,
	}
}

func (s *InventoryOperationService) ExecuteOperation(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	op *netproto.InventoryOp,
) *OperationResult {
	switch kind := op.Kind.(type) {
	case *netproto.InventoryOp_Move:
		return s.ExecuteMove(w, playerID, playerHandle, op.OpId, kind.Move, op.Expected)
	case *netproto.InventoryOp_DropToWorld:
		return s.ExecuteDropToWorld(w, playerID, playerHandle, op.OpId, kind.DropToWorld, op.Expected)
	default:
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Unknown operation type",
		}
	}
}

func (s *InventoryOperationService) ExecuteMove(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	opID uint64,
	moveSpec *netproto.InventoryMoveSpec,
	expected []*netproto.InventoryExpected,
) *OperationResult {
	// 1. Resolve source container
	srcInfo, err := s.validator.ResolveContainer(w, moveSpec.Src, playerID, playerHandle)
	if err != nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: err.Code,
			Message:   err.Message,
		}
	}

	// 2. Resolve destination container
	dstInfo, err := s.validator.ResolveContainer(w, moveSpec.Dst, playerID, playerHandle)
	if err != nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: err.Code,
			Message:   err.Message,
		}
	}

	// 3. Validate expected versions (optimistic concurrency)
	containers := make(map[string]*ContainerInfo)
	srcKey := MakeContainerKeyFromInfo(srcInfo.Container.OwnerID, srcInfo.Container.Kind, srcInfo.Container.Key)
	dstKey := MakeContainerKeyFromInfo(dstInfo.Container.OwnerID, dstInfo.Container.Kind, dstInfo.Container.Key)
	containers[srcKey] = srcInfo
	containers[dstKey] = dstInfo

	if err := s.validator.ValidateExpectedVersions(w, expected, containers); err != nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: err.Code,
			Message:   err.Message,
		}
	}

	// 4. Find the item in source container
	itemID := types.EntityID(moveSpec.ItemId)
	srcItemIndex, srcItem := s.validator.FindItemInContainer(srcInfo.Container, itemID)
	if srcItem == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message:   "Item not found in source container",
		}
	}

	// 5. Validate item can be placed in destination
	dstEquipSlot := netproto.EquipSlot_EQUIP_SLOT_NONE
	if moveSpec.DstEquipSlot != nil {
		dstEquipSlot = *moveSpec.DstEquipSlot
	}

	if err := s.validator.ValidateItemAllowedInContainer(w, srcItem, dstInfo, dstEquipSlot); err != nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: err.Code,
			Message:   err.Message,
		}
	}

	// 6. Check placement and handle swap/merge
	var placementResult *PlacementResult

	switch dstInfo.Container.Kind {
	case constt.InventoryGrid:
		dstX, dstY := uint8(0), uint8(0)
		if moveSpec.DstPos != nil {
			dstX = uint8(moveSpec.DstPos.X)
			dstY = uint8(moveSpec.DstPos.Y)
		} else {
			// Find free space
			found, x, y := s.placementService.FindFreeSpace(dstInfo.Container, srcItem.W, srcItem.H)
			if !found {
				return &OperationResult{
					Success:   false,
					ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL,
					Message:   "No free space in destination",
				}
			}
			dstX, dstY = x, y
		}
		placementResult = s.placementService.CheckGridPlacement(
			dstInfo.Container, srcItem, dstX, dstY, moveSpec.AllowSwapOrMerge,
		)

	case constt.InventoryHand:
		placementResult = s.placementService.CheckHandPlacement(
			dstInfo.Container, srcItem, moveSpec.AllowSwapOrMerge,
		)

	case constt.InventoryEquipment:
		placementResult = s.placementService.CheckEquipmentPlacement(
			dstInfo.Container, srcItem, dstEquipSlot, moveSpec.AllowSwapOrMerge,
		)

	default:
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Unsupported destination container type",
		}
	}

	if !placementResult.Success {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL,
			Message:   "Cannot place item at destination",
		}
	}

	// 7. Execute the operation
	sameSrcDst := srcInfo.Handle == dstInfo.Handle

	if placementResult.MergedQuantity > 0 {
		// Merge operation
		return s.executeMerge(w, srcInfo, dstInfo, playerHandle, srcItemIndex, placementResult, sameSrcDst, moveSpec)
	}

	if placementResult.SwapItem != nil {
		// Swap operation - validate reverse placement
		if !s.placementService.ValidateSwap(
			srcInfo.Container, srcItem,
			dstInfo.Container, placementResult.SwapItem,
			srcItem.X, srcItem.Y,
		) {
			return &OperationResult{
				Success:   false,
				ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL,
				Message:   "Cannot swap items - destination item doesn't fit in source",
			}
		}
		return s.executeSwap(w, srcInfo, dstInfo, playerHandle, srcItemIndex, placementResult, dstEquipSlot, sameSrcDst, moveSpec)
	}

	// Simple move
	return s.executeSimpleMove(w, srcInfo, dstInfo, playerHandle, srcItemIndex, placementResult, dstEquipSlot, sameSrcDst, moveSpec)
}

func (s *InventoryOperationService) executeMerge(
	w *ecs.World,
	srcInfo, dstInfo *ContainerInfo,
	playerHandle types.Handle,
	srcItemIndex int,
	placement *PlacementResult,
	sameSrcDst bool,
	moveSpec *netproto.InventoryMoveSpec,
) *OperationResult {
	srcItem := srcInfo.Container.Items[srcItemIndex]

	// Update destination item quantity
	ecs.MutateComponent[components.InventoryContainer](w, dstInfo.Handle, func(c *components.InventoryContainer) bool {
		c.Items[placement.MergeTargetIndex].Quantity += placement.MergedQuantity
		c.Version++
		return true
	})

	// Update or remove source item
	ecs.MutateComponent[components.InventoryContainer](w, srcInfo.Handle, func(c *components.InventoryContainer) bool {
		if placement.RemainingInSrc == 0 {
			// Remove item from source
			c.Items = append(c.Items[:srcItemIndex], c.Items[srcItemIndex+1:]...)
			// If source was HAND and item fully consumed, reset hand offset
			if c.Kind == constt.InventoryHand {
				c.HandMouseOffsetX = 0
				c.HandMouseOffsetY = 0
			}
		} else {
			c.Items[srcItemIndex].Quantity = placement.RemainingInSrc
		}
		if !sameSrcDst {
			c.Version++
		}
		return true
	})

	// Refresh container info for response
	updatedSrc, _ := ecs.GetComponent[components.InventoryContainer](w, srcInfo.Handle)
	srcInfo.Container = &updatedSrc

	result := &OperationResult{
		Success:           true,
		UpdatedContainers: []*ContainerInfo{srcInfo},
	}

	if !sameSrcDst {
		updatedDst, _ := ecs.GetComponent[components.InventoryContainer](w, dstInfo.Handle)
		dstInfo.Container = &updatedDst
		result.UpdatedContainers = append(result.UpdatedContainers, dstInfo)

		if placement.RemainingInSrc == 0 {
			reconcileNestedContainerOwnerLink(w, srcInfo.Owner, playerHandle, srcItem.ItemID, dstInfo.Handle)
			appendClosedNestedRefIfPresent(w, result, srcItem.ItemID)
		}
	}

	return result
}

func (s *InventoryOperationService) executeSwap(
	w *ecs.World,
	srcInfo, dstInfo *ContainerInfo,
	playerHandle types.Handle,
	srcItemIndex int,
	placement *PlacementResult,
	dstEquipSlot netproto.EquipSlot,
	sameSrcDst bool,
	moveSpec *netproto.InventoryMoveSpec,
) *OperationResult {
	// Get the items to swap
	srcItem := srcInfo.Container.Items[srcItemIndex]
	swapItem := *placement.SwapItem

	// Store original positions
	origSrcX, origSrcY := srcItem.X, srcItem.Y
	origSrcSlot := srcItem.EquipSlot

	// Update source container - replace src item with swap item
	ecs.MutateComponent[components.InventoryContainer](w, srcInfo.Handle, func(c *components.InventoryContainer) bool {
		// Update swap item position to source position
		swapItem.X = origSrcX
		swapItem.Y = origSrcY
		swapItem.EquipSlot = origSrcSlot
		c.Items[srcItemIndex] = swapItem
		// If source is HAND, swap item goes into hand — use default offset
		if c.Kind == constt.InventoryHand {
			c.HandMouseOffsetX = constt.DefaultHandMouseOffset
			c.HandMouseOffsetY = constt.DefaultHandMouseOffset
		}
		c.Version++
		return true
	})

	// Update destination container - replace swap item with src item
	ecs.MutateComponent[components.InventoryContainer](w, dstInfo.Handle, func(c *components.InventoryContainer) bool {
		srcItem.X = placement.X
		srcItem.Y = placement.Y
		srcItem.EquipSlot = dstEquipSlot
		c.Items[placement.SwapItemIndex] = srcItem
		// If dst is HAND, set hand offset from moveSpec or use default
		if c.Kind == constt.InventoryHand {
			applyHandOffset(c, moveSpec)
		}
		if !sameSrcDst {
			c.Version++
		}
		return true
	})

	// Refresh container info for response
	updatedSrc, _ := ecs.GetComponent[components.InventoryContainer](w, srcInfo.Handle)
	srcInfo.Container = &updatedSrc

	result := &OperationResult{
		Success:           true,
		UpdatedContainers: []*ContainerInfo{srcInfo},
	}

	if !sameSrcDst {
		updatedDst, _ := ecs.GetComponent[components.InventoryContainer](w, dstInfo.Handle)
		dstInfo.Container = &updatedDst
		result.UpdatedContainers = append(result.UpdatedContainers, dstInfo)

		reconcileNestedContainerOwnerLink(w, srcInfo.Owner, playerHandle, srcItem.ItemID, dstInfo.Handle)
		reconcileNestedContainerOwnerLink(w, srcInfo.Owner, playerHandle, swapItem.ItemID, srcInfo.Handle)
		appendClosedNestedRefIfPresent(w, result, srcItem.ItemID)
		appendClosedNestedRefIfPresent(w, result, swapItem.ItemID)
	}

	return result
}

func (s *InventoryOperationService) executeSimpleMove(
	w *ecs.World,
	srcInfo, dstInfo *ContainerInfo,
	playerHandle types.Handle,
	srcItemIndex int,
	placement *PlacementResult,
	dstEquipSlot netproto.EquipSlot,
	sameSrcDst bool,
	moveSpec *netproto.InventoryMoveSpec,
) *OperationResult {
	// Get the item to move
	srcItem := srcInfo.Container.Items[srcItemIndex]

	if sameSrcDst {
		// Moving within same container - just update position
		ecs.MutateComponent[components.InventoryContainer](w, srcInfo.Handle, func(c *components.InventoryContainer) bool {
			c.Items[srcItemIndex].X = placement.X
			c.Items[srcItemIndex].Y = placement.Y
			c.Items[srcItemIndex].EquipSlot = dstEquipSlot
			c.Version++
			return true
		})
	} else {
		// Moving between containers
		// Remove from source
		ecs.MutateComponent[components.InventoryContainer](w, srcInfo.Handle, func(c *components.InventoryContainer) bool {
			c.Items = append(c.Items[:srcItemIndex], c.Items[srcItemIndex+1:]...)
			// If source was HAND, reset hand offset
			if c.Kind == constt.InventoryHand {
				c.HandMouseOffsetX = 0
				c.HandMouseOffsetY = 0
			}
			c.Version++
			return true
		})

		// Add to destination
		ecs.MutateComponent[components.InventoryContainer](w, dstInfo.Handle, func(c *components.InventoryContainer) bool {
			srcItem.X = placement.X
			srcItem.Y = placement.Y
			srcItem.EquipSlot = dstEquipSlot
			c.Items = append(c.Items, srcItem)
			// If dst is HAND, set hand offset from moveSpec or use default
			if c.Kind == constt.InventoryHand {
				applyHandOffset(c, moveSpec)
			}
			c.Version++
			return true
		})
	}

	// Refresh container info for response
	updatedSrc, _ := ecs.GetComponent[components.InventoryContainer](w, srcInfo.Handle)
	srcInfo.Container = &updatedSrc

	result := &OperationResult{
		Success:           true,
		UpdatedContainers: []*ContainerInfo{srcInfo},
	}

	if !sameSrcDst {
		updatedDst, _ := ecs.GetComponent[components.InventoryContainer](w, dstInfo.Handle)
		dstInfo.Container = &updatedDst
		result.UpdatedContainers = append(result.UpdatedContainers, dstInfo)

		reconcileNestedContainerOwnerLink(w, srcInfo.Owner, playerHandle, srcItem.ItemID, dstInfo.Handle)
		appendClosedNestedRefIfPresent(w, result, srcItem.ItemID)
	}

	return result
}

// applyHandOffset sets HandMouseOffsetX/Y on the container from moveSpec.HandPos,
// or uses DefaultHandMouseOffset if not provided.
func applyHandOffset(c *components.InventoryContainer, moveSpec *netproto.InventoryMoveSpec) {
	if moveSpec.HandPos != nil {
		c.HandMouseOffsetX = int16(moveSpec.HandPos.MouseOffsetX)
		c.HandMouseOffsetY = int16(moveSpec.HandPos.MouseOffsetY)
	} else {
		c.HandMouseOffsetX = constt.DefaultHandMouseOffset
		c.HandMouseOffsetY = constt.DefaultHandMouseOffset
	}
}

func (s *InventoryOperationService) ExecuteDropToWorld(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	opID uint64,
	moveSpec *netproto.InventoryMoveSpec,
	expected []*netproto.InventoryExpected,
) *OperationResult {
	if s.idAllocator == nil || s.persister == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "drop dependencies not configured",
		}
	}

	// 1. Resolve source container and find item
	srcInfo, verr := s.validator.ResolveContainer(w, moveSpec.Src, playerID, playerHandle)
	if verr != nil {
		return &OperationResult{Success: false, ErrorCode: verr.Code, Message: verr.Message}
	}

	itemID := types.EntityID(moveSpec.ItemId)
	srcItemIndex, srcItem := s.validator.FindItemInContainer(srcInfo.Container, itemID)
	if srcItem == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message:   "Item not found in source container",
		}
	}

	// 2. Validate expected versions
	if len(expected) > 0 {
		containers := map[string]*ContainerInfo{
			MakeContainerKeyFromInfo(srcInfo.Container.OwnerID, srcInfo.Container.Kind, srcInfo.Container.Key): srcInfo,
		}
		if verr := s.validator.ValidateExpectedVersions(w, expected, containers); verr != nil {
			return &OperationResult{Success: false, ErrorCode: verr.Code, Message: verr.Message}
		}
	}

	// 3. Get player position for drop location
	playerTransform, hasTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	if !hasTransform {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Player has no transform",
		}
	}

	playerInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, playerHandle)
	if !hasInfo {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Player has no entity info",
		}
	}

	playerChunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, playerHandle)
	if !hasChunkRef {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Player has no chunk ref",
		}
	}

	// 4. Use item ID as the dropped entity ID (object.id == item.id == inventory.owner_id)
	droppedEntityID := itemID
	nowUnix := ecs.GetResource[ecs.TimeState](w).Now.Unix()

	// Serialize nested inventory before removing item (needed for DB persistence)
	nestedInvData := serializeNestedForDrop(w, itemID)
	hasNestedItems := nestedInvData != nil && len(nestedInvData.Items) > 0

	// Resolve item resource (check nested items for visual)
	resource := srcItem.Resource
	if itemDef, ok := itemdefs.Global().GetByID(int(srcItem.TypeID)); ok {
		resource = itemDef.ResolveResource(hasNestedItems)
	}

	dropX := int(playerTransform.X)
	dropY := int(playerTransform.Y)

	// 5. Remove item from source container
	ecs.MutateComponent[components.InventoryContainer](w, srcInfo.Handle, func(c *components.InventoryContainer) bool {
		c.Items = append(c.Items[:srcItemIndex], c.Items[srcItemIndex+1:]...)
		if c.Kind == constt.InventoryHand {
			c.HandMouseOffsetX = 0
			c.HandMouseOffsetY = 0
		}
		c.Version++
		return true
	})

	// 5b. Detach nested container from player (keep alive in ECS + RefIndex)
	detachNestedContainer(w, playerHandle, itemID)

	// 6. Create dropped entity in ECS
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
			Name:     nil,
			Resource: resource,
		})

		ecs.AddComponent(w, h, components.DroppedItem{
			DropTime:        nowUnix,
			DropperID:       playerID,
			ContainedItemID: itemID,
		})

		// Create inventory container for the dropped item
		container := components.InventoryContainer{
			OwnerID: droppedEntityID,
			Kind:    constt.InventoryDroppedItem,
			Key:     0,
			Version: 1,
			Items: []components.InvItem{
				{
					ItemID:   itemID,
					TypeID:   srcItem.TypeID,
					Resource: resource,
					Quality:  srcItem.Quality,
					Quantity: srcItem.Quantity,
					W:        srcItem.W,
					H:        srcItem.H,
				},
			},
		}
		containerHandle := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, containerHandle, container)

		refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
		refIndex.Add(constt.InventoryDroppedItem, droppedEntityID, 0, containerHandle)
	})

	if droppedHandle == types.InvalidHandle {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Failed to spawn dropped entity",
		}
	}

	// 7. Persist to DB (object + inventory including nested)
	droppedData := droppedItemData{
		HasInventory:    true,
		ContainedItemID: uint64(itemID),
		DropTime:        nowUnix,
		DropperID:       uint64(playerID),
	}
	objectJSON, _ := json.Marshal(droppedData)

	// Serialize the dropped container for the inventory table
	droppedItem := InventoryItemV1{
		ItemID:          uint64(itemID),
		TypeID:          srcItem.TypeID,
		Quality:         srcItem.Quality,
		Quantity:        srcItem.Quantity,
		NestedInventory: nestedInvData,
	}
	invData := InventoryDataV1{
		Kind:    uint8(constt.InventoryDroppedItem),
		Key:     0,
		Version: 1,
		Items:   []InventoryItemV1{droppedItem},
	}
	inventoryJSON, _ := json.Marshal(invData)

	if err := s.persister.PersistDroppedObject(
		droppedEntityID, constt.DroppedItemTypeID,
		playerInfo.Region, dropX, dropY, playerInfo.Layer,
		playerChunkRef.CurrentChunkX, playerChunkRef.CurrentChunkY,
		objectJSON, inventoryJSON,
	); err != nil {
		s.logger.Error("Failed to persist dropped object",
			zap.Uint64("entity_id", uint64(droppedEntityID)),
			zap.Error(err))
	}

	// 8. Build result
	updatedSrc, _ := ecs.GetComponent[components.InventoryContainer](w, srcInfo.Handle)
	srcInfo.Container = &updatedSrc

	s.logger.Debug("Item dropped to world",
		zap.Uint64("player_id", uint64(playerID)),
		zap.Uint64("dropped_entity_id", uint64(droppedEntityID)),
		zap.Uint64("item_id", uint64(itemID)),
		zap.Int("x", dropX),
		zap.Int("y", dropY))

	return &OperationResult{
		Success:                true,
		UpdatedContainers:      []*ContainerInfo{srcInfo},
		SpawnedDroppedEntityID: &droppedEntityID,
	}
}

func (s *InventoryOperationService) ExecutePickupFromWorld(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	droppedEntityID types.EntityID,
	dstRef *netproto.InventoryRef,
) *OperationResult {
	if s.persister == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "drop dependencies not configured",
		}
	}

	// 1. Find the dropped entity by ID
	droppedHandle := w.GetHandleByEntityID(droppedEntityID)
	if droppedHandle == types.InvalidHandle {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message:   "Dropped entity not found",
		}
	}

	// Verify it's actually a dropped item
	_, hasDropped := ecs.GetComponent[components.DroppedItem](w, droppedHandle)
	if !hasDropped {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Entity is not a dropped item",
		}
	}

	// 2. Check distance
	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, playerHandle)
	droppedTransform, hasDroppedTransform := ecs.GetComponent[components.Transform](w, droppedHandle)
	if !hasPlayerTransform || !hasDroppedTransform {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Missing transform",
		}
	}

	dx := playerTransform.X - droppedTransform.X
	dy := playerTransform.Y - droppedTransform.Y
	distSq := dx*dx + dy*dy
	if distSq > constt.DroppedPickupRadiusSq {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_OUT_OF_RANGE,
			Message:   "Too far to pick up",
		}
	}

	// 3. Get the item from the dropped entity's container
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	containerHandle, found := refIndex.Lookup(constt.InventoryDroppedItem, droppedEntityID, 0)
	if !found {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message:   "Dropped item container not found",
		}
	}

	droppedContainer, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
	if !hasContainer || len(droppedContainer.Items) == 0 {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message:   "Dropped item container is empty",
		}
	}

	srcItem := droppedContainer.Items[0]

	// 4. Resolve destination container
	dstInfo, verr := s.validator.ResolveContainer(w, dstRef, playerID, playerHandle)
	if verr != nil {
		return &OperationResult{Success: false, ErrorCode: verr.Code, Message: verr.Message}
	}

	// 5. Validate item can be placed in destination
	if verr := s.validator.ValidateItemAllowedInContainer(w, &srcItem, dstInfo, netproto.EquipSlot_EQUIP_SLOT_NONE); verr != nil {
		return &OperationResult{Success: false, ErrorCode: verr.Code, Message: verr.Message}
	}

	// 6. Check placement
	var placementResult *PlacementResult
	switch dstInfo.Container.Kind {
	case constt.InventoryGrid:
		found, x, y := s.placementService.FindFreeSpace(dstInfo.Container, srcItem.W, srcItem.H)
		if !found {
			return &OperationResult{
				Success:   false,
				ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL,
				Message:   "No free space in destination",
			}
		}
		placementResult = s.placementService.CheckGridPlacement(dstInfo.Container, &srcItem, x, y, false)
	case constt.InventoryHand:
		placementResult = s.placementService.CheckHandPlacement(dstInfo.Container, &srcItem, false)
	default:
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Unsupported destination container type for pickup",
		}
	}

	if !placementResult.Success {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL,
			Message:   "Cannot place item at destination",
		}
	}

	// 7. Add item to destination container
	ecs.MutateComponent[components.InventoryContainer](w, dstInfo.Handle, func(c *components.InventoryContainer) bool {
		srcItem.X = placementResult.X
		srcItem.Y = placementResult.Y
		c.Items = append(c.Items, srcItem)
		if c.Kind == constt.InventoryHand {
			c.HandMouseOffsetX = constt.DefaultHandMouseOffset
			c.HandMouseOffsetY = constt.DefaultHandMouseOffset
		}
		c.Version++
		return true
	})

	// 8. Attach or create nested container for container items (e.g. seed_bag with seeds)
	var nestedHandle types.Handle
	if srcItemDef, ok := itemdefs.Global().GetByID(int(srcItem.TypeID)); ok {
		nestedHandle = ensureNestedContainer(w, playerHandle, &srcItem, srcItemDef)
	}

	// 9. Delete dropped entity from ECS and InventoryRefIndex
	droppedInfo, _ := ecs.GetComponent[components.EntityInfo](w, droppedHandle)
	deleteDroppedEntityFromECS(w, droppedEntityID, droppedHandle, s.logger)

	// 10. Soft-delete from DB
	if droppedInfo.Region > 0 {
		if err := s.persister.DeleteDroppedObject(droppedInfo.Region, droppedEntityID); err != nil {
			s.logger.Error("Failed to soft-delete picked up object from DB",
				zap.Uint64("entity_id", uint64(droppedEntityID)),
				zap.Error(err))
		}
	}

	// 11. Build result
	updatedOwner, _ := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	updatedDst, _ := ecs.GetComponent[components.InventoryContainer](w, dstInfo.Handle)
	dstInfo.Container = &updatedDst
	dstInfo.Owner = &updatedOwner

	updatedContainers := []*ContainerInfo{dstInfo}
	if nestedHandle != 0 {
		nestedContainer, _ := ecs.GetComponent[components.InventoryContainer](w, nestedHandle)
		updatedContainers = append(updatedContainers, &ContainerInfo{
			Handle:    nestedHandle,
			Container: &nestedContainer,
			Owner:     &updatedOwner,
		})
	}

	s.logger.Debug("Item picked up from world",
		zap.Uint64("player_id", uint64(playerID)),
		zap.Uint64("dropped_entity_id", uint64(droppedEntityID)),
		zap.Uint64("item_id", uint64(srcItem.ItemID)))

	return &OperationResult{
		Success:                  true,
		UpdatedContainers:        updatedContainers,
		DespawnedDroppedEntityID: &droppedEntityID,
	}
}

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
