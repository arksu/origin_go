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

func quickTestRef(w *ecs.World, handle types.Handle) *netproto.InventoryRef {
	container, _ := ecs.GetComponent[components.InventoryContainer](w, handle)
	return &netproto.InventoryRef{Kind: netproto.InventoryKind(container.Kind), OwnerId: uint64(container.OwnerID), InventoryKey: container.Key}
}

func quickTestGrid(f buildDepositFixture, key uint32, width, height uint8) types.Handle {
	handle := createGridContainer(f.w, f.playerID, key, width, height)
	ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, f.playerID, key, handle)
	ecs.MutateComponent[components.InventoryOwner](f.w, f.player, func(owner *components.InventoryOwner) bool {
		owner.Inventories = append(owner.Inventories, components.InventoryLink{Kind: constt.InventoryGrid, OwnerID: f.playerID, Key: key, Handle: handle})
		return true
	})
	return handle
}

func quickTestMove(f buildDepositFixture, refs ...*netproto.InventoryRef) (*netproto.InventoryMoveSpec, []*netproto.InventoryExpected) {
	move, expected := f.move(f.grid)
	move.Dst = nil
	move.Modifiers = inventoryModifierShift
	move.OpenInventories = refs
	return move, expected
}

func TestQuickTransferCircularFallbackPreservesWholeStack(t *testing.T) {
	f := setupBuildDeposit(t)
	addItemToContainer(f.w, f.grid, f.item)
	destination := quickTestGrid(f, 1, 2, 1)
	existing := components.InvItem{ItemID: 6000, TypeID: 1, Quantity: 95, Quality: 99, W: 1, H: 1}
	addItemToContainer(f.w, destination, existing)
	full := quickTestGrid(f, 2, 1, 1)
	addItemToContainer(f.w, full, components.InvItem{ItemID: 6001, TypeID: 3, Quantity: 1, W: 1, H: 1})
	nested := f.addNested(t)
	bag, _ := itemdefs.Global().GetByKey("bag")
	bag.Container.Rules.AllowTags = []string{"seed"}
	foreign := createGridContainer(f.w, 7000, 0, 4, 4)
	ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, 7000, 0, foreign)
	f.build.Items[0].ItemKey = "stone"
	move, expected := quickTestMove(f, quickTestRef(f.w, destination), quickTestRef(f.w, f.grid), quickTestRef(f.w, full),
		quickTestRef(f.w, nested), &netproto.InventoryRef{OwnerId: 8000}, quickTestRef(f.w, foreign),
		&netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	result := executor.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{OpId: 17, Expected: expected, Kind: &netproto.InventoryOp_Move{Move: move}})
	require.True(t, result.Success, "result: %+v", result)
	require.Zero(t, result.BuildTargetID)
	after, _ := ecs.GetComponent[components.InventoryContainer](f.w, destination)
	require.Len(t, after.Items, 2)
	require.Equal(t, existing, after.Items[0], "quick transfer must not merge into an existing stack")
	moved := f.item
	moved.X, moved.Y = 1, 0
	require.Equal(t, moved, after.Items[1])
	source, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
	require.Len(t, source.Items, 1, "only the unrelated bag remains")
	require.Equal(t, types.EntityID(5000), source.Items[0].ItemID)
	for _, skipped := range []types.Handle{full, nested, foreign} {
		container, _ := ecs.GetComponent[components.InventoryContainer](f.w, skipped)
		require.Equal(t, uint64(1), container.Version, "rejected candidates must remain untouched")
	}
	require.Empty(t, f.build.Items[0].PutItems)
}

func TestQuickTransferUsesFirstCandidateAfterSource(t *testing.T) {
	f := setupBuildDeposit(t)
	addItemToContainer(f.w, f.grid, f.item)
	before := quickTestGrid(f, 1, 2, 2)
	after := quickTestGrid(f, 2, 2, 2)
	move, expected := quickTestMove(f, quickTestRef(f.w, before), quickTestRef(f.w, f.grid), quickTestRef(f.w, after))
	result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
	require.True(t, result.Success, "result: %+v", result)
	first, _ := ecs.GetComponent[components.InventoryContainer](f.w, before)
	second, _ := ecs.GetComponent[components.InventoryContainer](f.w, after)
	require.Empty(t, first.Items)
	require.Equal(t, f.item.ItemID, second.Items[0].ItemID)
}

