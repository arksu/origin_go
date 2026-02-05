package inventory

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

// InventoryExecutor implements systems.InventoryOperationExecutor interface
type InventoryExecutor struct {
	service *InventoryOperationService
	logger  *zap.Logger
}

func NewInventoryExecutor(logger *zap.Logger) *InventoryExecutor {
	return &InventoryExecutor{
		service: NewInventoryOperationService(logger),
		logger:  logger,
	}
}

func (e *InventoryExecutor) ExecuteOperation(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	op *netproto.InventoryOp,
) systems.InventoryOpResult {
	result := e.service.ExecuteOperation(w, playerID, playerHandle, op)

	// Convert internal result to systems.InventoryOpResult
	opResult := systems.InventoryOpResult{
		Success:           result.Success,
		ErrorCode:         result.ErrorCode,
		Message:           result.Message,
		UpdatedContainers: make([]systems.InventoryContainerState, 0, len(result.UpdatedContainers)),
	}

	// Convert updated containers
	for _, container := range result.UpdatedContainers {
		containerState := e.convertContainerToState(w, container)
		opResult.UpdatedContainers = append(opResult.UpdatedContainers, containerState)
	}

	return opResult
}

func (e *InventoryExecutor) convertContainerToState(w *ecs.World, info *ContainerInfo) systems.InventoryContainerState {
	state := systems.InventoryContainerState{
		OwnerID: info.Container.OwnerID,
		Kind:    uint8(info.Container.Kind),
		Key:     info.Container.Key,
		Version: info.Container.Version,
		Width:   info.Container.Width,
		Height:  info.Container.Height,
		Items:   make([]systems.InventoryItemState, 0, len(info.Container.Items)),
	}

	for _, item := range info.Container.Items {
		state.Items = append(state.Items, e.convertItemToState(w, info, item))
	}

	return state
}

func (e *InventoryExecutor) convertItemToState(w *ecs.World, info *ContainerInfo, item components.InvItem) systems.InventoryItemState {
	itemState := systems.InventoryItemState{
		ItemID:    item.ItemID,
		TypeID:    item.TypeID,
		Resource:  item.Resource,
		Quality:   item.Quality,
		Quantity:  item.Quantity,
		W:         item.W,
		H:         item.H,
		X:         item.X,
		Y:         item.Y,
		EquipSlot: item.EquipSlot,
	}

	// Look for nested inventory owned by this item using Owner from ContainerInfo
	nestedContainer := e.findNestedInventory(w, info.Owner, item.ItemID)
	if nestedContainer != nil {
		nestedState := e.convertContainerToState(w, &ContainerInfo{
			Container: nestedContainer,
			Owner:     info.Owner,
		})
		itemState.NestedInventory = &nestedState
	}

	return itemState
}

// findNestedInventory searches for an inventory container owned by the given itemID
func (e *InventoryExecutor) findNestedInventory(w *ecs.World, owner *components.InventoryOwner, itemID types.EntityID) *components.InventoryContainer {
	if owner == nil {
		return nil
	}

	// Search for inventory where OwnerID matches the itemID
	for _, link := range owner.Inventories {
		container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !hasContainer {
			continue
		}
		if container.OwnerID == itemID {
			return &container
		}
	}

	return nil
}
