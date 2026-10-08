package inventory

import (
	"encoding/json"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPlayerSnapshotPreservesEntireInventoryTree(t *testing.T) {
	w, registry, player, roots := objectLootCaptureFixture()
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	owner := components.InventoryOwner{}
	for _, root := range roots {
		owner.Inventories = append(owner.Inventories, components.InventoryLink{
			Kind: root.Kind, Key: root.Key, OwnerID: root.OwnerID, Handle: root.Handle,
		})
	}
	ecs.AddComponent(w, player, owner)
	saver := NewInventorySaver(zap.NewNop())
	snapshots, err := saver.SerializeInventoriesStrict(w, 100, player)
	require.NoError(t, err)
	require.Len(t, snapshots, 2, "nested roots are embedded even without player owner links")
	var root InventoryDataV1
	require.NoError(t, json.Unmarshal(snapshots[0].Data, &root))
	require.Equal(t, uint64(104), root.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].ItemID)
	require.Equal(t, uint32(5), root.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].Quantity)

	restored := ecs.NewWorldForTesting()
	loaded, err := NewInventoryLoader(zap.NewNop()).LoadPlayerInventories(restored, 100, []InventoryDataV1{root})
	require.NoError(t, err)
	require.Len(t, loaded.ContainerHandles, 3)
	for _, h := range loaded.ContainerHandles {
		container, ok := ecs.GetComponent[components.InventoryContainer](restored, h)
		require.True(t, ok)
		ecs.GetResource[ecs.InventoryRefIndex](restored).Add(container.Kind, container.OwnerID, container.Key, h)
	}
	container, ok := ecs.GetComponent[components.InventoryContainer](restored, loaded.ContainerHandles[0])
	require.True(t, ok)
	copy, err := SerializeInventoryTree(restored, container)
	require.NoError(t, err)
	require.Equal(t, root, copy)
	deepest, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 103, 0)
	ecs.WithComponent(w, deepest, func(c *components.InventoryContainer) { c.Items[0].Quantity = 7 })
	require.Equal(t, uint32(5), copy.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].Quantity)
}

func TestInventoryTreeRejectsMissingStaleForeignAndCyclicChildren(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*ecs.World, types.Handle)
	}{
		{"missing", func(w *ecs.World, _ types.Handle) {
			ecs.GetResource[ecs.InventoryRefIndex](w).Remove(constt.InventoryGrid, 103, 0)
		}},
		{"zero handle", func(w *ecs.World, _ types.Handle) {
			ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, 103, 0, types.InvalidHandle)
		}},
		{"stale", func(w *ecs.World, child types.Handle) { w.Despawn(child); w.SpawnWithoutExternalID() }},
		{"foreign", func(w *ecs.World, child types.Handle) {
			ecs.WithComponent(w, child, func(c *components.InventoryContainer) { c.OwnerID = 999 })
		}},
		{"wrong key", func(w *ecs.World, child types.Handle) {
			ecs.WithComponent(w, child, func(c *components.InventoryContainer) { c.Key = 5 })
		}},
		{"wrong kind", func(w *ecs.World, child types.Handle) {
			ecs.WithComponent(w, child, func(c *components.InventoryContainer) { c.Kind = constt.InventoryHand })
		}},
		{"cycle", func(w *ecs.World, child types.Handle) {
			ecs.WithComponent(w, child, func(c *components.InventoryContainer) {
				c.Items[0] = components.InvItem{ItemID: 102, TypeID: 2, Quality: 10, Quantity: 1, W: 1, H: 1}
			})
		}},
		{"duplicate", func(w *ecs.World, child types.Handle) {
			ecs.WithComponent(w, child, func(c *components.InventoryContainer) { c.Items = append(c.Items, c.Items[0]) })
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			w, registry, _, roots := objectLootCaptureFixture()
			previous := itemdefs.Global()
			itemdefs.SetGlobalForTesting(registry)
			t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
			child, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 103, 0)
			scenario.change(w, child)
			root, _ := ecs.GetComponent[components.InventoryContainer](w, roots[0].Handle)
			count := w.EntityCount()
			snapshot, err := SerializeInventoryTree(w, root)
			require.ErrorIs(t, err, ErrInvalidInventoryTree)
			require.Equal(t, InventoryDataV1{}, snapshot)
			require.Equal(t, count, w.EntityCount())
		})
	}
}

