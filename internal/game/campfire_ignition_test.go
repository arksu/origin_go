package game

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"origin/internal/builddefs"
	constt "origin/internal/const"
	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func loadIgnitionCampfire(t *testing.T) *objectdefs.ObjectDef {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	previousItems := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	items, err := itemdefs.LoadFromDirectory("../../data/items", zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	registry, err := objectdefs.LoadFromDirectory("../../data/objects", behaviors.MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(registry)
	def, ok := registry.GetByKey("campfire")
	require.True(t, ok)
	require.Equal(t, "unlit", def.Station.InitialState)
	require.EqualValues(t, 5, def.BurnerConfig.InitialFuel)
	return def
}

func TestCampfireSpawnBuildAndReloadPreserveInactiveFuel(t *testing.T) {
	def := loadIgnitionCampfire(t)
	for _, fromBuild := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		ecs.SetResource(w, ecs.TimeState{Tick: 100})
		var target types.Handle
		if fromBuild {
			target = w.Spawn(2, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityInfo{TypeID: constt.BuildObjectTypeID})
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.Appearance{Resource: "build"})
				ecs.AddComponent(w, h, components.ObjectInternalState{})
			})
			service := &BuildService{behaviorRegistry: behaviors.MustDefaultRegistry()}
			service.transformCompletedBuildTarget(w, 2, target, &builddefs.BuildDef{ObjectKey: def.Key}, &components.BuildBehaviorState{}, def)
		} else {
			target = gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{
				EntityID: 2, InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: behaviors.MustDefaultRegistry(),
			})
		}
		ecs.AddComponent(w, target, components.ChunkRef{})
		assertUnlitCampfire(t, w, target)
		outcomes := 0
		burning := ecssystems.NewBehaviorTickSystem(nil, ecssystems.BehaviorTickSystemConfig{
			BehaviorRegistry: behaviors.MustDefaultRegistry(),
			ExecutionDeps:    &contracts.ExecutionDeps{ExhaustBurner: func(*ecs.World, types.Handle) bool { outcomes++; return true }},
		})
		ecs.SetResource(w, ecs.TimeState{Tick: 1000000})
		burning.Update(w, 0)
		assertUnlitCampfire(t, w, target)
		require.Zero(t, outcomes)
		raw, err := (&gameworld.ObjectFactory{}).Serialize(w, target)
		require.NoError(t, err)
		reloaded := ecs.NewWorldForTesting()
		ecs.SetResource(reloaded, ecs.TimeState{Tick: 2000000})
		restored, err := (&gameworld.ObjectFactory{}).Build(reloaded, raw, nil)
		require.NoError(t, err)
		restoreIgnitionState(t, reloaded, restored, raw)
		require.NoError(t, behaviors.MustDefaultRegistry().InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
			World: reloaded, Handle: restored, EntityID: 2, EntityType: uint32(def.DefID), Reason: contracts.ObjectBehaviorInitReasonRestore,
		}, def.BehaviorOrder))
		ecssystems.RecomputeObjectBehaviorsNow(reloaded, nil, nil, behaviors.MustDefaultRegistry(), []types.Handle{restored})
		assertUnlitCampfire(t, reloaded, restored)
		require.True(t, (&burnerExhaustionHandler{}).ReconcileRestoredObject(reloaded, restored))
	}
	resourcesJSON, err := os.ReadFile("../../web_new/src/game/objects/structures.json")
	require.NoError(t, err)
	var resources map[string]map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resourcesJSON, &resources))
	require.NotEmpty(t, resources[def.Key]["unlit"])
	require.NotEmpty(t, resources[def.Key]["burning"])
}

func assertUnlitCampfire(t *testing.T, w *ecs.World, h types.Handle) {
	t.Helper()
	station, ok := ecs.GetComponent[components.StationState](w, h)
	require.True(t, ok)
	require.Equal(t, "unlit", station.CurrentState)
	require.True(t, station.HasCapability("cooking"))
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, ok)
	fuel, ok := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.True(t, ok)
	require.EqualValues(t, 5, fuel.Fuel)
	require.Zero(t, fuel.NextFuelBurnAtTick)
	appearance, _ := ecs.GetComponent[components.Appearance](w, h)
	require.Equal(t, "campfire/unlit", appearance.Resource)
}

func restoreIgnitionState(t *testing.T, w *ecs.World, h types.Handle, raw *repository.Object) {
	t.Helper()
	factory := &gameworld.ObjectFactory{}
	state, err := factory.DeserializeObjectState(raw)
	require.NoError(t, err)
	ecs.AddComponent(w, h, components.ObjectInternalState{State: state})
	factory.RestoreDerivedComponentsFromState(w, h)
}

