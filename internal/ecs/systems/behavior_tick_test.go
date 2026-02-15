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
