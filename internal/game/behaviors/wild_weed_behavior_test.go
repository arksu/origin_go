package behaviors

import (
	"encoding/json"
	"testing"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func wildWeedFixture(t *testing.T, weed *components.WildWeedBehaviorState) (*ecs.World, types.Handle, types.Handle) {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 41, Key: wildWeedItemKey, HP: 100,
		Components:     &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 4, H: 4, Layer: 1, Mask: 1}},
		Behaviors:      map[string]json.RawMessage{wildWeedBehaviorKey: json.RawMessage(`{}`)},
		WildWeedConfig: &objectdefs.WildWeedBehaviorConfig{},
	}}))
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 100
	player := w.Spawn(1, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.Movement{Mode: constt.Walk, State: constt.StateIdle})
		ecs.AddComponent(w, handle, components.EntityStats{Stamina: 150, Energy: 1000})
		ecs.AddComponent(w, handle, components.CharacterProfile{Attributes: characterattrs.Default()})
	})
	plant := w.Spawn(2, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{TypeID: 41, Quality: 88, Behaviors: []string{wildWeedBehaviorKey}})
		ecs.AddComponent(w, handle, components.ObjectInternalState{HP: 73, HasHP: true})
		ecs.AddComponent(w, handle, components.Collider{HalfWidth: 2, HalfHeight: 2, Layer: 1, Mask: 1})
	})
	if weed != nil {
		setWildWeedState(w, plant, weed)
	}
	require.NoError(t, (wildWeedBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{
		World: w, Handle: plant, EntityID: 2, EntityType: 41, Reason: contracts.ObjectBehaviorInitReasonRestore,
	}))
	return w, player, plant
}

func wildWeedCycle(t *testing.T, w *ecs.World, player, plant types.Handle, give contracts.GiveItemFn) *contracts.BehaviorCycleContext {
	t.Helper()
	result := (wildWeedBehavior{}).ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: plant, ActionID: wildWeedActionID,
	})
	require.True(t, result.OK)
	action, exists := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	require.True(t, exists)
	require.Equal(t, uint32(10), action.CycleDurationTicks)
	require.True(t, action.MutatesItems)
	action.CycleElapsedTicks = action.CycleDurationTicks
	ecs.AddComponent(w, player, action)
	return &contracts.BehaviorCycleContext{
		World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: plant,
		ActionID: wildWeedActionID, Action: action, Deps: &contracts.ExecutionDeps{GiveItem: give},
	}
}

func assertWildWeedStage(t *testing.T, w *ecs.World, plant types.Handle, stage int) {
	t.Helper()
	require.Equal(t, stage, wildWeedState(w, plant).Stage)
	internal, exists := ecs.GetComponent[components.ObjectInternalState](w, plant)
	require.True(t, exists)
	require.Equal(t, float64(73), internal.HP)
	require.True(t, internal.HasHP)
	info, _ := ecs.GetComponent[components.EntityInfo](w, plant)
	require.Equal(t, uint32(41), info.TypeID)
	require.Equal(t, uint32(88), info.Quality)
	id, exists := w.GetExternalID(plant)
	require.True(t, exists)
	require.Equal(t, types.EntityID(2), id)
	collider, exists := ecs.GetComponent[components.Collider](w, plant)
	require.True(t, exists, "growing plants retain a collider for damage")
	require.Equal(t, float64(2), collider.HalfWidth)
	require.Equal(t, float64(2), collider.HalfHeight)
	if stage < wildWeedMatureStage {
		require.Zero(t, collider.Layer)
		require.Zero(t, collider.Mask)
	} else {
		require.Equal(t, uint64(1), collider.Layer)
		require.Equal(t, uint64(1), collider.Mask)
	}
}

func TestWildWeedSpawnDefaultsToMature(t *testing.T) {
	w, _, plant := wildWeedFixture(t, nil)
	assertWildWeedStage(t, w, plant, 3)
	require.Zero(t, wildWeedState(w, plant).SpotID)
	require.Zero(t, wildWeedState(w, plant).NextGrowthRuntimeSeconds)
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
	require.Equal(t, []string{"wild_weed.stage3"}, (wildWeedBehavior{}).ApplyRuntime(&contracts.BehaviorRuntimeContext{World: w, Handle: plant}).Flags)
}

