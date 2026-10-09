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

func preparedSkullFixture(t *testing.T) (*ecs.World, types.EntityID, types.Handle, types.Handle, types.Handle, types.Handle, *InventoryExecutor) {
	t.Helper()
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 3014, Key: "skull", Name: "Skull", Size: itemdefs.Size{W: 1, H: 1}, Resource: "items/skull.png", DiscoveryLP: 17},
		{DefID: 200, Key: "seed_bag_mini", Size: itemdefs.Size{W: 1, H: 1}, Resource: "items/bag_seed_empty.png", Visual: &itemdefs.Visual{NestedInventory: &itemdefs.NestedInventoryVisual{Empty: "items/bag_seed_empty.png", HasItems: "items/bag_seed_full.png"}}, Container: &itemdefs.ContainerDef{Size: itemdefs.Size{W: 1, H: 1}}},
		{DefID: 202, Key: "ore", Size: itemdefs.Size{W: 1, H: 1}},
	}))
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w, id, player, root, nested, hand, _ := setupGiveItemWorld(t)
	ecs.AddComponent(w, player, components.CharacterProfile{})
	return w, id, player, root, nested, hand, NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
}

func preparedSkullItem() components.InvItem {
	return components.InvItem{ItemID: 9000, TypeID: 3014, Quality: 7, Quantity: 1, W: 1, H: 1, Skull: skullMetadataFixture()}
}

func finishSkullGrantCapture(t *testing.T, p *PreparedSkullGrant, w *ecs.World, budget int) int {
	t.Helper()
	for count := 1; count <= 1000; count++ {
		done, err := p.CaptureBatch(w, budget)
		require.NoError(t, err)
		if done {
			return count
		}
	}
	t.Fatal("capture did not finish")
	return 0
}

