package inventory

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildTakeBackExactPlacementCommitsOneUnitWithOccupiedHand(t *testing.T) {
	f := setupBuildDeposit(t)
	f.build.Items[0].PutItems = []components.BuildPutItemState{{ItemKey: "branch", Quality: 7, Count: 2}, {ItemKey: "branch", Quality: 31, Count: 1}}
	addItemToContainer(f.w, f.hand, components.InvItem{ItemID: 6000, TypeID: 3, Quantity: 1, W: 1, H: 1})
	handBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
	ecs.AddComponent(f.w, f.player, components.CharacterProfile{})
	slot := uint32(0)
	move := &netproto.InventoryMoveSpec{Src: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)},
		BuildSlot: &slot, Dst: quickTestRef(f.w, f.grid), DstPos: &netproto.GridPos{X: 3, Y: 4}}
	observed := 0
	check := func(handle types.Handle) {
		if handle != f.target && handle != f.grid {
			return
		}
		observed++
		grid, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
		require.Len(t, grid.Items, 1)
		require.Equal(t, uint32(31), grid.Items[0].Quality)
		require.Equal(t, uint32(2), f.build.Items[0].PutCount())
	}
	f.w.AddComponentObserver(components.ObjectInternalStateComponentID, check)
	f.w.AddComponentObserver(components.InventoryContainerComponentID, check)
	executor := NewInventoryExecutor(zap.NewNop(), &dropTestIDAllocator{next: 9000}, nil, nil, nil)
	result := executor.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{OpId: 1,
		Expected: []*netproto.InventoryExpected{{Ref: move.Dst, ExpectedRevision: 1}}, Kind: &netproto.InventoryOp_Move{Move: move}})
	require.True(t, result.Success, "result: %+v", result)
	require.Equal(t, 2, observed, "both component observers see the committed build and destination")
	require.Equal(t, f.targetID, result.BuildTargetID)
	require.Len(t, result.UpdatedContainers, 1)
	grid, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
	require.Equal(t, []components.InvItem{{ItemID: 9001, TypeID: 1, Quality: 31, Quantity: 1, W: 1, H: 1, X: 3, Y: 4}}, grid.Items)
	require.Equal(t, uint64(2), grid.Version)
	require.Equal(t, []components.BuildPutItemState{{ItemKey: "branch", Quality: 7, Count: 2}}, f.build.Items[0].PutItems)
	handAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
	require.Equal(t, handBefore, handAfter)
	profile, _ := ecs.GetComponent[components.CharacterProfile](f.w, f.player)
	require.Equal(t, []string{"branch"}, profile.Discovery)
}

func TestBuildTakeBackExactPlacementRejectionsPreserveDeposit(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		position netproto.GridPos
		edit     func(buildDepositFixture, *netproto.InventoryMoveSpec, []*netproto.InventoryExpected)
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
		{name: "stale destination revision", position: netproto.GridPos{X: 3}, code: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			edit: func(_ buildDepositFixture, _ *netproto.InventoryMoveSpec, expected []*netproto.InventoryExpected) {
				expected[0].ExpectedRevision++
			}},
		{name: "closed destination", position: netproto.GridPos{X: 3}, code: netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
			edit: func(f buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
				foreign := createGridContainer(f.w, 7000, 0, 5, 5)
				ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, 7000, 0, foreign)
				move.Dst = quickTestRef(f.w, foreign)
			}},
		{name: "link removed", position: netproto.GridPos{X: 3}, code: netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT,
			edit: func(f buildDepositFixture, _ *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
				ecs.GetResource[ecs.LinkState](f.w).RemoveLink(f.playerID)
			}},
		{name: "swap flag rejected", position: netproto.GridPos{X: 3}, code: netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
			edit: func(_ buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
				move.AllowSwapOrMerge = true
			}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := setupBuildDeposit(t)
			branch, _ := itemdefs.Global().GetByKey("branch")
			branch.Size = itemdefs.Size{W: 2, H: 2}
			f.build.Items[0].PutItems = []components.BuildPutItemState{{ItemKey: "branch", Quality: 23, Count: 2}}
			putBefore := append([]components.BuildPutItemState(nil), f.build.Items[0].PutItems...)
			allocator := &dropTestIDAllocator{next: 9000}
			f.service.idAllocator = allocator
			addItemToContainer(f.w, f.hand, components.InvItem{ItemID: 6000, TypeID: 3, Quantity: 1, W: 1, H: 1})
			addItemToContainer(f.w, f.grid, components.InvItem{ItemID: 6001, TypeID: 1, Quantity: 95, W: 1, H: 1, X: 2, Y: 2})
			addItemToContainer(f.w, f.grid, components.InvItem{ItemID: 6002, TypeID: 3, Quantity: 1, W: 1, H: 1, X: 0, Y: 3})
			slot := uint32(0)
			move := &netproto.InventoryMoveSpec{Src: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)},
				BuildSlot: &slot, Dst: quickTestRef(f.w, f.grid), DstPos: &scenario.position}
			expected := []*netproto.InventoryExpected{{Ref: move.Dst, ExpectedRevision: 1}}
			if scenario.edit != nil {
				scenario.edit(f, move, expected)
			}
			gridBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
			gridBefore.Items = append([]components.InvItem(nil), gridBefore.Items...)
			handBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
			result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
			require.False(t, result.Success)
			code := scenario.code
			if code == 0 {
				code = netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL
			}
			require.Equal(t, code, result.ErrorCode)
			require.Empty(t, result.UpdatedContainers)
			require.Zero(t, result.BuildTargetID)
			require.Equal(t, putBefore, f.build.Items[0].PutItems)
			require.Equal(t, types.EntityID(9000), allocator.next)
			gridAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
			handAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
			require.Equal(t, gridBefore, gridAfter)
			require.Equal(t, handBefore, handAfter)
			state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
			require.False(t, state.IsDirty)
			if move.Dst.OwnerId != uint64(f.playerID) {
				foreign, _ := ecs.GetResource[ecs.InventoryRefIndex](f.w).Lookup(constt.InventoryGrid, types.EntityID(move.Dst.OwnerId), 0)
				container, _ := ecs.GetComponent[components.InventoryContainer](f.w, foreign)
				require.Empty(t, container.Items)
				require.Equal(t, uint64(1), container.Version)
			}
		})
	}
}
