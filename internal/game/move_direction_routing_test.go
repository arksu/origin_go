package game

import (
	"math"
	"testing"
	"time"

	"go.uber.org/zap"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/timeutil"
)

func TestPlayerActionRoutesMoveDirection(t *testing.T) {
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 40, MaxCommandsPerTickPerClient: 20})
	clock := timeutil.NewManualClock(time.Unix(1234, 0))
	g := &Game{clock: clock, logger: zap.NewNop(), shardManager: &ShardManager{shards: map[int]*Shard{0: {playerInbox: inbox}}}}
	client := &network.Client{ID: 1, CharacterID: 42}
	client.InWorld.Store(true)
	client.StreamEpoch.Store(7)
	for index, direction := range []*netproto.MoveDirection{
		{X: -0.31622776, Y: -0.9486833, InputRevision: 1, StreamEpoch: 7},
		{X: 1, InputRevision: 2, StreamEpoch: 7},
		{InputRevision: 3, StreamEpoch: 7},
	} {
		g.handlePlayerAction(client, uint32(index+1), &netproto.C2S_PlayerAction{Action: &netproto.C2S_PlayerAction_MoveDirection{MoveDirection: direction}})
		commands := inbox.Drain()
		if len(commands) != 1 {
			t.Fatalf("expected one command, got %d", len(commands))
		}
		command := commands[0]
		if command.CommandType != network.CmdMoveDirection || command.Payload != direction || command.ClientID != 1 || command.CharacterID != 42 || command.Layer != 0 || command.CommandID != uint64(index+1) || !command.ReceivedAt.Equal(clock.WallNow()) {
			t.Fatalf("wrong routing: %+v", command)
		}
	}
	for _, direction := range []*netproto.MoveDirection{
		nil,
		{X: 1, StreamEpoch: 7},
		{X: float32(math.NaN()), InputRevision: 4, StreamEpoch: 7},
		{Y: float32(math.Inf(1)), InputRevision: 4, StreamEpoch: 7},
		{X: float32(math.Inf(-1)), InputRevision: 4, StreamEpoch: 7},
		{X: 1.01, InputRevision: 4, StreamEpoch: 7},
		{Y: -1.01, InputRevision: 4, StreamEpoch: 7},
		{X: 1, InputRevision: 4, StreamEpoch: 6},
		{X: 1, InputRevision: 4},
	} {
		g.handlePlayerAction(client, 4, &netproto.C2S_PlayerAction{Action: &netproto.C2S_PlayerAction_MoveDirection{MoveDirection: direction}})
	}
	g.handlePlayerAction(client, 4, nil)
	g.handlePlayerAction(client, 4, &netproto.C2S_PlayerAction{Action: (*netproto.C2S_PlayerAction_MoveDirection)(nil)})
	client.InWorld.Store(false)
	g.handlePlayerAction(client, 4, &netproto.C2S_PlayerAction{Action: &netproto.C2S_PlayerAction_MoveDirection{MoveDirection: &netproto.MoveDirection{X: 1, InputRevision: 4, StreamEpoch: 7}}})
	if commands := inbox.Drain(); len(commands) != 0 {
		t.Fatalf("invalid input enqueued: %v", commands)
	}
}