func TestPreparedSkullGrantPlacementAndDurableSnapshot(t *testing.T) {
	for _, destination := range []string{"root", "nested", "hand", "rules reject nested"} {
		t.Run(destination, func(t *testing.T) {
			w, id, player, root, nested, hand, executor := preparedSkullFixture(t)
			expected := root
			if destination != "root" {
				addItemToContainer(w, root, components.InvItem{ItemID: 6000, TypeID: 202, Quality: 1, Quantity: 1, W: 1, H: 1, X: 1})
				expected = nested
			}
			if destination == "hand" {
				addItemToContainer(w, nested, components.InvItem{ItemID: 6001, TypeID: 202, Quality: 1, Quantity: 1, W: 1, H: 1})
				expected = hand
			}
			if destination == "rules reject nested" {
				bag, _ := itemdefs.Global().GetByID(200)
				bag.Container.Rules.AllowTags = []string{"seed"}
				expected = hand
			}
			beforeRoot, _ := ecs.GetComponent[components.InventoryContainer](w, root)
			beforeDest, _ := ecs.GetComponent[components.InventoryContainer](w, expected)
			plan, err := executor.PrepareSkullGrant(w, id, player)
			require.NoError(t, err)
			require.Equal(t, expected, plan.destination.Handle)
			profile, _ := ecs.GetComponent[components.CharacterProfile](w, player)
			require.Empty(t, profile.Discovery)
			require.True(t, ecs.ReserveInventoryOwner(w, id, player))
			require.Greater(t, finishSkullGrantCapture(t, plan, w, 1), 1)
			item := preparedSkullItem()
			snapshots, err := plan.Build(item)
			require.NoError(t, err)
			require.Len(t, snapshots, 2)
			var rootData, handData InventoryDataV1
			for _, snapshot := range snapshots {
				if snapshot.Kind == int16(constt.InventoryGrid) {
					require.NoError(t, json.Unmarshal(snapshot.Data, &rootData))
				}
				if snapshot.Kind == int16(constt.InventoryHand) {
					require.NoError(t, json.Unmarshal(snapshot.Data, &handData))
				}
			}
			if expected == root {
				require.Equal(t, int(beforeRoot.Version+1), rootData.Version)
				require.Equal(t, item.Skull, rootData.Items[1].Skull)
			} else if expected == nested {
				require.Equal(t, int(beforeRoot.Version+1), rootData.Version)
				require.Equal(t, int(beforeDest.Version+1), rootData.Items[0].NestedInventory.Version)
				require.Equal(t, item.Skull, rootData.Items[0].NestedInventory.Items[0].Skull)
			} else {
				require.Equal(t, int(beforeRoot.Version), rootData.Version)
				require.Equal(t, int(beforeDest.Version+1), handData.Version)
				require.Equal(t, item.Skull, handData.Items[0].Skull)
				require.Equal(t, int16(constt.DefaultHandMouseOffset), handData.HandMouseOffsetX)
				require.Equal(t, int16(constt.DefaultHandMouseOffset), handData.HandMouseOffsetY)
				restored := ecs.NewWorldForTesting()
				loaded, err := NewInventoryLoader(zap.NewNop()).LoadPlayerInventories(restored, id, []InventoryDataV1{handData})
				require.NoError(t, err)
				require.Len(t, loaded.ContainerHandles, 1)
				restoredHand, _ := ecs.GetComponent[components.InventoryContainer](restored, loaded.ContainerHandles[0])
				require.Equal(t, handData.HandMouseOffsetX, restoredHand.HandMouseOffsetX)
				require.Equal(t, handData.HandMouseOffsetY, restoredHand.HandMouseOffsetY)
				require.Equal(t, item.Skull, restoredHand.Items[0].Skull)
			}
			unchanged, _ := ecs.GetComponent[components.InventoryContainer](w, expected)
			require.Len(t, unchanged.Items, len(beforeDest.Items), "Build cannot mutate live inventory")
			profile, _ = ecs.GetComponent[components.CharacterProfile](w, player)
			require.Empty(t, profile.Discovery)
			result, err := plan.Apply(w, item)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.Equal(t, uint32(1), result.GrantedCount)
			require.Equal(t, expected == hand, result.PlacedInHand)
			require.Equal(t, int64(17), result.DiscoveryLPGained)
			actual, _ := ecs.GetComponent[components.InventoryContainer](w, expected)
			require.Len(t, actual.Items, len(beforeDest.Items)+1)
			require.Equal(t, item.Skull, actual.Items[len(actual.Items)-1].Skull)
			require.NotSame(t, item.Skull, actual.Items[len(actual.Items)-1].Skull)
			require.Equal(t, beforeDest.Version+1, actual.Version)
			if expected == nested {
				updatedRoot, _ := ecs.GetComponent[components.InventoryContainer](w, root)
				require.Equal(t, "items/bag_seed_full.png", updatedRoot.Items[0].Resource)
				require.Equal(t, beforeRoot.Version+1, updatedRoot.Version, "visual cascade must not double-bump the committed root revision")
				require.Equal(t, uint64(rootData.Version), updatedRoot.Version)
				require.Len(t, result.UpdatedContainers, 2)
			}
			_, err = plan.Apply(w, item)
			require.ErrorIs(t, err, ErrInvalidSkullGrant)
		})
	}
}

func TestPreparedSkullGrantRejectsFullOrLockedRecipient(t *testing.T) {
	for _, scenario := range []string{"full", "KO", "reserved"} {
		t.Run(scenario, func(t *testing.T) {
			w, id, player, root, nested, hand, executor := preparedSkullFixture(t)
			if scenario == "full" {
				addItemToContainer(w, root, components.InvItem{ItemID: 6000, TypeID: 202, Quality: 1, Quantity: 1, W: 1, H: 1, X: 1})
				addItemToContainer(w, nested, components.InvItem{ItemID: 6001, TypeID: 202, Quality: 1, Quantity: 1, W: 1, H: 1})
				addItemToContainer(w, hand, components.InvItem{ItemID: 6002, TypeID: 202, Quality: 1, Quantity: 1, W: 1, H: 1})
			} else if scenario == "KO" {
				ecs.AddComponent(w, player, components.EntityHealth{IsLying: true})
			} else {
				require.True(t, ecs.ReserveInventoryOwner(w, id, player))
			}
			plan, err := executor.PrepareSkullGrant(w, id, player)
			require.Nil(t, plan)
			if scenario == "full" {
				require.ErrorIs(t, err, ErrSkullGrantNoSpace)
			} else {
				require.ErrorIs(t, err, ErrInvalidSkullGrant)
			}
		})
	}
}

