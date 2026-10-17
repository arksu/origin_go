package game

import (
	"context"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type hearthWindowSender struct {
	opened []*netproto.S2C_CustomWindow
	closed []types.EntityID
}

func (*hearthWindowSender) SendMiniAlert(types.EntityID, *netproto.S2C_MiniAlert) {}
func (s *hearthWindowSender) SendCustomWindow(_ types.EntityID, window *netproto.S2C_CustomWindow) {
	s.opened = append(s.opened, window)
}
func (s *hearthWindowSender) SendCustomWindowClosed(_ types.EntityID, target types.EntityID) {
	s.closed = append(s.closed, target)
}

type hearthWindowFixture struct {
	shard          *Shard
	runtime        *hearth.Runtime
	player, target types.Handle
	commands       *systems.NetworkCommandSystem
	inbox          *network.PlayerCommandInbox
	actions        *ContextActionService
	sender         *hearthWindowSender
	commandID      uint64
}

func newHearthWindowFixture(t *testing.T, owner types.EntityID, startX float64) *hearthWindowFixture {
	t.Helper()
	loadHearthBuildRecipeForTest(t)
	shard, runtime, target := newHearthCollisionTestShard(t)
	w := shard.world
	require.NoError(t, runtime.Index.LoadIfUnknown(owner, &hearth.Home{
		Object: hearth.ObjectKey{Region: 1, ID: 200}, X: 200, Y: 200, OwnerName: "Owner <Марко>",
	}))
	player := w.Spawn(10, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: startX, Y: 200})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: constt.PlayerLayer, Mask: constt.PlayerMask})
		ecs.AddComponent(w, h, components.Movement{Speed: 20, Mode: constt.Walk})
		ecs.AddComponent(w, h, components.EntityHealth{SHP: 100, HHP: 100})
		ecs.AddComponent(w, h, components.ChunkRef{})
		ecs.AddComponent(w, h, components.CollisionResult{})
	})
	shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().AddDynamic(player, int(startX), 200)
	sender := &hearthWindowSender{}
	actions := NewContextActionService(w, shard.eventBus, nil, nil, sender, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), shard.logger)
	config := network.DefaultCommandQueueConfig()
	inbox := network.NewPlayerCommandInbox(config)
	commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(config), nil, nil, nil, nil, nil, 0, shard.logger)
	commands.SetContextActionService(actions)
	w.AddSystem(commands)
	w.AddSystem(systems.NewResetSystem(shard.logger))
	w.AddSystem(systems.NewMovementSystem(w, shard.chunkManager, shard.logger))
	collision := systems.NewCollisionSystem(w, shard.chunkManager, shard.logger, 0, constt.ChunkWorldSize, 0, constt.ChunkWorldSize, 0)
	collision.SetObjectCollisionFilter(runtime.IgnoreObjectCollision)
	w.AddSystem(collision)
	w.AddSystem(systems.NewTransformUpdateSystem(w, shard.chunkManager, shard.eventBus, shard.logger))
	links := systems.NewLinkSystem(shard.eventBus, shard.logger)
	links.SetAdditionalContact(runtime.HasOwnerContact)
	w.AddSystem(links)
	return &hearthWindowFixture{shard: shard, runtime: runtime, player: player, target: target, commands: commands, inbox: inbox, actions: actions, sender: sender}
}

func (f *hearthWindowFixture) click(t *testing.T, target uint64, button netproto.MapClickButton, x int32) {
	t.Helper()
	f.commandID++
	require.NoError(t, f.inbox.Enqueue(&network.PlayerCommand{ClientID: 1, CharacterID: 10, CommandID: f.commandID,
		CommandType: network.CmdMapClick, Payload: &netproto.MapClick{TargetEntityId: target, X: x, Y: 200, Button: button}}))
	f.commands.Update(f.shard.world, .1)
}

func (f *hearthWindowFixture) ticks(count int) {
	for range count {
		f.shard.mu.Lock()
		f.shard.world.Update(.1)
		f.shard.mu.Unlock()
	}
}

