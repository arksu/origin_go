package inventory

import (
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func objectLootCaptureFixture() (*ecs.World, *itemdefs.Registry, types.Handle, []ecs.InventoryRefEntry) {
	w := ecs.NewWorldWithCapacity(64, nil, 0)
	target := w.Spawn(100, nil)
	ecs.AddComponent(w, target, components.ObjectInternalState{HP: 0, HasHP: true})
	registry := itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1, Key: "ore", Resource: "ore", Size: itemdefs.Size{W: 1, H: 1}, Stack: &itemdefs.Stack{Mode: itemdefs.StackModeStack, Max: 1000}},
		{DefID: 2, Key: "bag", Resource: "bag", Size: itemdefs.Size{W: 1, H: 1}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 4, H: 4}}},
	})
	addObjectLootContainer(w, components.InventoryContainer{
		OwnerID: 100, Kind: constt.InventoryGrid, Key: 5, Version: 7, Width: 4, Height: 4,
		Items: []components.InvItem{{ItemID: 101, TypeID: 1, Resource: "ore", Quality: 12, Quantity: 3, W: 1, H: 1}},
	})
	addObjectLootContainer(w, components.InventoryContainer{
		OwnerID: 100, Kind: constt.InventoryGrid, Key: 0, Version: 1, Width: 4, Height: 4,
		Items: []components.InvItem{{ItemID: 102, TypeID: 2, Quality: 10, Quantity: 1, W: 1, H: 1}},
	})
	addObjectLootContainer(w, components.InventoryContainer{
		OwnerID: 102, Kind: constt.InventoryGrid, Version: 2, Width: 4, Height: 4,
		Items: []components.InvItem{{ItemID: 103, TypeID: 2, Quality: 20, Quantity: 1, W: 1, H: 1, X: 1}},
	})
	addObjectLootContainer(w, components.InventoryContainer{
		OwnerID: 103, Kind: constt.InventoryGrid, Version: 4, Width: 4, Height: 4,
		Items: []components.InvItem{{ItemID: 104, TypeID: 1, Quality: 30, Quantity: 5, W: 1, H: 1, X: 2, Y: 1}},
	})
	roots := ecs.GetResource[ecs.InventoryRefIndex](w).EntriesByOwnerInto(100, nil)
	return w, registry, target, roots
}

func addObjectLootContainer(w *ecs.World, container components.InventoryContainer) types.Handle {
	handle := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, handle, container)
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(container.Kind, container.OwnerID, container.Key, handle)
	return handle
}

func finishObjectLootCapture(t *testing.T, capture *ObjectLootCapture, w *ecs.World, registry *itemdefs.Registry, budget int) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		done, err := capture.CaptureBatch(w, registry, budget)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("capture did not complete")
}

func TestObjectLootCaptureOwnsAllRootsAndNestedTrees(t *testing.T) {
	w, registry, _, roots := objectLootCaptureFixture()
	capture := NewObjectLootCapture(100, roots)
	roots[0].Handle = types.InvalidHandle // Constructor owns its reference snapshot.
	done, err := capture.CaptureBatch(w, registry, 1)
	require.NoError(t, err)
	require.False(t, done)
	require.Nil(t, capture.Items())
	require.Nil(t, capture.ContainerRefs())
	finishObjectLootCapture(t, capture, w, registry, 1)
	items := capture.Items()
	require.Len(t, items, 2)
	require.Equal(t, types.EntityID(102), items[0].ItemID)
	require.Equal(t, types.EntityID(101), items[1].ItemID)
	require.Equal(t, uint32(3), items[1].Quantity)
	require.Equal(t, uint32(12), items[1].Quality)
	inner := items[0].NestedInventory.Items[0].NestedInventory
	require.NotNil(t, inner)
	require.Equal(t, uint64(104), inner.Items[0].ItemID)
	require.Equal(t, uint32(5), inner.Items[0].Quantity)
	require.Equal(t, uint8(2), inner.Items[0].X)
	require.Equal(t, uint8(1), inner.Items[0].Y)
	require.Equal(t, 4, inner.Version)
	require.Len(t, capture.ContainerRefs(), 4)
	require.Equal(t, types.EntityID(104), capture.MaxItemID())
	require.Equal(t, uint32(5), capture.ContainerRefs()[3].Key)

	child, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 103, 0)
	ecs.MutateComponent[components.InventoryContainer](w, child, func(container *components.InventoryContainer) bool {
		container.Items[0].Quantity = 99
		container.Version++
		return true
	})
	require.Equal(t, uint32(5), inner.Items[0].Quantity, "snapshot must own item values")
}

