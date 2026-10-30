package game

import (
	"encoding/json"
	"testing"

	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"

	"github.com/stretchr/testify/require"
)

func TestObjectDestructionPreservesZeroQualitySeedAndProduct(t *testing.T) {
	f := newDestructionServiceFixture(t)
	f.service.deps.Items = itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 4002, Key: "seed_wheat", Resource: "seed_wheat", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 4007, Key: "carrot", Resource: "carrot", Size: itemdefs.Size{W: 1, H: 1}},
		{DefID: 2, Key: "bag", Resource: "bag", Size: itemdefs.Size{W: 1, H: 1}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 4, H: 4}}},
	})
	seed := components.InvItem{ItemID: 20, TypeID: 4002, Quality: 0, Quantity: 1, W: 1, H: 1}
	bag := lootItem(30, 2, 1)
	bag.X = 1
	root := f.addContainer(1, 0, seed, bag)
	nested := f.addContainer(30, 0, components.InvItem{ItemID: 900, TypeID: 4007, Quality: 0, Quantity: 1, W: 1, H: 1})
	result, err := f.damage.Apply(f.target, 100)
	require.NoError(t, err)
	require.True(t, result.EnteredDestruction)
	f.service.Update()
	f.chunks.runOne(t)
	f.service.Update()
	require.Zero(t, f.service.PendingCount())
	require.False(t, f.w.Alive(f.target))
	require.False(t, f.w.Alive(root))
	require.False(t, f.w.Alive(nested))
	require.Len(t, f.chunks.inserted, 2)
	seenTypes := make(map[uint32]int)
	for _, record := range f.chunks.inserted {
		var data inventory.InventoryDataV1
		require.NoError(t, json.Unmarshal(record.inventory.Data, &data))
		require.Len(t, data.Items, 1)
		item := data.Items[0]
		seenTypes[item.TypeID]++
		switch item.TypeID {
		case 4002:
			require.Zero(t, item.Quality)
		case 2:
			require.NotNil(t, item.NestedInventory)
			require.Len(t, item.NestedInventory.Items, 1)
			require.Equal(t, uint32(4007), item.NestedInventory.Items[0].TypeID)
			require.Zero(t, item.NestedInventory.Items[0].Quality)
		default:
			t.Fatalf("unexpected loot type %d", item.TypeID)
		}
	}
	require.Equal(t, map[uint32]int{4002: 1, 2: 1}, seenTypes)
}
