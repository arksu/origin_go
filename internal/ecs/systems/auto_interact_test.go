package systems

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type autoPickupExecutorStub struct {
	destinations []*netproto.InventoryRef
	results      []InventoryOpResult
}

func (s *autoPickupExecutorStub) ExecuteOperation(
	_ *ecs.World,
	_ types.EntityID,
	_ types.Handle,
	_ *netproto.InventoryOp,
) InventoryOpResult {
	return InventoryOpResult{}
}

func (s *autoPickupExecutorStub) ExecutePickupFromWorld(
	_ *ecs.World,
	_ types.EntityID,
	_ types.Handle,
	_ types.EntityID,
	dstRef *netproto.InventoryRef,
) InventoryOpResult {
	s.destinations = append(s.destinations, dstRef)
	return s.results[len(s.destinations)-1]
}

func TestAutoPickupRetriesEmptyHandWhenGridIsFull(t *testing.T) {
	w := ecs.NewWorldForTesting()
	playerID := types.EntityID(100)
	playerHandle := w.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryOwner{})
	})

	gridHandle := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, gridHandle, components.InventoryContainer{
		OwnerID: playerID,
		Kind:    constt.InventoryGrid,
		Key:     7,
	})
	ecs.MutateComponent[components.InventoryOwner](w, playerHandle, func(owner *components.InventoryOwner) bool {
		owner.Inventories = []components.InventoryLink{{
			Kind:    constt.InventoryGrid,
			Key:     7,
			OwnerID: playerID,
			Handle:  gridHandle,
		}}
		return true
	})

	handHandle := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, handHandle, components.InventoryContainer{
		OwnerID: playerID,
		Kind:    constt.InventoryHand,
	})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryHand, playerID, 0, handHandle)

	executor := &autoPickupExecutorStub{results: []InventoryOpResult{
		{Success: false, ErrorCode: netproto.ErrorCode_ERROR_CODE_INVENTORY_FULL},
		{Success: true},
	}}
	system := NewAutoInteractSystem(executor, nil, nil, nil)
	system.executePickup(w, playerID, playerHandle, components.PendingInteraction{
		TargetEntityID: 900,
	})

	if len(executor.destinations) != 2 {
		t.Fatalf("pickup attempts = %d, want 2", len(executor.destinations))
	}
	if executor.destinations[0].Kind != netproto.InventoryKind_INVENTORY_KIND_GRID {
		t.Fatalf("first pickup destination = %v, want grid", executor.destinations[0].Kind)
	}
	if executor.destinations[1].Kind != netproto.InventoryKind_INVENTORY_KIND_HAND {
		t.Fatalf("second pickup destination = %v, want hand", executor.destinations[1].Kind)
	}
}

func TestAutoPickupRunsOnceAfterArrival(t *testing.T) {
	world := ecs.NewWorldForTesting()
	target := world.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 200})
	})
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving})
		ecs.AddComponent(w, h, components.PendingInteraction{TargetEntityID: 2, TargetHandle: target, Range: 5})
	})
	hand := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, hand, components.InventoryContainer{OwnerID: 1, Kind: constt.InventoryHand})
	ecs.GetResource[ecs.InventoryRefIndex](world).Add(constt.InventoryHand, 1, 0, hand)
	executor := &autoPickupExecutorStub{results: []InventoryOpResult{{Success: true}}}
	system := NewAutoInteractSystem(executor, nil, nil, nil)
	system.Update(world, 0)
	if len(executor.destinations) != 0 {
		t.Fatal("pickup executed before arrival")
	}
	ecs.WithComponent(world, player, func(position *components.Transform) { position.X, position.Y = 100, 200 })
	system.Update(world, 0)
	system.Update(world, 0)
	if len(executor.destinations) != 1 {
		t.Fatal("arrival must execute pickup exactly once")
	}
	if _, pending := ecs.GetComponent[components.PendingInteraction](world, player); pending {
		t.Fatal("completed pickup left a pending interaction")
	}
}
