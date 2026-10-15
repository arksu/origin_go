package inventory

import (
	"fmt"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type buildDepositFixture struct {
	w                          *ecs.World
	playerID, targetID         types.EntityID
	player, grid, hand, target types.Handle
	build                      *components.BuildBehaviorState
	item                       components.InvItem
	service                    *InventoryOperationService
}

func setupBuildDeposit(t *testing.T) buildDepositFixture {
	t.Helper()
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "branch", Name: "Branch", Tags: []string{"material"}, Size: itemdefs.Size{W: 1, H: 1}, Stack: &itemdefs.Stack{Mode: itemdefs.StackModeStack, Max: 100}},
		{DefID: 2, Key: "bag", Name: "Bag", Size: itemdefs.Size{W: 1, H: 1}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 4, H: 4}}, Visual: &itemdefs.Visual{NestedInventory: &itemdefs.NestedInventoryVisual{HasItems: "bag_full", Empty: "bag_empty"}}},
		{DefID: 3, Key: "stone", Name: "Stone", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 10, Key: "player", Name: "Player"},
		{DefID: 20, Key: "chest", Name: "Chest"},
	}))
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	w, playerID, player := setupTestWorld(t)
	grid, hand := setupPlayerWithInventories(w, playerID, player)
	ecs.AddComponent(w, player, components.EntityInfo{TypeID: 10})
	targetID := types.EntityID(2000)
	target := w.Spawn(targetID, nil)
	ecs.AddComponent(w, target, components.EntityInfo{TypeID: constt.BuildObjectTypeID})
	build := &components.BuildBehaviorState{Items: []components.BuildRequiredItemState{{ItemKey: "branch", RequiredCount: 3}}}
	state := components.ObjectInternalState{}
	components.SetBehaviorState(&state, buildBehaviorStateKey, build)
	state.IsDirty = false
	ecs.AddComponent(w, target, state)
	ecs.GetResource[ecs.LinkState](w).SetLink(ecs.PlayerLink{PlayerID: playerID, PlayerHandle: player, TargetID: targetID, TargetHandle: target})
	return buildDepositFixture{
		w: w, playerID: playerID, targetID: targetID, player: player, grid: grid, hand: hand, target: target, build: build,
		item:    components.InvItem{ItemID: 4000, TypeID: 1, Resource: "branch", Quality: 23, Quantity: 5, W: 1, H: 1, X: 2, Y: 3, Skull: skullMetadataFixture()},
		service: NewInventoryOperationService(zap.NewNop(), nil, nil),
	}
}

func (f buildDepositFixture) move(source types.Handle) (*netproto.InventoryMoveSpec, []*netproto.InventoryExpected) {
	c, _ := ecs.GetComponent[components.InventoryContainer](f.w, source)
	ref := &netproto.InventoryRef{Kind: netproto.InventoryKind(c.Kind), OwnerId: uint64(c.OwnerID), InventoryKey: c.Key}
	return &netproto.InventoryMoveSpec{
		Src: ref, Dst: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_BUILD, OwnerId: uint64(f.targetID)}, ItemId: uint64(f.item.ItemID),
	}, []*netproto.InventoryExpected{{Ref: ref, ExpectedRevision: c.Version}}
}

func (f buildDepositFixture) addNested(t *testing.T) types.Handle {
	t.Helper()
	bag := components.InvItem{ItemID: 5000, TypeID: 2, Resource: "bag_full", Quantity: 1, W: 1, H: 1}
	addItemToContainer(f.w, f.grid, bag)
	nested := createGridContainer(f.w, bag.ItemID, 0, 4, 4)
	ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, bag.ItemID, 0, nested)
	addNestedOwnerLink(f.w, f.player, bag.ItemID, nested)
	return nested
}

