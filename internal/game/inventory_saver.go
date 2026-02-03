package game

import (
	"database/sql"
	"encoding/json"
	_const "origin/internal/const"
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
	world interface{},
	characterID types.EntityID,
	handle types.Handle,
) []systems.InventorySnapshot {
	w, ok := world.(*ecs.World)
	if !ok {
		is.logger.Error("Invalid world type passed to SerializeInventories")
		return nil
	}
	inventoryOwner, hasInventory := ecs.GetComponent[components.InventoryOwner](w, handle)
	if !hasInventory {
		is.logger.Debug("Character has no InventoryOwner component",
			zap.Int64("character_id", int64(characterID)))
		return nil
	}

	snapshots := make([]systems.InventorySnapshot, 0, len(inventoryOwner.Inventories))

	for _, link := range inventoryOwner.Inventories {
		container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
		if !hasContainer {
			is.logger.Warn("Invalid container handle in InventoryOwner",
				zap.Int64("character_id", int64(characterID)),
				zap.Uint8("kind", uint8(link.Kind)),
				zap.Uint32("key", link.Key))
			continue
		}

		data := is.buildInventoryData(&container)
		jsonData, err := json.Marshal(data)
		if err != nil {
			is.logger.Error("Failed to marshal inventory data",
				zap.Int64("character_id", int64(characterID)),
				zap.Uint8("kind", uint8(container.Kind)),
				zap.Uint32("key", container.Key),
				zap.Error(err))
			continue
		}

		snapshot := systems.InventorySnapshot{
			CharacterID:  int64(characterID),
			Kind:         int16(container.Kind),
			InventoryKey: int16(container.Key),
			Data:         jsonData,
			Version:      int(container.Version + 1),
		}

		if container.Kind == _const.InventoryGrid {
			snapshot.Width = sql.NullInt16{Int16: int16(container.Width), Valid: true}
			snapshot.Height = sql.NullInt16{Int16: int16(container.Height), Valid: true}
		}

		snapshots = append(snapshots, snapshot)
	}

	return snapshots
}

func (is *InventorySaver) buildInventoryData(container *components.InventoryContainer) InventoryDataV1 {
	items := make([]InventoryItemV1, len(container.Items))

	for i, item := range container.Items {
		items[i] = InventoryItemV1{
			ItemID:   item.ItemID,
			TypeID:   item.TypeID,
			Quality:  item.Quality,
			Quantity: item.Quantity,
			X:        item.X,
			Y:        item.Y,
		}

		if container.Kind == _const.InventoryEquipment {
			items[i].EquipSlot = is.convertEquipSlot(item.EquipSlot)
		}
	}

	return InventoryDataV1{
		Version: 1,
		Items:   items,
	}
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
