package game

import (
	"context"
	"testing"
	"time"

	"origin/internal/builddefs"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type dryingFrameSender struct {
	opened  map[types.EntityID]*netproto.InventoryState
	updated map[types.EntityID][]*netproto.InventoryState
}

func (s *dryingFrameSender) SendContainerOpened(id types.EntityID, state *netproto.InventoryState) {
	s.opened[id] = state
}
func (s *dryingFrameSender) SendInventoryUpdate(id types.EntityID, states []*netproto.InventoryState) {
	s.updated[id] = states
}
func (*dryingFrameSender) SendContainerClosed(types.EntityID, *netproto.InventoryRef) {}

type dryingFrameIDs struct{ next types.EntityID }

func (ids *dryingFrameIDs) GetFreeID() types.EntityID { ids.next++; return ids.next }

func loadDryingFrameCatalog(t *testing.T) (*objectdefs.ObjectDef, *builddefs.BuildDef) {
	t.Helper()
	previousItems, previousObjects, previousBuilds := itemdefs.Global(), objectdefs.Global(), builddefs.Global()
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
		builddefs.SetGlobalForTesting(previousBuilds)
	})
	items, err := itemdefs.LoadFromDirectory("../../data/items", zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	objects, err := objectdefs.LoadFromDirectory("../../data/objects", behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	builds, err := builddefs.LoadFromDirectory("../../data/builds", zap.NewNop())
	require.NoError(t, err)
	builddefs.SetGlobalForTesting(builds)
	frame, found := objects.GetByKey("drying_frame")
	require.True(t, found)
	build, found := builds.GetByKey("drying_frame")
	require.True(t, found)
	return frame, build
}

func TestDryingFrameCatalogBuildCompletionAndOpen(t *testing.T) {
	frame, build := loadDryingFrameCatalog(t)
	require.Equal(t, []builddefs.BuildInput{{ItemKey: "branch", Count: 1, QualityWeight: 1}}, build.Inputs)
	require.Equal(t, uint32(20), build.TicksRequired)
	require.Equal(t, float64(8), build.StaminaCost)
	require.Empty(t, build.RequiredSkills)
	require.Empty(t, build.RequiredDiscovery)
	require.False(t, frame.HasBehavior("lift"))
	require.Equal(t, 19, frame.DefID)
	require.True(t, frame.IsStatic)
	require.EqualValues(t, 100, frame.HP)
	require.Len(t, frame.DryingConfig.Processes, 1)
	process := frame.DryingConfig.Processes[0]
	require.Equal(t, "wild_windsown_weed", process.InputItemKey)
	require.Equal(t, []string{"seed_wheat", "seed_carrot", "seed_beet", "seed_potato"}, process.OutputItemKeys)
	require.Equal(t, int64(120), process.DurationSeconds)
	require.Equal(t, "herbs", process.VisualCategory)
	require.Equal(t, []string{"drying"}, frame.Station.Capabilities)
	require.Equal(t, []string{"ready"}, frame.Station.States)
	require.Equal(t, "ready", frame.Station.InitialState)
	w := ecs.NewWorldForTesting()
	registry := behaviors.MustDefaultRegistry()
	factory := gameworld.NewObjectFactory(nil)
	player := w.Spawn(1, nil)
	ecs.AddComponent(w, player, components.EntityStats{Stamina: 1000, Energy: 1000})
	buildDef, _ := objectdefs.Global().GetByKey("build")
	require.NotNil(t, buildDef)
	site := gameworld.SpawnEntityFromDef(w, buildDef, gameworld.DefSpawnParams{EntityID: 2})
	ecs.WithComponent(w, site, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, "build", &components.BuildBehaviorState{
			BuildKey: build.Key, BuildDefID: build.DefID, ObjectKey: frame.Key, ObjectTypeID: uint32(frame.DefID),
			Items: []components.BuildRequiredItemState{{ItemKey: "branch", RequiredCount: 1, PutItems: []components.BuildPutItemState{{ItemKey: "branch", Count: 1, Quality: 17}}}},
		})
	})
	ecs.GetResource[ecs.LinkState](w).SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: site})
	service := &BuildService{world: w, objectInvInit: factory, behaviorRegistry: registry, logger: zap.NewNop()}
	require.Equal(t, contracts.BehaviorCycleDecisionComplete, service.HandleBuildCycleComplete(w, 1, player, components.ActiveCyclicAction{ActionID: "build", TargetID: 2}))
	stats, _ := ecs.GetComponent[components.EntityStats](w, player)
	require.Equal(t, float64(992), stats.Stamina)
	info, _ := ecs.GetComponent[components.EntityInfo](w, site)
	require.Equal(t, uint32(frame.DefID), info.TypeID)
	station, found := ecs.GetComponent[components.StationState](w, site)
	require.True(t, found)
	require.Equal(t, []string{"drying"}, station.Capabilities)
	require.Equal(t, "ready", station.CurrentState)
	collider, _ := ecs.GetComponent[components.Collider](w, site)
	require.Equal(t, float64(4), collider.HalfWidth)
	require.Equal(t, float64(10), collider.HalfHeight)
	appearance, _ := ecs.GetComponent[components.Appearance](w, site)
	require.Equal(t, "dframe/empty", appearance.Resource)
	sender := &dryingFrameSender{opened: make(map[types.EntityID]*netproto.InventoryState), updated: make(map[types.EntityID][]*netproto.InventoryState)}
	open := NewOpenContainerService(w, nil, sender, nil)
	ecs.GetResource[ecs.LinkState](w).SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: site})
	require.Nil(t, open.HandleOpenRequest(w, 1, player, &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 2}))
	require.Equal(t, "Drying Frame", sender.opened[1].Title)
	require.Equal(t, uint32(2), sender.opened[1].GetGrid().Width)
	require.Equal(t, uint32(2), sender.opened[1].GetGrid().Height)
}