func TestCampfireLinkedIgnitionCycleAndCraftRefresh(t *testing.T) {
	def := loadIgnitionCampfire(t)
	previousCrafts := craftdefs.Global()
	t.Cleanup(func() { craftdefs.SetGlobalForTesting(previousCrafts) })
	craftdefs.SetGlobalForTesting(craftdefs.NewRegistry([]craftdefs.CraftDef{{
		DefID: 1, Key: "cook_meat", RequiredLinkedObject: "campfire",
		StationRequirements: []craftdefs.StationRequirement{{Capability: "cooking", State: "burning"}},
	}}))
	for _, outcome := range []string{"success", "interrupted", "insufficient_stamina", "target_changed"} {
		t.Run(outcome, func(t *testing.T) {
			bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
			t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
			w := ecs.NewWorld(bus, 0)
			ecs.SetResource(w, ecs.TimeState{Tick: 100})
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Movement{})
				ecs.AddComponent(w, h, components.CharacterProfile{})
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 50})
			})
			target := gameworld.SpawnEntityFromDef(w, def, gameworld.DefSpawnParams{
				EntityID: 2, InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: behaviors.MustDefaultRegistry(),
			})
			finishes := &testCyclicActionFinishSender{}
			service := NewContextActionService(w, bus, nil, nil, nil, finishes, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)
			progress := &testCyclicActionProgressSender{}
			cycles := NewCyclicActionSystem(service, progress, nil)
			craftSender := &craftStationTestSender{}
			crafting := NewCraftingService(w, bus, nil, craftSender, zap.NewNop())
			ecs.GetResource[ecs.OpenedWindowsState](w).Open(1, "craft")
			actions := service.ComputeActions(w, 1, player, 2, target)
			require.Len(t, actions, 1)
			require.Equal(t, "Light my fire", actions[0].Title)
			ecs.AddComponent(w, player, components.PendingContextAction{TargetEntityID: 2, ActionID: actions[0].ActionID})
			_, active := ecs.GetComponent[components.ActiveCyclicAction](w, player)
			require.False(t, active, "ignition waits for the normal link-created trigger")
			ecs.GetResource[ecs.LinkState](w).SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target})
			require.NoError(t, bus.PublishSync(ecs.NewLinkCreatedEvent(0, 1, 2)))
			require.False(t, crafting.buildCraftList(w, 1, player)[0].Flags.StationRequirementsMet)
			require.Len(t, craftSender.craftLists, 1)
			for range 9 {
				cycles.Update(w, 0)
			}
			require.NotEmpty(t, progress.messages)
			assertUnlitCampfire(t, w, target)
			switch outcome {
			case "interrupted":
				ecs.GetResource[ecs.LinkState](w).RemoveLink(1)
			case "insufficient_stamina":
				ecs.WithComponent(w, player, func(stats *components.EntityStats) { stats.Stamina = 49 })
			case "target_changed":
				ecs.WithComponent(w, target, func(station *components.StationState) { station.CurrentState = "burning" })
			}
			cycles.Update(w, 0)
			_, active = ecs.GetComponent[components.ActiveCyclicAction](w, player)
			require.False(t, active, "ignition runs exactly one cycle")
			require.Len(t, finishes.messages, 1)
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			if outcome != "success" {
				require.Equal(t, netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED, finishes.messages[0].Result)
				if outcome == "insufficient_stamina" {
					require.EqualValues(t, 49, stats.Stamina)
				} else {
					require.EqualValues(t, 50, stats.Stamina)
				}
				if outcome != "target_changed" {
					assertUnlitCampfire(t, w, target)
				}
				return
			}
			require.Equal(t, netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_COMPLETED, finishes.messages[0].Result)
			require.Zero(t, stats.Stamina)
			require.Len(t, craftSender.craftLists, 2)
			require.True(t, craftSender.craftLists[1].Recipes[0].Flags.StationRequirementsMet)
			ecssystems.RecomputeObjectBehaviorsNow(w, bus, nil, behaviors.MustDefaultRegistry(), []types.Handle{target})
			appearance, _ := ecs.GetComponent[components.Appearance](w, target)
			require.Equal(t, "campfire/burning", appearance.Resource, "recompute must not revert the resource")
			ecs.AddComponent(w, target, components.ChunkRef{})
			raw, err := (&gameworld.ObjectFactory{}).Serialize(w, target)
			require.NoError(t, err)
			reloaded := ecs.NewWorldForTesting()
			ecs.SetResource(reloaded, ecs.TimeState{Tick: 100})
			restored, err := (&gameworld.ObjectFactory{}).Build(reloaded, raw, nil)
			require.NoError(t, err)
			restoreIgnitionState(t, reloaded, restored, raw)
			require.NoError(t, behaviors.MustDefaultRegistry().InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
				World: reloaded, Handle: restored, EntityID: 2, EntityType: uint32(def.DefID), Reason: contracts.ObjectBehaviorInitReasonRestore,
			}, def.BehaviorOrder))
			restoredAppearance, _ := ecs.GetComponent[components.Appearance](reloaded, restored)
			require.Equal(t, "campfire/burning", restoredAppearance.Resource)
			fuelState, _ := ecs.GetComponent[components.ObjectInternalState](reloaded, restored)
			fuel, _ := components.GetBehaviorState[components.BurnerBehaviorState](fuelState, "burner")
			require.EqualValues(t, 100+def.BurnerConfig.TicksPerFuel, fuel.NextFuelBurnAtTick)
			outcomes := 0
			burning := ecssystems.NewBehaviorTickSystem(nil, ecssystems.BehaviorTickSystemConfig{
				BehaviorRegistry: behaviors.MustDefaultRegistry(),
				ExecutionDeps: &contracts.ExecutionDeps{ExhaustBurner: func(_ *ecs.World, _ types.Handle) bool {
					require.Equal(t, "ash", def.BurnerConfig.DropItem)
					outcomes++
					return true
				}},
			})
			ecs.SetResource(reloaded, ecs.TimeState{Tick: 100 + uint64(5*def.BurnerConfig.TicksPerFuel) - 1})
			burning.Update(reloaded, 0)
			require.EqualValues(t, 1, fuel.Fuel)
			require.Zero(t, outcomes)
			ecs.GetResource[ecs.TimeState](reloaded).Tick++
			burning.Update(reloaded, 0)
			burning.Update(reloaded, 0)
			require.Equal(t, 1, outcomes)
			ecssystems.RecomputeObjectBehaviorsNow(reloaded, nil, nil, behaviors.MustDefaultRegistry(), []types.Handle{restored})
			restoredAppearance, _ = ecs.GetComponent[components.Appearance](reloaded, restored)
			require.Equal(t, "campfire/unlit", restoredAppearance.Resource)
		})
	}
}
