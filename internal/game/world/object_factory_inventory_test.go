package world

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func TestSerializeObjectInventories_IncludesAllRootKinds(t *testing.T) {
	world := ecs.NewWorldForTesting()
	factory := &ObjectFactory{}

	objectID := types.EntityID(9201)
	objectHandle := world.Spawn(objectID, nil)

	gridHandle := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, gridHandle, components.InventoryContainer{
		OwnerID: objectID,
		Kind:    constt.InventoryGrid,
		Key:     0,
		Version: 2,
		Width:   4,
		Height:  4,
		Items: []components.InvItem{
			{ItemID: 50001, TypeID: 1, Quantity: 1, W: 1, H: 1, X: 0, Y: 0, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_NONE},
		},
	})

	equipmentHandle := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, equipmentHandle, components.InventoryContainer{
		OwnerID: objectID,
		Kind:    constt.InventoryEquipment,
		Key:     0,
		Version: 3,
		Items: []components.InvItem{
			{ItemID: 50002, TypeID: 2, Quantity: 1, W: 1, H: 1, EquipSlot: netproto.EquipSlot_EQUIP_SLOT_HEAD},
		},
	})

	handHandle := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, handHandle, components.InventoryContainer{
		OwnerID: objectID,
		Kind:    constt.InventoryHand,
		Key:     0,
		Version: 4,
		Items: []components.InvItem{
			{ItemID: 50003, TypeID: 3, Quantity: 1, W: 1, H: 1},
		},
	})

	ecs.AddComponent(world, objectHandle, components.InventoryOwner{
		Inventories: []components.InventoryLink{
			{Kind: constt.InventoryGrid, Key: 0, OwnerID: objectID, Handle: gridHandle},
			{Kind: constt.InventoryEquipment, Key: 0, OwnerID: objectID, Handle: equipmentHandle},
			{Kind: constt.InventoryHand, Key: 0, OwnerID: objectID, Handle: handHandle},
		},
	})

	inventories, err := factory.SerializeObjectInventories(world, objectHandle)
	if err != nil {
		t.Fatalf("unexpected serialize error: %v", err)
	}
	if len(inventories) != 3 {
		t.Fatalf("expected 3 root inventories, got %d", len(inventories))
	}

	foundKinds := map[constt.InventoryKind]bool{}
	for _, inv := range inventories {
		if inv.OwnerID != int64(objectID) {
			t.Fatalf("unexpected owner id %d", inv.OwnerID)
		}
		foundKinds[constt.InventoryKind(inv.Kind)] = true
	}

	if !foundKinds[constt.InventoryGrid] {
		t.Fatalf("missing grid inventory in serialized rows")
	}
	if !foundKinds[constt.InventoryEquipment] {
		t.Fatalf("missing equipment inventory in serialized rows")
	}
	if !foundKinds[constt.InventoryHand] {
		t.Fatalf("missing hand inventory in serialized rows")
	}
}
