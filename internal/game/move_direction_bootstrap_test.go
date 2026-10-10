package game

import (
	"testing"
	"time"

	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/timeutil"
	"origin/internal/types"
)

func TestDirectionBootstrapAndIngressToMovement(t *testing.T) {
	client, connection := connectPlayerSpawnTestClient(t)
	client.CharacterID = 1
	client.InWorld.Store(true)
	client.StreamEpoch.Store(7)
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Speed: 32, Mode: constt.Walk})
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
	})
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 40, MaxCommandsPerTickPerClient: 20})
	shard := &Shard{world: w, playerInbox: inbox, Clients: map[types.EntityID]*network.Client{1: client}}
	clock := timeutil.NewManualClock(time.Unix(10000, 0))
	g := &Game{logger: zap.NewNop(), clock: clock, tickRate: 10, cfg: &config.Config{Game: config.GameConfig{TickRate: 10}}, shardManager: &ShardManager{shards: map[int]*Shard{0: shard}}}
	constantsData, err := marshalServerConstants(g.tickRate)
	require.NoError(t, err)
	g.serverConstantsData = constantsData
	require.True(t, g.sendAuthenticatedBootstrap(client, 1))
	g.sendPlayerEnterWorld(client, 1, shard, repository.Character{Name: "wasd-test"})
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(time.Second)))
	wire, _, err := wsutil.ReadServerData(connection)
	require.NoError(t, err)
	var auth netproto.ServerMessage
	require.NoError(t, proto.Unmarshal(wire, &auth))
	require.True(t, auth.GetAuthResult().GetSuccess())
	wire, _, err = wsutil.ReadServerData(connection)
	require.NoError(t, err)
	var constants netproto.ServerMessage
	require.NoError(t, proto.Unmarshal(wire, &constants))
	require.True(t, constants.GetServerConstants().GetDirectionalMovementSupported())
	require.Equal(t, uint32(10), constants.GetServerConstants().GetTickRate())
	wire, _, err = wsutil.ReadServerData(connection)
	require.NoError(t, err)
	var snapshot netproto.ServerMessage
	require.NoError(t, proto.Unmarshal(wire, &snapshot))
	require.Equal(t, uint32(7), snapshot.GetPlayerEnterWorld().GetStreamEpoch())
	require.Equal(t, uint64(1), snapshot.GetPlayerEnterWorld().GetEntityId())
	require.Equal(t, "wasd-test", snapshot.GetPlayerEnterWorld().GetName())
	require.NotNil(t, snapshot.GetPlayerEnterWorld().GetAudio())
	commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20}), nil, nil, nil, nil, nil, 0, zap.NewNop())
	commands.SetDirectionalSessionValidator(shard.validDirectionalSession)
	mover := systems.NewMovementSystem(w, nil, zap.NewNop())
	ecs.SetResource(w, ecs.TimeState{Now: time.Unix(100, 0), WallNow: clock.WallNow()})
	for index, vector := range [][2]float32{{1, 0}, {0, 0}} {
		g.handlePlayerAction(client, uint32(index+1), &netproto.C2S_PlayerAction{Action: &netproto.C2S_PlayerAction_MoveDirection{MoveDirection: &netproto.MoveDirection{X: vector[0], Y: vector[1], InputRevision: uint32(index + 1), StreamEpoch: 7}}})
		commands.Update(w, .1)
		systems.NewResetSystem(zap.NewNop()).Update(w, .1)
		mover.Update(w, .1)
		moves := ecs.GetResource[ecs.MovedEntities](w)
		require.Equal(t, 1, moves.Count)
		require.InDelta(t, 100+float64(vector[0])*3.2, moves.IntentX[0], 1e-10)
		movement, _ := ecs.GetComponent[components.Movement](w, player)
		require.Equal(t, index == 0, movement.TargetType == constt.TargetDirection)
	}
}