func TestInventoryTreeSnapshotGrowsBeyondFixedTraversalBuffers(t *testing.T) {
	_, registry, _, _ := objectLootCaptureFixture()
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldWithCapacity(100, nil, 0)
	for ownerID := types.EntityID(100); ownerID < 164; ownerID++ {
		item := components.InvItem{ItemID: ownerID + 1, TypeID: 2, Quality: 10, Quantity: 1, W: 1, H: 1}
		if ownerID == 163 {
			item.TypeID = 1
		}
		addObjectLootContainer(w, components.InventoryContainer{OwnerID: ownerID, Kind: constt.InventoryGrid,
			Width: 4, Height: 4, Version: 1, Items: []components.InvItem{item},
		})
	}
	h, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 100, 0)
	root, _ := ecs.GetComponent[components.InventoryContainer](w, h)
	snapshot, err := SerializeInventoryTree(w, root)
	require.NoError(t, err)
	current := &snapshot
	for id := uint64(101); id <= 164; id++ {
		require.Len(t, current.Items, 1)
		require.Equal(t, id, current.Items[0].ItemID)
		if id == 164 {
			require.Nil(t, current.Items[0].NestedInventory)
		} else {
			require.NotNil(t, current.Items[0].NestedInventory)
			current = current.Items[0].NestedInventory
		}
	}
}

func TestPlayerDropAndPickupPersistDeepContainerContents(t *testing.T) {
	_, registry, _, _ := objectLootCaptureFixture()
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w, playerID, player := setupTestWorld(t)
	root, _ := setupPlayerWithInventories(w, playerID, player)
	ecs.AddComponent(w, player, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, player, components.EntityInfo{Region: 1})
	ecs.AddComponent(w, player, components.ChunkRef{})
	bag := components.InvItem{ItemID: 102, TypeID: 2, Quality: 10, Quantity: 1, W: 1, H: 1}
	ecs.WithComponent(w, root, func(c *components.InventoryContainer) { c.Items = append(c.Items, bag) })
	addObjectLootContainer(w, components.InventoryContainer{OwnerID: 102, Kind: constt.InventoryGrid, Version: 2, Width: 4, Height: 4,
		Items: []components.InvItem{{ItemID: 103, TypeID: 2, Quality: 20, Quantity: 1, W: 1, H: 1}},
	})
	deep := addObjectLootContainer(w, components.InventoryContainer{OwnerID: 103, Kind: constt.InventoryGrid, Version: 4, Width: 4, Height: 4,
		Items: []components.InvItem{{ItemID: 104, TypeID: 1, Quality: 30, Quantity: 5, W: 1, H: 1, X: 2, Y: 1}},
	})
	persister := &failingDroppedPersister{}
	service := NewInventoryOperationService(zap.NewNop(), &dropTestIDAllocator{next: 9000}, persister)
	drop := service.ExecuteDropToWorld(w, playerID, player, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(playerID)}, ItemId: 102,
	}, nil)
	require.True(t, drop.Success, "%+v", drop)
	require.Len(t, persister.records, 1)
	var dropped InventoryDataV1
	require.NoError(t, json.Unmarshal(persister.records[0].InventoryData, &dropped))
	require.Equal(t, uint64(104), dropped.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].ItemID)
	pickup := service.ExecutePickupFromWorld(w, playerID, player, 102,
		&netproto.InventoryRef{Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(playerID)})
	require.True(t, pickup.Success, "%+v", pickup)
	require.True(t, w.Alive(deep), "pickup transfers the inventory tree without deleting its descendants")
	snapshot := inventorySnapshotData(t, persister.inventories, constt.InventoryGrid)
	require.Equal(t, uint64(104), snapshot.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].ItemID)
	require.Equal(t, uint32(5), snapshot.Items[0].NestedInventory.Items[0].NestedInventory.Items[0].Quantity)
}