func TestQuickTransferSkipsItemsOwnNestedInventory(t *testing.T) {
	f := setupBuildDeposit(t)
	nested := f.addNested(t)
	destination := quickTestGrid(f, 1, 2, 2)
	move, expected := quickTestMove(f, quickTestRef(f.w, f.grid), quickTestRef(f.w, nested), quickTestRef(f.w, destination))
	move.ItemId = 5000
	result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
	require.True(t, result.Success, "result: %+v", result)
	child, _ := ecs.GetComponent[components.InventoryContainer](f.w, nested)
	require.Empty(t, child.Items)
	parent, _ := ecs.GetComponent[components.InventoryContainer](f.w, destination)
	require.Equal(t, types.EntityID(5000), parent.Items[0].ItemID)
}

func TestQuickTransferBuildDestinationReturnsRefreshMetadata(t *testing.T) {
	f := setupBuildDeposit(t)
	addItemToContainer(f.w, f.grid, f.item)
	move, expected := quickTestMove(f, quickTestRef(f.w, f.grid), &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	result := executor.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{OpId: 1, Expected: expected, Kind: &netproto.InventoryOp_Move{Move: move}})
	require.True(t, result.Success, "result: %+v", result)
	require.Equal(t, f.targetID, result.BuildTargetID)
	require.Equal(t, uint32(3), f.build.Items[0].PutCount())
	source, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
	require.Equal(t, uint32(2), source.Items[0].Quantity)
}

func TestQuickTransferRejectsMalformedListsAndStaleSources(t *testing.T) {
	for _, scenario := range []struct {
		name string
		edit func(*netproto.InventoryMoveSpec, []*netproto.InventoryExpected)
	}{
		{"too many refs", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.OpenInventories = make([]*netproto.InventoryRef, maxQuickTransferRefs+1)
		}},
		{"source absent", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.OpenInventories = move.OpenInventories[1:]
		}},
		{"duplicate ref", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.OpenInventories = append(move.OpenInventories, move.Src)
		}},
		{"nil ref", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) { move.OpenInventories[1] = nil }},
		{"invalid kind", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.OpenInventories[1].Kind = netproto.InventoryKind_INVENTORY_KIND_HAND
		}},
		{"explicit destination", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.Dst = move.OpenInventories[1]
		}},
		{"explicit placement", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.DstPos = &netproto.GridPos{}
		}},
		{"swap or merge", func(move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) { move.AllowSwapOrMerge = true }},
		{"stale source revision", func(_ *netproto.InventoryMoveSpec, expected []*netproto.InventoryExpected) {
			expected[0].ExpectedRevision++
		}},
		{"destination revision supplied", func(move *netproto.InventoryMoveSpec, expected []*netproto.InventoryExpected) {
			expected[0].Ref = move.OpenInventories[1]
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := setupBuildDeposit(t)
			addItemToContainer(f.w, f.grid, f.item)
			destination := quickTestGrid(f, 1, 2, 2)
			move, expected := quickTestMove(f, quickTestRef(f.w, f.grid), quickTestRef(f.w, destination))
			scenario.edit(move, expected)
			result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
			require.False(t, result.Success)
			require.Empty(t, result.UpdatedContainers)
			source, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
			require.Equal(t, []components.InvItem{f.item}, source.Items)
			require.Equal(t, uint64(1), source.Version)
			dest, _ := ecs.GetComponent[components.InventoryContainer](f.w, destination)
			require.Empty(t, dest.Items)
			require.Equal(t, uint64(1), dest.Version)
		})
	}
}

