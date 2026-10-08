package world

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

func TestObjectFactoryCapacityPreservesWholeInventoryTree(t *testing.T) {
	previousObjects, previousItems := objectdefs.Global(), itemdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previousObjects); itemdefs.SetGlobalForTesting(previousItems) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{HP: 100,
		DefID: 701, Key: "capacity_box", Resource: "box", BehaviorOrder: []string{"container"},
		Behaviors:  map[string]json.RawMessage{"container": json.RawMessage(`{}`)},
		Components: &objectdefs.Components{Inventory: []objectdefs.InventoryDef{{Kind: "grid", W: 2, H: 2}}},
	}}))
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 900, Key: "capacity_bag", Size: itemdefs.Size{W: 1, H: 1}}}))
	for _, freeSlots := range []int{1, 2} {
		t.Run(string(rune('0'+freeSlots)), func(t *testing.T) {
			w := ecs.NewWorldWithCapacity(3, nil, 0)
			occupied := make([]types.Handle, 3-freeSlots)
			for i := range occupied {
				occupied[i] = w.Spawn(types.EntityID(i+1), nil)
			}
			raw := &repository.Object{Hp: sql.NullFloat64{Float64: 100, Valid: true}, ID: 100, TypeID: 701, Quality: 10}
			inventories := []repository.Inventory{{OwnerID: 100, Kind: int16(constt.InventoryGrid), Version: 1, Data: json.RawMessage(`{"width":2,"height":2,"items":[{"item_id":200,"type_id":900,"quantity":1,"nested_inventory":{"kind":0,"width":1,"height":1,"items":[]}}]}`)}}
			factory := NewObjectFactory(nil)
			handle, err := factory.Build(w, raw, inventories)
			require.ErrorIs(t, err, ecs.ErrEntityCapacityExhausted)
			require.ErrorIs(t, err, ErrEntitySpawnFailed)
			require.Equal(t, types.InvalidHandle, handle)
			require.Equal(t, len(occupied), w.EntityCount(), "failed restore must not leave a partial object/container")
			require.Equal(t, types.InvalidHandle, w.GetHandleByEntityID(100))
			for _, h := range occupied {
				require.True(t, w.Despawn(h))
			}
			handle, err = factory.Build(w, raw, inventories)
			require.NoError(t, err)
			require.True(t, w.Alive(handle))
			require.Equal(t, 3, w.EntityCount())
			index := ecs.GetResource[ecs.InventoryRefIndex](w)
			root, ok := index.Lookup(constt.InventoryGrid, 100, 0)
			require.True(t, ok)
			contents, ok := ecs.GetComponent[components.InventoryContainer](w, root)
			require.True(t, ok)
			require.Len(t, contents.Items, 1)
			nested, ok := index.Lookup(constt.InventoryGrid, 200, 0)
			require.True(t, ok)
			require.True(t, w.Alive(nested))
		})
	}
}

func TestDroppedObjectCapacityDoesNotLeaveInventoryWithoutObject(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 900, Key: "capacity_stone", Resource: "stone", Size: itemdefs.Size{W: 1, H: 1}}}))
	w := ecs.NewWorldWithCapacity(2, nil, 0)
	occupied := w.Spawn(1, nil)
	metadata, err := json.Marshal(DroppedItemData{HasInventory: true, ContainedItemID: 100, DropTime: 0})
	require.NoError(t, err)
	raw := &repository.Object{ID: 100, TypeID: constt.DroppedItemTypeID, Data: pqtype.NullRawMessage{Valid: true, RawMessage: metadata}}
	inventories := []repository.Inventory{{OwnerID: 100, Kind: int16(constt.InventoryDroppedItem), Version: 1, Data: json.RawMessage(`{"items":[{"item_id":100,"type_id":900,"quantity":1}]}`)}}
	factory := NewObjectFactory(nil)
	handle, err := factory.Build(w, raw, inventories)
	require.ErrorIs(t, err, ecs.ErrEntityCapacityExhausted)
	require.Equal(t, types.InvalidHandle, handle)
	require.Equal(t, 1, w.EntityCount())
	require.True(t, w.Despawn(occupied))
	handle, err = factory.Build(w, raw, inventories)
	require.NoError(t, err)
	require.True(t, w.Alive(handle))
	require.Equal(t, 2, w.EntityCount())
}

type capacityDroppedInventoryLoader struct{}

func (capacityDroppedInventoryLoader) LoadDroppedInventory(w *ecs.World, ownerID types.EntityID) (types.Handle, error) {
	handle := w.SpawnWithoutExternalID()
	if handle == types.InvalidHandle {
		return handle, ecs.ErrEntityCapacityExhausted
	}
	ecs.AddComponent(w, handle, components.InventoryContainer{OwnerID: ownerID, Kind: constt.InventoryDroppedItem, Items: []components.InvItem{{ItemID: ownerID, TypeID: 900, Quantity: 1, Resource: "stone"}}})
	return handle, nil
}

func TestDroppedObjectLoaderCapacityFailureRemainsRetryableAfterCleanup(t *testing.T) {
	w := ecs.NewWorldWithCapacity(2, nil, 0)
	occupied := w.Spawn(1, nil)
	metadata, err := json.Marshal(DroppedItemData{HasInventory: true, ContainedItemID: 100})
	require.NoError(t, err)
	raw := &repository.Object{ID: 100, TypeID: constt.DroppedItemTypeID, Data: pqtype.NullRawMessage{Valid: true, RawMessage: metadata}}
	factory := NewObjectFactory(capacityDroppedInventoryLoader{})
	handle, err := factory.Build(w, raw, nil)
	require.ErrorIs(t, err, ecs.ErrEntityCapacityExhausted)
	require.ErrorIs(t, err, ErrEntitySpawnFailed)
	require.Equal(t, types.InvalidHandle, handle)
	require.Equal(t, 1, w.EntityCount(), "temporary inventory must be cleaned up")
	require.True(t, w.Alive(occupied))
	require.True(t, w.Despawn(occupied))
	handle, err = factory.Build(w, raw, nil)
	require.NoError(t, err)
	require.True(t, w.Alive(handle))
	require.Equal(t, 2, w.EntityCount())
}