func TestPreparedSkullGrantAcceptedKOAndCorpse(t *testing.T) {
	for _, death := range []bool{false, true} {
		t.Run(map[bool]string{false: "KO", true: "corpse"}[death], func(t *testing.T) {
			w, id, player, root, _, _, executor := preparedSkullFixture(t)
			plan, err := executor.PrepareSkullGrant(w, id, player)
			require.NoError(t, err)
			require.True(t, ecs.ReserveInventoryOwner(w, id, player))
			ecs.AddComponent(w, player, components.EntityHealth{IsLying: true})
			if death {
				ecs.RemoveComponent[components.EntityHealth](w, player)
				ecs.RemoveComponent[components.CharacterProfile](w, player)
				ecs.AddComponent(w, player, components.ObjectInternalState{HP: 100, HasHP: true})
			}
			finishSkullGrantCapture(t, plan, w, 1)
			item := preparedSkullItem()
			_, err = plan.Build(item)
			require.NoError(t, err)
			result, err := plan.Apply(w, item)
			require.NoError(t, err)
			require.True(t, result.Success)
			container, _ := ecs.GetComponent[components.InventoryContainer](w, root)
			require.Equal(t, types.EntityID(9000), container.Items[1].ItemID)
		})
	}
}

func TestPreparedSkullGrantRevalidatesReservationAndVersions(t *testing.T) {
	for _, scenario := range []string{"release before capture", "root changed after capture", "stale recipient"} {
		t.Run(scenario, func(t *testing.T) {
			w, id, player, root, _, _, executor := preparedSkullFixture(t)
			plan, err := executor.PrepareSkullGrant(w, id, player)
			require.NoError(t, err)
			require.True(t, ecs.ReserveInventoryOwner(w, id, player))
			if scenario == "release before capture" {
				require.True(t, ecs.ReleaseInventoryOwner(w, id, player))
				_, err := plan.CaptureBatch(w, 100)
				require.ErrorIs(t, err, ErrInvalidObjectLootCapture)
				return
			}
			finishSkullGrantCapture(t, plan, w, 100)
			item := preparedSkullItem()
			_, err = plan.Build(item)
			require.NoError(t, err)
			if scenario == "root changed after capture" {
				ecs.WithComponent(w, root, func(c *components.InventoryContainer) { c.Version++ })
			} else {
				w.Despawn(player)
				w.Spawn(id, nil)
			}
			_, err = plan.Apply(w, item)
			require.ErrorIs(t, err, ErrInvalidSkullGrant)
			container, _ := ecs.GetComponent[components.InventoryContainer](w, root)
			require.Len(t, container.Items, 1)
		})
	}
}

func TestPreparedSkullGrantCaptureIsBoundedAndAcceptsZeroQuality(t *testing.T) {
	w, id, player, root, _, hand, executor := preparedSkullFixture(t)
	// Make a large full root with one retained bag. Item records do not require
	// ECS entities; capture must still yield instead of monopolizing its lock.
	ecs.WithComponent(w, root, func(c *components.InventoryContainer) {
		c.Width, c.Height = 20, 20
		for index := 1; index < 400; index++ {
			c.Items = append(c.Items, components.InvItem{ItemID: types.EntityID(10000 + index), TypeID: 202, Quality: 0, Quantity: 1, W: 1, H: 1, X: uint8(index % 20), Y: uint8(index / 20)})
		}
	})
	// Keep the nested bag occupied so the normal fallback is hand.
	nested, _ := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryGrid, 5000, 0)
	addItemToContainer(w, nested, components.InvItem{ItemID: 6001, TypeID: 202, Quality: 1, Quantity: 1, W: 1, H: 1})
	plan, err := executor.PrepareSkullGrant(w, id, player)
	require.NoError(t, err)
	require.Equal(t, hand, plan.destination.Handle)
	require.True(t, ecs.ReserveInventoryOwner(w, id, player))
	done, err := plan.CaptureBatch(w, 100)
	require.NoError(t, err)
	require.False(t, done)
	require.LessOrEqual(t, len(plan.capture.seenItems), 100)
	item := preparedSkullItem()
	item.Quality = 0
	_, err = plan.Build(item)
	require.ErrorIs(t, err, ErrInvalidSkullGrant, "incomplete capture cannot create a durable snapshot")
	require.GreaterOrEqual(t, finishSkullGrantCapture(t, plan, w, 100), 4)
	_, err = plan.Build(item)
	require.NoError(t, err)
	_, err = plan.Apply(w, item)
	require.NoError(t, err)
	container, _ := ecs.GetComponent[components.InventoryContainer](w, hand)
	require.Equal(t, uint32(0), container.Items[0].Quality, "inherit the exact skeleton quality")
}