func TestQuickTransferAcceptsBoundedListAtMaximum(t *testing.T) {
	f := setupBuildDeposit(t)
	addItemToContainer(f.w, f.grid, f.item)
	destination := quickTestGrid(f, 1, 2, 2)
	refs := []*netproto.InventoryRef{quickTestRef(f.w, f.grid)}
	for i := 1; i < maxQuickTransferRefs-1; i++ {
		refs = append(refs, &netproto.InventoryRef{OwnerId: uint64(10000 + i)})
	}
	refs = append(refs, quickTestRef(f.w, destination))
	move, expected := quickTestMove(f, refs...)
	result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
	require.True(t, result.Success, "result: %+v", result)
}

func TestQuickTransferExhaustedCandidatesLeaveSourceUnchanged(t *testing.T) {
	f := setupBuildDeposit(t)
	addItemToContainer(f.w, f.grid, f.item)
	full := quickTestGrid(f, 1, 1, 1)
	addItemToContainer(f.w, full, components.InvItem{ItemID: 6000, TypeID: 3, Quantity: 1, W: 1, H: 1})
	move, expected := quickTestMove(f, quickTestRef(f.w, f.grid), quickTestRef(f.w, full))
	result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
	require.False(t, result.Success)
	require.Equal(t, netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL, result.ErrorCode)
	source, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
	require.Equal(t, []components.InvItem{f.item}, source.Items)
	require.Equal(t, uint64(1), source.Version)
}

func quickBuildTake(f buildDepositFixture, refs ...*netproto.InventoryRef) *netproto.InventoryMoveSpec {
	slot := uint32(0)
	return &netproto.InventoryMoveSpec{Src: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)},
		BuildSlot: &slot, Modifiers: inventoryModifierShift, OpenInventories: refs}
}

func TestQuickBuildTakeBackFallbackAtomicOneUnitWithOccupiedHand(t *testing.T) {
	f := setupBuildDeposit(t)
	f.build.Items[0].PutItems = []components.BuildPutItemState{{ItemKey: "branch", Quality: 7, Count: 2}, {ItemKey: "branch", Quality: 31, Count: 1}, {ItemKey: "branch", Quality: 99}}
	allocator := &dropTestIDAllocator{next: 9000}
	f.service.idAllocator = allocator
	addItemToContainer(f.w, f.hand, components.InvItem{ItemID: 6000, TypeID: 3, Quantity: 1, W: 1, H: 1})
	handBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
	nested := f.addNested(t)
	bag, _ := itemdefs.Global().GetByKey("bag")
	bag.Container.Rules.AllowTags = []string{"seed"}
	full := quickTestGrid(f, 1, 1, 1)
	addItemToContainer(f.w, full, components.InvItem{ItemID: 6001, TypeID: 3, Quantity: 1, W: 1, H: 1})
	buildRef := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)}
	move := quickBuildTake(f, quickTestRef(f.w, f.grid), buildRef, quickTestRef(f.w, nested), quickTestRef(f.w, full))
	observed := 0
	check := func(handle types.Handle) {
		if handle != f.target && handle != f.grid {
			return
		}
		observed++
		grid, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
		require.Len(t, grid.Items, 2)
		require.Equal(t, types.EntityID(9001), grid.Items[1].ItemID)
		require.Equal(t, uint32(2), f.build.Items[0].PutCount())
	}
	f.w.AddComponentObserver(components.ObjectInternalStateComponentID, check)
	f.w.AddComponentObserver(components.InventoryContainerComponentID, check)
	result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, nil)
	require.True(t, result.Success, "result: %+v", result)
	require.Equal(t, 2, observed)
	require.Equal(t, f.targetID, result.BuildTargetID)
	require.Equal(t, types.EntityID(9001), allocator.next, "failed candidates do not allocate item IDs")
	require.Equal(t, []components.BuildPutItemState{{ItemKey: "branch", Quality: 7, Count: 2}, {ItemKey: "branch", Quality: 99}}, f.build.Items[0].PutItems)
	grid, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
	require.Equal(t, components.InvItem{ItemID: 9001, TypeID: 1, Resource: "", Quality: 31, Quantity: 1, W: 1, H: 1, X: 1}, grid.Items[1])
	handAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
	require.Equal(t, handBefore, handAfter)
}

