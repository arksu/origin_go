package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func resolveLinkedBuild(w *ecs.World, playerID types.EntityID, ref *netproto.InventoryRef) (types.Handle, *components.BuildBehaviorState, *ValidationError) {
	if ref == nil || ref.Kind != netproto.InventoryKind_INVENTORY_KIND_BUILD || ref.OwnerId == 0 || ref.InventoryKey != 0 {
		return types.InvalidHandle, nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid build inventory reference")
	}
	targetID := types.EntityID(ref.OwnerId)
	if ecs.ObjectDestructionOwnerPending(w, targetID) || ecs.InventoryOwnerReserved(w, targetID) {
		return types.InvalidHandle, nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, "Build site is unavailable")
	}
	target := w.GetHandleByEntityID(targetID)
	if target == types.InvalidHandle || !w.Alive(target) {
		return types.InvalidHandle, nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND, "Build target not found")
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, target)
	if !ok || info.TypeID != constt.BuildObjectTypeID {
		return types.InvalidHandle, nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Target is not a build site")
	}
	link, linked := ecs.GetResource[ecs.LinkState](w).GetLink(playerID)
	if !linked || link.TargetID != targetID {
		return types.InvalidHandle, nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, "Player is not linked to this construction site")
	}
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, target)
	if !ok {
		return types.InvalidHandle, nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Build state is unavailable")
	}
	build, ok := components.GetBehaviorState[components.BuildBehaviorState](state, buildBehaviorStateKey)
	if !ok || build == nil {
		return types.InvalidHandle, nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Build state is unavailable")
	}
	return target, build, nil
}

type buildTakeSource struct {
	target     types.Handle
	slotIndex  int
	stackIndex int
	definition *itemdefs.ItemDef
	item       components.InvItem
}

func (s *InventoryOperationService) resolveBuildTakeSource(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, move *netproto.InventoryMoveSpec) (*buildTakeSource, *ValidationError) {
	if move.BuildSlot == nil || move.ItemId != 0 {
		return nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Build take-back requires a slot instead of an item ID")
	}
	if ecs.HasComponent[components.LiftCarryState](w, playerHandle) {
		return nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, "LIFT_ACTION_LOCKED")
	}
	target, build, err := resolveLinkedBuild(w, playerID, move.Src)
	if err != nil {
		return nil, err
	}
	if uint64(*move.BuildSlot) >= uint64(len(build.Items)) {
		return nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid build requirement slot")
	}
	slotIndex := int(*move.BuildSlot)
	stacks := build.Items[slotIndex].PutItems
	stackIndex := len(stacks) - 1
	for stackIndex >= 0 && stacks[stackIndex].Count == 0 {
		stackIndex--
	}
	if stackIndex < 0 {
		return nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Build requirement has no deposited items")
	}
	registry := itemdefs.Global()
	if registry == nil || s.idAllocator == nil {
		return nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Build take-back inventory is unavailable")
	}
	stack := stacks[stackIndex]
	definition, found := registry.GetByKey(stack.ItemKey)
	if !found || definition == nil {
		return nil, NewValidationError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Item definition not found")
	}
	return &buildTakeSource{target: target, slotIndex: slotIndex, stackIndex: stackIndex, definition: definition,
		item: components.InvItem{TypeID: uint32(definition.DefID), Resource: definition.ResolveResource(false),
			Quality: stack.Quality, Quantity: 1, W: uint8(definition.Size.W), H: uint8(definition.Size.H)}}, nil
}

