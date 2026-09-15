package inventory

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestEquipmentMoveSwapAndUnequipMarkPublicVisual(t *testing.T) {
	previous := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{
		{DefID: 1002, Key: "stone_axe", Allowed: itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true), EquipmentSlots: []string{"left_hand", "right_hand"}}},
		{DefID: 1003, Key: "shield", Allowed: itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true), EquipmentSlots: []string{"left_hand"}}},
	}))
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previous) })
	w, ownerID, owner := setupTestWorld(t)
	previousObjects := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, Key: "player", Name: "Player"}}))
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previousObjects) })
	ecs.AddComponent(w, owner, components.EntityInfo{TypeID: 1})
	_, hand := setupPlayerWithInventories(w, ownerID, owner)
	equipment := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, equipment, components.InventoryContainer{OwnerID: ownerID, Kind: constt.InventoryEquipment, Version: 1})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryEquipment, ownerID, 0, equipment)
	ecs.MutateComponent[components.InventoryOwner](w, owner, func(c *components.InventoryOwner) bool {
		c.Inventories = append(c.Inventories, components.InventoryLink{Kind: constt.InventoryEquipment, OwnerID: ownerID, Handle: equipment})
		return true
	})
	addItemToContainer(w, hand, components.InvItem{ItemID: 201, TypeID: 1002, Quantity: 1, W: 1, H: 1})
	executor := NewInventoryExecutor(zap.NewNop(), nil, nil, nil, nil)
	ref := func(kind constt.InventoryKind) *netproto.InventoryRef {
		return &netproto.InventoryRef{Kind: netproto.InventoryKind(kind), OwnerId: uint64(ownerID)}
	}
	move := func(itemID uint64, src, dst constt.InventoryKind, slot netproto.EquipSlot, swap bool) bool {
		return executor.ExecuteOperation(w, ownerID, owner, &netproto.InventoryOp{OpId: 1, Kind: &netproto.InventoryOp_Move{Move: &netproto.InventoryMoveSpec{
			Src: ref(src), Dst: ref(dst), ItemId: itemID, DstEquipSlot: &slot, AllowSwapOrMerge: swap,
		}}}).Success
	}
	drain := func() []types.Handle { return ecs.GetResource[ecs.CharacterVisualDirtyQueue](w).Drain(0, nil) }
	require.True(t, move(201, constt.InventoryHand, constt.InventoryEquipment, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND, false))
	require.Equal(t, []types.Handle{owner}, drain())
	addItemToContainer(w, equipment, components.InvItem{ItemID: 202, TypeID: 1003, Quantity: 1, W: 1, H: 1, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND})
	before, _ := ecs.GetComponent[components.InventoryContainer](w, equipment)
	beforeVersion := before.Version
	require.False(t, move(201, constt.InventoryEquipment, constt.InventoryEquipment, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND, true), "reverse swap would place shield in right hand")
	require.Empty(t, drain())
	after, _ := ecs.GetComponent[components.InventoryContainer](w, equipment)
	require.Equal(t, beforeVersion, after.Version)
	require.Equal(t, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND, after.Items[0].EquipSlot)
	require.Equal(t, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND, after.Items[1].EquipSlot)
	ecs.MutateComponent[components.InventoryContainer](w, equipment, func(c *components.InventoryContainer) bool { c.Items[1].TypeID = 1002; return true })
	require.True(t, move(201, constt.InventoryEquipment, constt.InventoryEquipment, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND, true))
	require.Equal(t, []types.Handle{owner}, drain())
	require.True(t, move(201, constt.InventoryEquipment, constt.InventoryHand, netproto.EquipSlot_EQUIP_SLOT_NONE, false))
	require.Equal(t, []types.Handle{owner}, drain())
	require.True(t, move(201, constt.InventoryHand, constt.InventoryGrid, netproto.EquipSlot_EQUIP_SLOT_NONE, false))
	require.Empty(t, drain(), "drag-hand and backpack changes are private")
}
