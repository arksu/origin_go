package behaviors

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type dryingFixture struct {
	w                *ecs.World
	frame, inventory types.Handle
	updates          int
	deps             *contracts.ExecutionDeps
	behavior         dryingBehavior
}

func dryingTestDefinition() *objectdefs.ObjectDef {
	return &objectdefs.ObjectDef{
		DefID: 1, Key: "test_frame", HP: 10,
		Behaviors:  map[string]json.RawMessage{"container": json.RawMessage(`{}`), "drying": json.RawMessage(`{}`)},
		Components: &objectdefs.Components{Inventory: []objectdefs.InventoryDef{{W: 2, H: 2, Kind: "grid"}}},
	}
}

func dryingTestItems(t *testing.T) {
	t.Helper()
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 10, Key: "wet_a", Size: itemdefs.Size{W: 1, H: 1}, Resource: "wet_a"},
		{DefID: 11, Key: "dry_a", Size: itemdefs.Size{W: 1, H: 1}, Resource: "dry_a"},
		{DefID: 20, Key: "wet_b", Size: itemdefs.Size{W: 1, H: 1}, Resource: "wet_b"},
		{DefID: 21, Key: "dry_b", Size: itemdefs.Size{W: 1, H: 1}, Resource: "dry_b"},
	}))
}

func newDryingFixture(t *testing.T) *dryingFixture {
	t.Helper()
	return newDryingFixtureWithConfig(t, `{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":10},{"inputItemKey":"wet_b","outputItemKeys":["dry_b"],"durationSeconds":25}]}`)
}

func newDryingFixtureWithConfig(t *testing.T, raw string) *dryingFixture {
	t.Helper()
	dryingTestItems(t)
	def := dryingTestDefinition()
	_, err := (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{
		Def: def, RawConfig: []byte(raw),
	})
	require.NoError(t, err)
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{*def}))
	f := &dryingFixture{w: ecs.NewWorld(nil, 0)}
	ecs.SetResource(f.w, ecs.TimeState{RuntimeSecondsTotal: 100, Tick: 100})
	f.frame = f.w.Spawn(100, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{TypeID: 1, Quality: 80, Behaviors: []string{"container", "drying"}})
		ecs.AddComponent(w, handle, components.ObjectInternalState{HasHP: true, HP: 10})
	})
	f.inventory = f.w.SpawnWithoutExternalID()
	ecs.AddComponent(f.w, f.inventory, components.InventoryContainer{OwnerID: 100, Kind: constt.InventoryGrid, Width: 2, Height: 2, Version: 1})
	ecs.GetResource[ecs.InventoryRefIndex](f.w).Add(constt.InventoryGrid, 100, 0, f.inventory)
	f.deps = &contracts.ExecutionDeps{IDAllocator: &testIDAllocator{next: 1000}, RootInventoryUpdate: func(_ *ecs.World, rootID types.EntityID) {
		require.EqualValues(t, 100, rootID)
		f.updates++
	}}
	require.NoError(t, f.behavior.InitObject(&contracts.BehaviorObjectInitContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonSpawn}))
	return f
}

func (f *dryingFixture) mutate(t *testing.T, items []components.InvItem) {
	t.Helper()
	ecs.WithComponent(f.w, f.inventory, func(container *components.InventoryContainer) { container.Items = items; container.Version++ })
	_, err := f.behavior.OnRootInventoryMutation(&contracts.RootInventoryMutationContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1})
	require.NoError(t, err)
}

func (f *dryingFixture) state(t *testing.T) *components.DryingBehaviorState {
	t.Helper()
	internal, found := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
	require.True(t, found)
	state, found := components.GetBehaviorState[components.DryingBehaviorState](internal, "drying")
	require.True(t, found)
	return state
}

func (f *dryingFixture) run(t *testing.T, runtime int64) contracts.BehaviorTickResult {
	t.Helper()
	result, err := f.behavior.OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, CurrentRuntimeSeconds: runtime, Deps: f.deps})
	require.NoError(t, err)
	return result
}

func wetDryingItem(id types.EntityID, typeID uint32, x, y uint8, quality uint32) components.InvItem {
	return components.InvItem{ItemID: id, TypeID: typeID, W: 1, H: 1, X: x, Y: y, Quantity: 1, Quality: quality}
}

