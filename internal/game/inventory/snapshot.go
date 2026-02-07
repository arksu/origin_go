package inventory

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type SnapshotSender struct {
	logger *zap.Logger
}

func NewSnapshotSender(logger *zap.Logger) *SnapshotSender {
	return &SnapshotSender{
		logger: logger,
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

		// Only send root-level inventories (owned by character), not nested ones
		if container.OwnerID != characterID {
			continue
		}

		invState := ss.buildInventoryState(world, container)
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
	container components.InventoryContainer,
) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(container.Kind),
		OwnerId:      uint64(container.OwnerID),
		InventoryKey: container.Key,
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: container.Version,
	}

	switch container.Kind {
	case constt.InventoryGrid:
		invState.State = &netproto.InventoryState_Grid{
			Grid: ss.buildGridState(world, container),
		}
	case constt.InventoryEquipment:
		invState.State = &netproto.InventoryState_Equipment{
			Equipment: ss.buildEquipmentState(world, container),
		}
	case constt.InventoryHand:
		invState.State = &netproto.InventoryState_Hand{
			Hand: ss.buildHandState(world, container),
		}
	}

	return invState
}

func (ss *SnapshotSender) buildGridState(
	world *ecs.World,
	container components.InventoryContainer,
) *netproto.InventoryGridState {
	items := make([]*netproto.GridItem, 0, len(container.Items))

	for _, invItem := range container.Items {
		itemInstance := ss.buildItemInstance(world, invItem)
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
) *netproto.InventoryEquipmentState {
	items := make([]*netproto.EquipmentItem, 0, len(container.Items))

	for _, invItem := range container.Items {
		itemInstance := ss.buildItemInstance(world, invItem)
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
) *netproto.InventoryHandState {
	handState := &netproto.InventoryHandState{}

	if len(container.Items) > 0 {
		itemInstance := ss.buildItemInstance(world, container.Items[0])
		handState.Item = itemInstance
		handState.HandPos = &netproto.HandPos{
			MouseOffsetX: int32(container.HandMouseOffsetX),
			MouseOffsetY: int32(container.HandMouseOffsetY),
		}
	}

	return handState
}

func (ss *SnapshotSender) buildItemInstance(
	world *ecs.World,
	invItem components.InvItem,
) *netproto.ItemInstance {
	itemInstance := &netproto.ItemInstance{
		ItemId:   uint64(invItem.ItemID),
		TypeId:   invItem.TypeID,
		Resource: invItem.Resource,
		Quality:  invItem.Quality,
		Quantity: invItem.Quantity,
		W:        uint32(invItem.W),
		H:        uint32(invItem.H),
	}

	// Check if this item has a nested container via index (O(1))
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	if _, found := refIndex.Lookup(constt.InventoryGrid, invItem.ItemID, 0); found {
		itemInstance.NestedRef = &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(invItem.ItemID),
			InventoryKey: 0,
		}
	}

	return itemInstance
}
