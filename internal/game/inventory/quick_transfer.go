package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const (
	inventoryModifierShift = 1
	maxQuickTransferRefs   = 64
)

func (s *InventoryOperationService) executeQuickTransfer(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	opID uint64,
	move *netproto.InventoryMoveSpec,
	expected []*netproto.InventoryExpected,
) *OperationResult {
	if move.Src == nil || move.Dst != nil || move.DstPos != nil || move.DstEquipSlot != nil ||
		move.HandPos != nil || move.Quantity != nil || move.AllowSwapOrMerge || len(move.OpenInventories) > maxQuickTransferRefs {
		return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Invalid quick transfer request"}
	}
	sourceKey := ecs.InventoryRefKey{Kind: constt.InventoryKind(move.Src.Kind), OwnerID: types.EntityID(move.Src.OwnerId), Key: move.Src.InventoryKey}
	var refs [maxQuickTransferRefs]ecs.InventoryRefKey
	sourceIndex := -1
	for i, ref := range move.OpenInventories {
		if ref == nil || ref.OwnerId == 0 || (ref.Kind != netproto.InventoryKind_INVENTORY_KIND_GRID && ref.Kind != netproto.InventoryKind_INVENTORY_KIND_BUILD) {
			return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Invalid open inventory reference"}
		}
		key := ecs.InventoryRefKey{Kind: constt.InventoryKind(ref.Kind), OwnerID: types.EntityID(ref.OwnerId), Key: ref.InventoryKey}
		for _, previous := range refs[:i] {
			if previous == key {
				return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Duplicate open inventory reference"}
			}
		}
		refs[i] = key
		if key == sourceKey {
			sourceIndex = i
		}
	}
	if sourceIndex < 0 {
		return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Source inventory is not open"}
	}

	// Source failures end the operation before any candidate is considered.
	// Candidate admission then uses current server state under the same shard lock.
	switch move.Src.Kind {
	case netproto.InventoryKind_INVENTORY_KIND_GRID:
		if move.BuildSlot != nil {
			return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Build slot requires a build source"}
		}
		source, err := s.validator.ResolveContainer(w, move.Src, playerID, playerHandle)
		if err != nil {
			return &OperationResult{ErrorCode: err.Code, Message: err.Message}
		}
		containers := map[string]*ContainerInfo{MakeContainerKeyFromInfo(source.Container.OwnerID, source.Container.Kind, source.Container.Key): source}
		if err := s.validator.ValidateExpectedVersions(w, expected, containers); err != nil {
			return &OperationResult{ErrorCode: err.Code, Message: err.Message}
		}
		_, item := s.validator.FindItemInContainer(source.Container, types.EntityID(move.ItemId))
		if item == nil {
			return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND, Message: "Item not found in source container"}
		}
		if item.Quantity == 0 {
			return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Source item quantity is empty"}
		}
	case netproto.InventoryKind_INVENTORY_KIND_BUILD:
		if len(expected) != 0 {
			return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Build source has no inventory revision"}
		}
		if _, err := s.resolveBuildTakeSource(w, playerID, playerHandle, move); err != nil {
			return &OperationResult{ErrorCode: err.Code, Message: err.Message}
		}
	default:
		return &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, Message: "Unsupported quick transfer source"}
	}

	candidateMove := &netproto.InventoryMoveSpec{Src: move.Src, ItemId: move.ItemId, BuildSlot: move.BuildSlot}
	lastFailure := &OperationResult{ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL, Message: "No open inventory can accept this item"}
	for offset := 1; offset < len(move.OpenInventories); offset++ {
		index := (sourceIndex + offset) % len(move.OpenInventories)
		candidate := move.OpenInventories[index]
		// A container item cannot be moved into its own open inventory.
		if move.Src.Kind == netproto.InventoryKind_INVENTORY_KIND_GRID && candidate.Kind == netproto.InventoryKind_INVENTORY_KIND_GRID &&
			candidate.OwnerId == move.ItemId && candidate.InventoryKey == 0 {
			continue
		}
		if move.Src.Kind == netproto.InventoryKind_INVENTORY_KIND_BUILD && candidate.Kind == netproto.InventoryKind_INVENTORY_KIND_BUILD {
			continue
		}
		candidateMove.Dst = candidate
		result := s.ExecuteMove(w, playerID, playerHandle, opID, candidateMove, nil)
		if result.Success {
			return result
		}
		switch result.ErrorCode {
		case netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL,
			netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND, netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT:
			lastFailure = result
		default:
			return result
		}
	}
	return lastFailure
}
