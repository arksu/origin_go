package inventory

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"

	"github.com/stretchr/testify/require"
)

func TestExecuteMoveExactGridPlacement(t *testing.T) {
	for _, sameGrid := range []bool{false, true} {
		name := "between grids"
		if sameGrid {
			name = "within grid overlapping original footprint"
		}
		t.Run(name, func(t *testing.T) {
			f := setupBuildDeposit(t)
			f.item.X, f.item.Y, f.item.W, f.item.H = 0, 0, 2, 2
			addItemToContainer(f.w, f.grid, f.item)
			addItemToContainer(f.w, f.hand, components.InvItem{ItemID: 6000, TypeID: 3, Quantity: 1, W: 1, H: 1})
			handBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
			destination := f.grid
			position := &netproto.GridPos{X: 1, Y: 1}
			if !sameGrid {
				destination = quickTestGrid(f, 1, 5, 5)
				position = &netproto.GridPos{X: 3, Y: 3}
			}
			move := &netproto.InventoryMoveSpec{Src: quickTestRef(f.w, f.grid), Dst: quickTestRef(f.w, destination),
				ItemId: uint64(f.item.ItemID), DstPos: position}
			expected := []*netproto.InventoryExpected{{Ref: move.Src, ExpectedRevision: 1}}
			if !sameGrid {
				expected = append(expected, &netproto.InventoryExpected{Ref: move.Dst, ExpectedRevision: 1})
			}
			result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
			require.True(t, result.Success, "result: %+v", result)
			after, _ := ecs.GetComponent[components.InventoryContainer](f.w, destination)
			moved := f.item
			moved.X, moved.Y = uint8(position.X), uint8(position.Y)
			require.Equal(t, []components.InvItem{moved}, after.Items)
			require.Equal(t, uint64(2), after.Version)
			if !sameGrid {
				source, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
				require.Empty(t, source.Items)
				require.Equal(t, uint64(2), source.Version)
			}
			handAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
			require.Equal(t, handBefore, handAfter)
		})
	}
}

func TestExecuteMoveExactGridRejectionsPreserveBothContainers(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		position netproto.GridPos
		edit     func(*netproto.InventoryMoveSpec, []*netproto.InventoryExpected)
		code     netproto.ErrorCode
	}{
		{name: "occupied stack cannot merge", position: netproto.GridPos{X: 2, Y: 2}},
		{name: "occupied other item cannot swap", position: netproto.GridPos{X: 0, Y: 3}},
		{name: "footprint beyond right edge", position: netproto.GridPos{X: 4, Y: 1}},
		{name: "footprint beyond bottom edge", position: netproto.GridPos{X: 1, Y: 4}},
		{name: "x cannot wrap to zero", position: netproto.GridPos{X: 256}},
		{name: "y cannot wrap to zero", position: netproto.GridPos{Y: 256}},
		{name: "maximum x", position: netproto.GridPos{X: ^uint32(0)}},
		{name: "maximum y", position: netproto.GridPos{Y: ^uint32(0)}},
		{name: "stale source revision", position: netproto.GridPos{X: 3, Y: 0}, code: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			edit: func(_ *netproto.InventoryMoveSpec, expected []*netproto.InventoryExpected) {
				expected[0].ExpectedRevision++
			}},
		{name: "stale destination revision", position: netproto.GridPos{X: 3, Y: 0}, code: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			edit: func(_ *netproto.InventoryMoveSpec, expected []*netproto.InventoryExpected) {
				expected[len(expected)-1].ExpectedRevision++
			}},
	} {
		for _, sameGrid := range []bool{false, true} {
			name := scenario.name + "/between grids"
			if sameGrid {
				name = scenario.name + "/within grid"
			}
			t.Run(name, func(t *testing.T) {
				f := setupBuildDeposit(t)
				f.item.X, f.item.Y, f.item.W, f.item.H = 0, 0, 2, 2
				addItemToContainer(f.w, f.grid, f.item)
				destination := f.grid
				if !sameGrid {
					destination = quickTestGrid(f, 1, 5, 5)
				}
				addItemToContainer(f.w, destination, components.InvItem{ItemID: 6000, TypeID: 1, Quantity: 95, W: 1, H: 1, X: 2, Y: 2})
				addItemToContainer(f.w, destination, components.InvItem{ItemID: 6001, TypeID: 3, Quantity: 1, W: 1, H: 1, X: 0, Y: 3})
				move := &netproto.InventoryMoveSpec{Src: quickTestRef(f.w, f.grid), Dst: quickTestRef(f.w, destination),
					ItemId: uint64(f.item.ItemID), DstPos: &scenario.position}
				expected := []*netproto.InventoryExpected{{Ref: move.Src, ExpectedRevision: 1}}
				if !sameGrid {
					expected = append(expected, &netproto.InventoryExpected{Ref: move.Dst, ExpectedRevision: 1})
				}
				if scenario.edit != nil {
					scenario.edit(move, expected)
				}
				sourceBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
				sourceBefore.Items = append([]components.InvItem(nil), sourceBefore.Items...)
				destinationBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, destination)
				destinationBefore.Items = append([]components.InvItem(nil), destinationBefore.Items...)
				result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
				require.False(t, result.Success)
				code := scenario.code
				if code == 0 {
					code = netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL
				}
				require.Equal(t, code, result.ErrorCode)
				require.Empty(t, result.UpdatedContainers)
				sourceAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
				destinationAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, destination)
				require.Equal(t, sourceBefore, sourceAfter)
				require.Equal(t, destinationBefore, destinationAfter)
			})
		}
	}
}

func TestExecuteMoveExactGridRevalidatesAccess(t *testing.T) {
	for _, foreignSource := range []bool{false, true} {
		name := "closed destination"
		if foreignSource {
			name = "closed source"
		}
		t.Run(name, func(t *testing.T) {
			f := setupBuildDeposit(t)
			foreign := createGridContainer(f.w, 7000, 0, 5, 5)
			ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, 7000, 0, foreign)
			source, destination := f.grid, foreign
			if foreignSource {
				source, destination = foreign, f.grid
			}
			addItemToContainer(f.w, source, f.item)
			move := &netproto.InventoryMoveSpec{Src: quickTestRef(f.w, source), Dst: quickTestRef(f.w, destination),
				ItemId: uint64(f.item.ItemID), DstPos: &netproto.GridPos{X: 1, Y: 1}}
			result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, nil)
			require.False(t, result.Success)
			require.Equal(t, netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, result.ErrorCode)
			sourceAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, source)
			destinationAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, destination)
			require.Equal(t, []components.InvItem{f.item}, sourceAfter.Items)
			require.Equal(t, uint64(1), sourceAfter.Version)
			require.Empty(t, destinationAfter.Items)
			require.Equal(t, uint64(1), destinationAfter.Version)
		})
	}
}