func TestObjectLootCaptureEmptyAndOtherRootKinds(t *testing.T) {
	w := ecs.NewWorldWithCapacity(16, nil, 0)
	target := w.Spawn(100, nil)
	ecs.AddComponent(w, target, components.ObjectInternalState{HasHP: true})
	registry := itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1, Size: itemdefs.Size{W: 1, H: 1}, Resource: "tool"}})
	empty := NewObjectLootCapture(100, nil)
	finishObjectLootCapture(t, empty, w, registry, 1)
	require.Empty(t, empty.Items())
	require.Empty(t, empty.ContainerRefs())
	addObjectLootContainer(w, components.InventoryContainer{OwnerID: 100, Kind: constt.InventoryHand, Key: 3, Version: 2,
		Items: []components.InvItem{{ItemID: 101, TypeID: 1, Quantity: 1, Quality: 10, W: 1, H: 1}},
	})
	addObjectLootContainer(w, components.InventoryContainer{OwnerID: 100, Kind: constt.InventoryGrid, Key: 7, Version: 3, Width: 4, Height: 4})
	capture := NewObjectLootCapture(100, ecs.GetResource[ecs.InventoryRefIndex](w).EntriesByOwnerInto(100, nil))
	finishObjectLootCapture(t, capture, w, registry, 1)
	require.Len(t, capture.Items(), 1)
	require.Len(t, capture.ContainerRefs(), 2)
	require.Equal(t, constt.InventoryHand, capture.ContainerRefs()[1].Kind)
	require.Equal(t, uint32(3), capture.ContainerRefs()[1].Key)
}

func TestObjectLootCaptureRejectsDamagedStateWithoutMutation(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*ecs.World, types.Handle, []ecs.InventoryRefEntry)
	}{
		{"stale root", func(w *ecs.World, _ types.Handle, roots []ecs.InventoryRefEntry) {
			w.Despawn(roots[0].Handle)
			w.SpawnWithoutExternalID()
		}},
		{"foreign owner", func(w *ecs.World, _ types.Handle, roots []ecs.InventoryRefEntry) {
			ecs.MutateComponent[components.InventoryContainer](w, roots[0].Handle, func(c *components.InventoryContainer) bool { c.OwnerID = 200; return true })
		}},
		{"unknown definition", func(w *ecs.World, _ types.Handle, roots []ecs.InventoryRefEntry) {
			ecs.MutateComponent[components.InventoryContainer](w, roots[0].Handle, func(c *components.InventoryContainer) bool { c.Items[0].TypeID = 999; return true })
		}},
		{"zero quality", func(w *ecs.World, _ types.Handle, roots []ecs.InventoryRefEntry) {
			ecs.MutateComponent[components.InventoryContainer](w, roots[0].Handle, func(c *components.InventoryContainer) bool { c.Items[0].Quality = 0; return true })
		}},
		{"zero quantity", func(w *ecs.World, _ types.Handle, roots []ecs.InventoryRefEntry) {
			ecs.MutateComponent[components.InventoryContainer](w, roots[0].Handle, func(c *components.InventoryContainer) bool { c.Items[0].Quantity = 0; return true })
		}},
		{"SQL identity overflow", func(w *ecs.World, _ types.Handle, _ []ecs.InventoryRefEntry) {
			h, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 103, 0)
			ecs.MutateComponent[components.InventoryContainer](w, h, func(c *components.InventoryContainer) bool { c.Items[0].ItemID = math.MaxInt64 + 1; return true })
		}},
		{"stacked bag", func(w *ecs.World, _ types.Handle, roots []ecs.InventoryRefEntry) {
			ecs.MutateComponent[components.InventoryContainer](w, roots[0].Handle, func(c *components.InventoryContainer) bool { c.Items[0].Quantity = 2; return true })
		}},
		{"missing bag contents", func(w *ecs.World, _ types.Handle, _ []ecs.InventoryRefEntry) {
			ecs.GetResource[ecs.InventoryRefIndex](w).Remove(constt.InventoryGrid, 102, 0)
		}},
		{"duplicate item", func(w *ecs.World, _ types.Handle, roots []ecs.InventoryRefEntry) {
			ecs.MutateComponent[components.InventoryContainer](w, roots[1].Handle, func(c *components.InventoryContainer) bool { c.Items[0].ItemID = 104; return true })
		}},
		{"cycle", func(w *ecs.World, _ types.Handle, _ []ecs.InventoryRefEntry) {
			h, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 103, 0)
			ecs.MutateComponent[components.InventoryContainer](w, h, func(c *components.InventoryContainer) bool {
				c.Items[0] = components.InvItem{ItemID: 102, TypeID: 2, Quantity: 1, Quality: 10, W: 1, H: 1}
				return true
			})
		}},
		{"target revived", func(w *ecs.World, target types.Handle, _ []ecs.InventoryRefEntry) {
			ecs.AddComponent(w, target, components.ObjectInternalState{HasHP: true, HP: 1})
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			w, registry, target, roots := objectLootCaptureFixture()
			scenario.change(w, target, roots)
			beforeCount := w.EntityCount()
			capture := NewObjectLootCapture(100, roots)
			_, err := capture.CaptureBatch(w, registry, 100)
			require.ErrorIs(t, err, ErrInvalidObjectLootCapture)
			require.Nil(t, capture.Items())
			require.Nil(t, capture.ContainerRefs())
			require.Equal(t, beforeCount, w.EntityCount())
			require.True(t, w.Alive(target))
			_, err = capture.CaptureBatch(w, registry, 100)
			require.ErrorIs(t, err, ErrInvalidObjectLootCapture)
		})
	}
}