func (s *InventoryOperationService) executeMoveFromBuild(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, move *netproto.InventoryMoveSpec, expected []*netproto.InventoryExpected) *OperationResult {
	if move.Dst.Kind != netproto.InventoryKind_INVENTORY_KIND_GRID || move.DstEquipSlot != nil ||
		move.HandPos != nil || move.Quantity != nil || move.AllowSwapOrMerge {
		return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Build take-back requires grid placement without swap or merge"}
	}
	source, err := s.resolveBuildTakeSource(w, playerID, playerHandle, move)
	if err != nil {
		return &OperationResult{ErrorCode: err.Code, Message: err.Message}
	}
	destination, err := s.validator.ResolveContainer(w, move.Dst, playerID, playerHandle)
	if err != nil {
		return &OperationResult{ErrorCode: err.Code, Message: err.Message}
	}
	containers := map[string]*ContainerInfo{MakeContainerKeyFromInfo(destination.Container.OwnerID, destination.Container.Kind, destination.Container.Key): destination}
	if err := s.validator.ValidateExpectedVersions(w, expected, containers); err != nil {
		return &OperationResult{ErrorCode: err.Code, Message: err.Message}
	}
	if err := s.validator.ValidateItemAllowedInContainer(w, &source.item, destination, netproto.EquipSlot_EQUIP_SLOT_NONE); err != nil {
		return &OperationResult{ErrorCode: err.Code, Message: err.Message}
	}
	var x, y uint8
	if move.DstPos != nil {
		placement := s.placementService.CheckGridPlacement(destination.Container, &source.item, move.DstPos.X, move.DstPos.Y, false)
		if !placement.Success {
			return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL, Message: "Cannot place item at destination"}
		}
		x, y = placement.X, placement.Y
	} else {
		found, freeX, freeY := s.placementService.FindFreeSpace(destination.Container, source.item.W, source.item.H)
		if !found {
			return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL, Message: "No free space in destination"}
		}
		x, y = freeX, freeY
	}
	source.item.ItemID = s.idAllocator.GetFreeID()
	source.item.X, source.item.Y = x, y
	// Commit both sides before component observers or executor hooks can run.
	committed := ecs.MutateComponent[components.InventoryContainer](w, destination.Handle, func(container *components.InventoryContainer) bool {
		if container.Version != destination.Container.Version {
			return false
		}
		return ecs.MutateComponent[components.ObjectInternalState](w, source.target, func(state *components.ObjectInternalState) bool {
			build, ok := components.GetBehaviorState[components.BuildBehaviorState](*state, buildBehaviorStateKey)
			if !ok || build == nil || source.slotIndex >= len(build.Items) {
				return false
			}
			slot := &build.Items[source.slotIndex]
			if source.stackIndex >= len(slot.PutItems) {
				return false
			}
			stack := &slot.PutItems[source.stackIndex]
			if stack.Count == 0 || stack.ItemKey != source.definition.Key || stack.Quality != source.item.Quality {
				return false
			}
			container.Items = append(container.Items, source.item)
			container.Version++
			stack.Count--
			if stack.Count == 0 {
				slot.PutItems = append(slot.PutItems[:source.stackIndex], slot.PutItems[source.stackIndex+1:]...)
			}
			state.IsDirty = true
			return true
		})
	})
	if !committed {
		return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, Message: "Failed to take back build item"}
	}
	nested := ensureNestedContainer(w, playerHandle, &source.item, source.definition)
	if nested != 0 {
		reconcileNestedContainerOwnerLink(w, destination.Owner, playerHandle, source.item.ItemID, destination.Handle)
	}
	owner, _ := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	current, _ := ecs.GetComponent[components.InventoryContainer](w, destination.Handle)
	destination.Owner, destination.Container = &owner, &current
	result := &OperationResult{Success: true, UpdatedContainers: []*ContainerInfo{destination}, BuildTargetID: types.EntityID(move.Src.OwnerId)}
	if nested != 0 {
		child, _ := ecs.GetComponent[components.InventoryContainer](w, nested)
		result.UpdatedContainers = append(result.UpdatedContainers, &ContainerInfo{Handle: nested, Container: &child, Owner: &owner})
	}
	s.recordDiscoveryOnGive(w, playerHandle, source.definition)
	return result
}
