package systems

import (
	"fmt"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"
)

type testBehaviorTickRegistry struct {
	byKey map[string]contracts.Behavior
}

func (r *testBehaviorTickRegistry) GetBehavior(key string) (contracts.Behavior, bool) {
	behavior, ok := r.byKey[key]
	return behavior, ok
}

func (r *testBehaviorTickRegistry) Keys() []string {
	keys := make([]string, 0, len(r.byKey))
	for key := range r.byKey {
		keys = append(keys, key)
	}
	return keys
}

func (r *testBehaviorTickRegistry) IsRegisteredBehaviorKey(key string) bool {
	_, ok := r.byKey[key]
	return ok
}

func (r *testBehaviorTickRegistry) ValidateBehaviorKeys(keys []string) error {
	for _, key := range keys {
		if !r.IsRegisteredBehaviorKey(key) {
			return fmt.Errorf("unknown behavior %q", key)
		}
	}
	return nil
}

func (r *testBehaviorTickRegistry) InitObjectBehaviors(_ *contracts.BehaviorObjectInitContext, _ []string) error {
	return nil
}

type testScheduledTickBehavior struct {
	calls int
}

func (b *testScheduledTickBehavior) Key() string {
	return "grow"
}

func (b *testScheduledTickBehavior) OnScheduledTick(_ *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
	b.calls++
	return contracts.BehaviorTickResult{StateChanged: true}, nil
}

func TestBehaviorTickSystem_MarksDirtyOnStateChange(t *testing.T) {
	world := ecs.NewWorldForTesting()
	behavior := &testScheduledTickBehavior{}
	registry := &testBehaviorTickRegistry{
		byKey: map[string]contracts.Behavior{
			"grow": behavior,
		},
	}
	system := NewBehaviorTickSystem(nil, BehaviorTickSystemConfig{
		BudgetPerTick:    8,
		BehaviorRegistry: registry,
	})

	entityID := types.EntityID(2001)
	handle := world.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    1,
			Behaviors: []string{"grow"},
			IsStatic:  true,
		})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.Appearance{Resource: "test"})
	})

	ecs.ScheduleBehaviorTick(world, entityID, "grow", 1)
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 1}

	system.Update(world, 0.05)

	if behavior.calls != 1 {
		t.Fatalf("expected scheduled behavior to be called once, got %d", behavior.calls)
	}

	drained := ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](world).Drain(8, nil)
	if len(drained) != 1 || drained[0] != handle {
		t.Fatalf("expected object to be marked dirty, got %+v", drained)
	}
}

type testRuntimeScheduledBehavior struct {
	calls     []string
	contexts  []contracts.BehaviorRuntimeTickContext
	onRuntime func(*contracts.BehaviorRuntimeTickContext)
	result    contracts.BehaviorTickResult
}

func (b *testRuntimeScheduledBehavior) Key() string { return "scheduled" }

func (b *testRuntimeScheduledBehavior) OnScheduledTick(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
	b.calls = append(b.calls, fmt.Sprintf("tick:%d", ctx.EntityID))
	return b.result, nil
}

func (b *testRuntimeScheduledBehavior) OnScheduledRuntimeTick(ctx *contracts.BehaviorRuntimeTickContext) (contracts.BehaviorTickResult, error) {
	b.calls = append(b.calls, fmt.Sprintf("runtime:%d", ctx.EntityID))
	b.contexts = append(b.contexts, *ctx)
	if b.onRuntime != nil {
		b.onRuntime(ctx)
	}
	return b.result, nil
}

func spawnRuntimeScheduledTestObject(w *ecs.World, entityID types.EntityID) types.Handle {
	return w.Spawn(entityID, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{TypeID: 77, Behaviors: []string{"scheduled"}})
		ecs.AddComponent(w, handle, components.ObjectInternalState{})
	})
}

func newRuntimeScheduledTestSystem(behavior *testRuntimeScheduledBehavior, budget int, deps *contracts.ExecutionDeps) *BehaviorTickSystem {
	return NewBehaviorTickSystem(nil, BehaviorTickSystemConfig{
		BudgetPerTick:    budget,
		BehaviorRegistry: &testBehaviorTickRegistry{byKey: map[string]contracts.Behavior{"scheduled": behavior}},
		ExecutionDeps:    deps,
	})
}

func TestBehaviorTickSystemRuntimeSharesBudgetAfterTickDeadlines(t *testing.T) {
	w := ecs.NewWorldForTesting()
	behavior := &testRuntimeScheduledBehavior{}
	system := newRuntimeScheduledTestSystem(behavior, 4, nil)
	for entityID := types.EntityID(1); entityID <= 3; entityID++ {
		handle := spawnRuntimeScheduledTestObject(w, entityID)
		ecs.ScheduleBehaviorTick(w, entityID, "scheduled", 10)
		ecs.ScheduleBehaviorRuntime(w, handle, entityID, "scheduled", 10)
	}
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 10, RuntimeSecondsTotal: 10}
	system.Update(w, 0)
	want := []string{"tick:1", "tick:2", "tick:3", "runtime:1"}
	if fmt.Sprint(behavior.calls) != fmt.Sprint(want) {
		t.Fatalf("tick deadlines lost precedence or budget was exceeded: got %v, want %v", behavior.calls, want)
	}
	if pending := ecs.GetResource[ecs.BehaviorRuntimeSchedule](w).PendingCount(); pending != 2 {
		t.Fatalf("runtime work exceeded remaining budget; pending=%d", pending)
	}
	system.Update(w, 0)
	want = append(want, "runtime:2", "runtime:3")
	if fmt.Sprint(behavior.calls) != fmt.Sprint(want) {
		t.Fatalf("deferred runtime entries did not execute next update: %v", behavior.calls)
	}
}

