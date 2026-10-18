package behaviors

import (
	"encoding/json"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func dryingProgressContext(f *dryingFixture, item components.InvItem) *contracts.InventoryItemProgressContext {
	container, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	return &contracts.InventoryItemProgressContext{World: f.w, Handle: f.frame, EntityID: 100, EntityType: 1, Container: container, Item: item}
}

func TestDryingItemProgressProjectsIndependentDeadlinesWithoutMutation(t *testing.T) {
	f := newDryingFixture(t)
	first, second := wetDryingItem(200, 10, 0, 0, 37), wetDryingItem(201, 20, 1, 0, 8)
	f.mutate(t, []components.InvItem{first})
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 105
	f.mutate(t, []components.InvItem{first, second})
	ecs.WithComponent(f.w, f.frame, func(state *components.ObjectInternalState) { state.IsDirty = false })
	internal, _ := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
	before, err := json.Marshal(internal)
	require.NoError(t, err)
	container, _ := ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	beforeContainer, err := json.Marshal(container)
	require.NoError(t, err)
	queued := ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](f.w).PendingCount()

	// A due but not yet executed process still describes the authoritative input.
	ecs.GetResource[ecs.TimeState](f.w).RuntimeSecondsTotal = 1000
	for _, scenario := range []struct {
		item               components.InvItem
		duration, deadline int64
	}{{first, 10, 110}, {second, 25, 130}} {
		progress := f.behavior.InventoryItemProgress(dryingProgressContext(f, scenario.item))
		require.NotNil(t, progress)
		require.Equal(t, scenario.duration, progress.DurationSeconds)
		require.Equal(t, scenario.deadline, progress.GetCompletionRuntimeSeconds())
		progress.DurationSeconds = 1 // Network snapshots own their payload.
		require.Equal(t, scenario.duration, f.behavior.InventoryItemProgress(dryingProgressContext(f, scenario.item)).DurationSeconds)
	}
	internal, _ = ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
	after, err := json.Marshal(internal)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	container, _ = ecs.GetComponent[components.InventoryContainer](f.w, f.inventory)
	afterContainer, err := json.Marshal(container)
	require.NoError(t, err)
	require.Equal(t, string(beforeContainer), string(afterContainer))
	require.Equal(t, queued, ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](f.w).PendingCount())
	require.Zero(t, f.updates)
	schedule := ecs.GetResource[ecs.BehaviorRuntimeSchedule](f.w)
	require.Equal(t, 1, schedule.PendingCount())
	_, due := schedule.PopDue(109)
	require.False(t, due)
	entry, due := schedule.PopDue(110)
	require.True(t, due)
	require.Equal(t, int64(110), entry.DueRuntimeSeconds, "projection must not reschedule")
}

func TestDryingItemProgressRejectsStaleOrIneligibleStateWithoutRepair(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*dryingFixture, *contracts.InventoryItemProgressContext)
	}{
		{"wrong owner id", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.EntityID++ }},
		{"wrong owner type", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.EntityType++ }},
		{"wrong container owner", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Container.OwnerID++ }},
		{"nested inventory", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Container.Key = 1 }},
		{"hand", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) {
			ctx.Container.Kind = constt.InventoryHand
		}},
		{"stale generation", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			f.w.Despawn(f.frame)
			f.w.Spawn(100, nil)
		}},
		{"missing runtime state", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			ecs.WithComponent(f.w, f.frame, func(state *components.ObjectInternalState) { state.State = nil })
		}},
		{"missing entry", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) { f.state(t).Entries = nil }},
		{"invalid deadline", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			f.state(t).Entries[0].CompletionRuntimeSeconds = 0
		}},
		{"too many entries", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			f.state(t).Entries = make([]components.DryingItemState, components.MaxDryingItems+1)
		}},
		{"no recipe", func(_ *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			def, _ := objectdefs.Global().GetByID(1)
			def.DryingConfig.Processes = nil
		}},
		{"zero duration", func(_ *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			def, _ := objectdefs.Global().GetByID(1)
			def.DryingConfig.Processes[0].DurationSeconds = 0
		}},
		{"output item", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Item.TypeID = 11 }},
		{"different item", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Item.ItemID++ }},
		{"different cell", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Item.X = 1 }},
		{"stacked input", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Item.Quantity = 2 }},
		{"invalid size", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Item.W = 2 }},
		{"outside grid", func(_ *dryingFixture, ctx *contracts.InventoryItemProgressContext) { ctx.Item.X = 2 }},
		{"removed input", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			ecs.WithComponent(f.w, f.inventory, func(c *components.InventoryContainer) { c.Items = nil })
		}},
		{"moved live input", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			ecs.WithComponent(f.w, f.inventory, func(c *components.InventoryContainer) { c.Items[0].X = 1 })
		}},
		{"invalid live input", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			ecs.WithComponent(f.w, f.inventory, func(c *components.InventoryContainer) { c.Items[0].Quantity = 2 })
		}},
		{"too many live items", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			ecs.WithComponent(f.w, f.inventory, func(c *components.InventoryContainer) {
				c.Items = append(c.Items, make([]components.InvItem, components.MaxDryingItems)...)
			})
		}},
		{"wrong root owner", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			ecs.WithComponent(f.w, f.inventory, func(c *components.InventoryContainer) { c.OwnerID++ })
		}},
		{"wrong root size", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) {
			ecs.WithComponent(f.w, f.inventory, func(c *components.InventoryContainer) { c.Width = 3 })
		}},
		{"missing inventory", func(f *dryingFixture, _ *contracts.InventoryItemProgressContext) { f.w.Despawn(f.inventory) }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newDryingFixture(t)
			item := wetDryingItem(200, 10, 0, 0, 37)
			f.mutate(t, []components.InvItem{item})
			ctx := dryingProgressContext(f, item)
			scenario.change(f, ctx)
			before, found := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
			beforeJSON, err := json.Marshal(before)
			require.NoError(t, err)
			require.Nil(t, f.behavior.InventoryItemProgress(ctx))
			after, stillFound := ecs.GetComponent[components.ObjectInternalState](f.w, f.frame)
			require.Equal(t, found, stillFound)
			afterJSON, err := json.Marshal(after)
			require.NoError(t, err)
			require.Equal(t, string(beforeJSON), string(afterJSON))
		})
	}
	require.Nil(t, (dryingBehavior{}).InventoryItemProgress(nil))
}

func TestDryingItemProgressMissingStateDoesNotCreateScheduler(t *testing.T) {
	f := newDryingFixture(t)
	w := ecs.NewWorld(nil, 0)
	frame := w.Spawn(100, nil)
	ecs.AddComponent(w, frame, components.EntityInfo{TypeID: 1})
	ecs.AddComponent(w, frame, components.ObjectInternalState{})
	item := wetDryingItem(200, 10, 0, 0, 37)
	container := components.InventoryContainer{OwnerID: 100, Kind: constt.InventoryGrid, Width: 2, Height: 2, Items: []components.InvItem{item}}
	root := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, root, container)
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, 100, 0, root)
	require.False(t, ecs.HasResource[ecs.BehaviorRuntimeSchedule](w))
	require.Nil(t, f.behavior.InventoryItemProgress(&contracts.InventoryItemProgressContext{
		World: w, Handle: frame, EntityID: types.EntityID(100), EntityType: 1, Container: container, Item: item,
	}))
	require.False(t, ecs.HasResource[ecs.BehaviorRuntimeSchedule](w))
	internal, _ := ecs.GetComponent[components.ObjectInternalState](w, frame)
	require.Nil(t, internal.State)
	require.False(t, internal.IsDirty)
}