func TestBuildDepositHandAndGridFullOrPartial(t *testing.T) {
	for _, sourceKind := range []constt.InventoryKind{constt.InventoryHand, constt.InventoryGrid} {
		for _, quantity := range []uint32{2, 5} {
			t.Run(fmt.Sprintf("source=%d/quantity=%d", sourceKind, quantity), func(t *testing.T) {
				f := setupBuildDeposit(t)
				source := f.grid
				if sourceKind == constt.InventoryHand {
					source = f.hand
				} else {
					addItemToContainer(f.w, f.hand, components.InvItem{ItemID: 6000, TypeID: 3, Quantity: 1})
				}
				f.item.Quantity = quantity
				addItemToContainer(f.w, source, f.item)
				ecs.MutateComponent[components.InventoryContainer](f.w, source, func(c *components.InventoryContainer) bool {
					c.HandMouseOffsetX, c.HandMouseOffsetY = 11, 13
					return true
				})
				before, _ := ecs.GetComponent[components.InventoryContainer](f.w, source)
				handBefore, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
				move, expected := f.move(source)
				result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
				require.True(t, result.Success, "result: %+v", result)
				require.Len(t, result.UpdatedContainers, 1)
				require.Equal(t, source, result.UpdatedContainers[0].Handle)
				require.NotNil(t, result.UpdatedContainers[0].Owner)
				owner, _ := ecs.GetComponent[components.InventoryOwner](f.w, f.player)
				require.Equal(t, owner, *result.UpdatedContainers[0].Owner)
				after, _ := ecs.GetComponent[components.InventoryContainer](f.w, source)
				require.Equal(t, before.Version+1, after.Version)
				require.Equal(t, after, *result.UpdatedContainers[0].Container)
				transferred := quantity
				if transferred > 3 {
					transferred = 3
				}
				require.Equal(t, []components.BuildPutItemState{{ItemKey: "branch", Quality: f.item.Quality, Count: transferred}}, f.build.Items[0].PutItems)
				state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
				require.True(t, state.IsDirty)
				if quantity == transferred {
					require.Empty(t, after.Items)
				} else {
					remainder := f.item
					remainder.Quantity -= transferred
					require.Equal(t, []components.InvItem{remainder}, after.Items)
					require.Same(t, f.item.Skull, after.Items[0].Skull)
				}
				if sourceKind == constt.InventoryHand && quantity == transferred {
					require.Zero(t, after.HandMouseOffsetX)
					require.Zero(t, after.HandMouseOffsetY)
				} else {
					require.Equal(t, before.HandMouseOffsetX, after.HandMouseOffsetX)
					require.Equal(t, before.HandMouseOffsetY, after.HandMouseOffsetY)
				}
				if sourceKind == constt.InventoryGrid {
					handAfter, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.hand)
					require.Equal(t, handBefore, handAfter, "direct grid deposit never occupies the hand")
				}
			})
		}
	}
}

func TestBuildDepositKeepsFirstMatchingRequirementAndQuality(t *testing.T) {
	f := setupBuildDeposit(t)
	f.build.Items = []components.BuildRequiredItemState{
		{ItemKey: "branch", RequiredCount: 2, BuildCount: 1, PutItems: []components.BuildPutItemState{{ItemKey: "branch", Quality: 7, Count: 1}}},
		{ItemTag: "material", RequiredCount: 5, BuildCount: 1, PutItems: []components.BuildPutItemState{{ItemKey: "branch", Quality: f.item.Quality, Count: 1}}},
		{ItemKey: "branch", RequiredCount: 10},
	}
	addItemToContainer(f.w, f.grid, f.item)
	move, expected := f.move(f.grid)
	result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
	require.True(t, result.Success, "result: %+v", result)
	require.Equal(t, uint32(1), f.build.Items[0].PutCount())
	require.Equal(t, []components.BuildPutItemState{{ItemKey: "branch", Quality: f.item.Quality, Count: 4}}, f.build.Items[1].PutItems)
	require.Empty(t, f.build.Items[2].PutItems, "one deposit keeps the existing first matching requirement rule")
	after, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
	require.Equal(t, uint32(2), after.Items[0].Quantity)
}

