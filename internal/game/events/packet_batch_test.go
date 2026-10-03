package events

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/sounddefs"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type batchFixture struct {
	manager    *game.ShardManager
	shard      *game.Shard
	dispatcher *NetworkVisibilityDispatcher
	bus        *eventbus.EventBus
	server     *network.Server
	url        string
	connected  chan *network.Client
}

func newBatchFixture(t *testing.T) *batchFixture {
	t.Helper()
	previous := actiondefs.Global()
	actiondefs.SetGlobalForTesting(actiondefs.NewRegistry([]actiondefs.Definition{{ID: "lift"}, {ID: "lift_down"}, {ID: "plow_tile"}, {ID: "dig"}}))
	t.Cleanup(func() { actiondefs.SetGlobalForTesting(previous) })
	previousSounds := sounddefs.Global()
	sounds, err := sounddefs.NewRegistry(nil)
	require.NoError(t, err)
	sounddefs.SetGlobalForTesting(sounds)
	t.Cleanup(func() { sounddefs.SetGlobalForTesting(previousSounds) })
	cfg := &config.Config{Game: config.GameConfig{
		MaxEntities: 2048, MaxLayers: 1, EventBusMinWorkers: 1, EventBusMaxWorkers: 1, WorkerPoolSize: 1,
		ChunkLRUCapacity: 16, ChunkLRUTTL: 60, LoadWorkers: 1, WorldWidthChunks: 8, WorldHeightChunks: 8,
		SendChannelBuffer: 1024, CommandQueueSize: 128, PlayerSaveInterval: time.Hour, Audio: config.DefaultAudioConfig(),
	}, Network: config.NetworkConfig{ReadTimeout: time.Minute, WriteTimeout: time.Second}}
	logger := zap.NewNop()
	manager := game.NewShardManager(cfg, nil, nil, gameworld.NewObjectFactory(nil), nil, false, logger)
	t.Cleanup(manager.Stop)
	server := network.NewServer(&cfg.Network, &cfg.Game, logger)
	f := &batchFixture{manager: manager, shard: manager.GetShard(0), bus: manager.EventBus(), server: server, connected: make(chan *network.Client, 8)}
	f.dispatcher = NewNetworkVisibilityDispatcher(manager, logger)
	server.SetOnConnect(func(c *network.Client) { f.connected <- c })
	mux := http.NewServeMux()
	require.NoError(t, server.Start("127.0.0.1:0", mux))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	f.url = "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	return f
}

func (f *batchFixture) connect(t *testing.T, id types.EntityID) (*network.Client, net.Conn) {
	t.Helper()
	conn, _, _, err := ws.Dial(context.Background(), f.url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var client *network.Client
	select {
	case client = <-f.connected:
	case <-time.After(time.Second):
		t.Fatal("missing server client")
	}
	client.CharacterID = id
	client.InWorld.Store(true)
	client.StreamEpoch.Store(17)
	f.shard.ClientsMu.Lock()
	f.shard.Clients[id] = client
	f.shard.ClientsMu.Unlock()
	return client, conn
}

// A sentinel in the same client queue makes zero/extra message assertions
// deterministic without a timeout-based guess about network delivery.
func (f *batchFixture) drain(t *testing.T, client *network.Client, conn net.Conn) []*netproto.ServerMessage {
	t.Helper()
	barrier, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
	require.NoError(t, err)
	require.True(t, client.SendCritical(barrier))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	var messages []*netproto.ServerMessage
	for {
		payload, op, err := wsutil.ReadServerData(conn)
		require.NoError(t, err)
		require.Equal(t, ws.OpBinary, op)
		message := &netproto.ServerMessage{}
		require.NoError(t, proto.Unmarshal(payload, message))
		if message.GetPong() != nil {
			return messages
		}
		messages = append(messages, message)
	}
}

func spawnBatchTarget(w *ecs.World, id types.EntityID, resource string) types.Handle {
	return w.Spawn(id, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: float64(id), Y: 20})
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1001})
		ecs.AddComponent(w, h, components.Appearance{Resource: resource})
	})
}

