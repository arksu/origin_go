package behaviors

import (
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func cropCatalog(t *testing.T) *objectdefs.Registry {
	t.Helper()
	previousItems, previousObjects := itemdefs.Global(), objectdefs.Global()
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	items, err := itemdefs.LoadFromDirectory(filepath.Join("..", "..", "..", "data", "items"), zap.NewNop())
	require.NoError(t, err)
	itemdefs.SetGlobalForTesting(items)
	objects, err := objectdefs.LoadFromDirectory(filepath.Join("..", "..", "..", "data", "objects"), MustDefaultRegistry(), zap.NewNop())
	require.NoError(t, err)
	objectdefs.SetGlobalForTesting(objects)
	return objects
}

func cropFixture(t *testing.T, key string, saved *components.CropBehaviorState) (*ecs.World, types.Handle, types.Handle) {
	t.Helper()
	def, exists := cropCatalog(t).GetByKey(key)
	require.True(t, exists)
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 100
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 9})
	})
	plant := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: uint32(def.DefID), Quality: 88, Behaviors: []string{cropBehaviorKey}})
		ecs.AddComponent(w, h, components.ObjectInternalState{HP: 73, HasHP: true})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 2, HalfHeight: 2, Layer: 1, Mask: 1})
	})
	if saved != nil {
		setCropState(w, plant, saved)
	}
	require.NoError(t, (cropBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{
		World: w, Handle: plant, EntityID: 2, EntityType: uint32(def.DefID), Reason: contracts.ObjectBehaviorInitReasonRestore,
	}))
	ecs.GetResource[ecs.LinkState](w).SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: plant})
	return w, player, plant
}

func assertCropState(t *testing.T, w *ecs.World, plant types.Handle, stage int, deadline int64) {
	t.Helper()
	require.Equal(t, &components.CropBehaviorState{Stage: stage, NextGrowthRuntimeSeconds: deadline}, CropState(w, plant))
	collider, exists := ecs.GetComponent[components.Collider](w, plant)
	require.True(t, exists, "all stages retain a collider for ordinary damage")
	require.Equal(t, float64(2), collider.HalfWidth)
	require.Equal(t, float64(2), collider.HalfHeight)
	require.Zero(t, collider.Layer)
	require.Zero(t, collider.Mask)
	internal, _ := ecs.GetComponent[components.ObjectInternalState](w, plant)
	require.Equal(t, float64(73), internal.HP)
	require.True(t, internal.HasHP)
	info, _ := ecs.GetComponent[components.EntityInfo](w, plant)
	require.Equal(t, uint32(88), info.Quality)
	id, exists := w.GetExternalID(plant)
	require.True(t, exists)
	require.Equal(t, types.EntityID(2), id)
}

func TestCropGrowthBoundariesAndTerminalSchedule(t *testing.T) {
	w, _, plant := cropFixture(t, "crop_carrot", nil)
	assertCropState(t, w, plant, 0, 100+4*3600)
	for _, check := range []struct {
		now      int64
		stage    int
		deadline int64
		changed  bool
	}{
		{100 + 4*3600 - 1, 0, 100 + 4*3600, false},
		{100 + 4*3600, 1, 100 + 8*3600, true},
		{100 + 8*3600 + 9, 2, 100 + 16*3600, true},
		{100 + 16*3600, 3, 0, true},
		{math.MaxInt64, 3, 0, false},
	} {
		changed, err := AdvanceCropGrowth(w, plant, 2, check.now)
		require.NoError(t, err)
		require.Equal(t, check.changed, changed)
		assertCropState(t, w, plant, check.stage, check.deadline)
		require.Equal(t, []string{"crop.stage" + string(rune('0'+check.stage))}, (cropBehavior{}).ApplyRuntime(&contracts.BehaviorRuntimeContext{World: w, Handle: plant}).Flags)
	}
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
}

func TestCropRestoreKeepsDeadlineAndCatchesUpInThreeTransitions(t *testing.T) {
	saved := &components.CropBehaviorState{Stage: 0, NextGrowthRuntimeSeconds: 123}
	w, _, plant := cropFixture(t, "crop_wheat", saved)
	assertCropState(t, w, plant, 0, 123)
	timeState := ecs.GetResource[ecs.TimeState](w)
	timeState.Tick = 9_000_000
	timeState.UnixMs = math.MaxInt64
	require.NoError(t, InitializeCropState(w, plant, 2))
	assertCropState(t, w, plant, 0, 123)
	schedule := ecs.EnsureBehaviorRuntimeSchedule(w)
	_, due := schedule.PopDue(122)
	require.False(t, due)
	entry, due := schedule.PopDue(123 + 18*3600)
	require.True(t, due)
	result, err := (cropBehavior{}).OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{
		World: w, Handle: entry.Handle, EntityID: entry.EntityID, EntityType: 43, CurrentRuntimeSeconds: 123 + 18*3600,
	})
	require.NoError(t, err)
	require.True(t, result.StateChanged)
	assertCropState(t, w, plant, 3, 0)
	require.Zero(t, schedule.PendingCount())
	w.Despawn(plant)
	_, valid := CropDefinition(w, plant, 2)
	require.False(t, valid)
}