func TestBehaviorTickSystemRuntimeClockAndContext(t *testing.T) {
	w := ecs.NewWorldForTesting()
	state := &components.RuntimeObjectState{Behaviors: map[string]any{"scheduled": "test-state"}}
	handle := spawnRuntimeScheduledTestObject(w, 10)
	ecs.MutateComponent[components.ObjectInternalState](w, handle, func(internal *components.ObjectInternalState) bool {
		internal.State = state
		return true
	})
	deps := &contracts.ExecutionDeps{}
	behavior := &testRuntimeScheduledBehavior{result: contracts.BehaviorTickResult{StateChanged: true}}
	system := newRuntimeScheduledTestSystem(behavior, 2, deps)
	ecs.ScheduleBehaviorRuntime(w, handle, 10, "scheduled", 100)
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 10000, RuntimeSecondsTotal: 99}
	system.Update(w, 0)
	if len(behavior.calls) != 0 {
		t.Fatal("tick advancement fired runtime deadline early")
	}
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 10000, RuntimeSecondsTotal: 101}
	system.Update(w, 0)
	if len(behavior.contexts) != 1 {
		t.Fatalf("runtime advancement without ticks did not execute: %v", behavior.calls)
	}
	ctx := behavior.contexts[0]
	if ctx.World != w || ctx.Handle != handle || ctx.EntityID != 10 || ctx.EntityType != 77 || ctx.BehaviorKey != "scheduled" || ctx.CurrentRuntimeSeconds != 101 || ctx.CurrentState != state || ctx.Deps != deps {
		t.Fatalf("incorrect runtime callback context: %+v", ctx)
	}
	dirty := ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](w).Drain(2, nil)
	if len(dirty) != 1 || dirty[0] != handle {
		t.Fatalf("state change was not queued for runtime recomputation: %v", dirty)
	}
}

func TestBehaviorTickSystemRuntimeRejectsStaleOrChangedObjects(t *testing.T) {
	for _, scenario := range []string{"stale-generation", "entity-id-mismatch", "removed-behavior", "missing-entity-info", "pending-destruction"} {
		t.Run(scenario, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			behavior := &testRuntimeScheduledBehavior{}
			system := newRuntimeScheduledTestSystem(behavior, 2, nil)
			handle := spawnRuntimeScheduledTestObject(w, 10)
			entityID := types.EntityID(10)
			switch scenario {
			case "stale-generation":
				w.Despawn(handle)
				spawnRuntimeScheduledTestObject(w, entityID)
			case "entity-id-mismatch":
				entityID = 11
			case "removed-behavior":
				ecs.MutateComponent[components.EntityInfo](w, handle, func(info *components.EntityInfo) bool {
					info.Behaviors = nil
					return true
				})
			case "missing-entity-info":
				ecs.RemoveComponent[components.EntityInfo](w, handle)
			case "pending-destruction":
				ecs.InitResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{handle: true}})
			}
			ecs.ScheduleBehaviorRuntime(w, handle, entityID, "scheduled", 1)
			*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{RuntimeSecondsTotal: 1}
			system.Update(w, 0)
			if len(behavior.calls) != 0 || ecs.GetResource[ecs.BehaviorRuntimeSchedule](w).PendingCount() != 0 {
				t.Fatalf("invalid runtime callback was not discarded: %v", behavior.calls)
			}
		})
	}
}

func TestBehaviorTickSystemRuntimeInvalidEntriesStillSpendBudget(t *testing.T) {
	w := ecs.NewWorldForTesting()
	behavior := &testRuntimeScheduledBehavior{}
	system := newRuntimeScheduledTestSystem(behavior, 2, nil)
	for entityID := types.EntityID(1); entityID <= 3; entityID++ {
		handle := spawnRuntimeScheduledTestObject(w, entityID)
		if entityID < 3 {
			ecs.RemoveComponent[components.EntityInfo](w, handle)
		}
		ecs.ScheduleBehaviorRuntime(w, handle, entityID, "scheduled", 1)
	}
	*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{RuntimeSecondsTotal: 1}
	system.Update(w, 0)
	if len(behavior.calls) != 0 || ecs.GetResource[ecs.BehaviorRuntimeSchedule](w).PendingCount() != 1 {
		t.Fatal("invalid runtime entries bypassed the bounded work budget")
	}
	system.Update(w, 0)
	if len(behavior.calls) != 1 || behavior.calls[0] != "runtime:3" {
		t.Fatalf("remaining valid entry was not executed next update: %v", behavior.calls)
	}
}

func TestBehaviorTickSystemRuntimeImmediateReschedulingIsBounded(t *testing.T) {
	w := ecs.NewWorldForTesting()
	handle := spawnRuntimeScheduledTestObject(w, 1)
	behavior := &testRuntimeScheduledBehavior{onRuntime: func(ctx *contracts.BehaviorRuntimeTickContext) {
		ecs.ScheduleBehaviorRuntime(ctx.World, ctx.Handle, ctx.EntityID, ctx.BehaviorKey, ctx.CurrentRuntimeSeconds)
	}}
	system := newRuntimeScheduledTestSystem(behavior, 3, nil)
	ecs.ScheduleBehaviorRuntime(w, handle, 1, "scheduled", 0)
	system.Update(w, 0)
	if len(behavior.calls) != 3 || ecs.GetResource[ecs.BehaviorRuntimeSchedule](w).PendingCount() != 1 {
		t.Fatalf("immediate requeue exceeded runtime budget: calls=%d", len(behavior.calls))
	}
}
