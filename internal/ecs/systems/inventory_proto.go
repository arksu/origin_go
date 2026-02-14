package systems

import (
	"fmt"
	constt "origin/internal/const"
	netproto "origin/internal/network/proto"
	"strings"
)

// BuildInventoryStateProto converts an InventoryContainerState to a proto InventoryState.
// Shared by NetworkCommandSystem and AutoInteractSystem.
func BuildInventoryStateProto(container InventoryContainerState) *netproto.InventoryState {
	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind(container.Kind),
		OwnerId:      uint64(container.OwnerID),
		InventoryKey: container.Key,
	}

	invState := &netproto.InventoryState{
		Ref:      ref,
		Revision: container.Version,
	}

	switch constt.InventoryKind(container.Kind) {
	case constt.InventoryGrid:
		title := strings.TrimSpace(container.Title)
		if title == "" {
			panic(fmt.Sprintf("build inventory state: empty grid title (ownerID=%d key=%d)", container.OwnerID, container.Key))
		}
		invState.Title = title
		gridItems := make([]*netproto.GridItem, 0, len(container.Items))
		for _, item := range container.Items {
			gridItems = append(gridItems, &netproto.GridItem{
				X:    uint32(item.X),
				Y:    uint32(item.Y),
				Item: BuildItemInstanceProto(item),
			})
		}
		invState.State = &netproto.InventoryState_Grid{
			Grid: &netproto.InventoryGridState{
				Width:  uint32(container.Width),
				Height: uint32(container.Height),
				Items:  gridItems,
			},
		}

	case constt.InventoryHand:
		handState := &netproto.InventoryHandState{}
		if len(container.Items) > 0 {
			handState.Item = BuildItemInstanceProto(container.Items[0])
			handState.HandPos = &netproto.HandPos{
				MouseOffsetX: int32(container.HandMouseOffsetX),
				MouseOffsetY: int32(container.HandMouseOffsetY),
			}
		}
		invState.State = &netproto.InventoryState_Hand{
			Hand: handState,
		}

	case constt.InventoryEquipment:
		equipItems := make([]*netproto.EquipmentItem, 0, len(container.Items))
		for _, item := range container.Items {
			equipItems = append(equipItems, &netproto.EquipmentItem{
				Slot: item.EquipSlot,
				Item: BuildItemInstanceProto(item),
			})
		}
		invState.State = &netproto.InventoryState_Equipment{
			Equipment: &netproto.InventoryEquipmentState{
				Items: equipItems,
			},
		}
	}

	return invState
}

// BuildItemInstanceProto creates a proto ItemInstance from InventoryItemState.
func BuildItemInstanceProto(item InventoryItemState) *netproto.ItemInstance {
	instance := &netproto.ItemInstance{
		ItemId:   uint64(item.ItemID),
		TypeId:   item.TypeID,
		Resource: item.Resource,
		Quality:  item.Quality,
		Quantity: item.Quantity,
		W:        uint32(item.W),
		H:        uint32(item.H),
	}
	if item.NestedRef != nil {
		instance.NestedRef = item.NestedRef
	}
	return instance
}