func TestMoveBatchOutboundObserverSubsets(t *testing.T) {
	f := newBatchFixture(t)
	w := f.shard.World()
	observerA := w.Spawn(1, nil)
	observerB := w.Spawn(2, nil)
	observerC := w.Spawn(3, nil)
	clientA, connA := f.connect(t, 1)
	clientB, connB := f.connect(t, 2)
	clientC, connC := f.connect(t, 3)
	first := spawnBatchTarget(w, 10, "tree")
	second := spawnBatchTarget(w, 11, "tree")
	third := spawnBatchTarget(w, 12, "tree")
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.ObserversByVisibleTarget[first] = map[types.Handle]struct{}{observerA: {}}
	visibility.ObserversByVisibleTarget[second] = map[types.Handle]struct{}{observerA: {}, observerB: {}}
	visibility.ObserversByVisibleTarget[third] = map[types.Handle]struct{}{observerC: {}}
	entries := []ecs.MoveBatchEntry{{EntityID: 10, Handle: first, X: 15, MoveSeq: 3, ServerTimeMs: 123}, {EntityID: 11, Handle: second, X: 25, IsTeleport: true, CarriedByEntityID: 10}, {EntityID: 12, Handle: third, X: 35}}
	require.NoError(t, f.dispatcher.handleObjectMoveBatch(context.Background(), ecs.NewObjectMoveBatchEvent(0, entries)))
	messagesA := f.drain(t, clientA, connA)
	require.Len(t, messagesA, 1)
	require.Equal(t, []*netproto.S2C_ObjectMove{buildObjectMove(&entries[0]), buildObjectMove(&entries[1])}, messagesA[0].GetObjectMoveBatch().Moves)
	messagesB := f.drain(t, clientB, connB)
	require.Len(t, messagesB, 1)
	require.True(t, proto.Equal(buildObjectMove(&entries[1]), messagesB[0].GetObjectMove()))
	messagesC := f.drain(t, clientC, connC)
	require.Len(t, messagesC, 1)
	require.EqualValues(t, 12, messagesC[0].GetObjectMove().EntityId)
	require.NoError(t, f.dispatcher.handleObjectMoveBatch(context.Background(), ecs.NewObjectMoveBatchEvent(0, nil)))
	require.Empty(t, f.drain(t, clientA, connA))
}

func TestSpawnBatchOutboundValidationEpochAndAppearance(t *testing.T) {
	f := newBatchFixture(t)
	w := f.shard.World()
	observer := w.Spawn(1, nil)
	client, conn := f.connect(t, 1)
	first := spawnBatchTarget(w, 10, "tree/mature")
	second := spawnBatchTarget(w, 11, "player") // Includes animation state: whole message uses critical delivery.
	invalidSnapshot := spawnBatchTarget(w, 12, "player")
	ecs.AddComponent(w, invalidSnapshot, components.ActionAnimation{Key: "missing"})
	invisible := spawnBatchTarget(w, 13, "tree")
	dead := spawnBatchTarget(w, 14, "tree")
	w.Despawn(dead)
	missing := w.Spawn(15, nil)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	for _, h := range []types.Handle{first, second, invalidSnapshot, missing} {
		visibility.ObserversByVisibleTarget[h] = map[types.Handle]struct{}{observer: {}}
	}
	event := ecs.NewEntitySpawnBatchEvent(1, []ecs.SpawnBatchEntry{{EntityID: 10, Handle: first}, {EntityID: 11, Handle: first}, {EntityID: 12, Handle: invalidSnapshot}, {EntityID: 11, Handle: second}, {EntityID: 13, Handle: invisible}, {EntityID: 14, Handle: dead}, {EntityID: 15, Handle: missing}}, 0)
	// Bootstrap is already queued; all entries carry the current epoch at dispatch.
	initial := &netproto.ServerMessage{Payload: &netproto.ServerMessage_PlayerEnterWorld{PlayerEnterWorld: &netproto.S2C_PlayerEnterWorld{EntityId: 1, StreamEpoch: 17}}}
	encoded, err := proto.Marshal(initial)
	require.NoError(t, err)
	client.Send(encoded)
	require.NoError(t, f.dispatcher.handleEntitySpawnBatch(context.Background(), event))
	messages := f.drain(t, client, conn)
	require.Len(t, messages, 2)
	require.EqualValues(t, 1, messages[0].GetPlayerEnterWorld().EntityId)
	require.EqualValues(t, 17, messages[0].GetPlayerEnterWorld().StreamEpoch)
	spawns := messages[1].GetObjectSpawnBatch().Spawns
	require.Len(t, spawns, 2)
	require.EqualValues(t, 10, spawns[0].EntityId)
	require.Equal(t, "tree/mature", spawns[0].ResourcePath)
	require.EqualValues(t, 11, spawns[1].EntityId)
	require.NotNil(t, spawns[1].ActionAnimation)
	for _, spawn := range spawns {
		require.EqualValues(t, 17, spawn.StreamEpoch)
	}
	delete(visibility.ObserversByVisibleTarget, second)
	client.StreamEpoch.Store(18)
	require.NoError(t, f.dispatcher.handleEntitySpawnBatch(context.Background(), event))
	messages = f.drain(t, client, conn)
	require.Len(t, messages, 1)
	require.EqualValues(t, 18, messages[0].GetObjectSpawn().StreamEpoch)
	ecs.WithComponent(w, first, func(a *components.Appearance) { a.Resource = "tree/cut" })
	require.NoError(t, f.dispatcher.handleEntityAppearanceChanged(context.Background(), ecs.NewEntityAppearanceChangedEvent(0, 10, first)))
	messages = f.drain(t, client, conn)
	require.Len(t, messages, 1)
	require.Equal(t, "tree/cut", messages[0].GetObjectSpawn().ResourcePath)
	client.InWorld.Store(false)
	require.NoError(t, f.dispatcher.handleEntitySpawnBatch(context.Background(), event))
	require.Empty(t, f.drain(t, client, conn))
	client.InWorld.Store(true)
	delete(visibility.ObserversByVisibleTarget, first)
	require.NoError(t, f.dispatcher.handleEntitySpawnBatch(context.Background(), event))
	require.Empty(t, f.drain(t, client, conn))
}