type dryingFrameFixture struct {
	w                   *ecs.World
	frame, root, player types.Handle
	registry            *behaviors.Registry
	deps                *contracts.ExecutionDeps
	executor            *inventory.InventoryExecutor
	sender              *dryingFrameSender
	inputTypeID         uint32
	outputTypeIDs       []uint32
}

func setupLiveDryingFrame(t *testing.T) dryingFrameFixture {
	t.Helper()
	frame, _ := loadDryingFrameCatalog(t)
	// Keep timer scenarios independent of live balance tuning in this fresh test catalog.
	frame.DryingConfig.Processes[0].DurationSeconds = 16 * 60 * 60
	registry := behaviors.MustDefaultRegistry()
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 100
	factory := gameworld.NewObjectFactory(nil)
	frameHandle := gameworld.SpawnEntityFromDef(w, frame, gameworld.DefSpawnParams{
		EntityID: 2, Quality: 800, InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: registry,
	})
	require.NotEqual(t, types.InvalidHandle, frameHandle)
	station, found := ecs.GetComponent[components.StationState](w, frameHandle)
	require.True(t, found)
	require.Equal(t, []string{"drying"}, station.Capabilities)
	require.Equal(t, "ready", station.CurrentState)
	factory.EnsureObjectInventoriesForDef(w, frameHandle, 2, frame)
	root, found := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 2, 0)
	require.True(t, found)
	player := w.Spawn(1, nil)
	playerDef, found := objectdefs.Global().GetByKey("player")
	require.True(t, found)
	ecs.AddComponent(w, player, components.EntityInfo{TypeID: uint32(playerDef.DefID)})
	hand := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, hand, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryHand, Version: 1})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryHand, 1, 0, hand)
	backpack := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, backpack, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryGrid, Width: 2, Height: 2, Version: 1})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, 1, 0, backpack)
	ecs.AddComponent(w, player, components.InventoryOwner{Inventories: []components.InventoryLink{
		{OwnerID: 1, Kind: constt.InventoryHand, Handle: hand},
		{OwnerID: 1, Kind: constt.InventoryGrid, Handle: backpack},
	}})
	opened := ecs.GetResource[ecs.OpenContainerState](w)
	for _, viewer := range []types.EntityID{1, 3} {
		opened.SetRootOpened(viewer, 2)
		opened.OpenRef(viewer, ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 2})
	}
	sender := &dryingFrameSender{updated: make(map[types.EntityID][]*netproto.InventoryState)}
	open := NewOpenContainerService(w, nil, sender, nil)
	open.SetBehaviorRegistry(registry)
	deps := &contracts.ExecutionDeps{IDAllocator: &dryingFrameIDs{next: 1000}, RootInventoryUpdate: func(w *ecs.World, id types.EntityID) { broadcastRootInventoryUpdate(w, open, id) }}
	executor := inventory.NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	executor.SetBehaviorRegistry(registry)
	executor.SetRootMutationHook(func(w *ecs.World, id types.EntityID) { notifyRootInventoryMutation(w, registry, deps, id) })
	process := frame.DryingConfig.Processes[0]
	outputTypes := make([]uint32, len(process.Outputs))
	for i, output := range process.Outputs {
		outputTypes[i] = output.TypeID
	}
	return dryingFrameFixture{
		w: w, frame: frameHandle, root: root, player: player, registry: registry,
		deps: deps, executor: executor, sender: sender, inputTypeID: process.InputTypeID, outputTypeIDs: outputTypes,
	}
}

