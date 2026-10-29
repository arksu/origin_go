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
	calls  int
	onTick func(*contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error)
}

func (b *testScheduledTickBehavior) Key() string {
	return "grow"
}

func (b *testScheduledTickBehavior) OnScheduledTick(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
	b.calls++
	if b.onTick != nil {
		return b.onTick(ctx)
	}
	return contracts.BehaviorTickResult{StateChanged: true}, nil
}

func TestBehaviorTickSystem_RechecksCanceledOrReplacedBatchEntries(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprintf("replace_%t", replace), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			for id := types.EntityID(1); id <= 3; id++ {
				w.Spawn(id, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.EntityInfo{Behaviors: []string{"grow"}})
				})
				ecs.ScheduleBehaviorTick(w, id, "grow", 1)
			}
			var called []types.EntityID
			behavior := &testScheduledTickBehavior{onTick: func(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
				called = append(called, ctx.EntityID)
				if ctx.EntityID == 1 {
					if replace {
						ecs.ScheduleBehaviorTick(w, 2, "grow", 20)
					} else {
						ecs.CancelBehaviorTick(w, 2, "grow")
					}
				}
				return contracts.BehaviorTickResult{}, nil
			}}
			system := NewBehaviorTickSystem(nil, BehaviorTickSystemConfig{BudgetPerTick: 2, BehaviorRegistry: &testBehaviorTickRegistry{byKey: map[string]contracts.Behavior{"grow": behavior}}})
			ecs.GetResource[ecs.TimeState](w).Tick = 1
			system.Update(w, 0)
			if fmt.Sprint(called) != "[1]" {
				t.Fatalf("canceled/replaced callback executed: %v", called)
			}
			system.Update(w, 0)
			if fmt.Sprint(called) != "[1 3]" {
				t.Fatalf("canceled entry did not spend batch budget: %v", called)
			}
			ecs.GetResource[ecs.TimeState](w).Tick = 20
			system.Update(w, 0)
			want := "[1 3]"
			if replace {
				want = "[1 3 2]"
			}
			if fmt.Sprint(called) != want {
				t.Fatalf("replacement deadline lost: got%v, want%s", called, want)
			}
			if ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount() != 0 {
				t.Fatal("one-shot registration retained")
			}
		})
	}
}

func TestBehaviorTickSystem_RecurringCallbackDoesNotAllocate(t *testing.T) {
	for _, population := range []int{1, 200} {
		t.Run(fmt.Sprintf("population_%d", population), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			for entityID := types.EntityID(1); entityID <= types.EntityID(population); entityID++ {
				w.Spawn(entityID, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.EntityInfo{Behaviors: []string{"grow"}})
				})
				ecs.ScheduleBehaviorTick(w, entityID, "grow", 1)
			}
			behavior := &testScheduledTickBehavior{onTick: func(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
				ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, ctx.BehaviorKey, ctx.CurrentTick+1)
				return contracts.BehaviorTickResult{}, nil
			}}
			system := NewBehaviorTickSystem(nil, BehaviorTickSystemConfig{
				BudgetPerTick:    population,
				BehaviorRegistry: &testBehaviorTickRegistry{byKey: map[string]contracts.Behavior{"grow": behavior}},
			})
			timeState := ecs.GetResource[ecs.TimeState](w)
			allocs := testing.AllocsPerRun(1000, func() {
				timeState.Tick++
				system.Update(w, 0)
			})
			if allocs != 0 {
				t.Fatalf("due callback and rearm allocated after warmup: %g allocs/run", allocs)
			}
			if behavior.calls != 1001*population || ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount() != population {
				t.Fatalf("measurement did not repeatedly execute and rearm: calls=%d", behavior.calls)
			}
		})
	}
}

