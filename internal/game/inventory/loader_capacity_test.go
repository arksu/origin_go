package inventory

import (
	"fmt"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadPlayerInventoriesRollsBackCapacityFailure(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1, Key: "bag", Size: itemdefs.Size{W: 1, H: 1}}}))
	for _, available := range []uint32{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("%d_free_slots", available), func(t *testing.T) {
			w := ecs.NewWorldWithCapacity(2+available, nil, 0)
			player := w.Spawn(10, nil)
			preexisting := w.SpawnWithoutExternalID()
			existingContainer := components.InventoryContainer{OwnerID: 10, Kind: constt.InventoryGrid, Key: 7, Items: []components.InvItem{{ItemID: 500}}}
			ecs.AddComponent(w, preexisting, existingContainer)
			index := ecs.GetResource[ecs.InventoryRefIndex](w)
			index.Add(constt.InventoryGrid, 10, 7, preexisting)
			deepest := InventoryDataV1{Kind: uint8(constt.InventoryGrid), Width: 1, Height: 1}
			nested := deepest
			nested.Items = []InventoryItemV1{{ItemID: 102, TypeID: 1, NestedInventory: &deepest}}
			root := deepest
			root.Items = []InventoryItemV1{{ItemID: 101, TypeID: 1, NestedInventory: &nested}}
			loader := NewInventoryLoader(zap.NewNop())
			for range 2 {
				result, err := loader.LoadPlayerInventories(w, 10, []InventoryDataV1{{Kind: uint8(constt.InventoryHand)}, root})
				require.ErrorIs(t, err, ecs.ErrEntityCapacityExhausted)
				require.Nil(t, result)
				require.Equal(t, 2, w.EntityCount(), "failed recursive load must free every root and unfinished parent")
				require.True(t, w.Alive(player))
				require.True(t, w.Alive(preexisting))
				got, ok := ecs.GetComponent[components.InventoryContainer](w, preexisting)
				require.True(t, ok)
				require.Equal(t, existingContainer, got)
				indexed, ok := index.Lookup(constt.InventoryGrid, 10, 7)
				require.True(t, ok)
				require.Equal(t, preexisting, indexed)
				_, invalidComponent := ecs.GetComponent[components.InventoryContainer](w, types.InvalidHandle)
				require.False(t, invalidComponent)
			}
			for range available {
				require.NotEqual(t, types.InvalidHandle, w.SpawnWithoutExternalID(), "rolled-back capacity must be reusable")
			}
		})
	}
}

func TestLoadPlayerInventoriesSucceedsAtExactCapacity(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 1, Key: "bag", Size: itemdefs.Size{W: 1, H: 1}}}))
	w := ecs.NewWorldWithCapacity(3, nil, 0)
	player := w.Spawn(10, nil)
	nested := InventoryDataV1{Kind: uint8(constt.InventoryGrid), Width: 1, Height: 1}
	root := nested
	root.Items = []InventoryItemV1{{ItemID: 101, TypeID: 1, NestedInventory: &nested}}
	result, err := NewInventoryLoader(zap.NewNop()).LoadPlayerInventories(w, 10, []InventoryDataV1{root})
	require.NoError(t, err)
	require.Len(t, result.ContainerHandles, 2)
	require.Equal(t, 3, w.EntityCount())
	require.True(t, w.Alive(player))
	owners := make([]types.EntityID, 0, 2)
	for _, handle := range result.ContainerHandles {
		require.True(t, w.Alive(handle))
		container, ok := ecs.GetComponent[components.InventoryContainer](w, handle)
		require.True(t, ok)
		owners = append(owners, container.OwnerID)
	}
	require.ElementsMatch(t, []types.EntityID{10, 101}, owners)
}
