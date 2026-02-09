package systems

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

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
			Behavior: []string{"container"},
		},
	}))

	w := ecs.NewWorldForTesting()
	sys := NewObjectBehaviorSystem(nil, nil)

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
