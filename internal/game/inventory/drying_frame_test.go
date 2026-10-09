package inventory

import (
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

type dryingInventoryFixture struct {
	w                                   *ecs.World
	playerID, frameID                   types.EntityID
	player, frame, backpack, hand, root types.Handle
}

func setupDryingInventory(t *testing.T, withRecipe bool) dryingInventoryFixture {
	t.Helper()
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 101, Key: "raw", Name: "Raw Material", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 102, Key: "dried", Name: "Dried Material", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 103, Key: "other", Name: "Other Item", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	config := &objectdefs.DryingBehaviorConfig{}
	if withRecipe {
		config.Processes = []objectdefs.DryingProcessConfig{{InputTypeID: 101, OutputTypeID: 102}}
	}
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 10, Key: "player", Name: "Player"},
		{DefID: 20, Key: "drying_frame", Name: "Drying Frame", DryingConfig: config},
		{DefID: 30, Key: "chest", Name: "Chest"},
	}))
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	w, playerID, player := setupTestWorld(t)
	backpack, hand := setupPlayerWithInventories(w, playerID, player)
	ecs.AddComponent(w, player, components.EntityInfo{TypeID: 10})
	ecs.AddComponent(w, player, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, player, components.ChunkRef{})
	frameID := types.EntityID(2000)
	frame := w.Spawn(frameID, nil)
	ecs.AddComponent(w, frame, components.EntityInfo{TypeID: 20})
	ecs.AddComponent(w, frame, components.ObjectInternalState{})
	root := createGridContainer(w, frameID, 0, 2, 2)
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, frameID, 0, root)
	opened := ecs.GetResource[ecs.OpenContainerState](w)
	opened.SetRootOpened(playerID, frameID)
	opened.OpenRef(playerID, ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: frameID})
	return dryingInventoryFixture{w: w, playerID: playerID, frameID: frameID, player: player, frame: frame, backpack: backpack, hand: hand, root: root}
}

func dryingInventoryRef(kind constt.InventoryKind, ownerID types.EntityID) *netproto.InventoryRef {
	return &netproto.InventoryRef{Kind: netproto.InventoryKind(kind), OwnerId: uint64(ownerID)}
}

func dryingTestItem(itemID types.EntityID, typeID uint32) components.InvItem {
	return components.InvItem{ItemID: itemID, TypeID: typeID, Quantity: 1, Quality: 17, W: 1, H: 1}
}

func TestDryingFrameAdmissionUsesSharedMoveValidation(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		recipe   bool
		typeID   uint32
		source   constt.InventoryKind
		quantity uint32
		allowed  bool
	}{
		{"empty recipes", false, 101, constt.InventoryHand, 1, false},
		{"raw from hand", true, 101, constt.InventoryHand, 1, true},
		{"output from backpack", true, 102, constt.InventoryGrid, 1, true},
		{"other from hand", true, 103, constt.InventoryHand, 1, false},
		{"other from backpack", true, 103, constt.InventoryGrid, 1, false},
		{"malformed stack", true, 101, constt.InventoryHand, 2, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := setupDryingInventory(t, scenario.recipe)
			source := f.hand
			if scenario.source == constt.InventoryGrid {
				source = f.backpack
			}
			item := dryingTestItem(3101, scenario.typeID)
			item.Quantity = scenario.quantity
			addItemToContainer(f.w, source, item)
			result := NewInventoryOperationService(zap.NewNop(), nil, nil).ExecuteMove(f.w, f.playerID, f.player, 1, &netproto.InventoryMoveSpec{
				Src: dryingInventoryRef(scenario.source, f.playerID), Dst: dryingInventoryRef(constt.InventoryGrid, f.frameID), ItemId: uint64(item.ItemID),
			}, nil)
			require.Equal(t, scenario.allowed, result.Success, "result: %+v", result)
			if !scenario.allowed {
				require.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
				require.Equal(t, "Item is not allowed in this drying frame", result.Message)
				root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
				require.Empty(t, root.Items)
				require.Equal(t, uint64(1), root.Version)
			}
		})
	}
}