func TestCropGrowthFailsClosedOnInvalidStateAndDeadlineOverflow(t *testing.T) {
	w, _, plant := cropFixture(t, "crop_carrot", nil)
	for _, invalid := range []*components.CropBehaviorState{
		{Stage: 4}, {Stage: 0}, {Stage: 1, NextGrowthRuntimeSeconds: math.MaxInt64},
	} {
		setCropState(w, plant, invalid)
		_, err := AdvanceCropGrowth(w, plant, 2, math.MaxInt64)
		require.Error(t, err)
		require.Equal(t, invalid, CropState(w, plant), "failed growth must not partially advance")
	}
	componentsState, _ := ecs.GetComponent[components.ObjectInternalState](w, plant)
	components.DeleteBehaviorState(&componentsState, cropBehaviorKey)
	ecs.AddComponent(w, plant, componentsState)
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = math.MaxInt64
	require.Error(t, InitializeCropState(w, plant, 2))
	require.Nil(t, CropState(w, plant))
}

func TestCropRestoreReconcilesBeforeInitialAppearance(t *testing.T) {
	w, _, plant := cropFixture(t, "crop_carrot", &components.CropBehaviorState{NextGrowthRuntimeSeconds: 123})
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 123 + 12*3600
	require.NoError(t, (cropBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{
		World: w, Handle: plant, EntityID: 2, EntityType: 44, Reason: contracts.ObjectBehaviorInitReasonRestore,
	}))
	assertCropState(t, w, plant, 3, 0)
	require.Equal(t, []string{"crop.stage3"}, (cropBehavior{}).ApplyRuntime(&contracts.BehaviorRuntimeContext{World: w, Handle: plant}).Flags)
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(w).PendingCount())
}

func TestCropInteractionsReconcileGrowthWithoutScheduler(t *testing.T) {
	for _, key := range []string{"crop_wheat", "crop_carrot", "crop_beet", "crop_potato"} {
		t.Run(key, func(t *testing.T) {
			w, player, plant := cropFixture(t, key, nil)
			ctx := &contracts.BehaviorActionListContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: plant}
			timeState := ecs.GetResource[ecs.TimeState](w)
			for _, hour := range []int64{0, 4, 8, 16, 24} {
				timeState.RuntimeSecondsTotal = 100 + hour*3600
				actions := (cropBehavior{}).ProvideActions(ctx)
				eligible := (key == "crop_carrot" && hour >= 8) || (key != "crop_wheat" && hour >= 16) || hour >= 24
				if eligible {
					require.Equal(t, []contracts.ContextAction{{ActionID: "harvest", Title: "Harvest"}}, actions)
				} else {
					require.Empty(t, actions)
				}
			}
		})
	}
}

func cropCycle(t *testing.T, w *ecs.World, player, plant types.Handle, deps *contracts.ExecutionDeps) *contracts.BehaviorCycleContext {
	t.Helper()
	result := (cropBehavior{}).ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: plant, ActionID: "harvest", Deps: deps,
	})
	require.True(t, result.OK)
	active, exists := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	require.True(t, exists)
	require.Equal(t, uint32(10), active.CycleDurationTicks)
	require.True(t, active.MutatesItems)
	active.CycleElapsedTicks = active.CycleDurationTicks
	ecs.AddComponent(w, player, active)
	return &contracts.BehaviorCycleContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: plant, ActionID: "harvest", Action: active, Deps: deps}
}

func TestCropCycleReconcilesStageAndDelegatesAllHarvestEffects(t *testing.T) {
	w, player, plant := cropFixture(t, "crop_carrot", &components.CropBehaviorState{Stage: 2, NextGrowthRuntimeSeconds: 100 + 16*3600})
	calls := 0
	ctx := cropCycle(t, w, player, plant, &contracts.ExecutionDeps{
		CropHarvest: func(gotWorld *ecs.World, playerID types.EntityID, playerHandle types.Handle, targetID types.EntityID, targetHandle types.Handle) contracts.BehaviorResult {
			calls++
			require.Same(t, w, gotWorld)
			require.Equal(t, types.EntityID(1), playerID)
			require.Equal(t, player, playerHandle)
			require.Equal(t, types.EntityID(2), targetID)
			require.Equal(t, plant, targetHandle)
			require.Equal(t, 3, CropState(w, plant).Stage, "completion catches up before harvest admission")
			return contracts.BehaviorResult{OK: true}
		},
		GiveItem: func(*ecs.World, types.EntityID, types.Handle, string, uint32, uint32) contracts.GiveItemOutcome {
			t.Fatal("crop behavior must not directly give items")
			return contracts.GiveItemOutcome{}
		},
	})
	ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal = 100 + 16*3600
	require.Equal(t, contracts.BehaviorCycleDecisionComplete, (cropBehavior{}).OnCycleComplete(ctx))
	require.Equal(t, 1, calls)
	stats, _ := ecs.GetComponent[components.EntityStats](w, player)
	require.Equal(t, float64(9), stats.Stamina, "callback owns exact stamina admission and payment")
	assertCropState(t, w, plant, 3, 0)
}

