package game

import (
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"testing"
)

func TestPlayerActionRoutesMapClickAndIgnoresRetiredActions(t *testing.T) {
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 20, MaxCommandsPerTickPerClient: 20})
	g := &Game{logger: zap.NewNop(), shardManager: &ShardManager{shards: map[int]*Shard{0: {playerInbox: inbox}}}}
	client := &network.Client{ID: 1, CharacterID: 42}
	for _, tag := range []byte{0x0a, 0x12} {
		var action netproto.C2S_PlayerAction
		if err := proto.Unmarshal([]byte{tag, 2, 8, 1}, &action); err != nil {
			t.Fatal(err)
		}
		g.handlePlayerAction(client, 1, &action)
	}
	if commands := inbox.Drain(); len(commands) != 0 {
		t.Fatal("retired input was enqueued")
	}
	g.handlePlayerAction(client, 2, &netproto.C2S_PlayerAction{Action: &netproto.C2S_PlayerAction_MapClick{MapClick: &netproto.MapClick{X: 42, Y: 99, TargetEntityId: 777}}})
	commands := inbox.Drain()
	if len(commands) != 1 || commands[0].CommandType != network.CmdMapClick {
		t.Fatalf("wrong routing: %v", commands)
	}
	click, ok := commands[0].Payload.(*netproto.MapClick)
	if !ok || click.X != 42 || click.Y != 99 || click.TargetEntityId != 777 {
		t.Fatalf("wrong payload: %v", commands[0].Payload)
	}
}