func TestDryingFrameRejectsUnsupportedReverseSwap(t *testing.T) {
	f := setupDryingInventory(t, true)
	addItemToContainer(f.w, f.root, dryingTestItem(3101, 101))
	addItemToContainer(f.w, f.hand, dryingTestItem(3102, 103))
	result := NewInventoryOperationService(zap.NewNop(), nil, nil).ExecuteMove(f.w, f.playerID, f.player, 1, &netproto.InventoryMoveSpec{
		Src: dryingInventoryRef(constt.InventoryGrid, f.frameID), Dst: dryingInventoryRef(constt.InventoryHand, f.playerID), ItemId: 3101, AllowSwapOrMerge: true,
	}, nil)
	require.False(t, result.Success)
	require.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
	root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
	require.Equal(t, types.EntityID(3101), root.Items[0].ItemID)
	require.Equal(t, uint64(1), root.Version)
}

func TestDryingFrameAdmissionDoesNotRestrictOrdinaryRoots(t *testing.T) {
	f := setupDryingInventory(t, false)
	ecs.WithComponent(f.w, f.frame, func(info *components.EntityInfo) { info.TypeID = 30 })
	addItemToContainer(f.w, f.hand, dryingTestItem(3101, 103))
	result := NewInventoryOperationService(zap.NewNop(), nil, nil).ExecuteMove(f.w, f.playerID, f.player, 1, &netproto.InventoryMoveSpec{
		Src: dryingInventoryRef(constt.InventoryHand, f.playerID), Dst: dryingInventoryRef(constt.InventoryGrid, f.frameID), ItemId: 3101,
	}, nil)
	require.True(t, result.Success)
}

func TestRootMutationHookIsImmediateAndResultUsesFreshSnapshot(t *testing.T) {
	f := setupDryingInventory(t, true)
	addItemToContainer(f.w, f.hand, dryingTestItem(3101, 101))
	e := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	called := 0
	e.SetRootMutationHook(func(w *ecs.World, rootID types.EntityID) {
		called++
		require.Equal(t, f.frameID, rootID)
		require.Zero(t, ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](w).PendingCount(), "notification precedes dirty queue")
		container, _ := ecs.GetComponent[components.InventoryContainer](w, f.root)
		require.Equal(t, types.EntityID(3101), container.Items[0].ItemID, "committed input is visible immediately")
		ecs.MutateComponent[components.InventoryContainer](w, f.root, func(c *components.InventoryContainer) bool {
			c.Items[0].ItemID = 3102
			c.Items[0].TypeID = 102
			c.Version++
			return true
		})
	})
	result := e.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{
		Src: dryingInventoryRef(constt.InventoryHand, f.playerID), Dst: dryingInventoryRef(constt.InventoryGrid, f.frameID), ItemId: 3101,
	}}})
	require.True(t, result.Success)
	require.Equal(t, 1, called)
	require.Equal(t, 1, ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](f.w).PendingCount())
	for _, container := range result.UpdatedContainers {
		if container.OwnerID == f.frameID {
			require.Equal(t, uint64(3), container.Version)
			require.Equal(t, types.EntityID(3102), container.Items[0].ItemID)
			require.Equal(t, uint32(102), container.Items[0].TypeID)
			return
		}
	}
	t.Fatal("updated frame snapshot is missing")
}