func TestDryingBehaviorValidatesEmptyAndResolvedProcesses(t *testing.T) {
	dryingTestItems(t)
	def := dryingTestDefinition()
	priority, err := (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: def, RawConfig: []byte(`{"processes":[]}`)})
	require.NoError(t, err)
	require.Positive(t, priority)
	require.NotNil(t, def.DryingConfig)
	require.False(t, def.DryingConfig.AllowsItemTypeID(10))
	_, err = (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: def, RawConfig: []byte(`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":300}]}`)})
	require.NoError(t, err)
	require.True(t, def.DryingConfig.AllowsItemTypeID(10))
	require.True(t, def.DryingConfig.AllowsItemTypeID(11))
	require.False(t, def.DryingConfig.AllowsItemTypeID(20))
	require.EqualValues(t, 300, def.DryingConfig.Processes[0].DurationSeconds)
	_, err = (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: def, RawConfig: []byte(`{"processes":[{"inputItemKey":" wet_a ","outputItemKeys":[" dry_a ","dry_b"],"durationSeconds":300,"visualCategory":"herbs"}]}`)})
	require.NoError(t, err)
	require.Equal(t, "wet_a", def.DryingConfig.Processes[0].InputItemKey)
	require.Equal(t, []string{"dry_a", "dry_b"}, def.DryingConfig.Processes[0].OutputItemKeys)
	require.Equal(t, []objectdefs.DryingOutputConfig{{TypeID: 11, Width: 1, Height: 1}, {TypeID: 21, Width: 1, Height: 1}}, def.DryingConfig.Processes[0].Outputs)
	require.Equal(t, "herbs", def.DryingConfig.Processes[0].VisualCategory)
	require.True(t, def.DryingConfig.AllowsItemTypeID(21))
}

func TestDryingBehaviorRejectsInvalidProcessesAndGrid(t *testing.T) {
	dryingTestItems(t)
	for _, raw := range []string{
		`{"processes":[{"inputItemKey":"wet_a","outputItemKey":"dry_a","durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":[],"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":null,"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"," "],"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"," dry_a "],"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a","missing"],"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":10,"visualCategory":"unknown"}]}`,
		`{"processes":[{"inputItemKey":"missing","outputItemKeys":["dry_a"],"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["missing"],"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":0}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":-10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":10},{"inputItemKey":"wet_a","outputItemKeys":["dry_b"],"durationSeconds":10}]}`,
		`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":10,"quantity":5}]}`,
	} {
		_, err := (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: dryingTestDefinition(), RawConfig: []byte(raw)})
		require.Error(t, err, raw)
	}
	for _, mutate := range []func(*objectdefs.ObjectDef){
		func(d *objectdefs.ObjectDef) { delete(d.Behaviors, "container") },
		func(d *objectdefs.ObjectDef) { d.Components.Inventory[0].W = 3 },
		func(d *objectdefs.ObjectDef) { d.Components.Inventory[0].Key = 1 },
		func(d *objectdefs.ObjectDef) {
			d.Components.Inventory = append(d.Components.Inventory, objectdefs.InventoryDef{W: 2, H: 2})
		},
	} {
		def := dryingTestDefinition()
		mutate(def)
		_, err := (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: def, RawConfig: []byte(`{}`)})
		require.Error(t, err)
	}
	for _, mutate := range []func(*itemdefs.ItemDef){
		func(d *itemdefs.ItemDef) { d.Stack = &itemdefs.Stack{Mode: "stack", Max: 10} },
		func(d *itemdefs.ItemDef) { d.Container = &itemdefs.ContainerDef{} },
		func(d *itemdefs.ItemDef) { denied := false; d.Allowed.Grid = &denied },
		func(d *itemdefs.ItemDef) { d.Size = itemdefs.Size{W: 2, H: 1} },
	} {
		output, _ := itemdefs.Global().GetByKey("dry_a")
		original := *output
		mutate(output)
		_, err := (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: dryingTestDefinition(), RawConfig: []byte(`{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_b","dry_a"],"durationSeconds":10}]}`)})
		require.Error(t, err)
		*output = original
	}
}

func TestDryingValidatesSharedOutputVisualCategories(t *testing.T) {
	dryingTestItems(t)
	for _, categories := range []struct {
		first, second string
		valid         bool
	}{
		{"herbs", "herbs", true},
		{"hide", "hide", true},
		{"", "", true},
		{"herbs", "hide", false},
		{"", "hide", false},
	} {
		cfg := contracts.DryingBehaviorConfig{Processes: []contracts.DryingProcessConfig{
			{InputItemKey: "wet_a", OutputItemKeys: []string{"dry_a"}, DurationSeconds: 10, VisualCategory: categories.first},
			{InputItemKey: "wet_b", OutputItemKeys: []string{"dry_b", "dry_a"}, DurationSeconds: 10, VisualCategory: categories.second},
		}}
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		_, err = (dryingBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{Def: dryingTestDefinition(), RawConfig: raw})
		if categories.valid {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "conflicting visual categories")
		}
	}
}

