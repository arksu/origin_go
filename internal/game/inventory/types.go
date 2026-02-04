package inventory

// Constants and types for inventory system

const (
	DefaultBackpackWidth  = 10
	DefaultBackpackHeight = 10
	LostAndFoundKey       = 9999
)

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
