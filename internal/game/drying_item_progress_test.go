package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestDryingItemProgressOpenMovesAndCompletion(t *testing.T) {
	f := setupLiveDryingFrame(t)
	definition, found := objectdefs.Global().GetByKey("drying_frame")
	require.True(t, found)
	duration := definition.DryingConfig.Processes[0].DurationSeconds
	f.addInput(101, 10, 0, 0)
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 105
	f.addInput(102, 20, 0, 1)
	f.sender.opened = make(map[types.EntityID]*netproto.InventoryState)
	open := NewOpenContainerService(f.w, nil, f.sender, nil)
	open.SetBehaviorRegistry(f.registry)
	root := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 2}
	hand := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: 1}
	ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: f.player, TargetID: 2, TargetHandle: f.frame})
	require.Nil(t, open.HandleOpenRequest(f.w, 1, f.player, root))

	assertProgress := func(item *netproto.ItemInstance, deadline int64) {
		t.Helper()
		if deadline == 0 {
			require.Nil(t, item.Progress)
			return
		}
		require.NotNil(t, item.Progress)
		require.Equal(t, duration, item.Progress.DurationSeconds)
		require.IsType(t, &netproto.ItemProgress_CompletionRuntimeSeconds{}, item.Progress.Timing)
		require.Equal(t, deadline, item.Progress.GetCompletionRuntimeSeconds())
	}
	assertGrid := func(state *netproto.InventoryState, deadlines map[uint64]int64) {
		t.Helper()
		require.NotNil(t, state)
		require.Len(t, state.GetGrid().Items, len(deadlines))
		for _, cell := range state.GetGrid().Items {
			deadline, found := deadlines[cell.Item.ItemId]
			require.True(t, found, "unexpected item %d", cell.Item.ItemId)
			assertProgress(cell.Item, deadline)
		}
	}
	assertGrid(f.sender.opened[1], map[uint64]int64{101: 100 + duration, 102: 105 + duration})

	move := func(src, dst *netproto.InventoryRef, x, y uint32, swap bool) (systems.InventoryOpResult, []*netproto.InventoryState) {
		t.Helper()
		result := f.executor.ExecuteOperation(f.w, 1, f.player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{
			Src: src, Dst: dst, ItemId: 101, DstPos: &netproto.GridPos{X: x, Y: y}, AllowSwapOrMerge: swap,
		}}})
		states := make([]*netproto.InventoryState, len(result.UpdatedContainers))
		for i, container := range result.UpdatedContainers {
			states[i] = systems.BuildInventoryStateProto(container)
		}
		if result.Success {
			open.BroadcastInventoryUpdates(f.w, 1, states)
		}
		return result, states
	}
	assertCurrent := func(first, second int64) {
		t.Helper()
		container, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
		assertGrid(open.buildInventoryStateFromContainer(f.w, container), map[uint64]int64{101: first, 102: second})
	}

	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 110
	result, states := move(root, root, 1, 0, false)
	require.True(t, result.Success, result.Message)
	require.Len(t, states, 1)
	assertGrid(states[0], map[uint64]int64{101: 110 + duration, 102: 105 + duration})
	assertGrid(f.sender.updated[3][0], map[uint64]int64{101: 110 + duration, 102: 105 + duration})
	// A snapshot captured before the move owns its original projected values.
	assertGrid(f.sender.opened[1], map[uint64]int64{101: 100 + duration, 102: 105 + duration})

	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 115
	result, _ = move(root, root, 1, 0, false)
	require.True(t, result.Success, result.Message)
	assertCurrent(110+duration, 105+duration)
	result, _ = move(root, root, 9, 0, false)
	require.False(t, result.Success)
	result, _ = move(root, root, 0, 1, false)
	require.False(t, result.Success)
	assertCurrent(110+duration, 105+duration)

	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 120
	result, _ = move(root, root, 0, 1, true)
	require.True(t, result.Success, result.Message)
	assertCurrent(120+duration, 120+duration)
	assertGrid(f.sender.updated[3][0], map[uint64]int64{101: 120 + duration, 102: 120 + duration})

	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 125
	result, states = move(root, hand, 0, 0, false)
	require.True(t, result.Success, result.Message)
	var removed *netproto.ItemInstance
	for _, state := range states {
		if state.GetHand() != nil {
			removed = state.GetHand().Item
		}
	}
	require.NotNil(t, removed)
	assertProgress(removed, 0)
	assertGrid(f.sender.updated[3][0], map[uint64]int64{102: 120 + duration})
	result, _ = move(hand, root, 0, 1, false)
	require.True(t, result.Success, result.Message)
	assertCurrent(125+duration, 120+duration)

	system := systems.NewBehaviorTickSystem(nil, systems.BehaviorTickSystemConfig{BehaviorRegistry: f.registry, ExecutionDeps: f.deps})
	for i, deadline := range []int64{120 + duration, 125 + duration} {
		clear(f.sender.updated)
		ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = deadline - 1
		system.Update(f.w, 0)
		require.Empty(t, f.sender.updated, "elapsed time alone must not broadcast progress")
		ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = deadline
		system.Update(f.w, 0)
		for _, viewer := range []types.EntityID{1, 3} {
			require.Len(t, f.sender.updated[viewer], 1)
			for _, cell := range f.sender.updated[viewer][0].GetGrid().Items {
				if cell.Item.ItemId == 101 && i == 0 {
					assertProgress(cell.Item, 125+duration)
				} else {
					require.Contains(t, f.outputTypeIDs, cell.Item.TypeId)
					assertProgress(cell.Item, 0)
				}
			}
		}
	}
}