func TestBuildDepositObserversSeeCommittedSourceAndBuild(t *testing.T) {
	f := setupBuildDeposit(t)
	addItemToContainer(f.w, f.grid, f.item)
	observed := 0
	check := func(h types.Handle) {
		if h != f.grid && h != f.target {
			return
		}
		observed++
		source, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
		require.Equal(t, uint32(2), source.Items[0].Quantity)
		require.Equal(t, uint32(3), f.build.Items[0].PutCount())
		require.Equal(t, uint64(2), source.Version)
	}
	f.w.AddComponentObserver(components.InventoryContainerComponentID, check)
	f.w.AddComponentObserver(components.ObjectInternalStateComponentID, check)
	move, expected := f.move(f.grid)
	result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
	require.True(t, result.Success, "result: %+v", result)
	require.Equal(t, 2, observed)
}

func TestBuildDepositRejectionsLeaveMaterialsUnchanged(t *testing.T) {
	for _, scenario := range []struct {
		name string
		code netproto.ErrorCode
		edit func(buildDepositFixture, *netproto.InventoryMoveSpec, []*netproto.InventoryExpected)
	}{
		{"stale revision", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(_ buildDepositFixture, _ *netproto.InventoryMoveSpec, expected []*netproto.InventoryExpected) {
			expected[0].ExpectedRevision++
		}},
		{"unneeded item", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(f buildDepositFixture, _ *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			f.build.Items[0].ItemKey = "stone"
		}},
		{"full requirement", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(f buildDepositFixture, _ *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			f.build.Items[0].BuildCount = 3
		}},
		{"missing item", netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND, func(_ buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.ItemId++
		}},
		{"missing link", netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, func(f buildDepositFixture, _ *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			ecs.GetResource[ecs.LinkState](f.w).RemoveLink(f.playerID)
		}},
		{"wrong link", netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, func(f buildDepositFixture, _ *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: f.playerID, TargetID: 9999})
		}},
		{"missing target", netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND, func(_ buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.Dst.OwnerId++
		}},
		{"wrong target type", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(f buildDepositFixture, _ *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			ecs.MutateComponent[components.EntityInfo](f.w, f.target, func(info *components.EntityInfo) bool { info.TypeID = 20; return true })
		}},
		{"locked player", netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, func(f buildDepositFixture, _ *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			ecs.AddComponent(f.w, f.player, components.EntityHealth{IsLying: true, SHP: 5, HHP: 20})
		}},
		{"equipment source", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(_ buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.Src.Kind = netproto.InventoryKind_INVENTORY_KIND_EQUIPMENT
		}},
		{"explicit position", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(_ buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.DstPos = &netproto.GridPos{}
		}},
		{"explicit quantity", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(_ buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			quantity := uint32(1)
			move.Quantity = &quantity
		}},
		{"swap or merge", netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, func(_ buildDepositFixture, move *netproto.InventoryMoveSpec, _ []*netproto.InventoryExpected) {
			move.AllowSwapOrMerge = true
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := setupBuildDeposit(t)
			addItemToContainer(f.w, f.grid, f.item)
			move, expected := f.move(f.grid)
			scenario.edit(f, move, expected)
			before, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
			before.Items = append([]components.InvItem(nil), before.Items...)
			putBefore := f.build.Items[0].PutCount()
			result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
			require.False(t, result.Success)
			require.Equal(t, scenario.code, result.ErrorCode, "result: %+v", result)
			require.Empty(t, result.UpdatedContainers)
			after, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
			require.Equal(t, before, after)
			require.Equal(t, putBefore, f.build.Items[0].PutCount())
			state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.target)
			require.False(t, state.IsDirty)
		})
	}
}

