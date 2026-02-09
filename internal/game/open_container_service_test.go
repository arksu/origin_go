package game

import (
	constt "origin/internal/const"
	"origin/internal/ecs"
	netproto "origin/internal/network/proto"
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
