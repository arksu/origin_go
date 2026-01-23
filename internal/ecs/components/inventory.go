package components

import (
	constt "origin/internal/const"
	"origin/internal/types"
)

// InventoryOwner represents an entity that owns a collection of inventories, identified by InventoryType and mapped to runtime handles via InventoryHandles.
type InventoryOwner struct {
	InventoryHandles map[constt.InventoryType]types.Handle
}

// Inventory represents a 2D container with a defined width and height that holds a collection of inventory items.
type Inventory struct {
	Width  int
	Height int
	Items  []InventoryItem
}

type InventoryItem struct {
	EntityID types.EntityID
	TypeID   types.ObjectType
	X, Y     int
	W, H     int
	Quality  int
	Count    int
}
