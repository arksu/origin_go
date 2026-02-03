package components

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// InventoryOwner represents an entity that owns a collection of inventories.
// Avoid map in ECS component: small slice is cheaper and deterministic.
type InventoryOwner struct {
	Inventories []InventoryLink
}

type InventoryLink struct {
	Kind constt.InventoryKind
	Key  uint32
	// Handle of entity that has InventoryContainer component
	Handle types.Handle
}

// InventoryContainer is a single component that represents any inventory kind:
// - Grid (2D placement)
// - Hand (single item)
// - Equipment (slot-based)
// - DroppedItem (single item in world)
type InventoryContainer struct {
	OwnerEntityID types.EntityID
	Kind          constt.InventoryKind
	Key           uint32
	Version       uint64

	// Only for Kind=InventoryGrid. For other kinds must be 0.
	Width  uint8
	Height uint8

	Items []InvItem

	// Optional occupancy cache for Grid:
	// len = int(Width)*int(Height), 0 = free, 1 = occupied
	// For non-grid kinds must be nil.
	Occupied []uint8
}

type InvItem struct {
	// item instance uid (proto.ItemInstance.item_id / InventoryMoveSpec.item_id)
	ItemID uint64

	// item def id (proto.ItemInstance.type_id)
	TypeID uint32

	Resource string

	Quality  uint32
	Quantity uint32

	// size in slots (no rotation); fits max 20x20 so uint8 is enough
	W uint8
	H uint8

	// Placement inside container (interpretation depends on InventoryContainer.Kind):
	// - Grid: X/Y are top-left coordinates, EquipSlot ignored
	// - Equipment: EquipSlot used, X/Y ignored
	// - Hand/Dropped: X/Y/EquipSlot ignored, len(Items) must be <= 1
	X         uint8
	Y         uint8
	EquipSlot netproto.EquipSlot
}

// Component IDs for inventory system
const (
	InventoryOwnerComponentID     = 19
	InventoryContainerComponentID = 20
)

func init() {
	ecs.RegisterComponent[InventoryOwner](InventoryOwnerComponentID)
	ecs.RegisterComponent[InventoryContainer](InventoryContainerComponentID)
}
