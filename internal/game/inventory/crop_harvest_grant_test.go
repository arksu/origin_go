package inventory

import (
	"encoding/json"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func cropGrantItems(count int) []components.InvItem {
	items := make([]components.InvItem, count)
	for i := range items {
		items[i] = components.InvItem{ItemID: types.EntityID(9000 + i), TypeID: 201, Quality: 4000000000, Quantity: 1, W: 1, H: 1}
	}
	return items
}

func TestCropGrantScratchPlacementRootBagHandAndGround(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())
	w, id, player, root, bag, hand, _ := setupGiveItemWorld(t)
	ecs.AddComponent(w, player, components.CharacterProfile{})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	grant, err := executor.PrepareCropGrant(w, id, player)
	require.NoError(t, err)
	require.True(t, ecs.ReserveInventoryOwner(w, id, player))
	for {
		done, err := grant.CaptureBatch(w, 1)
		require.NoError(t, err)
		if done {
			break
		}
	}
	snapshots, overflow, err := grant.Build(cropGrantItems(5))
	require.NoError(t, err)
	require.Len(t, overflow, 2)
	require.Equal(t, types.EntityID(9003), overflow[0].ItemID)
	require.Len(t, snapshots, 2)
	before, _ := ecs.GetComponent[components.InventoryContainer](w, root)
	require.Len(t, before.Items, 1)
	for _, snapshot := range snapshots {
		var saved InventoryDataV1
		require.NoError(t, json.Unmarshal(snapshot.Data, &saved))
		if saved.Kind == uint8(constt.InventoryGrid) {
			require.Len(t, saved.Items, 2)
			require.Equal(t, uint64(9000), saved.Items[1].ItemID)
			require.Len(t, saved.Items[0].NestedInventory.Items, 1)
			require.Equal(t, uint64(9001), saved.Items[0].NestedInventory.Items[0].ItemID)
		}
	}
	result, err := grant.Apply(w)
	require.NoError(t, err)
	require.Equal(t, uint32(3), result.GrantedCount)
	require.True(t, result.PlacedInHand)
	require.Equal(t, int64(77), result.DiscoveryLPGained)
	for i, h := range []types.Handle{root, bag, hand} {
		container, _ := ecs.GetComponent[components.InventoryContainer](w, h)
		item := container.Items[len(container.Items)-1]
		require.Equal(t, types.EntityID(9000+i), item.ItemID)
		require.Equal(t, uint32(4000000000), item.Quality)
	}
	updated, _ := ecs.GetComponent[components.InventoryContainer](w, root)
	require.Equal(t, "items/bag_seed_full.png", updated.Items[0].Resource)
	require.Equal(t, before.Version+1, updated.Version)
	_, err = grant.Apply(w)
	require.ErrorIs(t, err, ErrInvalidCropGrant)
}

func TestCropGrantAllGroundWithFullOrEmptyInventory(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{true: "no_roots", false: "full"}[empty], func(t *testing.T) {
			previous := itemdefs.Global()
			t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
			itemdefs.SetGlobalForTesting(createGiveItemRegistry())
			w, id, player, root, bag, hand, _ := setupGiveItemWorld(t)
			if empty {
				refs := ecs.GetResource[ecs.InventoryRefIndex](w)
				for _, ref := range refs.EntriesByOwnerInto(id, nil) {
					refs.Remove(ref.Kind, ref.OwnerID, ref.Key)
					w.Despawn(ref.Handle)
				}
				ecs.AddComponent(w, player, components.InventoryOwner{})
			} else {
				for _, h := range []types.Handle{root, bag, hand} {
					item := components.InvItem{ItemID: types.EntityID(7000 + h), TypeID: 202, Quality: 1, Quantity: 1, W: 1, H: 1}
					if h == root {
						item.X = 1
					}
					addItemToContainer(w, h, item)
				}
			}
			grant, err := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil).PrepareCropGrant(w, id, player)
			require.NoError(t, err)
			require.True(t, ecs.ReserveInventoryOwner(w, id, player))
			for {
				done, err := grant.CaptureBatch(w, 2)
				require.NoError(t, err)
				if done {
					break
				}
			}
			snapshots, overflow, err := grant.Build(cropGrantItems(4))
			require.NoError(t, err)
			require.Empty(t, snapshots)
			require.Len(t, overflow, 4)
			result, err := grant.Apply(w)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Zero(t, result.GrantedCount)
		})
	}
}

func TestCropGrantRejectsStaleIdentityBeforeAnyApply(t *testing.T) {
	previous := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())
	w, id, player, root, _, _, _ := setupGiveItemWorld(t)
	grant, err := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil).PrepareCropGrant(w, id, player)
	require.NoError(t, err)
	require.True(t, ecs.ReserveInventoryOwner(w, id, player))
	for {
		done, err := grant.CaptureBatch(w, 5)
		require.NoError(t, err)
		if done {
			break
		}
	}
	_, _, err = grant.Build(cropGrantItems(1))
	require.NoError(t, err)
	ecs.MutateComponent[components.InventoryContainer](w, root, func(c *components.InventoryContainer) bool { c.Version++; return true })
	_, err = grant.Apply(w)
	require.ErrorIs(t, err, ErrInvalidCropGrant)
	container, _ := ecs.GetComponent[components.InventoryContainer](w, root)
	require.Len(t, container.Items, 1)
}
