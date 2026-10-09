package game

import (
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func drainActionTransitions(t *testing.T, client *network.Client, conn net.Conn) []*netproto.ServerMessage {
	t.Helper()
	barrier, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
	require.NoError(t, err)
	require.True(t, client.SendCritical(barrier))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	var messages []*netproto.ServerMessage
	for {
		encoded, opcode, err := wsutil.ReadServerData(conn)
		require.NoError(t, err)
		require.Equal(t, ws.OpBinary, opcode)
		message := &netproto.ServerMessage{}
		require.NoError(t, proto.Unmarshal(encoded, message))
		if message.GetPong() != nil {
			return messages
		}
		require.True(t, message.GetActionStateChanged() != nil || message.GetCyclicActionFinished() != nil)
		messages = append(messages, message)
	}
}

func TestActionTransitionsCriticalFIFOAndHeading(t *testing.T) {
	f := newAttackResultFixture(t, 32)
	melee := meleeFixtureInWorld(t, f.shard.world)
	melee.actions.sender = f.shard
	client, conn := f.connect(t, 1, 7)
	melee.actions.nextGeneration = 1 << 53
	f.shard.mu.Lock()
	// No animation binding is installed in this fixture. Confirmation must still
	// carry an explicitly present zero angle and an exact uint64 generation.
	melee.start("axe_sweep", 0)
	melee.actions.Cancel(melee.world, 1, melee.owner)
	ecs.WithComponent(melee.world, melee.owner, func(transform *components.Transform) { transform.Direction = -0.25 })
	legacyAim := float32(2)
	melee.actions.ActivateRequest(melee.world, 1, melee.owner, &netproto.C2S_ActivateAction{ActionId: "axe_strike", StreamEpoch: 7, AimAngle: &legacyAim})
	melee.finish()
	f.shard.mu.Unlock()
	messages := drainActionTransitions(t, client, conn)
	require.Len(t, messages, 6)
	first := messages[0].GetActionStateChanged()
	require.Equal(t, "executing", first.Phase)
	require.EqualValues(t, uint64(1<<53)+1, first.ActionGeneration)
	require.NotNil(t, first.FacingAngle)
	require.Zero(t, *first.FacingAngle)
	require.EqualValues(t, 7, first.StreamEpoch)
	require.Equal(t, netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED, messages[1].GetCyclicActionFinished().Result)
	require.Equal(t, "idle", messages[2].GetActionStateChanged().Phase)
	require.Zero(t, messages[2].GetActionStateChanged().ActionGeneration)
	require.Nil(t, messages[2].GetActionStateChanged().FacingAngle)
	second := messages[3].GetActionStateChanged()
	require.Equal(t, "executing", second.Phase)
	require.EqualValues(t, uint64(1<<53)+2, second.ActionGeneration)
	require.Equal(t, float32(2*math.Pi-0.25), second.GetFacingAngle())
	require.Equal(t, netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_COMPLETED, messages[4].GetCyclicActionFinished().Result)
	require.Equal(t, "idle", messages[5].GetActionStateChanged().Phase)
	require.Equal(t, 940.0, melee.stamina())
}

func TestActionTransitionUsesCurrentConnectionAndEpoch(t *testing.T) {
	f := newAttackResultFixture(t, 16)
	oldClient, oldConn := f.connect(t, 1, 4)
	current, currentConn := f.connect(t, 1, 9)
	angle := float32(0)
	state := &netproto.S2C_ActionStateChanged{ActionId: "axe_sweep", Phase: "executing", ActionGeneration: 17, FacingAngle: &angle, StreamEpoch: 4}
	f.shard.mu.Lock()
	f.shard.SendActionStateChanged(1, state)
	f.shard.SendCyclicActionFinished(1, &netproto.S2C_CyclicActionFinished{ActionId: "axe_sweep"})
	f.shard.mu.Unlock()
	require.EqualValues(t, 4, state.StreamEpoch, "caller snapshot must retain its epoch")
	require.Empty(t, drainActionTransitions(t, oldClient, oldConn))
	messages := drainActionTransitions(t, current, currentConn)
	require.Len(t, messages, 2)
	require.EqualValues(t, 9, messages[0].GetActionStateChanged().StreamEpoch)
	for _, invalid := range []string{"not in world", "identity", "layer", "zero epoch"} {
		switch invalid {
		case "not in world":
			current.InWorld.Store(false)
		case "identity":
			current.InWorld.Store(true)
			current.CharacterID = 2
		case "layer":
			current.CharacterID = 1
			current.Layer++
		case "zero epoch":
			current.Layer = f.shard.layer
			current.StreamEpoch.Store(0)
		}
		f.shard.mu.Lock()
		f.shard.SendActionStateChanged(1, state)
		f.shard.SendCyclicActionFinished(1, &netproto.S2C_CyclicActionFinished{ActionId: "axe_sweep"})
		f.shard.mu.Unlock()
		require.Empty(t, drainActionTransitions(t, current, currentConn), invalid)
	}
}

func TestActionTransitionCriticalOverflowClosesWithoutReplayingDamage(t *testing.T) {
	for _, transition := range []string{"state", "finished"} {
		t.Run(transition, func(t *testing.T) {
			f := newAttackResultFixture(t, 1)
			melee := meleeFixtureInWorld(t, f.shard.world)
			melee.actions.sender = f.shard
			release := make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			f.server.SetOnConnect(func(client *network.Client) { f.connected <- client; <-release })
			client, _ := f.connect(t, 1, 7)
			disconnected := make(chan struct{})
			f.server.SetOnDisconnect(func(*network.Client) {
				f.shard.ClientsMu.Lock()
				delete(f.shard.Clients, types.EntityID(1))
				f.shard.ClientsMu.Unlock()
				close(disconnected)
			})
			// The writer cannot drain while onConnect is blocked. The first
			// executing confirmation occupies the only available queue slot.
			f.shard.mu.Lock()
			melee.start("axe_sweep", 0)
			if transition == "state" {
				melee.actions.SendState(melee.world, 1, melee.owner)
			}
			melee.finish()
			f.shard.mu.Unlock()
			select {
			case <-disconnected:
			case <-time.After(time.Second):
				t.Fatal("critical transition overflow did not close the connection")
			}
			require.False(t, client.SendCritical([]byte{1}))
			require.Equal(t, 940.0, melee.stamina())
			require.Len(t, melee.sender.attacks, 1)
			melee.finish()
			require.Equal(t, 940.0, melee.stamina())
			require.Len(t, melee.sender.attacks, 1)
			once.Do(func() { close(release) })
		})
	}
}
