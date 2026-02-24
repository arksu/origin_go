package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const buildBehaviorStateKey = "build"

func (s *InventoryOperationService) executeMoveToBuild(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	_ uint64,
	moveSpec *netproto.InventoryMoveSpec,
	expected []*netproto.InventoryExpected,
) *OperationResult {
	if moveSpec == nil || moveSpec.Src == nil || moveSpec.Dst == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Invalid move request",
		}
	}

	if constt.InventoryKind(moveSpec.Src.Kind) != constt.InventoryHand ||
		constt.InventoryKind(moveSpec.Dst.Kind) != constt.InventoryBuild {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Unsupported move for build inventory",
		}
	}
	if moveSpec.DstPos != nil || moveSpec.DstEquipSlot != nil || moveSpec.Quantity != nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Build move uses unsupported destination fields",
		}
	}
	if moveSpec.AllowSwapOrMerge {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Build move does not support swap or merge",
		}
	}
	if moveSpec.Dst.InventoryKey != 0 || moveSpec.Dst.OwnerId == 0 {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Invalid build destination reference",
		}
	}

	srcInfo, err := s.validator.ResolveContainer(w, moveSpec.Src, playerID, playerHandle)
	if err != nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: err.Code,
			Message:   err.Message,
		}
	}
	if srcInfo == nil || srcInfo.Container == nil || srcInfo.Container.Kind != constt.InventoryHand {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Source must be hand inventory",
		}
	}

	containers := map[string]*ContainerInfo{
		MakeContainerKeyFromInfo(srcInfo.Container.OwnerID, srcInfo.Container.Kind, srcInfo.Container.Key): srcInfo,
	}
	if err := s.validator.ValidateExpectedVersions(w, expected, containers); err != nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: err.Code,
			Message:   err.Message,
		}
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
	if srcItem.Quantity == 0 {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Source item quantity is empty",
		}
	}

	itemReg := itemdefs.Global()
	if itemReg == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Item registry unavailable",
		}
	}
	itemDef, ok := itemReg.GetByID(int(srcItem.TypeID))
	if !ok || itemDef == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Item definition not found",
		}
	}

	targetID := types.EntityID(moveSpec.Dst.OwnerId)
	targetHandle := w.GetHandleByEntityID(targetID)
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND,
			Message:   "Build target not found",
		}
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo || info.TypeID != constt.BuildObjectTypeID {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Target is not a build site",
		}
	}

	link, linked := ecs.GetResource[ecs.LinkState](w).GetLink(playerID)
	if !linked || link.TargetID != targetID {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
			Message:   "Player is not linked to this construction site",
		}
	}

	internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](w, targetHandle)
	if !hasInternalState {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Build state is unavailable",
		}
	}
	buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, buildBehaviorStateKey)
	if !ok || buildState == nil {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Build state is unavailable",
		}
	}

	slotIndex := -1
	var transferQty uint32
	for i := range buildState.Items {
		slot := &buildState.Items[i]
		if !buildSlotMatchesItem(slot, itemDef) {
			continue
		}

		put := uint64(slot.PutCount())
		built := uint64(slot.BuildCount)
		required := uint64(slot.RequiredCount)
		if put+built >= required {
			continue
		}

		remaining := uint32(required - (put + built))
		if remaining == 0 {
			continue
		}

		slotIndex = i
		transferQty = srcItem.Quantity
		if transferQty > remaining {
			transferQty = remaining
		}
		break
	}

	if slotIndex < 0 || transferQty == 0 {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			Message:   "Item is not required by this construction site",
		}
	}

	buildMutationOK := false
	ecs.MutateComponent[components.ObjectInternalState](w, targetHandle, func(state *components.ObjectInternalState) bool {
		currentBuild, hasBuild := components.GetBehaviorState[components.BuildBehaviorState](*state, buildBehaviorStateKey)
		if !hasBuild || currentBuild == nil || slotIndex < 0 || slotIndex >= len(currentBuild.Items) {
			return false
		}
		slot := &currentBuild.Items[slotIndex]
		slot.MergePutItem(itemDef.Key, srcItem.Quality, transferQty)
		state.IsDirty = true
		buildMutationOK = true
		return true
	})
	if !buildMutationOK {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Failed to update build state",
		}
	}

	handMutationOK := false
	ecs.MutateComponent[components.InventoryContainer](w, srcInfo.Handle, func(c *components.InventoryContainer) bool {
		if srcItemIndex < 0 || srcItemIndex >= len(c.Items) {
			return false
		}
		if c.Items[srcItemIndex].ItemID != itemID || c.Items[srcItemIndex].Quantity < transferQty {
			return false
		}
		if c.Items[srcItemIndex].Quantity == transferQty {
			c.Items = append(c.Items[:srcItemIndex], c.Items[srcItemIndex+1:]...)
			c.HandMouseOffsetX = 0
			c.HandMouseOffsetY = 0
		} else {
			c.Items[srcItemIndex].Quantity -= transferQty
		}
		c.Version++
		handMutationOK = true
		return true
	})
	if !handMutationOK {
		// Best-effort rollback to avoid losing materials if hand mutation unexpectedly fails.
		ecs.MutateComponent[components.ObjectInternalState](w, targetHandle, func(state *components.ObjectInternalState) bool {
			currentBuild, hasBuild := components.GetBehaviorState[components.BuildBehaviorState](*state, buildBehaviorStateKey)
			if !hasBuild || currentBuild == nil || slotIndex < 0 || slotIndex >= len(currentBuild.Items) {
				return false
			}
			currentBuild.Items[slotIndex].MergePutItem(itemDef.Key, srcItem.Quality, transferQty)
			state.IsDirty = true
			return true
		})
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Failed to update hand inventory",
		}
	}

	updatedOwner, hasOwner := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !hasOwner {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Player inventory owner missing",
		}
	}
	updatedHand, hasHand := ecs.GetComponent[components.InventoryContainer](w, srcInfo.Handle)
	if !hasHand {
		return &OperationResult{
			Success:   false,
			ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR,
			Message:   "Hand inventory missing after update",
		}
	}

	return &OperationResult{
		Success: true,
		UpdatedContainers: []*ContainerInfo{{
			Handle:    srcInfo.Handle,
			Container: &updatedHand,
			Owner:     &updatedOwner,
		}},
	}
}

func buildSlotMatchesItem(slot *components.BuildRequiredItemState, itemDef *itemdefs.ItemDef) bool {
	if slot == nil || itemDef == nil {
		return false
	}
	if slot.ItemKey != "" {
		return slot.ItemKey == itemDef.Key
	}
	if slot.ItemTag == "" {
		return false
	}
	for _, tag := range itemDef.Tags {
		if tag == slot.ItemTag {
			return true
		}
	}
	return false
}