func TestCropCycleRejectsStaleOrUnavailableHarvest(t *testing.T) {
	for _, reason := range []string{"canceled", "stale_cycle", "incomplete", "link_broken", "wrong_actor", "wrong_target", "locked", "target_gone", "no_callback", "rejected"} {
		t.Run(reason, func(t *testing.T) {
			w, player, plant := cropFixture(t, "crop_carrot", &components.CropBehaviorState{Stage: 3})
			calls := 0
			alerts := &testMiniAlertSender{}
			deps := &contracts.ExecutionDeps{Alerts: alerts, CropHarvest: func(*ecs.World, types.EntityID, types.Handle, types.EntityID, types.Handle) contracts.BehaviorResult {
				calls++
				return contracts.BehaviorResult{UserVisible: true, ReasonCode: "LOW_STAMINA", Severity: contracts.BehaviorAlertSeverityWarning}
			}}
			ctx := cropCycle(t, w, player, plant, deps)
			switch reason {
			case "canceled":
				ecs.RemoveComponent[components.ActiveCyclicAction](w, player)
			case "stale_cycle":
				ctx.Action.StartedTick++
			case "incomplete":
				ctx.Action.CycleElapsedTicks--
				ecs.AddComponent(w, player, ctx.Action)
			case "link_broken":
				ecs.GetResource[ecs.LinkState](w).RemoveLink(1)
			case "wrong_actor":
				ctx.PlayerID++
			case "wrong_target":
				ctx.TargetID++
			case "locked":
				ecs.AddComponent(w, player, components.EntityHealth{IsLying: true})
			case "target_gone":
				w.Despawn(plant)
			case "no_callback":
				deps.CropHarvest = nil
			}
			require.Equal(t, contracts.BehaviorCycleDecisionCanceled, (cropBehavior{}).OnCycleComplete(ctx))
			if reason == "rejected" {
				require.Equal(t, 1, calls)
				require.Len(t, alerts.alerts, 1)
				require.Equal(t, "LOW_STAMINA", alerts.alerts[0].ReasonCode)
			} else {
				require.Zero(t, calls)
			}
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			require.Equal(t, float64(9), stats.Stamina)
		})
	}
}

func TestCropRequiresDurableHarvestDependencyBeforeStarting(t *testing.T) {
	w, player, plant := cropFixture(t, "crop_carrot", &components.CropBehaviorState{Stage: 3})
	result := (cropBehavior{}).ExecuteAction(&contracts.BehaviorActionExecuteContext{
		World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: plant, ActionID: "harvest",
	})
	require.False(t, result.OK)
	require.Equal(t, "CROP_HARVEST_UNAVAILABLE", result.ReasonCode)
	_, active := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	require.False(t, active)
}

