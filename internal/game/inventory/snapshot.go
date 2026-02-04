package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type SnapshotSender struct {
	logger       *zap.Logger
	itemRegistry *itemdefs.Registry
}

func NewSnapshotSender(logger *zap.Logger, itemRegistry *itemdefs.Registry) *SnapshotSender {
	return &SnapshotSender{
		logger:       logger,
		itemRegistry: itemRegistry,
	}
}

func (ss *SnapshotSender) SendInventorySnapshots(
	w interface{},
	client *network.Client,
	characterID types.EntityID,
	handle types.Handle,
) {
	world := w.(*ecs.World)

	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](world, handle)
	if !hasOwner {
		ss.logger.Warn("Character has no InventoryOwner component",
			zap.Uint64("character_id", uint64(characterID)))
		return
	}

	inventoryStates := make([]*netproto.InventoryState, 0, len(owner.Inventories))

	for _, link := range owner.Inventories {
		if !world.Alive(link.Handle) {
			continue
		}

		container, hasContainer := ecs.GetComponent[components.InventoryContainer](world, link.Handle)
		if !hasContainer {
			continue
		}

		if container.OwnerID != characterID {
			continue
		}

		invState := ss.buildInventoryState(world, characterID, container, &owner)
		if invState != nil {
			inventoryStates = append(inventoryStates, invState)
		}
	}

	if len(inventoryStates) == 0 {
		ss.logger.Debug("No root inventories to send",
			zap.Uint64("character_id", uint64(characterID)))
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_InventoryUpdate{
			InventoryUpdate: &netproto.S2C_InventoryUpdate{
				Updated: inventoryStates,
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		ss.logger.Error("Failed to marshal inventory update",
			zap.Uint64("character_id", uint64(characterID)),
			zap.Error(err))
		return
	}

	client.Send(data)

	ss.logger.Debug("Sent inventory snapshots",
		zap.Uint64("character_id", uint64(characterID)),
		zap.Int("count", len(inventoryStates)))
}

func (ss *SnapshotSender) buildInventoryState(
	world *ecs.World,
	characterID types.EntityID,
	container components.InventoryContainer,
	owner *components.InventoryOwner,
) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(container.Kind),
		InventoryKey: container.Key,
	}

	ref.Owner = &netproto.InventoryRef_OwnerEntityId{
		OwnerEntityId: uint64(characterID),
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: container.Version,
	}

	switch container.Kind {
	case constt.InventoryGrid:
		invState.State = &netproto.InventoryState_Grid{
			Grid: ss.buildGridState(world, container, owner),
		}
	case constt.InventoryEquipment:
		invState.State = &netproto.InventoryState_Equipment{
			Equipment: ss.buildEquipmentState(world, container, owner),
		}
	case constt.InventoryHand:
		invState.State = &netproto.InventoryState_Hand{
			Hand: ss.buildHandState(world, container, owner),
		}
	}

	return invState
}

func (ss *SnapshotSender) buildGridState(
	world *ecs.World,
	container components.InventoryContainer,
	owner *components.InventoryOwner,
) *netproto.InventoryGridState {
	items := make([]*netproto.GridItem, 0, len(container.Items))

	for _, invItem := range container.Items {
		itemInstance := ss.buildItemInstance(world, invItem, owner)
		gridItem := &netproto.GridItem{
			X:    uint32(invItem.X),
			Y:    uint32(invItem.Y),
			Item: itemInstance,
		}
		items = append(items, gridItem)
	}

	return &netproto.InventoryGridState{
		Width:  uint32(container.Width),
		Height: uint32(container.Height),
		Items:  items,
	}
}

func (ss *SnapshotSender) buildEquipmentState(
	world *ecs.World,
	container components.InventoryContainer,
	owner *components.InventoryOwner,
) *netproto.InventoryEquipmentState {
	items := make([]*netproto.EquipmentItem, 0, len(container.Items))

	for _, invItem := range container.Items {
		itemInstance := ss.buildItemInstance(world, invItem, owner)
		equipItem := &netproto.EquipmentItem{
			Slot: invItem.EquipSlot,
			Item: itemInstance,
		}
		items = append(items, equipItem)
	}

	return &netproto.InventoryEquipmentState{
		Items: items,
	}
}

func (ss *SnapshotSender) buildHandState(
	world *ecs.World,
	container components.InventoryContainer,
	owner *components.InventoryOwner,
) *netproto.InventoryHandState {
	handState := &netproto.InventoryHandState{}

	if len(container.Items) > 0 {
		itemInstance := ss.buildItemInstance(world, container.Items[0], owner)
		handState.Item = itemInstance
	}

	return handState
}

func (ss *SnapshotSender) buildItemInstance(
	world *ecs.World,
	invItem components.InvItem,
	owner *components.InventoryOwner,
) *netproto.ItemInstance {
	nestedContainer := ss.findNestedInventory(world, invItem.ItemID, owner)
	hasNestedItems := nestedContainer != nil && len(nestedContainer.Items) > 0

	// Recompute resource based on current nested inventory state
	resource := invItem.Resource
	if itemDef, ok := ss.itemRegistry.GetByID(int(invItem.TypeID)); ok {
		resource = itemDef.ResolveResource(hasNestedItems)
	}

	itemInstance := &netproto.ItemInstance{
		ItemId:   invItem.ItemID,
		TypeId:   invItem.TypeID,
		Resource: resource,
		Quality:  invItem.Quality,
		Quantity: invItem.Quantity,
		W:        uint32(invItem.W),
		H:        uint32(invItem.H),
	}

	if nestedContainer != nil {
		nestedGridState := ss.buildNestedGridState(world, *nestedContainer, owner)
		itemInstance.NestedInventory = nestedGridState
	}

	return itemInstance
}

func (ss *SnapshotSender) buildNestedGridState(
	world *ecs.World,
	container components.InventoryContainer,
	owner *components.InventoryOwner,
) *netproto.InventoryGridState {
	items := make([]*netproto.GridItem, 0, len(container.Items))

	for _, invItem := range container.Items {
		itemInstance := ss.buildItemInstance(world, invItem, owner)
		gridItem := &netproto.GridItem{
			X:    uint32(invItem.X),
			Y:    uint32(invItem.Y),
			Item: itemInstance,
		}
		items = append(items, gridItem)
	}

	return &netproto.InventoryGridState{
		Width:  uint32(container.Width),
		Height: uint32(container.Height),
		Items:  items,
	}
}

func (ss *SnapshotSender) findNestedInventory(
	world *ecs.World,
	itemID uint64,
	owner *components.InventoryOwner,
) *components.InventoryContainer {
	// Use InventoryOwner links instead of full world query
	for _, link := range owner.Inventories {
		if !world.Alive(link.Handle) {
			continue
		}
		container, ok := ecs.GetComponent[components.InventoryContainer](world, link.Handle)
		if ok && container.OwnerID == types.EntityID(itemID) {
			return &container
		}
	}
	return nil
}