type openProgressTestRegistry struct {
	contracts.BehaviorRegistry
	progress *netproto.ItemProgress
}

func (r openProgressTestRegistry) GetBehavior(key string) (contracts.Behavior, bool) {
	return openProgressTestBehavior{r.progress}, key == "progress"
}

type openProgressTestBehavior struct{ progress *netproto.ItemProgress }

func (openProgressTestBehavior) Key() string { return "progress" }
func (b openProgressTestBehavior) InventoryItemProgress(*contracts.InventoryItemProgressContext) *netproto.ItemProgress {
	return b.progress
}

func TestItemProgressOpenContainerSerialization(t *testing.T) {
	f := setupLiveDryingFrame(t)
	ecs.WithComponent(f.w, f.frame, func(info *components.EntityInfo) { info.Behaviors = []string{"progress"} })
	service := NewOpenContainerService(f.w, nil, nil, nil)
	for _, progress := range []*netproto.ItemProgress{
		nil,
		{DurationSeconds: 120, Timing: &netproto.ItemProgress_CompletionRuntimeSeconds{CompletionRuntimeSeconds: 420}},
		{DurationSeconds: 120, Timing: &netproto.ItemProgress_PausedRemainingSeconds{PausedRemainingSeconds: 0}},
	} {
		service.SetBehaviorRegistry(openProgressTestRegistry{progress: progress})
		for _, kind := range []constt.InventoryKind{constt.InventoryGrid, constt.InventoryHand, constt.InventoryEquipment} {
			state := service.buildInventoryStateFromContainer(f.w, components.InventoryContainer{
				OwnerID: 2, Kind: kind, Width: 2, Height: 2,
				Items: []components.InvItem{{ItemID: 101, TypeID: f.inputTypeID, Quantity: 1, W: 1, H: 1}},
			})
			var item *netproto.ItemInstance
			switch kind {
			case constt.InventoryGrid:
				item = state.GetGrid().Items[0].Item
			case constt.InventoryHand:
				item = state.GetHand().Item
			case constt.InventoryEquipment:
				item = state.GetEquipment().Items[0].Item
			}
			require.True(t, proto.Equal(progress, item.Progress))
			if progress == nil {
				require.Nil(t, item.Progress)
			}
		}
	}
}
