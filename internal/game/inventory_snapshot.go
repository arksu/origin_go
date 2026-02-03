package game

import (
	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func (g *Game) sendInventorySnapshots(c *network.Client, playerHandle types.Handle, world *ecs.World) {
	inventoryOwner, hasInventory := ecs.GetComponent[components.InventoryOwner](world, playerHandle)
	if !hasInventory {
		g.logger.Warn("Player has no InventoryOwner component",
			zap.Uint64("client_id", c.ID),
			zap.Int64("character_id", int64(c.CharacterID)),
		)
		return
	}

	inventoryStates := make([]*netproto.InventoryState, 0, len(inventoryOwner.Inventories))

	for _, link := range inventoryOwner.Inventories {
		container, hasContainer := ecs.GetComponent[components.InventoryContainer](world, link.Handle)
		if !hasContainer {
			g.logger.Warn("Container handle invalid",
				zap.Uint64("client_id", c.ID),
				zap.Uint8("kind", uint8(link.Kind)),
				zap.Uint32("key", link.Key),
			)
			continue
		}

		state := g.buildInventoryState(&container)
		if state != nil {
			inventoryStates = append(inventoryStates, state)
		}
	}

	if len(inventoryStates) == 0 {
		g.logger.Debug("No inventory states to send",
			zap.Uint64("client_id", c.ID),
			zap.Int64("character_id", int64(c.CharacterID)),
		)
		return
	}

	msg := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_InventoryUpdate{
			InventoryUpdate: &netproto.S2C_InventoryUpdate{
				Updated: inventoryStates,
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		g.logger.Error("Failed to marshal inventory update",
			zap.Uint64("client_id", c.ID),
			zap.Error(err),
		)
		return
	}

	c.Send(data)

	g.logger.Debug("Sent inventory snapshots to client",
		zap.Uint64("client_id", c.ID),
		zap.Int64("character_id", int64(c.CharacterID)),
		zap.Int("containers", len(inventoryStates)),
	)
}

func (g *Game) buildInventoryState(container *components.InventoryContainer) *netproto.InventoryState {
	state := &netproto.InventoryState{
		Ref: &netproto.InventoryRef{
			OwnerEntityId: uint64(container.OwnerEntityID),
			Kind:          g.convertInventoryKind(container.Kind),
			InventoryKey:  container.Key,
		},
		Revision: container.Version,
	}

	switch container.Kind {
	case _const.InventoryGrid:
		gridState := &netproto.InventoryGridState{
			Width:  uint32(container.Width),
			Height: uint32(container.Height),
			Items:  make([]*netproto.GridItem, 0, len(container.Items)),
		}

		for _, item := range container.Items {
			gridItem := &netproto.GridItem{
				Item: &netproto.ItemInstance{
					ItemId:   item.ItemID,
					TypeId:   item.TypeID,
					Resource: item.Resource,
					Quality:  item.Quality,
					Quantity: item.Quantity,
				},
				X: uint32(item.X),
				Y: uint32(item.Y),
			}
			gridState.Items = append(gridState.Items, gridItem)
		}

		state.State = &netproto.InventoryState_Grid{Grid: gridState}

	case _const.InventoryEquipment:
		equipState := &netproto.InventoryEquipmentState{
			Items: make([]*netproto.EquipmentItem, 0, len(container.Items)),
		}

		for _, item := range container.Items {
			equipItem := &netproto.EquipmentItem{
				Item: &netproto.ItemInstance{
					ItemId:   item.ItemID,
					TypeId:   item.TypeID,
					Resource: item.Resource,
					Quality:  item.Quality,
					Quantity: item.Quantity,
				},
				Slot: item.EquipSlot,
			}
			equipState.Items = append(equipState.Items, equipItem)
		}

		state.State = &netproto.InventoryState_Equipment{Equipment: equipState}

	case _const.InventoryHand:
		handState := &netproto.InventoryHandState{}

		if len(container.Items) > 0 {
			item := container.Items[0]
			handState.Item = &netproto.ItemInstance{
				ItemId:   item.ItemID,
				TypeId:   item.TypeID,
				Resource: item.Resource,
				Quality:  item.Quality,
				Quantity: item.Quantity,
			}
		}

		state.State = &netproto.InventoryState_Hand{Hand: handState}

	default:
		g.logger.Warn("Unknown inventory kind",
			zap.Uint8("kind", uint8(container.Kind)),
		)
		return nil
	}

	return state
}

func (g *Game) convertInventoryKind(kind _const.InventoryKind) netproto.InventoryKind {
	switch kind {
	case _const.InventoryGrid:
		return netproto.InventoryKind_INVENTORY_KIND_GRID
	case _const.InventoryHand:
		return netproto.InventoryKind_INVENTORY_KIND_HAND
	case _const.InventoryEquipment:
		return netproto.InventoryKind_INVENTORY_KIND_EQUIPMENT
	case _const.InventoryDroppedItem:
		return netproto.InventoryKind_INVENTORY_KIND_DROPPED_ITEM
	default:
		return netproto.InventoryKind_INVENTORY_KIND_GRID
	}
}
