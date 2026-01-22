package components

import (
	constt "origin/internal/const"
	"origin/internal/types"
)

type InventoryOwner struct {
	InventoryHandles map[constt.InventoryType]types.Handle
}

type Inventory struct {
	Width  int
	Height int
	Items  []InvItemPlacement
}

type InvItemPlacement struct {
	EntityID types.EntityID
	TypeID   types.ObjectType
	X, Y     int
	W, H     int
	Quality  int
	Count    int
}