func TestObjectLootCaptureRechecksCompletedContainers(t *testing.T) {
	w, registry, _, roots := objectLootCaptureFixture()
	capture := NewObjectLootCapture(100, roots)
	for len(capture.refs) < 4 {
		_, err := capture.CaptureBatch(w, registry, 1)
		require.NoError(t, err)
	}
	ecs.MutateComponent[components.InventoryContainer](w, roots[0].Handle, func(c *components.InventoryContainer) bool { c.Version++; return true })
	_, err := capture.CaptureBatch(w, registry, 100)
	require.ErrorIs(t, err, ErrInvalidObjectLootCapture)
	require.Nil(t, capture.Items())
}

func TestObjectLootCaptureBoundsEmptyContainers(t *testing.T) {
	w := ecs.NewWorldWithCapacity(256, nil, 0)
	target := w.Spawn(100, nil)
	ecs.AddComponent(w, target, components.ObjectInternalState{HasHP: true})
	for key := uint32(0); key < 150; key++ {
		addObjectLootContainer(w, components.InventoryContainer{OwnerID: 100, Kind: constt.InventoryGrid, Key: key, Version: 1, Width: 1, Height: 1})
	}
	registry := itemdefs.NewRegistry(nil)
	capture := NewObjectLootCapture(100, ecs.GetResource[ecs.InventoryRefIndex](w).EntriesByOwnerInto(100, nil))
	done, err := capture.CaptureBatch(w, registry, 100)
	require.NoError(t, err)
	require.False(t, done)
	require.LessOrEqual(t, len(capture.refs), 100)
	finishObjectLootCapture(t, capture, w, registry, 100)
	require.Len(t, capture.ContainerRefs(), 150)
}

func TestObjectLootCaptureRejectsReusedTargetAndRegistry(t *testing.T) {
	for _, scenario := range []string{"target generation", "registry", "SQL owner identity"} {
		t.Run(scenario, func(t *testing.T) {
			w, registry, target, roots := objectLootCaptureFixture()
			capture := NewObjectLootCapture(100, roots)
			_, err := capture.CaptureBatch(w, registry, 1)
			require.NoError(t, err)
			switch scenario {
			case "target generation":
				w.Despawn(target)
				replacement := w.Spawn(100, nil)
				ecs.AddComponent(w, replacement, components.ObjectInternalState{HasHP: true})
			case "registry":
				registry = itemdefs.NewRegistry(nil)
			case "SQL owner identity":
				capture = NewObjectLootCapture(math.MaxInt64+1, nil)
			}
			_, err = capture.CaptureBatch(w, registry, 100)
			require.ErrorIs(t, err, ErrInvalidObjectLootCapture)
			require.Nil(t, capture.Items())
		})
	}
}

func BenchmarkObjectLootCapture(b *testing.B) {
	for _, scenario := range []struct {
		name  string
		noise int
		empty bool
	}{{"Simple", 0, true}, {"NestedContainers", 0, false}, {"Owners1000", 1000, false}, {"Owners100000", 100000, false}} {
		b.Run(scenario.name, func(b *testing.B) {
			w, registry, _, roots := objectLootCaptureFixture()
			if scenario.empty {
				w = ecs.NewWorldWithCapacity(4, nil, 0)
				target := w.Spawn(100, nil)
				ecs.AddComponent(w, target, components.ObjectInternalState{HasHP: true})
				roots = nil
			}
			// Noise does not need ECS handles: unrelated refs catch accidental index scans.
			for i := 0; i < scenario.noise; i++ {
				ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, types.EntityID(1000+i), 0, types.Handle(1000+i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				capture := NewObjectLootCapture(100, roots)
				for {
					done, err := capture.CaptureBatch(w, registry, 100)
					if err != nil {
						b.Fatal(err)
					}
					if done {
						break
					}
				}
			}
		})
	}
}