func TestQuickBuildTakeBackRejectionsPreserveDeposit(t *testing.T) {
	for _, scenario := range []struct {
		name string
		edit func(buildDepositFixture, *netproto.InventoryMoveSpec)
	}{
		{"missing slot", func(_ buildDepositFixture, move *netproto.InventoryMoveSpec) { move.BuildSlot = nil }},
		{"invalid slot", func(_ buildDepositFixture, move *netproto.InventoryMoveSpec) {
			slot := uint32(1)
			move.BuildSlot = &slot
		}},
		{"item ID instead of aggregate", func(_ buildDepositFixture, move *netproto.InventoryMoveSpec) { move.ItemId = 4000 }},
		{"missing link", func(f buildDepositFixture, _ *netproto.InventoryMoveSpec) {
			ecs.GetResource[ecs.LinkState](f.w).RemoveLink(f.playerID)
		}},
		{"carrying", func(f buildDepositFixture, _ *netproto.InventoryMoveSpec) {
			ecs.AddComponent(f.w, f.player, components.LiftCarryState{})
		}},
		{"locked", func(f buildDepositFixture, _ *netproto.InventoryMoveSpec) {
			ecs.AddComponent(f.w, f.player, components.EntityHealth{IsLying: true, SHP: 5, HHP: 20})
		}},
		{"full destination", func(f buildDepositFixture, _ *netproto.InventoryMoveSpec) {
			for y := uint8(0); y < 5; y++ {
				for x := uint8(0); x < 5; x++ {
					addItemToContainer(f.w, f.grid, components.InvItem{ItemID: types.EntityID(10000 + int(y)*5 + int(x)), TypeID: 3, Quantity: 1, W: 1, H: 1, X: x, Y: y})
				}
			}
		}},
		{"empty requirement", func(f buildDepositFixture, _ *netproto.InventoryMoveSpec) { f.build.Items[0].PutItems = nil }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := setupBuildDeposit(t)
			f.build.Items[0].PutItems = []components.BuildPutItemState{{ItemKey: "branch", Quality: 23, Count: 2}}
			allocator := &dropTestIDAllocator{next: 9000}
			f.service.idAllocator = allocator
			buildRef := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)}
			move := quickBuildTake(f, buildRef, quickTestRef(f.w, f.grid))
			scenario.edit(f, move)
			putBefore := f.build.Items[0].PutCount()
			result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, nil)
			require.False(t, result.Success)
			require.Empty(t, result.UpdatedContainers)
			require.Zero(t, result.BuildTargetID)
			require.Equal(t, putBefore, f.build.Items[0].PutCount())
			require.Equal(t, types.EntityID(9000), allocator.next)
			state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
			require.False(t, state.IsDirty)
		})
	}
}

func TestBuildTakeBackPreservesDiscoveryAndNestedGrantHooks(t *testing.T) {
	f := setupBuildDeposit(t)
	f.build.Items[0].PutItems = []components.BuildPutItemState{{ItemKey: "bag", Quality: 14, Count: 1}}
	ecs.AddComponent(f.w, f.player, components.CharacterProfile{})
	slot := uint32(0)
	move := &netproto.InventoryMoveSpec{Src: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)},
		BuildSlot: &slot, Dst: quickTestRef(f.w, f.grid)}
	executor := NewInventoryExecutor(zap.NewNop(), &dropTestIDAllocator{next: 9000}, nil, nil, nil)
	result := executor.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{OpId: 1, Kind: &netproto.InventoryOp_Move{Move: move}})
	require.True(t, result.Success, "result: %+v", result)
	require.Equal(t, f.targetID, result.BuildTargetID)
	child, found := ecs.GetResource[ecs.InventoryRefIndex](f.w).Lookup(constt.InventoryGrid, 9001, 0)
	require.True(t, found)
	require.True(t, f.w.Alive(child))
	require.True(t, NestedContainerOwnedByPlayer(f.w, f.player, 9001))
	profile, _ := ecs.GetComponent[components.CharacterProfile](f.w, f.player)
	require.Equal(t, []string{"bag"}, profile.Discovery)
	require.Len(t, result.UpdatedContainers, 2)
}
