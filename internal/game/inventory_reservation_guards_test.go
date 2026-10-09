package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInventoryReservationGuardsTransferBeforeCapture(t *testing.T) {
	w := ecs.NewWorldForTesting()
	owner := w.Spawn(41, nil)
	client := &network.Client{CharacterID: 41}
	client.InWorld.Store(true)
	shard := &Shard{world: w, Clients: map[types.EntityID]*network.Client{41: client}}
	require.True(t, ecs.ReserveInventoryOwner(w, 41, owner))
	service := NewPlayerTransferService(nil, zap.NewNop())
	participant := &transferSaveGuardParticipant{}
	service.RegisterParticipant(participant)
	_, err := service.detachTransferSource(PlayerTransferRequest{PlayerID: 41}, shard, repository.Character{ID: 41})
	require.ErrorContains(t, err, "inventory operation is still being committed")
	require.False(t, participant.captured)
	require.True(t, client.InWorld.Load())
	require.True(t, w.Alive(owner))
}

func TestInventoryReservationGuardsCorpseLiftAndAdminDestroy(t *testing.T) {
	w := ecs.NewWorldForTesting()
	owner := w.Spawn(41, nil)
	ecs.AddComponent(w, owner, components.EntityInfo{TypeID: 17, Behaviors: []string{"lift"}, Region: 1})
	lift := &LiftService{}
	require.True(t, lift.isLiftableTarget(w, owner))
	require.True(t, ecs.ReserveInventoryOwner(w, 41, owner))
	require.False(t, lift.isLiftableTarget(w, owner))
	chat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
	handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, nil, zap.NewNop())
	handler.HandleCommand(w, 1, types.InvalidHandle, "/destroy")
	handler.ExecutePendingDestroy(w, 1, 41)
	require.Equal(t, "Object destruction target is unavailable.", chat.messages[1])
	require.True(t, w.Alive(owner))
}
