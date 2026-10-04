package systems

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
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func TestIncapacitatedLinksBreakOnceAndDoNotResumeAfterStanding(t *testing.T) {
	for _, health := range []components.EntityHealth{
		{SHP: 5, HHP: 20, KOUntilUnixMs: 60000, IsLying: true},
		{SHP: 5, HHP: 20, IsLying: true},
		{SHP: 0, HHP: 20},
	} {
		bus := newTestEventBus(t)
		t.Cleanup(func() { shutdownTestEventBus(t, bus) })
		world := ecs.NewWorld(bus, 0)
		player := spawnPlayerForLinkTests(world, 1, 10, 10, 2)
		target := spawnTargetForLinkTests(world, 2, 12, 10)
		other := spawnPlayerForLinkTests(world, 3, 10, 10, 2)
		ecs.AddComponent(world, player, health)
		links := ecs.GetResource[ecs.LinkState](world)
		for id, handle := range map[types.EntityID]types.Handle{1: player, 3: other} {
			links.SetLink(ecs.PlayerLink{PlayerID: id, PlayerHandle: handle, TargetID: 2, TargetHandle: target, PlayerX: 10, PlayerY: 10, TargetX: 12, TargetY: 10})
		}
		links.SetIntent(1, 99, types.InvalidHandle, time.Time{})
		var broken []*ecs.LinkBrokenEvent
		created := 0
		bus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityHigh, func(_ context.Context, event eventbus.Event) error {
			broken = append(broken, event.(*ecs.LinkBrokenEvent))
			return nil
		})
		bus.SubscribeSync(ecs.TopicGameplayLinkCreated, eventbus.PriorityHigh, func(_ context.Context, _ eventbus.Event) error { created++; return nil })
		system := NewLinkSystem(bus, zap.NewNop())
		system.Update(world, 0)
		system.Update(world, 0)
		require.Len(t, broken, 1)
		require.Equal(t, ecs.LinkBreakKnockedOut, broken[0].BreakReason)
		require.NotContains(t, links.LinkedByPlayer, types.EntityID(1))
		require.NotContains(t, links.IntentByPlayer, types.EntityID(1))
		require.NotContains(t, links.PlayersByTarget[2], types.EntityID(1))
		require.Contains(t, links.PlayersByTarget[2], types.EntityID(3), "other players retain their links")
		links.SetIntent(1, 2, target, time.Time{})
		system.Update(world, 0)
		require.Zero(t, created)
		ecs.AddComponent(world, player, components.EntityHealth{SHP: 5, HHP: 20})
		system.Update(world, 0)
		require.Zero(t, created, "standing replayed the rejected intent")
		links.SetIntent(1, 2, target, time.Time{})
		system.Update(world, 0)
		require.Equal(t, 1, created, "a fresh explicit link is allowed after standing")
	}
}

func TestIncapacitatedMovementRejectsClicksDirectionsAndServerApproaches(t *testing.T) {
	for _, health := range []components.EntityHealth{
		{SHP: 5, HHP: 20, KOUntilUnixMs: 60000, IsLying: true},
		{SHP: 5, HHP: 20, IsLying: true},
	} {
		fixture := newDirectionalFixture(t)
		ecs.AddComponent(fixture.world, fixture.player, health)
		target := fixture.world.Spawn(2, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Transform{X: 200, Y: 100})
			ecs.AddComponent(w, h, components.Collider{})
		})
		for _, button := range []netproto.MapClickButton{netproto.MapClickButton_MAP_CLICK_BUTTON_PRIMARY, netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY} {
			for _, targetID := range []uint64{0, 2} {
				fixture.commands.handleMapClick(fixture.world, fixture.player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 200, Y: 100, Button: button, TargetEntityId: targetID}})
			}
		}
		require.False(t, fixture.commands.BeginActionMoveToLink(fixture.world, fixture.player, 1, 2, target))
		fixture.send(1, 0, 1, 0)
		require.Equal(t, constt.TargetNone, fixture.state().TargetType)
		require.Empty(t, ecs.GetResource[ecs.LinkState](fixture.world).IntentByPlayer)
		// Rejected fresh input is retired; its heartbeat cannot move after standing.
		ecs.AddComponent(fixture.world, fixture.player, components.EntityHealth{SHP: 5, HHP: 20})
		fixture.send(1, 0, 1, 0)
		require.Equal(t, constt.TargetNone, fixture.state().TargetType)
		fixture.send(1, 0, 2, 0)
		require.Equal(t, constt.TargetDirection, fixture.state().TargetType)
	}
}
