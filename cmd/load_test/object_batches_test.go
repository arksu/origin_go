package main

import (
	"net"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/protobuf/proto"

	netproto "origin/internal/network/proto"
)

func TestVirtualClientFullDecodeBeyondFirstHundredMessages(t *testing.T) {
	const playerID = 17
	spawn := func(id uint64, x int32) *netproto.S2C_ObjectSpawn {
		return &netproto.S2C_ObjectSpawn{EntityId: id, Position: &netproto.EntityPosition{Position: &netproto.Position{X: x, Y: -x}}}
	}
	move := func(id uint64, x int32) *netproto.S2C_ObjectMove {
		return &netproto.S2C_ObjectMove{EntityId: id, Movement: &netproto.EntityMovement{Position: &netproto.Position{X: x, Y: -x}}}
	}
	messages := []*netproto.ServerMessage{
		{Payload: &netproto.ServerMessage_PlayerEnterWorld{PlayerEnterWorld: &netproto.S2C_PlayerEnterWorld{EntityId: playerID}}},
		{Payload: &netproto.ServerMessage_ObjectSpawn{ObjectSpawn: spawn(playerID, 5)}},
	}
	for index := 0; index < 98; index++ {
		messages = append(messages, &netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMove{ObjectMove: move(18, int32(index))}})
	}
	messages = append(messages, &netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectSpawnBatch{ObjectSpawnBatch: &netproto.S2C_ObjectSpawnBatch{
		Spawns: []*netproto.S2C_ObjectSpawn{spawn(playerID, 10), spawn(18, 20)},
	}}})
	for index := 0; index < 3; index++ {
		messages = append(messages, &netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMoveBatch{ObjectMoveBatch: &netproto.S2C_ObjectMoveBatch{
			Moves: []*netproto.S2C_ObjectMove{move(playerID, int32(30+index)), move(18, 40), move(19, 50)},
		}}})
	}
	messages = append(messages, &netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMove{ObjectMove: move(playerID, 75)}})

	for _, fullDecode := range []bool{false, true} {
		t.Run(decodedEntryScope(fullDecode), func(t *testing.T) {
			metrics := NewMetrics()
			client := NewVirtualClient(&Config{FullDecode: fullDecode}, nil, nil, metrics, zap.NewNop())
			serverConn, clientConn := net.Pipe()
			t.Cleanup(func() { serverConn.Close(); clientConn.Close() })
			deadline := time.Now().Add(5 * time.Second)
			if err := serverConn.SetDeadline(deadline); err != nil {
				t.Fatal(err)
			}
			if err := clientConn.SetDeadline(deadline); err != nil {
				t.Fatal(err)
			}
			client.conn = clientConn
			readDone := make(chan struct{})
			go func() { client.readLoop(); close(readDone) }()
			var payloadBytes int64
			started := time.Now()
			for index, message := range messages {
				encoded, err := proto.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				payloadBytes += int64(len(encoded))
				// Even drain mode must count a fragmented message's complete payload.
				if index == 102 {
					midpoint := len(encoded) / 2
					for _, frame := range []ws.Frame{ws.NewFrame(ws.OpBinary, false, encoded[:midpoint]), ws.NewFrame(ws.OpContinuation, true, encoded[midpoint:])} {
						if err := ws.WriteFrame(serverConn, frame); err != nil {
							t.Fatal(err)
						}
					}
				} else if err := wsutil.WriteServerBinary(serverConn, encoded); err != nil {
					t.Fatal(err)
				}
			}
			serverConn.Close()
			select {
			case <-readDone:
			case <-time.After(5 * time.Second):
				t.Fatal("load client did not finish its receive stream")
			}
			snapshot := metrics.Snapshot()
			if snapshot.PacketsReceived != int64(len(messages)) || snapshot.BytesReceived != payloadBytes {
				t.Fatalf("message/byte counts differ: %+v, want messages=%d bytes=%d", snapshot, len(messages), payloadBytes)
			}
			if fullDecode {
				if snapshot.MessagesDecoded != 105 || snapshot.SpawnEntries != 3 || snapshot.MoveEntries != 108 || snapshot.MsgObjectSpawnBatch != 1 || snapshot.MsgObjectMoveBatch != 3 {
					t.Fatalf("full decode lost later batch content: %+v", snapshot)
				}
				if client.playerX.Load() != 75 || client.playerY.Load() != -75 || metrics.movesReceived.Load() != 4 {
					t.Fatalf("later batches/single did not update player: (%d,%d), moves=%d", client.playerX.Load(), client.playerY.Load(), metrics.movesReceived.Load())
				}
			} else {
				if snapshot.MessagesDecoded != 100 || snapshot.SpawnEntries != 1 || snapshot.MoveEntries != 98 || snapshot.MsgObjectSpawnBatch != 0 || snapshot.MsgObjectMoveBatch != 0 {
					t.Fatalf("drain mode should keep partial entry counters: %+v", snapshot)
				}
				if client.playerX.Load() != 5 || client.playerY.Load() != -5 {
					t.Fatal("drain mode unexpectedly tracked undecoded player positions")
				}
			}
			core, logs := observer.New(zap.InfoLevel)
			metrics.PrintSummary(zap.New(core), fullDecode)
			content := logs.FilterMessage("Decoded content").All()[0].ContextMap()
			if content["decoded_entry_scope"] != decodedEntryScope(fullDecode) || content["spawn_entries"] != snapshot.SpawnEntries || content["move_entries"] != snapshot.MoveEntries {
				t.Fatalf("summary lost count completeness: %v", content)
			}
			t.Logf("framed receive run: duration=%s messages=%d payload_bytes=%d decoded=%d spawn_entries=%d move_entries=%d player=(%d,%d)",
				time.Since(started), snapshot.PacketsReceived, snapshot.BytesReceived, snapshot.MessagesDecoded, snapshot.SpawnEntries, snapshot.MoveEntries, client.playerX.Load(), client.playerY.Load())
		})
	}
}

