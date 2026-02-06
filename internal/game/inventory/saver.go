package inventory

import (
	"encoding/json"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

type InventorySaver struct {
	logger *zap.Logger
}

func NewInventorySaver(logger *zap.Logger) *InventorySaver {
	return &InventorySaver{
		logger: logger,
	}
}

func (is *InventorySaver) SerializeInventories(
	w interface{},
	characterID types.EntityID,
	handle types.Handle,
) []systems.InventorySnapshot {
	world := w.(*ecs.World)
	result := make([]systems.InventorySnapshot, 0)

	owner, hasOwner := ecs.GetComponent[components.InventoryOwner](world, handle)
	if !hasOwner {
		return result
	}

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

		snapshot := is.serializeContainer(world, &owner, characterID, container)
		result = append(result, snapshot)
	}

	return result
}

func (is *InventorySaver) serializeContainer(
	world *ecs.World,
	owner *components.InventoryOwner,
	characterID types.EntityID,
	container components.InventoryContainer,
) systems.InventorySnapshot {
	items := make([]InventoryItemV1, 0, len(container.Items))

	for _, invItem := range container.Items {
		dbItem := InventoryItemV1{
			ItemID:    uint64(invItem.ItemID),
			TypeID:    invItem.TypeID,
			Quality:   invItem.Quality,
			Quantity:  invItem.Quantity,
			X:         invItem.X,
			Y:         invItem.Y,
			EquipSlot: is.convertEquipSlot(invItem.EquipSlot),
		}

		nestedContainer := is.findNestedInventory(world, owner, invItem.ItemID)
		if nestedContainer != nil {
			nestedData := is.serializeNestedInventory(world, owner, *nestedContainer)
			dbItem.NestedInventory = &nestedData
		}

		items = append(items, dbItem)
	}

	invData := InventoryDataV1{
		Kind:    uint8(container.Kind),
		Key:     container.Key,
		Width:   container.Width,
		Height:  container.Height,
		Version: int(container.Version),
		Items:   items,
	}

	data, err := json.Marshal(invData)
	if err != nil {
		is.logger.Error("Failed to marshal inventory data",
			zap.Uint64("character_id", uint64(characterID)),
			zap.Uint8("kind", uint8(container.Kind)),
			zap.Uint32("key", container.Key),
			zap.Error(err))
		data = []byte("{}")
	}

	return systems.InventorySnapshot{
		CharacterID:  int64(characterID),
		Kind:         int16(container.Kind),
		InventoryKey: int16(container.Key),
		Data:         data,
		Version:      int(container.Version),
	}
}

func (is *InventorySaver) serializeNestedInventory(
	world *ecs.World,
	owner *components.InventoryOwner,
	container components.InventoryContainer,
) InventoryDataV1 {
	items := make([]InventoryItemV1, 0, len(container.Items))

	for _, invItem := range container.Items {
		dbItem := InventoryItemV1{
			ItemID:    uint64(invItem.ItemID),
			TypeID:    invItem.TypeID,
			Quality:   invItem.Quality,
			Quantity:  invItem.Quantity,
			X:         invItem.X,
			Y:         invItem.Y,
			EquipSlot: is.convertEquipSlot(invItem.EquipSlot),
		}

		nestedContainer := is.findNestedInventory(world, owner, invItem.ItemID)
		if nestedContainer != nil {
			nestedData := is.serializeNestedInventory(world, owner, *nestedContainer)
			dbItem.NestedInventory = &nestedData
		}

		items = append(items, dbItem)
	}

	return InventoryDataV1{
		Kind:    uint8(container.Kind),
		Key:     container.Key,
		Width:   container.Width,
		Height:  container.Height,
		Version: int(container.Version),
		Items:   items,
	}
}

func (is *InventorySaver) findNestedInventory(
	world *ecs.World,
	owner *components.InventoryOwner,
	itemID types.EntityID,
) *components.InventoryContainer {
	if owner == nil {
		return nil
	}

	// Search for inventory where OwnerID matches the itemID using InventoryLink.OwnerID
	for _, link := range owner.Inventories {
		if link.OwnerID == itemID {
			container, hasContainer := ecs.GetComponent[components.InventoryContainer](world, link.Handle)
			if hasContainer {
				return &container
			}
		}
	}

	return nil
}

func (is *InventorySaver) convertEquipSlot(slot netproto.EquipSlot) string {
	switch slot {
	case netproto.EquipSlot_EQUIP_SLOT_HEAD:
		return "head"
	case netproto.EquipSlot_EQUIP_SLOT_CHEST:
		return "chest"
	case netproto.EquipSlot_EQUIP_SLOT_LEGS:
		return "legs"
	case netproto.EquipSlot_EQUIP_SLOT_FEET:
		return "feet"
	case netproto.EquipSlot_EQUIP_SLOT_HANDS:
		return "hands"
	case netproto.EquipSlot_EQUIP_SLOT_BACK:
		return "back"
	case netproto.EquipSlot_EQUIP_SLOT_NECK:
		return "neck"
	case netproto.EquipSlot_EQUIP_SLOT_RING_1:
		return "ring1"
	case netproto.EquipSlot_EQUIP_SLOT_RING_2:
		return "ring2"
	default:
		return ""
	}
}