func TestSpawnBatchCriticalDeliveryOnFullQueue(t *testing.T) {
	for _, name := range []string{"ordinary", "animation", "combat target", "combat execution"} {
		animated := name != "ordinary"
		t.Run(name, func(t *testing.T) {
			f := newBatchFixture(t)
			// onConnect runs before the client's write loop. Hold it here so filling
			// the real queue is deterministic and independent of socket buffer sizes.
			release := make(chan struct{})
			returned := make(chan struct{})
			f.server.SetOnConnect(func(client *network.Client) { f.connected <- client; <-release; close(returned) })
			client, conn := f.connect(t, 1)
			released := false
			t.Cleanup(func() {
				if !released {
					close(release)
				}
			})
			w := f.shard.World()
			observer := w.Spawn(1, nil)
			first := spawnBatchTarget(w, 10, "tree")
			resource := "tree"
			if name == "animation" {
				resource = "player"
			}
			second := spawnBatchTarget(w, 11, resource)
			if name == "combat target" {
				ecs.AddComponent(w, second, components.CombatTestTarget{HP: .6, MaxHP: 1, Revision: 2, Receiver: true})
			}
			if name == "combat execution" {
				ecs.AddComponent(w, second, components.CombatState{Revision: 2})
			}
			visibility := ecs.GetResource[ecs.VisibilityState](w)
			for _, h := range []types.Handle{first, second} {
				visibility.ObserversByVisibleTarget[h] = map[types.Handle]struct{}{observer: {}}
			}
			queued, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
			require.NoError(t, err)
			for i := 0; i < 1024; i++ {
				require.True(t, client.SendCritical(queued))
			}
			require.NoError(t, f.dispatcher.handleEntitySpawnBatch(context.Background(), ecs.NewEntitySpawnBatchEvent(1, []ecs.SpawnBatchEntry{{EntityID: 10, Handle: first}, {EntityID: 11, Handle: second}}, 0)))
			if animated {
				// Any animated included entry must close on overflow, never silently drop.
				select {
				case <-client.Done():
				case <-time.After(time.Second):
					t.Fatal("critical batch did not close full connection")
				}
				require.False(t, client.SendCritical(queued))
			} else {
				select {
				case <-client.Done():
					t.Fatal("ordinary batch unexpectedly closed connection")
				default:
				}
			}
			close(release)
			released = true
			<-returned
			if !animated {
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
				for i := 0; i < 1024; i++ {
					_, _, err := wsutil.ReadServerData(conn)
					require.NoError(t, err)
				}
				require.Empty(t, f.drain(t, client, conn), "ordinary overflowing batch is dropped under existing delivery policy")
			}
		})
	}
}