func TestBuildDepositUsesSourceAccessChecks(t *testing.T) {
	for _, scenario := range []string{"foreign grid", "stale nested ownership", "reserved external root"} {
		t.Run(scenario, func(t *testing.T) {
			f := setupBuildDeposit(t)
			var source types.Handle
			if scenario == "stale nested ownership" {
				source = f.addNested(t)
				ecs.MutateComponent[components.InventoryContainer](f.w, f.grid, func(c *components.InventoryContainer) bool { c.Items = nil; return true })
			} else {
				const ownerID types.EntityID = 3000
				owner := f.w.Spawn(ownerID, nil)
				source = createGridContainer(f.w, ownerID, 0, 4, 4)
				ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, ownerID, 0, source)
				if scenario == "reserved external root" {
					open := ecs.GetResource[ecs.OpenContainerState](f.w)
					open.SetRootOpened(f.playerID, ownerID)
					open.OpenRef(f.playerID, ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: ownerID})
					require.True(t, ecs.ReserveInventoryOwner(f.w, ownerID, owner))
				}
			}
			addItemToContainer(f.w, source, f.item)
			before, _ := ecs.GetComponent[components.InventoryContainer](f.w, source)
			move, expected := f.move(source)
			result := f.service.ExecuteMove(f.w, f.playerID, f.player, 1, move, expected)
			require.False(t, result.Success)
			require.Equal(t, netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, result.ErrorCode, "result: %+v", result)
			after, _ := ecs.GetComponent[components.InventoryContainer](f.w, source)
			require.Equal(t, before, after)
			require.Empty(t, f.build.Items[0].PutItems)
		})
	}
}

func TestBuildDepositFromNestedUpdatesParentAppearance(t *testing.T) {
	f := setupBuildDeposit(t)
	nested := f.addNested(t)
	f.item.Quantity = 1
	addItemToContainer(f.w, nested, f.item)
	move, expected := f.move(nested)
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	result := executor.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{OpId: 1, Expected: expected, Kind: &netproto.InventoryOp_Move{Move: move}})
	require.True(t, result.Success, "result: %+v", result)
	require.Len(t, result.UpdatedContainers, 2)
	parent, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.grid)
	require.Equal(t, "bag_empty", parent.Items[0].Resource)
	require.Equal(t, uint64(2), parent.Version)
	for _, updated := range result.UpdatedContainers {
		require.Equal(t, uint64(2), updated.Version)
		if updated.OwnerID == 5000 {
			require.Empty(t, updated.Items)
		} else {
			require.Equal(t, f.playerID, updated.OwnerID)
			require.Equal(t, "bag_empty", updated.Items[0].Resource)
		}
	}
}

func TestBuildDepositFromOpenedRootKeepsMutationHook(t *testing.T) {
	f := setupBuildDeposit(t)
	const ownerID types.EntityID = 3000
	owner := f.w.Spawn(ownerID, nil)
	ecs.AddComponent(f.w, owner, components.EntityInfo{TypeID: 20})
	ecs.AddComponent(f.w, owner, components.ObjectInternalState{})
	root := createGridContainer(f.w, ownerID, 0, 4, 4)
	ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, ownerID, 0, root)
	open := ecs.GetResource[ecs.OpenContainerState](f.w)
	open.SetRootOpened(f.playerID, ownerID)
	open.OpenRef(f.playerID, ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: ownerID})
	addItemToContainer(f.w, root, f.item)
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	called := 0
	executor.SetRootMutationHook(func(w *ecs.World, rootID types.EntityID) {
		called++
		require.Equal(t, ownerID, rootID)
		source, _ := ecs.GetComponent[components.InventoryContainer](w, root)
		require.Equal(t, uint32(2), source.Items[0].Quantity)
		require.Equal(t, uint32(3), f.build.Items[0].PutCount())
	})
	move, expected := f.move(root)
	result := executor.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{OpId: 1, Expected: expected, Kind: &netproto.InventoryOp_Move{Move: move}})
	require.True(t, result.Success, "result: %+v", result)
	require.Equal(t, 1, called)
	require.Equal(t, []types.Handle{owner}, ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](f.w).Drain(8, nil))
}