func TestRootMutationHookRepositionAndSameTickRemoveReturn(t *testing.T) {
	f := setupDryingInventory(t, true)
	addItemToContainer(f.w, f.root, dryingTestItem(3101, 101))
	e := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	var itemCounts []int
	e.SetRootMutationHook(func(w *ecs.World, rootID types.EntityID) {
		require.Equal(t, f.frameID, rootID)
		container, _ := ecs.GetComponent[components.InventoryContainer](w, f.root)
		itemCounts = append(itemCounts, len(container.Items))
	})
	move := func(src, dst *netproto.InventoryRef, x uint32) bool {
		return e.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{
			Src: src, Dst: dst, ItemId: 3101, DstPos: &netproto.GridPos{X: x},
		}}}).Success
	}
	root, hand := dryingInventoryRef(constt.InventoryGrid, f.frameID), dryingInventoryRef(constt.InventoryHand, f.playerID)
	require.True(t, move(root, root, 1))
	require.Equal(t, []int{1}, itemCounts, "same root is notified once")
	require.True(t, move(root, hand, 0))
	require.True(t, move(hand, root, 0))
	require.Equal(t, []int{1, 0, 1}, itemCounts, "separate commands within one tick each notify")
	require.False(t, move(root, root, 9))
	require.Equal(t, []int{1, 0, 1}, itemCounts, "failed operation has no notification")
}

func TestDryingFramePickupChecksAdmissionBeforeDurableTransfer(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		recipe    bool
		typeID    uint32
		errorCode netproto.ErrorCode
	}{
		{"empty recipes", false, 101, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST},
		{"other", true, 103, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST},
		{"raw passes admission then rolls back unsupported world transfer", true, 101, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR},
		{"output passes admission then rolls back unsupported world transfer", true, 102, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := setupDryingInventory(t, scenario.recipe)
			_, err := SpawnDroppedEntity(f.w, SpawnDroppedEntityParams{
				DroppedEntityID: 3101, ItemID: 3101, TypeID: scenario.typeID, Quality: 17, Quantity: 1, W: 1, H: 1,
				DropX: 10, DropY: 20, NowRuntimeSeconds: 1,
			})
			require.NoError(t, err)
			persister := &failingDroppedPersister{}
			e := NewInventoryExecutor(zap.NewNop(), nil, persister, nil, nil)
			called := 0
			e.SetRootMutationHook(func(_ *ecs.World, _ types.EntityID) { called++ })
			result := e.ExecutePickupFromWorld(f.w, f.playerID, f.player, 3101, dryingInventoryRef(constt.InventoryGrid, f.frameID))
			require.False(t, result.Success)
			require.Equal(t, scenario.errorCode, result.ErrorCode, "result: %+v", result)
			require.Zero(t, called)
			require.Zero(t, persister.deleted)
			root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
			require.Empty(t, root.Items)
			require.Equal(t, uint64(1), root.Version)
			require.NotEqual(t, types.InvalidHandle, f.w.GetHandleByEntityID(3101))
		})
	}
}

func TestRootMutationHookIsNotCalledForUncommittedWorldDrop(t *testing.T) {
	f := setupDryingInventory(t, true)
	addItemToContainer(f.w, f.root, dryingTestItem(3101, 101))
	persister := &failingDroppedPersister{}
	e := NewInventoryExecutor(zap.NewNop(), nil, persister, nil, nil)
	called := 0
	e.SetRootMutationHook(func(_ *ecs.World, _ types.EntityID) { called++ })
	result := e.ExecuteOperation(f.w, f.playerID, f.player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_DropToWorld{DropToWorld: &netproto.InventoryMoveSpec{
		Src: dryingInventoryRef(constt.InventoryGrid, f.frameID), ItemId: 3101,
	}}})
	// Existing durable transfers support player roots and their nested items;
	// direct world-root transfers must fail and restore their temporary changes.
	require.False(t, result.Success)
	require.Equal(t, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, result.ErrorCode)
	require.Zero(t, called)
	require.Zero(t, persister.persisted)
	root, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
	require.Equal(t, types.EntityID(3101), root.Items[0].ItemID)
	require.Equal(t, uint64(1), root.Version)
	require.Equal(t, types.InvalidHandle, f.w.GetHandleByEntityID(3101), "temporary dropped entity was removed")
}
