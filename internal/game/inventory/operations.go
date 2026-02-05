package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

type OperationResult struct {
	Success   bool
	ErrorCode netproto.ErrorCode
	Message   string

	// Updated containers (for sending to client)
	UpdatedContainers []*ContainerInfo

	// For drop_to_world operations
	SpawnedDroppedEntityID *types.EntityID

	// For pickup_from_world operations
	DespawnedDroppedEntityID *types.EntityID
}

type InventoryOperationService struct {
	validator        *Validator
	placementService *PlacementService
	logger           *zap.Logger
}

func NewInventoryOperationService(
	logger *zap.Logger,
) *InventoryOperationService {
	return &InventoryOperationService{
		validator:        NewValidator(),
		placementService: NewPlacementService(),
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

	if err := s.validator.ValidateItemAllowedInContainer(srcItem, dstInfo.Container, dstEquipSlot); err != nil {
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
		return s.executeMerge(w, srcInfo, dstInfo, srcItemIndex, placementResult, sameSrcDst)
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
		return s.executeSwap(w, srcInfo, dstInfo, srcItemIndex, placementResult, dstEquipSlot, sameSrcDst)
	}

	// Simple move
	return s.executeSimpleMove(w, srcInfo, dstInfo, srcItemIndex, placementResult, dstEquipSlot, sameSrcDst)
}

func (s *InventoryOperationService) executeMerge(
	w *ecs.World,
	srcInfo, dstInfo *ContainerInfo,
	srcItemIndex int,
	placement *PlacementResult,
	sameSrcDst bool,
) *OperationResult {
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
	}

	return result
}

func (s *InventoryOperationService) executeSwap(
	w *ecs.World,
	srcInfo, dstInfo *ContainerInfo,
	srcItemIndex int,
	placement *PlacementResult,
	dstEquipSlot netproto.EquipSlot,
	sameSrcDst bool,
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
		c.Version++
		return true
	})

	// Update destination container - replace swap item with src item
	ecs.MutateComponent[components.InventoryContainer](w, dstInfo.Handle, func(c *components.InventoryContainer) bool {
		srcItem.X = placement.X
		srcItem.Y = placement.Y
		srcItem.EquipSlot = dstEquipSlot
		c.Items[placement.SwapItemIndex] = srcItem
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
	}

	return result
}

func (s *InventoryOperationService) executeSimpleMove(
	w *ecs.World,
	srcInfo, dstInfo *ContainerInfo,
	srcItemIndex int,
	placement *PlacementResult,
	dstEquipSlot netproto.EquipSlot,
	sameSrcDst bool,
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
			c.Version++
			return true
		})

		// Add to destination
		ecs.MutateComponent[components.InventoryContainer](w, dstInfo.Handle, func(c *components.InventoryContainer) bool {
			srcItem.X = placement.X
			srcItem.Y = placement.Y
			srcItem.EquipSlot = dstEquipSlot
			c.Items = append(c.Items, srcItem)
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
	}

	return result
}

func (s *InventoryOperationService) ExecuteDropToWorld(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	opID uint64,
	moveSpec *netproto.InventoryMoveSpec,
	expected []*netproto.InventoryExpected,
) *OperationResult {
	// TODO: Implement drop_to_world
	// This requires:
	// 1. Remove item from source container
	// 2. Create a dropped entity in the world at player's position
	// 3. Return the spawned entity ID

	s.logger.Debug("ExecuteDropToWorld not implemented yet",
		zap.Uint64("op_id", opID),
		zap.Uint64("player_id", uint64(playerID)))

	return &OperationResult{
		Success:   false,
		ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
		Message:   "drop_to_world not implemented",
	}
}

func (s *InventoryOperationService) ExecutePickupFromWorld(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	opID uint64,
	moveSpec *netproto.InventoryMoveSpec,
	expected []*netproto.InventoryExpected,
) *OperationResult {
	// TODO: Implement pickup_from_world
	// This requires:
	// 1. Validate dropped entity exists and is in range
	// 2. Remove dropped entity from world
	// 3. Add item to destination container
	// 4. Return the despawned entity ID

	s.logger.Debug("ExecutePickupFromWorld not implemented yet",
		zap.Uint64("op_id", opID),
		zap.Uint64("player_id", uint64(playerID)))

	return &OperationResult{
		Success:   false,
		ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
		Message:   "pickup_from_world not implemented",
	}
}
