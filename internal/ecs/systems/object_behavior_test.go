package systems

import (
	"encoding/json"
	"fmt"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

type testRuntimeBehaviorRegistry struct {
	byKey map[string]types.Behavior
}

func (r *testRuntimeBehaviorRegistry) GetBehavior(key string) (types.Behavior, bool) {
	behavior, ok := r.byKey[key]
	return behavior, ok
}

func (r *testRuntimeBehaviorRegistry) Keys() []string {
	keys := make([]string, 0, len(r.byKey))
	for key := range r.byKey {
		keys = append(keys, key)
	}
	return keys
}

func (r *testRuntimeBehaviorRegistry) IsRegisteredBehaviorKey(key string) bool {
	_, ok := r.byKey[key]
	return ok
}

func (r *testRuntimeBehaviorRegistry) ValidateBehaviorKeys(keys []string) error {
	for _, key := range keys {
		if !r.IsRegisteredBehaviorKey(key) {
			return fmt.Errorf("unknown behavior %q", key)
		}
	}
	return nil
}

func (r *testRuntimeBehaviorRegistry) InitObjectBehaviors(_ *types.BehaviorObjectInitContext, _ []string) error {
	return nil
}

type testContainerRuntimeBehavior struct{}

func (testContainerRuntimeBehavior) Key() string { return "container" }

func (testContainerRuntimeBehavior) ApplyRuntime(ctx *types.BehaviorRuntimeContext) types.BehaviorRuntimeResult {
	world, ok := ctx.World.(*ecs.World)
	if !ok || world == nil {
		return types.BehaviorRuntimeResult{}
	}
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, ctx.EntityID, 0)
	if !found || !world.Alive(rootHandle) {
		return types.BehaviorRuntimeResult{}
	}
	rootContainer, hasContainer := ecs.GetComponent[components.InventoryContainer](world, rootHandle)
	if !hasContainer || len(rootContainer.Items) == 0 {
		return types.BehaviorRuntimeResult{}
	}
	return types.BehaviorRuntimeResult{
		Flags: []string{"container.has_items"},
	}
}

func TestObjectBehaviorSystem_ContainerFlagsAndAppearance(t *testing.T) {
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID:    10,
			Key:      "box",
			Resource: "obj/box/box.png",
			Appearance: []objectdefs.Appearance{
				{
					ID: "full",
					When: &objectdefs.AppearanceWhen{
						Flags: []string{"container.has_items"},
					},
					Resource: "obj/box/box_open.png",
				},
			},
			Behaviors: map[string]json.RawMessage{
				"container": json.RawMessage("{}"),
			},
			BehaviorOrder: []string{"container"},
		},
	}))

	w := ecs.NewWorldForTesting()
	behaviorRegistry := &testRuntimeBehaviorRegistry{
		byKey: map[string]types.Behavior{
			"container": testContainerRuntimeBehavior{},
		},
	}
	sys := NewObjectBehaviorSystem(nil, nil, ObjectBehaviorConfig{
		BudgetPerTick:       512,
		EnableDebugFallback: false,
		BehaviorRegistry:    behaviorRegistry,
	})

	objectID := types.EntityID(500)
	objectHandle := w.Spawn(objectID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    10,
			Behaviors: []string{"container"},
			IsStatic:  true,
			Region:    1,
			Layer:     0,
		})
		ecs.AddComponent(w, h, components.Appearance{
			Resource: "obj/box/box.png",
		})
		ecs.AddComponent(w, h, components.ObjectInternalState{
			IsDirty: false,
		})
	})

	containerHandle := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, containerHandle, components.InventoryContainer{
		OwnerID: objectID,
		Kind:    constt.InventoryGrid,
		Key:     0,
		Width:   5,
		Height:  5,
		Items: []components.InvItem{
			{ItemID: 1001, TypeID: 1, Quantity: 1, W: 1, H: 1},
		},
	})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, objectID, 0, containerHandle)
	ecs.MarkObjectBehaviorDirty(w, objectHandle)

	sys.Update(w, 0.05)

	state, _ := ecs.GetComponent[components.ObjectInternalState](w, objectHandle)
	if !state.IsDirty {
		t.Fatalf("expected object to be marked dirty after flags/resource change")
	}
	if len(state.Flags) != 1 || state.Flags[0] != "container.has_items" {
		t.Fatalf("unexpected flags: %#v", state.Flags)
	}

	appearance, _ := ecs.GetComponent[components.Appearance](w, objectHandle)
	if appearance.Resource != "obj/box/box_open.png" {
		t.Fatalf("unexpected appearance resource: got %q", appearance.Resource)
	}

	ecs.WithComponent(w, objectHandle, func(s *components.ObjectInternalState) {
		s.IsDirty = false
	})
	sys.Update(w, 0.05)
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, objectHandle)
	if state.IsDirty {
		t.Fatalf("expected object to remain clean when nothing changed")
	}

	ecs.WithComponent(w, containerHandle, func(c *components.InventoryContainer) {
		c.Items = nil
	})
	ecs.MarkObjectBehaviorDirty(w, objectHandle)
	sys.Update(w, 0.05)
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, objectHandle)
	if !state.IsDirty {
		t.Fatalf("expected dirty after container became empty")
	}
	if len(state.Flags) != 0 {
		t.Fatalf("expected empty flags after container emptied, got %#v", state.Flags)
	}
	appearance, _ = ecs.GetComponent[components.Appearance](w, objectHandle)
	if appearance.Resource != "obj/box/box.png" {
		t.Fatalf("unexpected appearance resource after empty: got %q", appearance.Resource)
	}
}
