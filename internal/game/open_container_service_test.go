package game

import (
	"context"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"
	"testing"
)

type testContainerSender struct {
	closed []closedEvent
}

type closedEvent struct {
	playerID types.EntityID
	ref      *netproto.InventoryRef
}

func (s *testContainerSender) SendContainerOpened(entityID types.EntityID, state *netproto.InventoryState) {
}
func (s *testContainerSender) SendInventoryUpdate(entityID types.EntityID, states []*netproto.InventoryState) {
}
func (s *testContainerSender) SendContainerClosed(entityID types.EntityID, ref *netproto.InventoryRef) {
	s.closed = append(s.closed, closedEvent{playerID: entityID, ref: ref})
}

func TestOpenContainerService_CloseRefsForOpenedPlayers(t *testing.T) {
	w := ecs.NewWorldForTesting()
	sender := &testContainerSender{}
	service := NewOpenContainerService(w, nil, sender, nil)

	openState := ecs.GetResource[ecs.OpenContainerState](w)
	key := ecs.InventoryRefKey{
		Kind:    constt.InventoryGrid,
		OwnerID: 9000001,
		Key:     0,
	}

	playerA := types.EntityID(1001)
	playerB := types.EntityID(1002)
	openState.OpenRef(playerA, key)
	openState.OpenRef(playerB, key)

	service.CloseRefsForOpenedPlayers(w, []*netproto.InventoryRef{
		{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(key.OwnerID),
			InventoryKey: key.Key,
		},
	})

	if openState.IsRefOpened(playerA, key) {
		t.Fatalf("expected ref to be closed for player %d", playerA)
	}
	if openState.IsRefOpened(playerB, key) {
		t.Fatalf("expected ref to be closed for player %d", playerB)
	}
	if len(sender.closed) != 2 {
		t.Fatalf("expected 2 close notifications, got %d", len(sender.closed))
	}
}

func TestOpenContainerService_HandleOpenRequest_MarksRootBehaviorDirty(t *testing.T) {
	w := ecs.NewWorldForTesting()
	sender := &testContainerSender{}
	service := NewOpenContainerService(w, nil, sender, nil)
	setObjectDefsForContainerTests()

	objectID := types.EntityID(2001)
	objectHandle := spawnContainerObjectForTest(w, objectID)
	playerID := types.EntityID(1001)
	playerHandle := w.Spawn(playerID, nil)

	ecs.GetResource[ecs.LinkState](w).SetLink(ecs.PlayerLink{
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     objectID,
		TargetHandle: objectHandle,
	})

	openErr := service.HandleOpenRequest(w, playerID, playerHandle, &netproto.InventoryRef{
		Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
		OwnerId:      uint64(objectID),
		InventoryKey: 0,
	})
	if openErr != nil {
		t.Fatalf("expected open request to succeed, got error: %+v", openErr)
	}

	dirty := ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](w).Drain(8, nil)
	if len(dirty) != 1 || dirty[0] != objectHandle {
		t.Fatalf("expected object behavior dirty queue to contain object handle %v, got %#v", objectHandle, dirty)
	}
}

func TestOpenContainerService_OnLinkBroken_MarksRootBehaviorDirty(t *testing.T) {
	w := ecs.NewWorldForTesting()
	sender := &testContainerSender{}
	service := NewOpenContainerService(w, nil, sender, nil)
	setObjectDefsForContainerTests()

	objectID := types.EntityID(2001)
	objectHandle := spawnContainerObjectForTest(w, objectID)
	playerID := types.EntityID(1001)

	openState := ecs.GetResource[ecs.OpenContainerState](w)
	openState.SetRootOpened(playerID, objectID)
	openState.OpenRef(playerID, ecs.InventoryRefKey{
		Kind:    constt.InventoryGrid,
		OwnerID: objectID,
		Key:     0,
	})

	err := service.onLinkBroken(context.Background(), ecs.NewLinkBrokenEvent(w.Layer, playerID, objectID, ecs.LinkBreakMoved))
	if err != nil {
		t.Fatalf("expected nil error from onLinkBroken, got: %v", err)
	}

	if _, hasRoot := openState.GetOpenedRoot(playerID); hasRoot {
		t.Fatalf("expected opened root to be cleared for player %d", playerID)
	}
	dirty := ecs.GetResource[ecs.ObjectBehaviorDirtyQueue](w).Drain(8, nil)
	if len(dirty) != 1 || dirty[0] != objectHandle {
		t.Fatalf("expected object behavior dirty queue to contain object handle %v, got %#v", objectHandle, dirty)
	}
}

func spawnContainerObjectForTest(w *ecs.World, objectID types.EntityID) types.Handle {
	objectHandle := w.Spawn(objectID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    10,
			Behaviors: []string{"container"},
			IsStatic:  true,
			Region:    1,
			Layer:     0,
		})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})

	containerHandle := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, containerHandle, components.InventoryContainer{
		OwnerID: objectID,
		Kind:    constt.InventoryGrid,
		Key:     0,
		Width:   4,
		Height:  4,
	})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, objectID, 0, containerHandle)
	return objectHandle
}

func setObjectDefsForContainerTests() {
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID: 10,
			Key:   "box",
			Name:  "Box",
		},
	}))
}