func (f dryingFrameFixture) entries(t *testing.T) map[types.EntityID]components.DryingItemState {
	t.Helper()
	internal, found := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
	require.True(t, found)
	state, found := components.GetBehaviorState[components.DryingBehaviorState](internal, "drying")
	require.True(t, found)
	entries := make(map[types.EntityID]components.DryingItemState, len(state.Entries))
	for _, entry := range state.Entries {
		entries[entry.InputItemID] = entry
	}
	return entries
}

func (f dryingFrameFixture) addInput(id types.EntityID, quality uint32, x, y uint8) {
	ecs.WithComponent(f.w, f.root, func(container *components.InventoryContainer) {
		container.Items = append(container.Items, components.InvItem{ItemID: id, TypeID: f.inputTypeID, Quality: quality, Quantity: 1, W: 1, H: 1, X: x, Y: y})
		container.Version++
	})
	notifyRootInventoryMutation(f.w, f.registry, f.deps, 2)
}

func TestDryingFrameFourIndependentSlotsAndViewerUpdates(t *testing.T) {
	f := setupLiveDryingFrame(t)
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	appearances := make(chan *ecs.EntityAppearanceChangedEvent, 4)
	bus.SubscribeAsync(ecs.TopicGameplayEntityAppearance, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		appearances <- event.(*ecs.EntityAppearanceChangedEvent)
		return nil
	})
	// Existing observers must see both transitions without needing to move.
	ecs.GetResource[ecs.VisibilityState](f.w).ObserversByVisibleTarget[f.frame] = map[types.Handle]struct{}{f.player: {}}
	runtime := systems.NewObjectBehaviorSystem(bus, nil, systems.ObjectBehaviorConfig{BehaviorRegistry: f.registry})
	for i := 0; i < 4; i++ {
		ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = int64(100 + 25*i)
		f.addInput(types.EntityID(101+i), uint32(37+i), uint8(i%2), uint8(i/2))
	}
	entries := f.entries(t)
	require.Len(t, entries, 4)
	for i := 0; i < 4; i++ {
		require.Equal(t, int64(57700+25*i), entries[types.EntityID(101+i)].CompletionRuntimeSeconds)
	}
	require.Equal(t, 1, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
	runtime.Update(f.w, 0)
	appearance, _ := ecs.GetComponent[components.Appearance](f.w, f.frame)
	require.Equal(t, "dframe/herbs", appearance.Resource)

	system := systems.NewBehaviorTickSystem(nil, systems.BehaviorTickSystemConfig{BehaviorRegistry: f.registry, ExecutionDeps: f.deps})
	for i := 0; i < 4; i++ {
		ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = int64(57699 + 25*i)
		system.Update(f.w, 0)
		container, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
		require.Equal(t, types.EntityID(101+i), container.Items[i].ItemID, "input survives until its own deadline")
		ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal++
		system.Update(f.w, 0)
		container, _ = ecs.GetComponent[components.InventoryContainer](f.w, f.root)
		item := container.Items[i]
		require.Equal(t, types.EntityID(1001+i), item.ItemID)
		require.Contains(t, f.outputTypeIDs, item.TypeID)
		require.Equal(t, uint32(37+i), item.Quality, "frame quality does not affect seed quality")
		require.Equal(t, uint32(1), item.Quantity)
		require.Equal(t, uint8(i%2), item.X)
		require.Equal(t, uint8(i/2), item.Y)
		require.Len(t, f.entries(t), 3-i)
		for _, viewer := range []types.EntityID{1, 3} {
			require.Len(t, f.sender.updated[viewer], 1)
			state := f.sender.updated[viewer][0]
			require.Equal(t, uint64(item.ItemID), state.GetGrid().Items[i].Item.ItemId)
			require.Equal(t, container.Version, state.Revision)
		}
		runtime.Update(f.w, 0)
		appearance, _ = ecs.GetComponent[components.Appearance](f.w, f.frame)
		require.Equal(t, "dframe/herbs", appearance.Resource, "both raw herbs and their finished seeds retain the herb visual")
	}
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
	for i := 0; i < 4; i++ {
		result := f.executor.ExecuteOperation(f.w, 1, f.player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{
			Src:    &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 2},
			Dst:    &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 1},
			ItemId: uint64(1001 + i), DstPos: &netproto.GridPos{X: uint32(i % 2), Y: uint32(i / 2)},
		}}})
		require.True(t, result.Success, result.Message)
		runtime.Update(f.w, 0)
		appearance, _ = ecs.GetComponent[components.Appearance](f.w, f.frame)
		if i < 3 {
			require.Equal(t, "dframe/herbs", appearance.Resource)
		} else {
			require.Equal(t, "dframe/empty", appearance.Resource, "removing the last seed clears the herb visual")
		}
	}
	for i := 0; i < 2; i++ {
		select {
		case event := <-appearances:
			require.Equal(t, types.EntityID(2), event.TargetID)
		case <-time.After(time.Second):
			t.Fatal("stationary observer did not receive appearance transition")
		}
	}
	require.NoError(t, bus.Shutdown(context.Background()))
	require.Empty(t, appearances, "unchanged herb appearance must not produce extra transitions")
}

