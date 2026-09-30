package game

import (
	"fmt"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/network"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type transferSaveGuardParticipant struct {
	captured bool
}

func (*transferSaveGuardParticipant) Key() string { return "save_guard" }

func (p *transferSaveGuardParticipant) CaptureSource(*Game, *Shard, PlayerTransferRequest, types.Handle) (any, error) {
	p.captured = true
	return nil, fmt.Errorf("capture must not run after save failure")
}

func (*transferSaveGuardParticipant) RestoreTarget(*Game, *Shard, PlayerTransferRequest, types.Handle, any) error {
	return nil
}

func (*transferSaveGuardParticipant) RestoreSourceRollback(*Game, *Shard, PlayerTransferRequest, types.Handle, any) error {
	return nil
}

func (*transferSaveGuardParticipant) OnTargetRestoreFailure(*Game, *Shard, PlayerTransferRequest, types.Handle, any, error) {
}

func TestPlayerTransferSaveFailurePreservesSourceBeforeCapture(t *testing.T) {
	w := ecs.NewWorldForTesting()
	playerID := types.EntityID(10)
	player := w.Spawn(playerID, nil)
	transform := components.Transform{X: 100, Y: 200}
	interaction := components.PendingInteraction{TargetEntityID: 20, Range: 5}
	ecs.AddComponent(w, player, transform)
	ecs.AddComponent(w, player, components.EntityStats{Stamina: 100, Energy: 100})
	ecs.AddComponent(w, player, interaction)
	container := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, container, components.InventoryContainer{OwnerID: playerID, Kind: constt.InventoryHand, Version: 1})
	ecs.AddComponent(w, player, components.InventoryOwner{Inventories: []components.InventoryLink{{OwnerID: playerID, Kind: constt.InventoryHand, Handle: container}}})
	ecs.GetResource[ecs.CharacterEntities](w).Add(playerID, player, time.Now().Add(time.Hour))
	client := &network.Client{ID: 1, CharacterID: playerID, Layer: 1}
	client.InWorld.Store(true)

	// A stopped empty saver gives a deterministic persistence error without a database.
	saver := systems.NewCharacterSaver(nil, 0, inventory.NewInventorySaver(zap.NewNop()), zap.NewNop())
	saver.Stop()
	shard := &Shard{world: w, Clients: map[types.EntityID]*network.Client{playerID: client}, characterSaver: saver}
	service := NewPlayerTransferService(nil, zap.NewNop())
	participant := &transferSaveGuardParticipant{}
	service.RegisterParticipant(participant)

	snapshot, err := service.detachTransferSource(PlayerTransferRequest{PlayerID: playerID, SourceLayer: 1, TargetLayer: 2}, shard, repository.Character{ID: int64(playerID)})

	require.ErrorContains(t, err, "save character before transfer: character saver is stopped")
	require.False(t, participant.captured, "participants may mutate carry state while capturing it")
	require.Same(t, client, snapshot.Client)
	require.Equal(t, 100, snapshot.SourceX)
	require.Equal(t, 200, snapshot.SourceY)
	require.Same(t, client, shard.Clients[playerID])
	require.True(t, client.InWorld.Load())
	require.Equal(t, 1, client.Layer)
	require.True(t, w.Alive(player))
	require.True(t, w.Alive(container))
	require.Equal(t, player, w.GetHandleByEntityID(playerID))
	currentTransform, hasTransform := ecs.GetComponent[components.Transform](w, player)
	require.True(t, hasTransform)
	require.Equal(t, transform, currentTransform)
	currentInteraction, hasInteraction := ecs.GetComponent[components.PendingInteraction](w, player)
	require.True(t, hasInteraction)
	require.Equal(t, interaction, currentInteraction)
	_, tracked := ecs.GetResource[ecs.CharacterEntities](w).Map[playerID]
	require.True(t, tracked)
}