func TestCropDefinitionValidation(t *testing.T) {
	objects := cropCatalog(t)
	carrot, _ := objects.GetByKey("crop_carrot")
	base := *carrot.CropConfig
	for _, check := range []struct {
		name   string
		change func(*contracts.CropBehaviorConfig)
		valid  bool
	}{
		{"defaults", func(c *contracts.CropBehaviorConfig) {
			c.PlantingSuccessChance = nil
			c.HarvestCycleDurationTicks = 0
			c.HarvestStaminaCost = 0
		}, true},
		{"zero chance", func(c *contracts.CropBehaviorConfig) { zero := 0.0; c.PlantingSuccessChance = &zero }, true},
		{"negative chance", func(c *contracts.CropBehaviorConfig) { chance := -0.1; c.PlantingSuccessChance = &chance }, false},
		{"chance above one", func(c *contracts.CropBehaviorConfig) { chance := 1.1; c.PlantingSuccessChance = &chance }, false},
		{"missing seed", func(c *contracts.CropBehaviorConfig) { c.SeedItemKey = "missing" }, false},
		{"wrong stage count", func(c *contracts.CropBehaviorConfig) { c.Stages = c.Stages[:3] }, false},
		{"zero growing duration", func(c *contracts.CropBehaviorConfig) { c.Stages[0].DurationSeconds = 0 }, false},
		{"duration overflow", func(c *contracts.CropBehaviorConfig) { c.Stages[0].DurationSeconds = math.MaxInt64 }, false},
		{"nonzero final duration", func(c *contracts.CropBehaviorConfig) { c.Stages[3].DurationSeconds = 1 }, false},
		{"missing final harvest", func(c *contracts.CropBehaviorConfig) { c.Stages[3].Harvest = nil }, false},
		{"early harvest", func(c *contracts.CropBehaviorConfig) { c.Stages[0].Harvest = c.Stages[3].Harvest }, false},
		{"unknown product", func(c *contracts.CropBehaviorConfig) { c.Stages[3].Harvest[0].ItemKey = "missing" }, false},
		{"zero minimum", func(c *contracts.CropBehaviorConfig) { c.Stages[3].Harvest[0].Min = 0 }, false},
		{"reversed count", func(c *contracts.CropBehaviorConfig) { c.Stages[3].Harvest[0].Min = 4 }, false},
		{"too many items", func(c *contracts.CropBehaviorConfig) { c.Stages[3].Harvest[0].Max = 32 }, false},
		{"duplicate product", func(c *contracts.CropBehaviorConfig) { c.Stages[3].Harvest[1].ItemKey = c.Stages[3].Harvest[0].ItemKey }, false},
	} {
		t.Run(check.name, func(t *testing.T) {
			data, err := json.Marshal(base)
			require.NoError(t, err)
			var cfg contracts.CropBehaviorConfig
			require.NoError(t, json.Unmarshal(data, &cfg))
			check.change(&cfg)
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			def := &objectdefs.ObjectDef{Components: &objectdefs.Components{Collider: &objectdefs.ColliderDef{W: 4, H: 4, Layer: 1, Mask: 1}}}
			_, err = (cropBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: def, RawConfig: raw})
			if !check.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.False(t, def.ContextMenuEvenForOneItemValue)
			require.Zero(t, def.Components.Collider.Layer)
			require.Zero(t, def.Components.Collider.Mask)
			require.Equal(t, uint32(10), def.CropConfig.HarvestCycleDurationTicks)
			require.Equal(t, float64(10), def.CropConfig.HarvestStaminaCost)
			if check.name == "zero chance" {
				require.Zero(t, *def.CropConfig.PlantingSuccessChance)
			} else {
				require.Equal(t, float64(1), *def.CropConfig.PlantingSuccessChance)
			}
		})
	}
	_, err := (cropBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: carrot, RawConfig: []byte(`{"unknown":1}`)})
	require.Error(t, err)
}

func TestCropCatalogTimingsAndHarvestProducts(t *testing.T) {
	objects := cropCatalog(t)
	seeds := make(map[string]bool)
	for _, check := range []struct {
		key   string
		hours []int64
		final []contracts.CropHarvestProduct
	}{
		{"crop_wheat", []int64{6, 6, 12, 0}, []contracts.CropHarvestProduct{{ItemKey: "seed_wheat", Min: 2, Max: 3}, {ItemKey: "straw", Min: 1, Max: 2}}},
		{"crop_carrot", []int64{4, 4, 8, 0}, []contracts.CropHarvestProduct{{ItemKey: "carrot", Min: 1, Max: 3}, {ItemKey: "seed_carrot", Min: 1, Max: 2}}},
		{"crop_beet", []int64{4, 4, 8, 0}, []contracts.CropHarvestProduct{{ItemKey: "beet", Min: 1, Max: 3}, {ItemKey: "seed_beet", Min: 1, Max: 2}}},
		{"crop_potato", []int64{4, 4, 8, 0}, []contracts.CropHarvestProduct{{ItemKey: "potato", Min: 2, Max: 4}, {ItemKey: "seed_potato", Min: 1, Max: 2}}},
	} {
		def, exists := objects.GetByKey(check.key)
		require.True(t, exists)
		require.Equal(t, 100, def.HP)
		require.False(t, def.Indestructible)
		require.False(t, def.Transient)
		require.True(t, def.IsStatic)
		require.False(t, def.ContextMenuEvenForOneItemValue)
		cfg := def.CropConfig
		require.NotNil(t, cfg)
		require.False(t, seeds[cfg.SeedItemKey], "every seed maps to one culture")
		seeds[cfg.SeedItemKey] = true
		for stage, hours := range check.hours {
			require.Equal(t, hours*3600, cfg.Stages[stage].DurationSeconds)
			if stage < 2 || (stage == 2 && check.key != "crop_carrot") {
				require.Empty(t, cfg.Stages[stage].Harvest)
			}
		}
		require.Equal(t, check.final, cfg.Stages[3].Harvest)
		if check.key == "crop_carrot" {
			require.Equal(t, []contracts.CropHarvestProduct{{ItemKey: "carrot", Min: 1, Max: 3}}, cfg.Stages[2].Harvest)
		}
	}
}