func TestWildWeedHarvestThenGrowthPreservesIdentityQualityAndHP(t *testing.T) {
	w, player, plant := wildWeedFixture(t, &components.WildWeedBehaviorState{SpotID: 99, Stage: 3})
	grants := 0
	ctx := wildWeedCycle(t, w, player, plant, func(w *ecs.World, playerID types.EntityID, handle types.Handle, key string, count, quality uint32) contracts.GiveItemOutcome {
		grants++
		require.Equal(t, types.EntityID(1), playerID)
		require.Equal(t, player, handle)
		require.Equal(t, wildWeedItemKey, key)
		require.Equal(t, uint32(1), count)
		require.Equal(t, uint32(88), quality)
		stats, _ := ecs.GetComponent[components.EntityStats](w, player)
		require.Equal(t, float64(150), stats.Stamina, "grant precedes payment")
		require.Equal(t, 3, wildWeedState(w, plant).Stage, "grant precedes cutting")
		return contracts.GiveItemOutcome{Success: true, GrantedCount: 1}
	})
	require.Equal(t, contracts.BehaviorCycleDecisionComplete, (wildWeedBehavior{}).OnCycleComplete(ctx))
	assertWildWeedStage(t, w, plant, 0)
	require.Equal(t, int64(99), wildWeedState(w, plant).SpotID)
	require.Equal(t, int64(100)+wildWeedGrowthSeconds, wildWeedState(w, plant).NextGrowthRuntimeSeconds)
	stats, _ := ecs.GetComponent[components.EntityStats](w, player)
	require.Equal(t, float64(140), stats.Stamina)
	require.Equal(t, contracts.BehaviorCycleDecisionCanceled, (wildWeedBehavior{}).OnCycleComplete(ctx))
	require.Equal(t, 1, grants, "one mature plant grants only once")
	require.Empty(t, (wildWeedBehavior{}).ProvideActions(&contracts.BehaviorActionListContext{World: w, TargetID: 2, TargetHandle: plant}))

	for _, check := range []struct {
		runtime int64
		stage   int
		changed bool
	}{
		{100 + wildWeedGrowthSeconds - 1, 0, false},
		{100 + wildWeedGrowthSeconds, 1, true},
		{100 + 2*wildWeedGrowthSeconds, 2, true},
		{100 + 3*wildWeedGrowthSeconds, 3, true},
	} {
		result, err := (wildWeedBehavior{}).OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{World: w, Handle: plant, EntityID: 2, EntityType: 41, CurrentRuntimeSeconds: check.runtime})
		require.NoError(t, err)
		require.Equal(t, check.changed, result.StateChanged)
		assertWildWeedStage(t, w, plant, check.stage)
	}
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
	require.Zero(t, wildWeedState(w, plant).NextGrowthRuntimeSeconds)
}

func TestWildWeedHarvestFailureHasNoEffect(t *testing.T) {
	for _, failure := range []string{"give_failed", "low_stamina", "canceled", "stale_cycle", "wrong_actor", "wrong_target", "incapacitated", "stale_handle"} {
		t.Run(failure, func(t *testing.T) {
			w, player, plant := wildWeedFixture(t, &components.WildWeedBehaviorState{SpotID: 99, Stage: 3})
			grants := 0
			ctx := wildWeedCycle(t, w, player, plant, func(*ecs.World, types.EntityID, types.Handle, string, uint32, uint32) contracts.GiveItemOutcome {
				grants++
				return contracts.GiveItemOutcome{Success: false}
			})
			expectedStamina := float64(150)
			switch failure {
			case "low_stamina":
				expectedStamina = 0
				ecs.WithComponent(w, player, func(stats *components.EntityStats) { stats.Stamina = 0 })
			case "canceled":
				ecs.RemoveComponent[components.ActiveCyclicAction](w, player)
			case "stale_cycle":
				ctx.Action.StartedTick++
			case "wrong_actor":
				ctx.PlayerID++
			case "wrong_target":
				ctx.TargetID++
			case "incapacitated":
				ecs.AddComponent(w, player, components.EntityHealth{IsLying: true, HHP: 100, SHP: 100})
			case "stale_handle":
				w.Despawn(plant)
				w.Spawn(3, nil)
			}
			require.Equal(t, contracts.BehaviorCycleDecisionCanceled, (wildWeedBehavior{}).OnCycleComplete(ctx))
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			require.Equal(t, expectedStamina, stats.Stamina)
			if failure == "give_failed" {
				require.Equal(t, 1, grants)
			} else {
				require.Zero(t, grants)
			}
			if w.Alive(plant) {
				assertWildWeedStage(t, w, plant, 3)
				require.Zero(t, wildWeedState(w, plant).NextGrowthRuntimeSeconds)
			}
			require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
		})
	}
}