func TestDryingConvertsFourItemsIndependentlyWithSameQualityAndPosition(t *testing.T) {
	f := newDryingFixture(t)
	f.mutate(t, []components.InvItem{
		wetDryingItem(200, 10, 0, 0, 37), wetDryingItem(201, 20, 1, 0, 8),
		wetDryingItem(202, 10, 0, 1, 100), wetDryingItem(203, 20, 1, 1, 0),
	})
	require.Len(t, f.state(t).Entries, 4)
	require.Equal(t, 1, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
	// A high frame quality, tick count and wall-clock time never accelerate drying.
	ecs.SetResource(f.w, ecs.TimeState{RuntimeSecondsTotal: 109, Tick: 999999, WallNow: time.Unix(99999999, 0)})
	require.False(t, f.run(t, 109).StateChanged)
	inventory, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	version := inventory.Version
	require.True(t, f.run(t, 110).StateChanged)
	inventory, _ = ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.Equal(t, version+1, inventory.Version)
	require.Equal(t, 1, f.updates)
	for i, item := range inventory.Items {
		require.EqualValues(t, 1, item.Quantity)
		require.EqualValues(t, i%2, item.X)
		require.EqualValues(t, i/2, item.Y)
	}
	require.EqualValues(t, 11, inventory.Items[0].TypeID)
	require.EqualValues(t, 37, inventory.Items[0].Quality)
	require.Equal(t, "dry_a", inventory.Items[0].Resource)
	require.EqualValues(t, 1001, inventory.Items[0].ItemID)
	require.EqualValues(t, 20, inventory.Items[1].TypeID)
	require.EqualValues(t, 100, inventory.Items[2].Quality)
	require.EqualValues(t, 1002, inventory.Items[2].ItemID)
	require.Len(t, f.state(t).Entries, 2)
	require.True(t, f.run(t, 125).StateChanged)
	inventory, _ = ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.EqualValues(t, 21, inventory.Items[1].TypeID)
	require.EqualValues(t, 8, inventory.Items[1].Quality)
	require.EqualValues(t, 0, inventory.Items[3].Quality)
	require.Empty(t, f.state(t).Entries)
	require.Equal(t, 0, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
	require.Equal(t, 2, f.updates)
	f.run(t, 1000)
	require.Equal(t, 2, f.updates)
}

func TestDryingRestartsInternalMoveAndSameTickReinsertion(t *testing.T) {
	f := newDryingFixture(t)
	item := wetDryingItem(200, 10, 0, 0, 37)
	f.mutate(t, []components.InvItem{item})
	require.EqualValues(t, 110, f.state(t).Entries[0].CompletionRuntimeSeconds)
	ecs.SetResource(f.w, ecs.TimeState{RuntimeSecondsTotal: 105, Tick: 100})
	item.X = 1
	f.mutate(t, []components.InvItem{item})
	require.EqualValues(t, 115, f.state(t).Entries[0].CompletionRuntimeSeconds)
	require.EqualValues(t, 1, f.state(t).Entries[0].InputX)
	f.mutate(t, nil)
	require.Empty(t, f.state(t).Entries)
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
	f.mutate(t, []components.InvItem{item})
	require.EqualValues(t, 115, f.state(t).Entries[0].CompletionRuntimeSeconds)
	f.run(t, 110)
	inventory, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.EqualValues(t, 10, inventory.Items[0].TypeID)
	f.run(t, 115)
	inventory, _ = ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.EqualValues(t, 11, inventory.Items[0].TypeID)
	require.EqualValues(t, 1, inventory.Items[0].X)
}

func TestDryingRestorePreservesDeadlineAndDefersOverdueConversion(t *testing.T) {
	for _, now := range []int64{105, 120} {
		t.Run(time.Duration(now).String(), func(t *testing.T) {
			f := newDryingFixture(t)
			f.mutate(t, []components.InvItem{wetDryingItem(200, 10, 0, 0, 37)})
			serialized, err := json.Marshal(f.state(t))
			require.NoError(t, err)
			var restored components.DryingBehaviorState
			require.NoError(t, json.Unmarshal(serialized, &restored))
			ecs.WithComponent(f.w, f.frame, func(internal *components.ObjectInternalState) {
				components.SetBehaviorState(internal, "drying", &restored)
				internal.IsDirty = false
			})
			ecs.CancelBehaviorRuntime(f.w, f.frame, "drying")
			ecs.SetResource(f.w, ecs.TimeState{RuntimeSecondsTotal: now, Tick: 1, WallNow: time.Unix(9999999, 0)})
			require.NoError(t, f.behavior.InitObject(&contracts.BehaviorObjectInitContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonRestore}))
			require.EqualValues(t, 110, f.state(t).Entries[0].CompletionRuntimeSeconds)
			inventory, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
			require.EqualValues(t, 10, inventory.Items[0].TypeID)
			f.run(t, now)
			inventory, _ = ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
			if now < 110 {
				require.EqualValues(t, 10, inventory.Items[0].TypeID)
			} else {
				require.EqualValues(t, 11, inventory.Items[0].TypeID)
			}
		})
	}
}

func TestDryingRejectsStaleIdentityAndDestruction(t *testing.T) {
	for _, guard := range []string{"entity_id", "type_id", "destroying", "stale_handle"} {
		t.Run(guard, func(t *testing.T) {
			f := newDryingFixture(t)
			f.mutate(t, []components.InvItem{wetDryingItem(200, 10, 0, 0, 37)})
			ctx := &contracts.BehaviorRuntimeTickContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, CurrentRuntimeSeconds: 1000, Deps: f.deps}
			switch guard {
			case "entity_id":
				ctx.EntityID = 999
			case "type_id":
				ctx.EntityType = 999
			case "destroying":
				ecs.InitResource(f.w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{f.frame: true}})
			case "stale_handle":
				f.w.Despawn(f.frame)
				f.w.Spawn(100, func(w *ecs.World, handle types.Handle) {
					ecs.AddComponent(w, handle, components.EntityInfo{TypeID: 1})
					ecs.AddComponent(w, handle, components.ObjectInternalState{})
				})
			}
			result, err := f.behavior.OnScheduledRuntimeTick(ctx)
			require.NoError(t, err)
			require.False(t, result.StateChanged)
			inventory, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
			require.EqualValues(t, 10, inventory.Items[0].TypeID)
			require.Zero(t, f.updates)
		})
	}
}