func TestVirtualClientSpawnEntriesWithMissingPosition(t *testing.T) {
	client := NewVirtualClient(&Config{FullDecode: true}, nil, nil, NewMetrics(), zap.NewNop())
	client.playerEntityID.Store(17)
	for _, entry := range []*netproto.S2C_ObjectSpawn{nil, {EntityId: 17}, {EntityId: 17, Position: &netproto.EntityPosition{Position: &netproto.Position{X: 60, Y: -30}}}} {
		client.handleObjectSpawn(entry)
	}
	if client.playerX.Load() != 60 || client.playerY.Load() != -30 || client.metrics.Snapshot().SpawnEntries != 2 {
		t.Fatalf("spawn position/count mismatch: (%d,%d), %+v", client.playerX.Load(), client.playerY.Load(), client.metrics.Snapshot())
	}
}

func TestVirtualClientCleanupWaitsForReceiveMetrics(t *testing.T) {
	metrics := NewMetrics()
	client := NewVirtualClient(&Config{FullDecode: true}, nil, &AccountPool{}, metrics, zap.NewNop())
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { serverConn.Close(); clientConn.Close() })
	client.conn = clientConn
	client.readDone = make(chan struct{})
	go func() {
		defer close(client.readDone)
		client.readLoop()
	}()
	message := &netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMoveBatch{ObjectMoveBatch: &netproto.S2C_ObjectMoveBatch{
		Moves: []*netproto.S2C_ObjectMove{{EntityId: 17}, {EntityId: 18}},
	}}}
	encoded, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if err := wsutil.WriteServerBinary(serverConn, encoded); err != nil {
		t.Fatal(err)
	}
	client.cleanup()
	snapshot := metrics.Snapshot()
	if snapshot.PacketsReceived != 1 || snapshot.MessagesDecoded != 1 || snapshot.MoveEntries != 2 || snapshot.BytesReceived != int64(len(encoded)) {
		t.Fatalf("cleanup returned before receive accounting completed: %+v", snapshot)
	}
}
