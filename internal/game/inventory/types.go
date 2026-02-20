package inventory

import (
	netproto "origin/internal/network/proto"
)

// Constants and types for inventory system

const (
	DefaultBackpackWidth  = 5
	DefaultBackpackHeight = 5
	LostAndFoundKey       = 9999
)

// EquipSlotToString converts a proto EquipSlot to its persistent string representation.
func EquipSlotToString(slot netproto.EquipSlot) string {
	switch slot {
	case netproto.EquipSlot_EQUIP_SLOT_HEAD:
		return "head"
	case netproto.EquipSlot_EQUIP_SLOT_CHEST:
		return "chest"
	case netproto.EquipSlot_EQUIP_SLOT_LEGS:
		return "legs"
	case netproto.EquipSlot_EQUIP_SLOT_FEET:
		return "feet"
	case netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND:
		return "left_hand"
	case netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND:
		return "right_hand"
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

// StringToEquipSlot converts a persistent string to a proto EquipSlot.
func StringToEquipSlot(slot string) netproto.EquipSlot {
	switch slot {
	case "head":
		return netproto.EquipSlot_EQUIP_SLOT_HEAD
	case "chest":
		return netproto.EquipSlot_EQUIP_SLOT_CHEST
	case "legs":
		return netproto.EquipSlot_EQUIP_SLOT_LEGS
	case "feet":
		return netproto.EquipSlot_EQUIP_SLOT_FEET
	case "left_hand":
		return netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND
	case "right_hand":
		return netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND
	case "back":
		return netproto.EquipSlot_EQUIP_SLOT_BACK
	case "neck":
		return netproto.EquipSlot_EQUIP_SLOT_NECK
	case "ring1":
		return netproto.EquipSlot_EQUIP_SLOT_RING_1
	case "ring2":
		return netproto.EquipSlot_EQUIP_SLOT_RING_2
	default:
		return netproto.EquipSlot_EQUIP_SLOT_NONE
	}
}

type InventoryDataV1 struct {
	Kind    uint8             `json:"kind"`
	Key     uint32            `json:"key"`
	Width   uint8             `json:"width,omitempty"`
	Height  uint8             `json:"height,omitempty"`
	Version int               `json:"v"`
	Items   []InventoryItemV1 `json:"items"`
}

type InventoryItemV1 struct {
	ItemID          uint64           `json:"item_id"`
	TypeID          uint32           `json:"type_id"`
	Quality         uint32           `json:"quality"`
	Quantity        uint32           `json:"quantity"`
	X               uint8            `json:"x,omitempty"`
	Y               uint8            `json:"y,omitempty"`
	EquipSlot       string           `json:"equip_slot,omitempty"`
	NestedInventory *InventoryDataV1 `json:"nested_inventory,omitempty"`
}