type dryingFailingAllocator struct{ calls int }

func (a *dryingFailingAllocator) GetFreeID() types.EntityID {
	a.calls++
	if a.calls == 2 {
		return 0
	}
	return 1001
}

func TestDryingFailedPreparationDoesNotPartiallyConvert(t *testing.T) {
	f := newDryingFixture(t)
	f.mutate(t, []components.InvItem{wetDryingItem(200, 10, 0, 0, 37), wetDryingItem(201, 10, 1, 0, 39)})
	f.deps.IDAllocator = &dryingFailingAllocator{}
	before, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	_, err := f.behavior.OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, CurrentRuntimeSeconds: 110, Deps: f.deps})
	require.ErrorContains(t, err, "invalid ID")
	after, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.Equal(t, before.Version, after.Version)
	require.EqualValues(t, 10, after.Items[0].TypeID)
	require.EqualValues(t, 10, after.Items[1].TypeID)
	require.Len(t, f.state(t).Entries, 2)
	require.Zero(t, f.updates)
	entry, due := ecs.EnsureBehaviorRuntimeSchedule(f.w).PopDue(111)
	require.True(t, due)
	require.EqualValues(t, 111, entry.DueRuntimeSeconds)
	f.deps.IDAllocator = &testIDAllocator{next: 2000}
	f.run(t, 111)
	after, _ = ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.EqualValues(t, 11, after.Items[0].TypeID)
	require.EqualValues(t, 11, after.Items[1].TypeID)
	require.Equal(t, 1, f.updates)
}

func TestDryingRevalidatesInputIdentityAndQuantity(t *testing.T) {
	f := newDryingFixture(t)
	f.mutate(t, []components.InvItem{wetDryingItem(200, 10, 0, 0, 37)})
	ecs.WithComponent(f.w, f.inventory, func(container *components.InventoryContainer) { container.Items[0].Quantity = 2 })
	f.run(t, 110)
	inventory, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.EqualValues(t, 10, inventory.Items[0].TypeID)
	require.Empty(t, f.state(t).Entries)
	require.Zero(t, f.updates)
}

