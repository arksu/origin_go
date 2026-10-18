package game

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/hearth"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// The ordinary spawn-test socket has no disconnect callback. These connections
// exercise the real offline transition and authentication lifecycle together.
type hearthReconnectTestServer struct {
	url          string
	connected    chan *network.Client
	disconnected chan *network.Client
	handled      chan *network.Client
	disconnectMu sync.Mutex
}

func newHearthReconnectTestServer(t *testing.T, g *Game) *hearthReconnectTestServer {
	t.Helper()
	server := network.NewServer(&config.NetworkConfig{ReadTimeout: time.Minute, WriteTimeout: time.Second},
		&config.GameConfig{SendChannelBuffer: 32, Audio: config.DefaultAudioConfig()}, g.logger)
	f := &hearthReconnectTestServer{
		connected: make(chan *network.Client, 4), disconnected: make(chan *network.Client, 4), handled: make(chan *network.Client, 4),
	}
	server.SetOnConnect(func(client *network.Client) { f.connected <- client })
	server.SetOnDisconnect(func(client *network.Client) {
		f.disconnectMu.Lock()
		defer f.disconnectMu.Unlock()
		g.handleDisconnect(client)
		f.disconnected <- client
	})
	server.SetOnMessage(func(client *network.Client, data []byte) {
		g.handlePacket(client, data)
		f.handled <- client
	})
	mux := http.NewServeMux()
	require.NoError(t, server.Start("127.0.0.1:0", mux))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	f.url = "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	var err error
	g.serverConstantsData, err = marshalServerConstants(g.cfg.Game.TickRate)
	require.NoError(t, err)
	return f
}

func (f *hearthReconnectTestServer) connect(t *testing.T) (*network.Client, net.Conn) {
	t.Helper()
	conn, _, _, err := ws.Dial(t.Context(), f.url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.closeConnection(conn) })
	return waitHearthTerminal(t, f.connected), conn
}

func (f *hearthReconnectTestServer) closeConnection(conn net.Conn) error {
	// Peer socket I/O is not a Go synchronization edge. Publish the test's
	// already-observed transfer/auth boundary to the real disconnect callback.
	f.disconnectMu.Lock()
	defer f.disconnectMu.Unlock()
	return conn.Close()
}

func (f *hearthReconnectTestServer) startAuthentication(t *testing.T, conn net.Conn) {
	t.Helper()
	packet, err := proto.Marshal(&netproto.ClientMessage{
		Sequence: 1, Payload: &netproto.ClientMessage_Auth{Auth: &netproto.C2S_Auth{Token: "hearth-reconnect"}},
	})
	require.NoError(t, err)
	require.NoError(t, wsutil.WriteClientBinary(conn, packet))
}

func (f *hearthReconnectTestServer) finishAuthentication(t *testing.T, client *network.Client, conn net.Conn, auth *netproto.ServerMessage) {
	t.Helper()
	require.Same(t, client, waitHearthTerminal(t, f.handled))
	if auth == nil {
		auth = readSoundLifecyclePacket(t, conn, func(message *netproto.ServerMessage) bool { return message.GetAuthResult() != nil })
	}
	require.True(t, auth.GetAuthResult().Success, auth.GetAuthResult().ErrorMessage)
	readSoundLifecyclePacket(t, conn, func(message *netproto.ServerMessage) bool { return message.GetServerConstants() != nil })
}

type hearthReconnectEntity struct {
	layer  int
	handle types.Handle
}

func hearthReconnectLiveEntities(f *hearthReturnTransferFixture, identity types.EntityID) []hearthReconnectEntity {
	var entities []hearthReconnectEntity
	for _, shard := range f.shards {
		shard.WithWorldRead(func(w *ecs.World) {
			// GetHandleByEntityID alone hides duplicate bodies in one World because
			// spawning the second body overwrites its reverse-index entry.
			w.Query().ForEach(func(handle types.Handle) {
				if external, exists := ecs.GetComponent[ecs.ExternalID](w, handle); exists && external.ID == identity && w.Alive(handle) {
					entities = append(entities, hearthReconnectEntity{shard.layer, handle})
				}
			})
		})
	}
	return entities
}

func prepareHearthReconnect(t *testing.T, sourceLayer int) (*hearthReturnTransferFixture, *hearthReconnectTestServer) {
	t.Helper()
	f := newHearthReturnTransferFixture(t, sourceLayer, true, true)
	server := newHearthReconnectTestServer(t, f.game)
	old := f.client
	f.client, f.connection = server.connect(t)
	f.client.CharacterID, f.client.Layer = 10, sourceLayer
	require.True(t, f.game.attachClientToWorld(f.source, f.client, 10, f.character, f.player))
	old.Close()
	_, err := f.db.Pool().Exec(t.Context(), `UPDATE character SET auth_token='hearth-reconnect', is_online=true WHERE id=10`)
	require.NoError(t, err)
	return f, server
}