func TestHearthWindowRMBApproachesLinksAndReopens(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		owner types.EntityID
		x     float64
	}{
		{"own", 10, 150}, {"foreign offline owner", 99, 150}, {"inside own", 10, 197}, {"center of own", 10, 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newHearthWindowFixture(t, scenario.owner, scenario.x)
			w := f.shard.world
			f.actions.ExecuteAction(w, 10, f.player, 200, f.target, "open")
			require.Empty(t, f.sender.opened, "execution without link must not expose the window")
			f.click(t, 200, netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, 200)
			require.Empty(t, f.sender.opened, "RMB alone does not open the window")
			f.ticks(35)
			require.Len(t, f.sender.opened, 1)
			require.Equal(t, "Hearth Fire", f.sender.opened[0].Title)
			require.Equal(t, "Owner", f.sender.opened[0].Fields[0].Label)
			require.Equal(t, "Owner <Марко>", f.sender.opened[0].Fields[0].Value)
			require.Len(t, f.sender.opened[0].Fields, 2)
			require.Equal(t, "Last disconnect", f.sender.opened[0].Fields[1].Label)
			require.Equal(t, "Unknown", f.sender.opened[0].Fields[1].Value)
			link, linked := ecs.GetResource[ecs.LinkState](w).GetLink(10)
			require.True(t, linked)
			require.Equal(t, f.target, link.TargetHandle)
			movement, _ := ecs.GetComponent[components.Movement](w, f.player)
			require.Equal(t, constt.StateIdle, movement.State)
			f.click(t, 200, netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, 200)
			require.Len(t, f.sender.opened, 2, "a locally closed window can reopen without moving")
			f.click(t, 0, netproto.MapClickButton_MAP_CLICK_BUTTON_PRIMARY, 150)
			f.ticks(2)
			require.Equal(t, []types.EntityID{200}, f.sender.closed)
		})
	}
}

func TestHearthWindowDisconnectRefreshesOnlyOnReopen(t *testing.T) {
	f := newHearthWindowFixture(t, 99, 180)
	clock := ecs.GetResource[ecs.TimeState](f.shard.world)
	clock.UnixMs = 10_000_000
	f.runtime.Index.RecordDisconnect(99, clock.UnixMs-2*3_600_000)
	f.click(t, 200, netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, 200)
	f.ticks(10)
	require.Len(t, f.sender.opened, 1)
	require.Equal(t, "2 hours ago", f.sender.opened[0].Fields[1].Value)
	f.runtime.Index.RecordDisconnect(99, clock.UnixMs-120_000)
	require.Len(t, f.sender.opened, 1, "metadata changes do not push window snapshots")
	f.click(t, 200, netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, 200)
	require.Len(t, f.sender.opened, 2)
	require.Equal(t, "2 minutes ago", f.sender.opened[1].Fields[1].Value)
	require.Equal(t, "2 hours ago", f.sender.opened[0].Fields[1].Value, "sent snapshots own their text")
}

func TestHearthWindowCanceledBlockedAndChangedApproaches(t *testing.T) {
	for _, scenario := range []string{"cancel", "blocked", "revoked", "despawn", "knockout", "no intent"} {
		t.Run(scenario, func(t *testing.T) {
			f := newHearthWindowFixture(t, 10, 150)
			w := f.shard.world
			if scenario == "no intent" {
				f.click(t, 0, netproto.MapClickButton_MAP_CLICK_BUTTON_PRIMARY, 240)
			} else {
				f.click(t, 200, netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, 200)
			}
			switch scenario {
			case "cancel":
				f.click(t, 0, netproto.MapClickButton_MAP_CLICK_BUTTON_PRIMARY, 130)
			case "blocked":
				addHearthCollisionTestObject(f.shard, 300, 175, 200, nil)
			case "revoked":
				_, err := f.runtime.Index.RevokeHome(10)
				require.NoError(t, err)
			case "despawn":
				w.Despawn(f.target)
			case "knockout":
				ecs.WithComponent(w, f.player, func(health *components.EntityHealth) { health.SHP = 0 })
			}
			f.ticks(60)
			require.Empty(t, f.sender.opened)
			if scenario == "no intent" {
				position, _ := ecs.GetComponent[components.Transform](w, f.player)
				require.InDelta(t, 240, position.X, 1, "ordinary passage through own hearth stays free")
			}
		})
	}
}

func TestHearthWindowLinkCloseUsesLayerAndCurrentTarget(t *testing.T) {
	f := newHearthWindowFixture(t, 99, 180)
	f.click(t, 200, netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, 200)
	f.ticks(10)
	require.Len(t, f.sender.opened, 1)
	require.NoError(t, f.shard.eventBus.PublishSync(ecs.NewLinkBrokenEvent(1, 10, 200, ecs.LinkBreakClosed)))
	require.Empty(t, f.sender.closed)
	ecs.WithComponent(f.shard.world, f.player, func(health *components.EntityHealth) { health.SHP = 0 })
	f.ticks(1)
	require.Equal(t, []types.EntityID{200}, f.sender.closed)
	require.NoError(t, f.actions.onLinkBroken(context.Background(), ecs.NewLinkBrokenEvent(0, 10, 999, ecs.LinkBreakClosed)))
	require.Len(t, f.sender.closed, 1, "unrelated objects do not send custom-window traffic")
}