func TestDryingNewInputIdentityStartsFreshDeadline(t *testing.T) {
	f := newDryingFixture(t)
	f.mutate(t, []components.InvItem{wetDryingItem(200, 10, 0, 0, 37)})
	ecs.WithComponent(f.w, f.inventory, func(container *components.InventoryContainer) { container.Items[0].ItemID = 201 })
	f.run(t, 110)
	inventory, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.EqualValues(t, 10, inventory.Items[0].TypeID)
	require.EqualValues(t, 201, f.state(t).Entries[0].InputItemID)
	require.EqualValues(t, 120, f.state(t).Entries[0].CompletionRuntimeSeconds)
	require.Zero(t, f.updates)
}

func TestDryingPoolCompletesBatchWithQualityAndPosition(t *testing.T) {
	f := newDryingFixtureWithConfig(t, `{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a","dry_b"],"durationSeconds":10}]}`)
	inputs := []components.InvItem{
		wetDryingItem(200, 10, 0, 0, 0), wetDryingItem(201, 10, 1, 0, 8),
		wetDryingItem(202, 10, 0, 1, 100), wetDryingItem(203, 10, 1, 1, 37),
	}
	f.mutate(t, slices.Clone(inputs))
	before, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.True(t, f.run(t, 110).StateChanged)
	after, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.Equal(t, before.Version+1, after.Version)
	for i, output := range after.Items {
		require.Contains(t, []uint32{11, 21}, output.TypeID)
		definition, found := itemdefs.Global().GetByID(int(output.TypeID))
		require.True(t, found)
		require.Equal(t, definition.Resource, output.Resource)
		require.Equal(t, inputs[i].Quality, output.Quality)
		require.Equal(t, inputs[i].X, output.X)
		require.Equal(t, inputs[i].Y, output.Y)
		require.EqualValues(t, 1, output.Quantity)
		require.EqualValues(t, 1, output.W)
		require.EqualValues(t, 1, output.H)
		require.EqualValues(t, 1001+i, output.ItemID)
	}
	require.Empty(t, f.state(t).Entries)
	require.Equal(t, 1, f.updates)
	require.Zero(t, ecs.EnsureBehaviorRuntimeSchedule(f.w).PendingCount())
}

func TestDryingValidatesEveryPoolAlternativeBeforeBatchCommit(t *testing.T) {
	for _, invalid := range []string{"missing", "width", "height", "stack", "container", "grid"} {
		t.Run(invalid, func(t *testing.T) {
			f := newDryingFixtureWithConfig(t, `{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":10},{"inputItemKey":"wet_b","outputItemKeys":["dry_a","dry_b"],"durationSeconds":10}]}`)
			f.mutate(t, []components.InvItem{wetDryingItem(200, 10, 0, 0, 37), wetDryingItem(201, 20, 1, 0, 39)})
			before, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
			before.Items = slices.Clone(before.Items)
			entries := slices.Clone(f.state(t).Entries)
			ecs.WithComponent(f.w, f.frame, func(internal *components.ObjectInternalState) { internal.IsDirty = false })
			output, _ := itemdefs.Global().GetByKey("dry_b")
			switch invalid {
			case "missing":
				other, _ := itemdefs.Global().GetByKey("dry_a")
				itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{*other}))
			case "width":
				output.Size.W = 2
			case "height":
				output.Size.H = 2
			case "stack":
				output.Stack = &itemdefs.Stack{Mode: itemdefs.StackModeStack, Max: 2}
			case "container":
				output.Container = &itemdefs.ContainerDef{}
			case "grid":
				denied := false
				output.Allowed.Grid = &denied
			}
			result, err := f.behavior.OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, CurrentRuntimeSeconds: 110, Deps: f.deps})
			require.ErrorContains(t, err, "output definition changed")
			require.False(t, result.StateChanged)
			after, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
			require.Equal(t, before, after)
			require.Equal(t, entries, f.state(t).Entries)
			internal, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
			require.False(t, internal.IsDirty)
			require.Zero(t, f.updates)
			entry, due := ecs.EnsureBehaviorRuntimeSchedule(f.w).PopDue(111)
			require.True(t, due)
			require.EqualValues(t, 111, entry.DueRuntimeSeconds)
		})
	}
}