func pauseHearthReconnectTransfer(t *testing.T, f *hearthReturnTransferFixture, rollback bool) (func(), <-chan hearthReconnectEntity) {
	t.Helper()
	req := f.request(t)
	if rollback {
		home := f.runtime.Index.Snapshot(10).Home
		addHearthCollisionTestObject(f.shards[0], 4000, home.X, home.Y, nil)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	// Release the transfer before fixture teardown even when an assertion fails.
	t.Cleanup(func() { unblock(); f.game.wg.Wait() })
	spawn := req.SpawnTarget
	req.SpawnTarget = func(shard *Shard, character repository.Character, runtime playerRuntimeState, finalize playerSpawnFinalizer) (types.Handle, repository.Character, error) {
		close(entered)
		<-release
		return spawn(shard, character, runtime, finalize)
	}
	result := make(chan hearthReconnectEntity, 1)
	onResult := req.OnResult
	req.OnResult = func(shard *Shard, handle types.Handle, err error) {
		onResult(shard, handle, err)
		assert.Equal(t, rollback, err != nil)
		result <- hearthReconnectEntity{shard.layer, shard.world.GetHandleByEntityID(10)}
	}
	require.NoError(t, f.game.transferService.RequestTransfer(req))
	waitHearthTerminal(t, entered)
	require.Empty(t, hearthReconnectLiveEntities(f, 10), "the accepted transfer has captured its source body")
	require.Empty(t, hearthReconnectLiveEntities(f, 3000), "the carry participant has captured the object")
	return unblock, result
}

func disconnectHearthReconnectSource(t *testing.T, f *hearthReturnTransferFixture, server *hearthReconnectTestServer) {
	t.Helper()
	require.NoError(t, server.closeConnection(f.connection))
	require.Same(t, f.client, waitHearthTerminal(t, server.disconnected))
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.True(t, row.IsOnline.Valid)
	require.False(t, row.IsOnline.Bool, "the real disconnect must admit the replacement authentication")
	require.True(t, row.DisconnectAt.Valid)
	require.True(t, row.DisconnectAt.Time.Equal(f.game.clock.WallNow()))
	require.Zero(t, f.runtime.Index.Snapshot(10).LastPingUnixMs, "a disconnect cannot invent a ping")
}

func waitHearthReconnectWork(t *testing.T, g *Game) {
	t.Helper()
	done := make(chan struct{})
	go func() { g.wg.Wait(); close(done) }()
	waitHearthTerminal(t, done)
}

func TestHearthReturnPostgresReconnectDuringCapturedTransfer(t *testing.T) {
	for _, sourceLayer := range []int{0, 1} {
		for _, rollback := range []bool{false, true} {
			t.Run(fmt.Sprintf("layer_%d_rollback_%t", sourceLayer, rollback), func(t *testing.T) {
				f, server := prepareHearthReconnect(t, sourceLayer)
				unblock, restored := pauseHearthReconnectTransfer(t, f, rollback)
				disconnectHearthReconnectSource(t, f, server)
				next, conn := server.connect(t)
				server.startAuthentication(t, conn)

				// Authentication must stay pending for the whole captured interval:
				// the old row still points to a body that has not been restored yet.
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
				payload, _, readErr := wsutil.ReadServerData(conn)
				var earlyAuth *netproto.ServerMessage
				if readErr == nil {
					earlyAuth = &netproto.ServerMessage{}
					require.NoError(t, proto.Unmarshal(payload, earlyAuth))
					assert.Nil(t, earlyAuth.GetAuthResult(), "authentication completed before transfer restoration")
				} else {
					var timeout net.Error
					require.ErrorAs(t, readErr, &timeout)
					require.True(t, timeout.Timeout())
				}
				assert.False(t, next.InWorld.Load())
				assert.Empty(t, hearthReconnectLiveEntities(f, 10), "login must not create a replacement body during transfer")

				// The auth transaction loaded the old name. Entry must use a fresh
				// character row after the transfer's position/layer write finishes.
				// This bounded write also proves the waiter released its row lock.
				updateCtx, cancelUpdate := context.WithTimeout(t.Context(), time.Second)
				_, err := f.db.Pool().Exec(updateCtx, `UPDATE character SET name='owner after transfer' WHERE id=10`)
				cancelUpdate()
				require.NoError(t, err)
				unblock()
				server.finishAuthentication(t, next, conn, earlyAuth)
				waitHearthReconnectWork(t, f.game)
				actual := waitHearthTerminal(t, restored)
				wantLayer, wantX, wantY := 0, f.runtime.Index.Snapshot(10).Home.X, f.runtime.Index.Snapshot(10).Home.Y
				if rollback {
					wantLayer, wantX, wantY = sourceLayer, 200, 200
				}
				require.Equal(t, wantLayer, actual.layer)
				entities := hearthReconnectLiveEntities(f, 10)
				require.Equal(t, []hearthReconnectEntity{actual}, entities, "all shard entities must contain exactly the restored body")
				entry := readSoundLifecycleEntry(t, conn)
				require.Equal(t, "owner after transfer", entry.Name)
				require.EqualValues(t, 10, entry.EntityId)
				require.True(t, next.InWorld.Load())
				require.Equal(t, actual.layer, next.Layer)
				require.Equal(t, next.StreamEpoch.Load(), entry.StreamEpoch)
				shard := f.shards[actual.layer]
				shard.WithWorldRead(func(w *ecs.World) {
					require.Same(t, next, shard.Clients[10])
					require.False(t, ecs.GetResource[ecs.DetachedEntities](w).IsDetached(10))
					require.Equal(t, !rollback, shard.actionService.isOnCooldown(w, actual.handle, hearth.ReturnActionID))
					require.False(t, ecs.HasComponent[components.LiftCarryState](w, actual.handle), "closed transfer client uses ordinary disconnect force-drop")
					health, ok := ecs.GetComponent[components.EntityHealth](w, actual.handle)
					require.True(t, ok)
					require.Equal(t, rollback, health.IsLying)
				})
				objects := hearthReconnectLiveEntities(f, 3000)
				require.Len(t, objects, 1, "the carried object must neither disappear nor duplicate")
				require.Equal(t, actual.layer, objects[0].layer)
				shard.WithWorldRead(func(w *ecs.World) {
					require.False(t, ecs.HasComponent[components.LiftedObjectState](w, objects[0].handle))
					playerPosition, _ := ecs.GetComponent[components.Transform](w, actual.handle)
					objectPosition, _ := ecs.GetComponent[components.Transform](w, objects[0].handle)
					require.EqualValues(t, wantX, playerPosition.X)
					require.EqualValues(t, wantY, playerPosition.Y)
					require.Equal(t, playerPosition.X, objectPosition.X)
					require.Equal(t, playerPosition.Y, objectPosition.Y)
				})
				row, err := f.db.Queries().GetCharacter(t.Context(), 10)
				require.NoError(t, err)
				require.Equal(t, actual.layer, row.Layer)
				require.Equal(t, wantX, row.X)
				require.Equal(t, wantY, row.Y)
				require.True(t, row.IsOnline.Bool)
			})
		}
	}
}

func TestHearthReturnPostgresReconnectWaitUsesSpawnDeadline(t *testing.T) {
	f, server := prepareHearthReconnect(t, 1)
	f.game.cfg.Game.SpawnTimeout = 150 * time.Millisecond
	unblock, restored := pauseHearthReconnectTransfer(t, f, false)
	disconnectHearthReconnectSource(t, f, server)
	next, conn := server.connect(t)
	server.startAuthentication(t, conn)
	message := readSoundLifecyclePacket(t, conn, func(message *netproto.ServerMessage) bool {
		return message.GetAuthResult() != nil
	})
	require.Same(t, next, waitHearthTerminal(t, server.handled))
	require.False(t, message.GetAuthResult().Success, "waiting authentication must time out while transfer remains unfinished")
	require.Contains(t, strings.ToLower(message.GetAuthResult().ErrorMessage), "timed out")
	require.Zero(t, next.CharacterID)
	require.False(t, next.InWorld.Load())
	require.Empty(t, hearthReconnectLiveEntities(f, 10))

	// The timed-out login never acquired a character association. The accepted
	// transfer must restore one detached body after both connections have closed.
	require.NoError(t, server.closeConnection(conn))
	require.Same(t, next, waitHearthTerminal(t, server.disconnected))
	unblock()
	waitHearthReconnectWork(t, f.game)
	actual := waitHearthTerminal(t, restored)
	require.Equal(t, []hearthReconnectEntity{actual}, hearthReconnectLiveEntities(f, 10))
	f.shards[actual.layer].WithWorldRead(func(w *ecs.World) {
		require.True(t, ecs.GetResource[ecs.DetachedEntities](w).IsDetached(10))
		require.Nil(t, f.shards[actual.layer].Clients[10])
	})
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.False(t, row.IsOnline.Bool)
}
