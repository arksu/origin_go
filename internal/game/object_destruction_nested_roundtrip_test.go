package game

import (
	"context"
	"encoding/json"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

// Exercise the durable replacement and the later ordinary chunk save, rather
// than only checking the owned JSON returned by destruction capture.
func TestObjectDestructionPostgresNestedTreeSurvivesChunkLifecycle(t *testing.T) {
	f := newObjectDestructionIntegrationFixture(t, 64, 50)
	w := f.shard.world
	index := ecs.GetResource[ecs.InventoryRefIndex](w)
	func() {
		f.shard.mu.Lock()
		defer f.shard.mu.Unlock()
		bag, found := index.Lookup(constt.InventoryGrid, 30, 0)
		require.True(t, found)
		ecs.WithComponent(w, bag, func(container *components.InventoryContainer) {
			container.Items = []components.InvItem{{ItemID: 901, TypeID: 2, Quality: 19, Quantity: 1, W: 1, H: 1}}
			container.Version++
		})
		inner := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, inner, components.InventoryContainer{
			OwnerID: 901, Kind: constt.InventoryGrid, Width: 4, Height: 4, Version: 1,
			Items: []components.InvItem{{ItemID: 902, TypeID: 2, Quality: 21, Quantity: 1, W: 1, H: 1}},
		})
		index.Add(constt.InventoryGrid, 901, 0, inner)
		f.owned = append(f.owned, ecs.InventoryRefEntry{InventoryRefKey: ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 901}, Handle: inner})
		deepest := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, deepest, components.InventoryContainer{
			OwnerID: 902, Kind: constt.InventoryGrid, Width: 4, Height: 4, Version: 1,
			Items: []components.InvItem{{ItemID: 903, TypeID: 1, Quality: 23, Quantity: 7, W: 1, H: 1}},
		})
		index.Add(constt.InventoryGrid, 902, 0, deepest)
		f.owned = append(f.owned, ecs.InventoryRefEntry{InventoryRefKey: ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 902}, Handle: deepest})
	}()

	f.hit(t)
	f.eventually(t, func() bool {
		return f.shard.objectDestruction.PendingCount() == 0 && w.Alive(w.GetHandleByEntityID(30))
	})
	assertSavedTree := func(quantity uint32) {
		t.Helper()
		rows, err := f.db.Queries().GetInventoriesByOwner(context.Background(), 30)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		var root inventory.InventoryDataV1
		require.NoError(t, json.Unmarshal(rows[0].Data, &root))
		require.Len(t, root.Items, 1)
		require.NotNil(t, root.Items[0].NestedInventory)
		outer := root.Items[0].NestedInventory
		require.Len(t, outer.Items, 1)
		require.Equal(t, uint64(901), outer.Items[0].ItemID)
		require.NotNil(t, outer.Items[0].NestedInventory)
		middle := outer.Items[0].NestedInventory
		require.Len(t, middle.Items, 1)
		require.Equal(t, uint64(902), middle.Items[0].ItemID)
		require.NotNil(t, middle.Items[0].NestedInventory)
		contents := middle.Items[0].NestedInventory.Items
		require.Len(t, contents, 1)
		require.Equal(t, uint64(903), contents[0].ItemID)
		require.Equal(t, quantity, contents[0].Quantity)
	}
	assertSavedTree(7)

	func() {
		f.shard.mu.Lock()
		defer f.shard.mu.Unlock()
		for _, ref := range f.owned {
			require.False(t, w.Alive(ref.Handle), "post-commit cleanup must remove the original entire tree")
		}
		materializedInner, found := index.Lookup(constt.InventoryGrid, 902, 0)
		require.True(t, found)
		ecs.WithComponent(w, materializedInner, func(container *components.InventoryContainer) {
			container.Items[0].Quantity = 8
			container.Version++
		})
		drop := w.GetHandleByEntityID(30)
		ecs.WithComponent(w, drop, func(state *components.ObjectInternalState) { state.IsDirty = true })
		materializedBag, found := index.Lookup(constt.InventoryGrid, 30, 0)
		require.True(t, found)
		f.cm.UnregisterEntity(9000)
		require.False(t, w.Alive(drop))
		require.False(t, w.Alive(materializedBag))
		require.False(t, w.Alive(materializedInner), "deactivation must remove deep runtime containers")
		_, found = index.Lookup(constt.InventoryGrid, 902, 0)
		require.False(t, found)
		chunk := f.cm.GetChunkFast(types.ChunkCoord{})
		require.NoError(t, chunk.SaveToDB(f.db, w, f.cm.ObjectFactory(), f.shard.logger))
	}()
	assertSavedTree(8)

	f.shard.mu.Lock()
	f.cm.RegisterEntity(9001, 50, 50, false)
	f.shard.mu.Unlock()
	f.eventually(t, func() bool { return w.Alive(w.GetHandleByEntityID(30)) })
	f.shard.WithWorldRead(func(w *ecs.World) {
		inner, found := index.Lookup(constt.InventoryGrid, 902, 0)
		require.True(t, found)
		contents, found := ecs.GetComponent[components.InventoryContainer](w, inner)
		require.True(t, found)
		require.Len(t, contents.Items, 1)
		require.Equal(t, types.EntityID(903), contents.Items[0].ItemID)
		require.Equal(t, uint32(8), contents.Items[0].Quantity)
	})
}
