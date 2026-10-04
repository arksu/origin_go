package game

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/network"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

func TestKnockoutUnlinksStationAndClosesExternalContainersOnce(t *testing.T) {
	bus := newGameTestEventBus(t)
	t.Cleanup(func() { shutdownGameTestEventBus(t, bus) })
	world := ecs.NewWorld(bus, 0)
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityHealth{SHP: 0, HHP: 20})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 100, Energy: 900})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving, TargetType: constt.TargetEntity, TargetHandle: 99})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{ActionID: "station", TargetKind: components.CyclicActionTargetObject, TargetID: 2})
		ecs.AddComponent(w, h, components.PendingContextAction{TargetEntityID: 4})
		ecs.AddComponent(w, h, components.PendingLiftTransition{})
		ecs.AddComponent(w, h, components.Collider{Phantom: &components.PhantomCollider{WorldX: 42}})
	})
	target := world.Spawn(2, nil)
	ecs.GetResource[ecs.CharacterEntities](world).Add(1, player, time.Time{})
	links := ecs.GetResource[ecs.LinkState](world)
	links.SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target})
	links.SetIntent(1, 4, types.InvalidHandle, time.Time{})
	opened := ecs.GetResource[ecs.OpenContainerState](world)
	opened.SetRootOpened(1, 2)
	for _, key := range []uint32{0, 7} {
		opened.OpenRef(1, ecs.InventoryRefKey{Kind: constt.InventoryGrid, OwnerID: 2, Key: key})
	}
	sender := &testContainerSender{}
	openService := NewOpenContainerService(world, bus, sender, nil)
	contextService := NewContextActionService(world, bus, openService, nil, nil, nil, nil, nil, nil, testSingleBehaviorRegistry{}, nil)
	broken := 0
	bus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityLow, func(_ context.Context, event eventbus.Event) error {
		require.Equal(t, ecs.LinkBreakKnockedOut, event.(*ecs.LinkBrokenEvent).BreakReason)
		broken++
		return nil
	})
	shard := &Shard{world: world, contextActions: contextService, logger: zap.NewNop()}
	*ecs.GetResource[ecs.TimeState](world) = ecs.TimeState{Tick: 1, UnixMs: 1000}
	system := NewPlayerDeathSystem(shard, PlayerDeathSystemConfig{})
	system.Update(world, 0)
	system.Update(world, 0)
	require.Equal(t, 1, broken)
	require.Len(t, sender.closed, 2)
	require.Empty(t, links.LinkedByPlayer)
	require.Empty(t, links.PlayersByTarget)
	require.Empty(t, links.IntentByPlayer)
	_, root := opened.GetOpenedRoot(1)
	require.False(t, root)
	_, pending := ecs.GetComponent[components.PendingContextAction](world, player)
	require.False(t, pending, "a different retarget must also be retired")
	_, cyclic := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	require.False(t, cyclic, "non-item station work must stop too")
	_, lifting := ecs.GetComponent[components.PendingLiftTransition](world, player)
	require.False(t, lifting)
	collider, _ := ecs.GetComponent[components.Collider](world, player)
	require.Nil(t, collider.Phantom)
	movement, _ := ecs.GetComponent[components.Movement](world, player)
	require.Equal(t, constt.TargetNone, movement.TargetType)
	require.Equal(t, constt.StateIdle, movement.State)
	health, _ := ecs.GetComponent[components.EntityHealth](world, player)
	require.Equal(t, int64(61000), health.KOUntilUnixMs)
}

func TestTeleportRejectsIncapacitatedPlayerBeforeLeavingOrParticipantCapture(t *testing.T) {
	for _, health := range []components.EntityHealth{
		{SHP: 5, HHP: 20, KOUntilUnixMs: 60000, IsLying: true},
		{SHP: 5, HHP: 20, IsLying: true},
	} {
		world := ecs.NewWorldForTesting()
		player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, health)
			ecs.AddComponent(w, h, components.Transform{X: 100, Y: 200})
		})
		client := &network.Client{ID: 2, CharacterID: 1, Layer: 0}
		client.InWorld.Store(true)
		shard := &Shard{world: world, Clients: map[types.EntityID]*network.Client{1: client}}
		participant := &transferSaveGuardParticipant{}
		service := NewPlayerTransferService(nil, nil)
		service.RegisterParticipant(participant)
		_, err := service.detachTransferSource(PlayerTransferRequest{PlayerID: 1}, shard, repository.Character{ID: 1})
		require.ErrorContains(t, err, "cannot teleport")
		require.False(t, participant.captured)
		require.True(t, world.Alive(player))
		require.True(t, client.InWorld.Load())
		require.Same(t, client, shard.Clients[1])
		_, cached := shard.offlineHealth.Load(types.EntityID(1))
		require.False(t, cached)

		chat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
		teleport := &mockAdminTeleportExecutor{}
		handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, nil, zap.NewNop())
		handler.SetTeleportExecutor(teleport)
		handler.handleTeleport(world, 1, nil)
		require.False(t, ecs.GetResource[ecs.PendingAdminTeleport](world).Get(1))
		handler.handleTeleport(world, 1, []string{"100", "200"})
		ecs.GetResource[ecs.PendingAdminTeleport](world).Set(1)
		handler.ExecutePendingTeleport(world, 1, player, 100, 200)
		require.Zero(t, teleport.calls)
		require.False(t, ecs.GetResource[ecs.PendingAdminTeleport](world).Get(1))
	}
}