func TestDryingFrameInventoryMovesResetOnlyAffectedTimers(t *testing.T) {
	f := setupLiveDryingFrame(t)
	f.addInput(101, 10, 0, 0)
	f.addInput(102, 20, 0, 1)
	f.addInput(103, 30, 1, 1)
	root := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 2}
	hand := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: 1}
	move := func(src, dst *netproto.InventoryRef, x, y uint32, swap bool) systems.InventoryOpResult {
		return f.executor.ExecuteOperation(f.w, 1, f.player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{
			Src: src, Dst: dst, ItemId: 101, DstPos: &netproto.GridPos{X: x, Y: y}, AllowSwapOrMerge: swap,
		}}})
	}
	assertDeadlines := func(first, second int64) {
		t.Helper()
		entries := f.entries(t)
		require.Len(t, entries, 3)
		require.Equal(t, first, entries[101].CompletionRuntimeSeconds)
		require.Equal(t, second, entries[102].CompletionRuntimeSeconds)
		require.Equal(t, int64(57700), entries[103].CompletionRuntimeSeconds, "unaffected slot retains its timer")
	}
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 150
	require.True(t, move(root, root, 1, 0, false).Success)
	assertDeadlines(57750, 57700)
	require.Equal(t, uint8(1), f.entries(t)[101].InputX)
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 175
	require.True(t, move(root, root, 1, 0, false).Success)
	assertDeadlines(57750, 57700)
	require.False(t, move(root, root, 9, 0, false).Success)
	require.False(t, move(root, root, 0, 1, false).Success)
	assertDeadlines(57750, 57700)
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 200
	require.True(t, move(root, root, 0, 1, true).Success)
	assertDeadlines(57800, 57800)
	require.Equal(t, uint8(0), f.entries(t)[101].InputX)
	require.Equal(t, uint8(1), f.entries(t)[101].InputY)
	require.Equal(t, uint8(1), f.entries(t)[102].InputX)
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 250
	require.True(t, move(root, hand, 0, 0, false).Success)
	require.NotContains(t, f.entries(t), types.EntityID(101))
	require.True(t, move(hand, root, 0, 1, false).Success)
	assertDeadlines(57850, 57800)
}

