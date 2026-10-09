package game

import (
	"encoding/json"
	"testing"

	"origin/internal/builddefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
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
	require.Empty(t, frame.DryingConfig.Processes)
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
	collider, _ := ecs.GetComponent[components.Collider](w, site)
	require.Equal(t, float64(12), collider.HalfWidth)
	require.Equal(t, float64(4), collider.HalfHeight)
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

func TestDryingFrameRealInventoryHookAndViewerUpdates(t *testing.T) {
	frame, _ := loadDryingFrameCatalog(t)
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 901, Key: "test_raw", Name: "Raw", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 902, Key: "test_dried", Name: "Dried", Size: itemdefs.Size{W: 1, H: 1}, Resource: "test/dried"},
	}))
	fixture := *frame
	fixture.Behaviors = map[string]json.RawMessage{"container": json.RawMessage(`{}`), "drying": json.RawMessage(`{"processes":[{"inputItemKey":"test_raw","outputItemKey":"test_dried","durationSeconds":300}]}`)}
	registry := behaviors.MustDefaultRegistry()
	behavior, _ := registry.GetBehavior("drying")
	_, err := behavior.(contracts.BehaviorDefConfigValidator).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: &fixture, RawConfig: fixture.Behaviors["drying"]})
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{fixture}))
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 100
	factory := gameworld.NewObjectFactory(nil)
	frameHandle := gameworld.SpawnEntityFromDef(w, &fixture, gameworld.DefSpawnParams{EntityID: 2, Quality: 800})
	factory.EnsureObjectInventoriesForDef(w, frameHandle, 2, &fixture)
	root, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 2, 0)
	ecs.WithComponent(w, root, func(c *components.InventoryContainer) {
		c.Items = []components.InvItem{{ItemID: 101, TypeID: 901, Quality: 37, Quantity: 1, W: 1, H: 1}}
	})
	player := w.Spawn(1, nil)
	hand := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, hand, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryHand, Version: 1})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryHand, 1, 0, hand)
	ecs.AddComponent(w, player, components.InventoryOwner{Inventories: []components.InventoryLink{{OwnerID: 1, Kind: constt.InventoryHand, Handle: hand}}})
	opened := ecs.GetResource[ecs.OpenContainerState](w)
	key := ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 2}
	for _, viewer := range []types.EntityID{1, 3} {
		opened.SetRootOpened(viewer, 2)
		opened.OpenRef(viewer, key)
	}
	sender := &dryingFrameSender{updated: make(map[types.EntityID][]*netproto.InventoryState)}
	open := NewOpenContainerService(w, nil, sender, nil)
	deps := &contracts.ExecutionDeps{IDAllocator: &dryingFrameIDs{next: 1000}, RootInventoryUpdate: func(w *ecs.World, id types.EntityID) { broadcastRootInventoryUpdate(w, open, id) }}
	notifyRootInventoryMutation(w, registry, deps, 2)
	state := func() *components.DryingBehaviorState {
		internal, _ := ecs.GetComponent[components.ObjectInternalState](w, frameHandle)
		state, _ := components.GetBehaviorState[components.DryingBehaviorState](internal, "drying")
		return state
	}
	require.Equal(t, int64(400), state().Entries[0].CompletionRuntimeSeconds)
	executor := inventory.NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	executor.SetRootMutationHook(func(w *ecs.World, id types.EntityID) { notifyRootInventoryMutation(w, registry, deps, id) })
	rootRef := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: 2}
	handRef := &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: 1}
	move := func(src, dst *netproto.InventoryRef, x uint32) {
		result := executor.ExecuteOperation(w, 1, player, &netproto.InventoryOp{Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{Src: src, Dst: dst, ItemId: 101, DstPos: &netproto.GridPos{X: x}}}})
		require.True(t, result.Success, result.Message)
	}
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 150
	move(rootRef, rootRef, 1)
	require.Equal(t, int64(400), state().Entries[0].CompletionRuntimeSeconds)
	move(rootRef, handRef, 0)
	require.Empty(t, state().Entries)
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
	move(handRef, rootRef, 1)
	require.Equal(t, int64(450), state().Entries[0].CompletionRuntimeSeconds)
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 450
	systems.NewBehaviorTickSystem(nil, systems.BehaviorTickSystemConfig{BehaviorRegistry: registry, ExecutionDeps: deps}).Update(w, 0)
	container, _ := ecs.GetComponent[components.InventoryContainer](w, root)
	require.Equal(t, types.EntityID(1001), container.Items[0].ItemID)
	require.Equal(t, uint32(902), container.Items[0].TypeID)
	require.Equal(t, uint32(37), container.Items[0].Quality)
	require.Equal(t, uint8(1), container.Items[0].X)
	for _, viewer := range []types.EntityID{1, 3} {
		require.Len(t, sender.updated[viewer], 1)
		require.Equal(t, uint64(1001), sender.updated[viewer][0].GetGrid().Items[0].Item.ItemId)
		require.Equal(t, container.Version, sender.updated[viewer][0].Revision)
	}
	require.Empty(t, state().Entries)
}