func TestDryingPreparesEveryPossibleFollowupDeadlineBeforeConversion(t *testing.T) {
	f := newDryingFixtureWithConfig(t, `{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a","wet_b"],"durationSeconds":10},{"inputItemKey":"wet_b","outputItemKeys":["dry_b"],"durationSeconds":25}]}`)
	f.mutate(t, []components.InvItem{wetDryingItem(200, 10, 0, 0, 37)})
	before, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	before.Items = slices.Clone(before.Items)
	result, err := f.behavior.OnScheduledRuntimeTick(&contracts.BehaviorRuntimeTickContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, CurrentRuntimeSeconds: math.MaxInt64 - 20, Deps: f.deps})
	require.ErrorContains(t, err, "runtime overflows")
	require.False(t, result.StateChanged)
	after, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.Equal(t, before, after)
	require.Len(t, f.state(t).Entries, 1)
	require.Zero(t, f.updates)
}

func TestDryingNoOpPreservesDeadlinesAndSwapRestartsOnlyMovedItems(t *testing.T) {
	f := newDryingFixture(t)
	items := []components.InvItem{wetDryingItem(200, 10, 0, 0, 37), wetDryingItem(201, 20, 1, 0, 39), wetDryingItem(202, 10, 0, 1, 12)}
	f.mutate(t, slices.Clone(items))
	original := slices.Clone(f.state(t).Entries)
	ecs.SetResource(f.w, ecs.TimeState{RuntimeSecondsTotal: 105})
	f.mutate(t, slices.Clone(items))
	require.Equal(t, original, f.state(t).Entries)
	items[0].X, items[1].X = items[1].X, items[0].X
	f.mutate(t, items)
	deadlines := make(map[types.EntityID]int64)
	for _, entry := range f.state(t).Entries {
		deadlines[entry.InputItemID] = entry.CompletionRuntimeSeconds
	}
	require.Equal(t, map[types.EntityID]int64{200: 115, 201: 130, 202: 110}, deadlines)
	f.run(t, 110)
	inventory, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	require.EqualValues(t, 10, inventory.Items[0].TypeID)
	require.EqualValues(t, 20, inventory.Items[1].TypeID)
	require.EqualValues(t, 11, inventory.Items[2].TypeID)
}

func TestDryingVisualFlagsReflectActualInventoryCombinations(t *testing.T) {
	for mask := 0; mask < 8; mask++ {
		f := newDryingFixtureWithConfig(t, `{"processes":[{"inputItemKey":"wet_a","outputItemKeys":["dry_a"],"durationSeconds":10,"visualCategory":"herbs"},{"inputItemKey":"wet_b","outputItemKeys":["dry_b"],"durationSeconds":25,"visualCategory":"hide"}]}`)
		var items []components.InvItem
		var want []string
		for i, typeID := range []uint32{10, 20, 21} {
			if mask&(1<<i) != 0 {
				items = append(items, wetDryingItem(types.EntityID(200+i), typeID, uint8(i%2), uint8(i/2), 37))
				want = append(want, []string{"drying.herbs", "drying.raw_hide", "drying.dried_hide"}[i])
			}
		}
		f.mutate(t, items)
		result := f.behavior.ApplyRuntime(&contracts.BehaviorRuntimeContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1})
		require.Equal(t, want, result.Flags)
		f.run(t, 110)
		result = f.behavior.ApplyRuntime(&contracts.BehaviorRuntimeContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1})
		require.Equal(t, want, result.Flags, "ready herbs keep the same flag")
		f.mutate(t, nil)
		require.Empty(t, f.behavior.ApplyRuntime(&contracts.BehaviorRuntimeContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1}).Flags)
	}
}

func TestDryingVisualInputClassificationTakesPrecedence(t *testing.T) {
	for _, category := range []string{"hide", ""} {
		cfg := contracts.DryingBehaviorConfig{Processes: []contracts.DryingProcessConfig{
			{InputItemKey: "wet_a", OutputItemKeys: []string{"wet_b"}, DurationSeconds: 10, VisualCategory: "herbs"},
			{InputItemKey: "wet_b", OutputItemKeys: []string{"dry_b"}, DurationSeconds: 25, VisualCategory: category},
		}}
		raw, err := json.Marshal(cfg)
		require.NoError(t, err)
		f := newDryingFixtureWithConfig(t, string(raw))
		f.mutate(t, []components.InvItem{wetDryingItem(200, 20, 0, 0, 37)})
		result := f.behavior.ApplyRuntime(&contracts.BehaviorRuntimeContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1})
		if category == "hide" {
			require.Equal(t, []string{"drying.raw_hide"}, result.Flags)
		} else {
			require.Empty(t, result.Flags)
		}
	}
}