func TestWildWeedRestoreSchedulesRuntimeAndCatchesUp(t *testing.T) {
	w, _, plant := wildWeedFixture(t, &components.WildWeedBehaviorState{SpotID: 99, Stage: 0, NextGrowthRuntimeSeconds: 100 + wildWeedGrowthSeconds})
	assertWildWeedStage(t, w, plant, 0)
	persisted, err := json.Marshal(wildWeedState(w, plant))
	require.NoError(t, err)
	w.Despawn(plant)
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())

	// A new World has unrelated local ticks and wall timestamps after restart.
	w, _, plant = wildWeedFixture(t, nil)
	var restored components.WildWeedBehaviorState
	require.NoError(t, json.Unmarshal(persisted, &restored))
	setWildWeedState(w, plant, &restored)
	timeState := ecs.GetResource[ecs.TimeState](w)
	timeState.Tick = 1
	timeState.UnixMs = 9_000_000_000
	timeState.RuntimeSecondsTotal = 100
	require.NoError(t, (wildWeedBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: plant, EntityID: 2, EntityType: 41, Reason: contracts.ObjectBehaviorInitReasonRestore}))
	assertWildWeedStage(t, w, plant, 0)
	schedule := ecs.EnsureBehaviorRuntimeSchedule(w)
	_, due := schedule.PopDue(100)
	require.False(t, due, "wall time and local ticks do not advance growth")
	entry, due := schedule.PopDue(100 + 3*wildWeedGrowthSeconds)
	require.True(t, due)
	result, err := (wildWeedBehavior{}).OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{World: w, Handle: entry.Handle, EntityID: entry.EntityID, EntityType: 41, CurrentRuntimeSeconds: 100 + 3*wildWeedGrowthSeconds})
	require.NoError(t, err)
	require.True(t, result.StateChanged)
	assertWildWeedStage(t, w, plant, 3)
	require.Equal(t, int64(99), wildWeedState(w, plant).SpotID)
	require.Zero(t, schedule.PendingCount())
}

func TestWildWeedDefConfigRequiresPlantItemAndPreservesPriority(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 41, Key: wildWeedItemKey}}))
	for _, check := range []struct {
		config   string
		valid    bool
		priority int
	}{
		{`{}`, true, defaultBehaviorPriority},
		{`{"priority":17}`, true, 17},
		{`{"seedOutcomes":["seed_wheat"]}`, false, 0},
		{`{"unknown":1}`, false, 0},
	} {
		def := &objectdefs.ObjectDef{}
		priority, err := (wildWeedBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: def, RawConfig: []byte(check.config)})
		if check.valid {
			require.NoError(t, err)
			require.NotNil(t, def.WildWeedConfig)
			require.Equal(t, check.priority, priority)
			require.Equal(t, priority, def.WildWeedConfig.Priority)
		} else {
			require.Error(t, err)
		}
	}
	for _, registry := range []*itemdefs.Registry{nil, itemdefs.NewRegistry(nil)} {
		itemdefs.SetGlobalForTesting(registry)
		_, err := (wildWeedBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: &objectdefs.ObjectDef{}, RawConfig: []byte(`{}`)})
		require.Error(t, err)
	}
}
