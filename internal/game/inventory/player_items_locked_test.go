package inventory

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"
	"testing"
)

func TestItemLockGuardsInitiatorBeforeAnyMutation(t *testing.T) {
	for index, state := range []components.EntityHealth{{SHP: 0, HHP: 20, KOUntilUnixMs: 60000}, {SHP: 5, HHP: 20, IsLying: true}, {SHP: 20, HHP: 20}} {
		world, id, player := setupTestWorld(t)
		ecs.AddComponent(world, player, state)
		if index == 2 {
			require.True(t, ecs.ReserveInventoryOwner(world, id, player))
		}
		ecs.AddComponent(world, player, components.CharacterProfile{})
		ecs.AddComponent(world, player, components.EntityStats{Stamina: 100, Energy: 900})
		grid := createGridContainer(world, id, 0, 4, 4)
		addItemToContainer(world, grid, components.InvItem{ItemID: 20, TypeID: 1, Quantity: 2, W: 1, H: 1})
		initial, _ := ecs.GetComponent[components.InventoryContainer](world, grid)
		service := NewInventoryOperationService(zap.NewNop(), nil, nil)
		for _, owner := range []types.EntityID{id, 2000} {
			for _, kind := range []netproto.InventoryKind{netproto.InventoryKind_INVENTORY_KIND_GRID, netproto.InventoryKind_INVENTORY_KIND_HAND, netproto.InventoryKind_INVENTORY_KIND_EQUIPMENT, netproto.InventoryKind_INVENTORY_KIND_BUILD} {
				ref := &netproto.InventoryRef{OwnerId: uint64(owner), Kind: kind, InventoryKey: 7}
				move := &netproto.InventoryMoveSpec{Src: ref, Dst: ref, ItemId: 20, AllowSwapOrMerge: true}
				operations := []*OperationResult{
					service.ExecuteMove(world, id, player, 1, move, nil),
					service.ExecuteDropToWorld(world, id, player, 1, move, nil),
					service.ExecutePickupFromWorld(world, id, player, 200, ref),
					service.ExecuteOperation(world, id, player, &netproto.InventoryOp{OpId: 1, Kind: &netproto.InventoryOp_Move{Move: move}}),
				}
				for _, result := range operations {
					require.False(t, result.Success)
					require.Equal(t, netproto.ErrorCode_ERROR_CODE_CANNOT_INTERACT, result.ErrorCode)
					require.Equal(t, playerstate.ItemsLockedReason, result.Message)
					require.Empty(t, result.UpdatedContainers)
					require.Empty(t, result.SpawnedDroppedEntityIDs)
				}
			}
		}
		require.False(t, service.GiveItem(world, id, player, "test_item", 1, 10).Success)
		require.False(t, service.GiveItemToHandOnly(world, id, player, "test_item", 1, 10).Success)
		executor := &InventoryExecutor{service: service}
		clone := initial
		clone.Items = nil
		prepared := &CraftConsumeInputsResult{Success: true, prepared: []craftPreparedContainer{{handle: grid, container: clone}}}
		require.False(t, executor.CommitPreparedCraftInputs(world, id, player, prepared).Success)
		require.False(t, executor.GiveCraftOutputOrDrop(world, id, player, "test_item", 1, 10).Success)
		require.False(t, executor.dropCraftOutputAtPlayer(world, id, player, "test_item", 10))
		current, _ := ecs.GetComponent[components.InventoryContainer](world, grid)
		require.Equal(t, initial, current)
		require.Equal(t, 2, world.EntityCount(), "a blocked operation spawned an item")
		stats, _ := ecs.GetComponent[components.EntityStats](world, player)
		require.Equal(t, 100.0, stats.Stamina)
		profile, _ := ecs.GetComponent[components.CharacterProfile](world, player)
		require.Empty(t, profile.Discovery)
		// The owner of someone else's container does not determine the initiator's lock.
		other := world.Spawn(2000, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.EntityHealth{SHP: 5, HHP: 20}) })
		require.False(t, playerstate.ItemsLocked(world, other))
		require.Equal(t, constt.InventoryGrid, current.Kind)
	}
}
