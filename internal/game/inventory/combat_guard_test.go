package inventory

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"testing"
)

func TestCombatEquipmentTransactionsAreAtomic(t *testing.T) {
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1002, Key: "stone_axe", Allowed: itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true), EquipmentSlots: []string{"left_hand", "right_hand"}}},
	}))
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	for _, recovery := range []bool{false, true} {
		w, ownerID, owner := setupTestWorld(t)
		grid, hand := setupPlayerWithInventories(w, ownerID, owner)
		equipment := w.SpawnWithoutExternalID()
		ecs.AddComponent(w, equipment, components.InventoryContainer{OwnerID: ownerID, Kind: constt.InventoryEquipment, Version: 1})
		ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryEquipment, ownerID, 0, equipment)
		ecs.WithComponent(w, owner, func(c *components.InventoryOwner) {
			c.Inventories = append(c.Inventories, components.InventoryLink{Kind: constt.InventoryEquipment, OwnerID: ownerID, Handle: equipment})
		})
		right, left := netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND
		for index, slot := range []netproto.EquipSlot{right, left} {
			addItemToContainer(w, equipment, components.InvItem{ItemID: 201 + types.EntityID(index), TypeID: 1002, Quantity: 1, W: 1, H: 1, EquipSlot: slot})
		}
		addItemToContainer(w, hand, components.InvItem{ItemID: 203, TypeID: 1002, Quantity: 1, W: 1, H: 1})
		addItemToContainer(w, grid, components.InvItem{ItemID: 204, TypeID: 1002, Quantity: 1, W: 1, H: 1})
		ecs.AddComponent(w, owner, components.CombatState{Execution: &components.CombatExecution{StrikeResolved: recovery}})
		service := NewInventoryOperationService(zap.NewNop(), fixedDroppedIDAllocator{}, &failingDroppedPersister{})
		ref := func(kind constt.InventoryKind) *netproto.InventoryRef {
			return &netproto.InventoryRef{Kind: netproto.InventoryKind(kind), OwnerId: uint64(ownerID)}
		}
		for _, attempt := range []struct {
			item     uint64
			from, to constt.InventoryKind
			slot     netproto.EquipSlot
		}{
			{201, constt.InventoryEquipment, constt.InventoryHand, 0},
			{202, constt.InventoryEquipment, constt.InventoryGrid, 0},
			{201, constt.InventoryEquipment, constt.InventoryEquipment, left},
			{203, constt.InventoryHand, constt.InventoryEquipment, right},
			{204, constt.InventoryGrid, constt.InventoryEquipment, left},
		} {
			result := service.ExecuteMove(w, ownerID, owner, 1, &netproto.InventoryMoveSpec{Src: ref(attempt.from), Dst: ref(attempt.to), ItemId: attempt.item, DstEquipSlot: &attempt.slot, AllowSwapOrMerge: true}, nil)
			require.False(t, result.Success)
			require.Contains(t, result.Message, "Combat equipment")
		}
		dropped := service.ExecuteDropToWorld(w, ownerID, owner, 1, &netproto.InventoryMoveSpec{Src: ref(constt.InventoryEquipment), ItemId: 201}, nil)
		require.False(t, dropped.Success)
		require.Contains(t, dropped.Message, "Combat equipment")
		after, _ := ecs.GetComponent[components.InventoryContainer](w, equipment)
		require.Equal(t, uint64(1), after.Version)
		require.Equal(t, right, after.Items[0].EquipSlot)
		require.Equal(t, left, after.Items[1].EquipSlot)
		moved := service.ExecuteMove(w, ownerID, owner, 2, &netproto.InventoryMoveSpec{Src: ref(constt.InventoryGrid), Dst: ref(constt.InventoryGrid), ItemId: 204, DstPos: &netproto.GridPos{X: 2, Y: 2}}, nil)
		require.True(t, moved.Success, moved.Message)
	}
}