func TestDryingFrameFinishedSeedWithdrawalMarksChunkDirty(t *testing.T) {
	f := setupLiveDryingFrame(t)
	f.addInput(101, 10, 0, 0)
	f.addInput(102, 20, 1, 0)
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 57700
	systems.NewBehaviorTickSystem(nil, systems.BehaviorTickSystemConfig{
		BehaviorRegistry: f.registry, ExecutionDeps: f.deps,
	}).Update(f.w, 0)
	before, found := ecs.GetComponent[components.InventoryContainer](f.w, f.root)
	require.True(t, found)
	require.Len(t, before.Items, 2)
	withdrawnID, remainingSeed := before.Items[0].ItemID, before.Items[1]
	require.Empty(t, f.entries(t))
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())

	// Start at a clean persistence checkpoint with the frame closed and only
	// finished seeds remaining. Inventory changes can no longer change timers.
	opened := ecs.GetResource[ecs.OpenContainerState](f.w)
	opened.CloseAllForPlayer(1)
	opened.CloseAllForPlayer(3)
	runtime := systems.NewObjectBehaviorSystem(nil, nil, systems.ObjectBehaviorConfig{BehaviorRegistry: f.registry})
	runtime.Update(f.w, 0)
	ecs.WithComponent(f.w, f.frame, func(state *components.ObjectInternalState) { state.IsDirty = false })
	initialState, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
	initialFlags := append([]string(nil), initialState.Flags...)
	require.Equal(t, []string{"container.has_items", "drying.herbs"}, initialFlags)
	initialAppearance, _ := ecs.GetComponent[components.Appearance](f.w, f.frame)
	require.Equal(t, "dframe/herbs", initialAppearance.Resource)
	ecs.AddComponent(f.w, f.frame, components.ChunkRef{})
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 16)
	chunk.Spatial().AddStatic(f.frame, 0, 0)
	require.False(t, chunk.IsDirty(f.w))

	f.sender.opened = make(map[types.EntityID]*netproto.InventoryState)
	open := NewOpenContainerService(f.w, nil, f.sender, nil)
	root := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 2}
	ecs.GetResource[ecs.LinkState](f.w).SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: f.player, TargetID: 2, TargetHandle: f.frame})
	require.Nil(t, open.HandleOpenRequest(f.w, 1, f.player, root))
	result := f.executor.ExecuteOperation(f.w, 1, f.player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{
		Src: root, Dst: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 1},
		ItemId: uint64(withdrawnID), DstPos: &netproto.GridPos{},
	}}})
	require.True(t, result.Success, result.Message)
	mutationState, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
	// Both commands precede deferred recompute, so container.open never appears
	// in the computed flags and cannot incidentally mark the frame for saving.
	require.Nil(t, open.HandleCloseRequest(f.w, 1, f.player, root))
	runtime.Update(f.w, 0)
	state, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
	appearance, _ := ecs.GetComponent[components.Appearance](f.w, f.frame)
	require.Equal(t, initialFlags, state.Flags)
	require.Equal(t, initialAppearance, appearance)
	require.Empty(t, f.entries(t))
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
	require.True(t, mutationState.IsDirty, "committed withdrawal must dirty the owner synchronously even without timer changes")
	require.True(t, state.IsDirty, "unchanged runtime flags must not erase inventory persistence intent")
	require.True(t, chunk.IsDirty(f.w), "chunk save filtering must include the changed frame inventory")

	factory := gameworld.NewObjectFactory(nil)
	saved, err := factory.Serialize(f.w, f.frame)
	require.NoError(t, err)
	rows, err := factory.SerializeObjectInventories(f.w, f.frame)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	restoredWorld := ecs.NewWorldForTesting()
	_, err = factory.BuildForChunk(restoredWorld, saved, rows)
	require.NoError(t, err)
	restoredRoot, found := ecs.GetResource[ecs.InventoryRefIndex](restoredWorld).Lookup(constt.InventoryGrid, 2, 0)
	require.True(t, found)
	restored, found := ecs.GetComponent[components.InventoryContainer](restoredWorld, restoredRoot)
	require.True(t, found)
	require.Equal(t, []components.InvItem{remainingSeed}, restored.Items, "withdrawn seed must stay removed after restore")
	require.Equal(t, before.Version+1, restored.Version)
}

func TestDryingFrameCatalogAppearancePriority(t *testing.T) {
	frame, _ := loadDryingFrameCatalog(t)
	for _, scenario := range []struct {
		flags    []string
		resource string
	}{
		{nil, "dframe/empty"},
		{[]string{"drying.herbs"}, "dframe/herbs"},
		{[]string{"drying.dried_hide"}, "dframe/dried_hide"},
		{[]string{"drying.herbs", "drying.dried_hide"}, "dframe/dried_hide_herbs"},
		{[]string{"drying.raw_hide", "drying.dried_hide"}, "dframe/raw_hide"},
		{[]string{"drying.herbs", "drying.raw_hide", "drying.dried_hide"}, "dframe/raw_hide_herbs"},
	} {
		require.Equal(t, scenario.resource, objectdefs.ResolveAppearanceResource(frame, scenario.flags))
	}
}