func TestBehaviorTickSystem_RearmWaitsUntilNextUpdate(t *testing.T) {
	w := ecs.NewWorldForTesting()
	w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{Behaviors: []string{"grow"}})
	})
	behavior := &testScheduledTickBehavior{onTick: func(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
		ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, ctx.BehaviorKey, ctx.CurrentTick)
		return contracts.BehaviorTickResult{}, nil
	}}
	system := NewBehaviorTickSystem(nil, BehaviorTickSystemConfig{
		BudgetPerTick:    8,
		BehaviorRegistry: &testBehaviorTickRegistry{byKey: map[string]contracts.Behavior{"grow": behavior}},
	})
	ecs.ScheduleBehaviorTick(w, 1, "grow", 0)
	for update := 1; update <= 3; update++ {
		system.Update(w, 0)
		if behavior.calls != update || ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount() != 1 {
			t.Fatalf("immediate tick rearm changed batch semantics: calls=%d update=%d", behavior.calls, update)
		}
	}
}

func TestBehaviorTickSystem_TickContextAndErrorContinue(t *testing.T) {
	w := ecs.NewWorldForTesting()
	state := &components.RuntimeObjectState{Behaviors: map[string]any{"grow": "test-state"}}
	deps := &contracts.ExecutionDeps{}
	var contexts []contracts.BehaviorTickContext
	behavior := &testScheduledTickBehavior{onTick: func(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
		contexts = append(contexts, *ctx)
		if ctx.EntityID == 1 {
			return contracts.BehaviorTickResult{}, fmt.Errorf("test failure")
		}
		return contracts.BehaviorTickResult{}, nil
	}}
	system := NewBehaviorTickSystem(nil, BehaviorTickSystemConfig{
		BudgetPerTick:    8,
		BehaviorRegistry: &testBehaviorTickRegistry{byKey: map[string]contracts.Behavior{"grow": behavior}},
		ExecutionDeps:    deps,
	})
	var handles [2]types.Handle
	for entityID := types.EntityID(1); entityID <= 2; entityID++ {
		handles[entityID-1] = w.Spawn(entityID, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.EntityInfo{TypeID: 77, Behaviors: []string{"grow"}})
			ecs.AddComponent(w, h, components.ObjectInternalState{State: state})
		})
		ecs.ScheduleBehaviorTick(w, entityID, "grow", 10)
	}
	ecs.GetResource[ecs.TimeState](w).Tick = 10
	system.Update(w, 0)
	if len(contexts) != 2 {
		t.Fatalf("error stopped the remaining callback: got %d calls", len(contexts))
	}
	for i, ctx := range contexts {
		if ctx.World != w || ctx.Handle != handles[i] || ctx.EntityID != types.EntityID(i+1) || ctx.EntityType != 77 || ctx.BehaviorKey != "grow" || ctx.CurrentTick != 10 || ctx.CurrentState != state || ctx.Deps != deps {
			t.Fatalf("incorrect tick callback context: %+v", ctx)
		}
	}
	if system.tickContext != (contracts.BehaviorTickContext{}) {
		t.Fatal("borrowed context retained callback references")
	}
	if ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount() != 0 {
		t.Fatal("one-shot callbacks left pending deadlines")
	}
}

func TestBehaviorTickSystem_InvalidTickEntriesSpendBudget(t *testing.T) {
	w := ecs.NewWorldForTesting()
	behavior := &testRuntimeScheduledBehavior{}
	system := newRuntimeScheduledTestSystem(behavior, 2, nil)
	for entityID := types.EntityID(1); entityID <= 3; entityID++ {
		handle := spawnRuntimeScheduledTestObject(w, entityID)
		if entityID < 3 {
			ecs.RemoveComponent[components.EntityInfo](w, handle)
		}
		ecs.ScheduleBehaviorTick(w, entityID, "scheduled", 1)
		if entityID == 3 {
			ecs.ScheduleBehaviorRuntime(w, handle, entityID, "scheduled", 0)
		}
	}
	ecs.GetResource[ecs.TimeState](w).Tick = 1
	system.Update(w, 0)
	if len(behavior.calls) != 0 || ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount() != 1 || ecs.GetResource[ecs.BehaviorRuntimeSchedule](w).PendingCount() != 1 {
		t.Fatal("invalid tick entries bypassed the shared work budget")
	}
	system.Update(w, 0)
	if fmt.Sprint(behavior.calls) != fmt.Sprint([]string{"tick:3", "runtime:3"}) {
		t.Fatalf("deferred tick and runtime work lost ordering: %v", behavior.calls)
	}
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
