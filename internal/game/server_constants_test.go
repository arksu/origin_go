package game

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"origin/internal/characterattrs"
	"origin/internal/config"
	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/timeutil"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func newConstantsTestGame(t *testing.T) *Game {
	t.Helper()
	g := &Game{logger: zap.NewNop(), clock: timeutil.NewManualClock(time.Now()), tickRate: 25, cfg: &config.Config{}, ctx: context.Background()}
	var err error
	g.serverConstantsData, err = marshalServerConstants(g.tickRate)
	require.NoError(t, err)
	g.initializeRuntimeAccumulator()
	g.state.Store(int32(GameStateRunning))
	return g
}

// Pausing in onConnect keeps the real client's writer/read loops unstarted. This
// makes send-buffer overflow deterministic without sleeps or private-field hooks.
func connectConstantsTestClient(t *testing.T, g *Game, sendBuffer int) (*network.Client, net.Conn, func()) {
	t.Helper()
	server := network.NewServer(&config.NetworkConfig{ReadTimeout: time.Minute, WriteTimeout: time.Second}, &config.GameConfig{SendChannelBuffer: sendBuffer, Audio: config.DefaultAudioConfig()}, zap.NewNop())
	connected := make(chan *network.Client, 1)
	startLoops := make(chan struct{})
	var resumeOnce sync.Once
	resume := func() { resumeOnce.Do(func() { close(startLoops) }) }
	server.SetOnConnect(func(c *network.Client) { connected <- c; <-startLoops })
	server.SetOnDisconnect(g.handleDisconnect)
	mux := http.NewServeMux()
	require.NoError(t, server.Start("127.0.0.1:0", mux))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	t.Cleanup(resume)
	conn, _, _, err := ws.Dial(context.Background(), "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	select {
	case client := <-connected:
		return client, conn, resume
	case <-time.After(time.Second):
		t.Fatal("client did not connect")
		return nil, nil, nil
	}
}

func readConstantsTestMessage(t *testing.T, connection net.Conn) *netproto.ServerMessage {
	t.Helper()
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(time.Second)))
	wire, _, err := wsutil.ReadServerData(connection)
	require.NoError(t, err)
	message := new(netproto.ServerMessage)
	require.NoError(t, proto.Unmarshal(wire, message))
	return message
}

func TestServerConstantsUseInitializedRules(t *testing.T) {
	g := newConstantsTestGame(t)
	g.cfg.Game.TickRate = 99 // The initialized loop rate is the authoritative value.
	var message netproto.ServerMessage
	require.NoError(t, proto.Unmarshal(g.serverConstantsData, &message))
	constants := message.GetServerConstants()
	require.Equal(t, uint32(_const.CoordPerTile), constants.CoordPerTile)
	require.Equal(t, uint32(_const.ChunkSize), constants.ChunkSize)
	require.Equal(t, uint32(g.tickRate), constants.TickRate)
	require.True(t, constants.DirectionalMovementSupported)
	require.Equal(t, uint32(timeutil.RealSecondsPerGameDay), constants.RealSecondsPerGameDay)
	require.Equal(t, uint32(timeutil.HoursPerDay), constants.HoursPerDay)
	require.Equal(t, uint32(timeutil.DaysPerMonth), constants.DaysPerMonth)
	require.Equal(t, uint32(timeutil.MonthsPerYear), constants.MonthsPerYear)
	for _, tickRate := range []int{0, -1} {
		_, err := marshalServerConstants(tickRate)
		require.Error(t, err)
	}
}

func TestAuthenticatedBootstrapConstantsOnceBeforePongsAndWorldEntries(t *testing.T) {
	g := newConstantsTestGame(t)
	client, connection, resume := connectConstantsTestClient(t, g, 32)
	// This test exercises delivery after association; avoid a DB write on cleanup.
	g.state.Store(int32(GameStateStarting))
	client.CharacterID = 17
	require.True(t, g.sendAuthenticatedBootstrap(client, 1))
	client.StreamEpoch.Store(7)
	g.handlePing(client, 2, &netproto.C2S_Ping{ClientTimeMs: 123})
	g.sendPlayerEnterWorld(client, 17, nil, repository.Character{Name: "calendar-test"})
	client.StreamEpoch.Store(9) // Transfer entry changes only per-entry state.
	g.sendPlayerEnterWorld(client, 17, nil, repository.Character{Name: "calendar-test"})
	// An attempted identity switch must not access the DB or resend constants.
	g.handleAuth(client, 3, &netproto.C2S_Auth{Token: "another-character"})
	clock := g.clock.(*timeutil.ManualClock)
	clock.Advance(5 * time.Second)
	g.handlePing(client, 4, &netproto.C2S_Ping{ClientTimeMs: 456})
	clock.Advance(200 * time.Millisecond)
	g.handlePing(client, 5, &netproto.C2S_Ping{ClientTimeMs: 789})
	resume()
	auth := readConstantsTestMessage(t, connection)
	require.True(t, auth.GetAuthResult().GetSuccess())
	require.Equal(t, uint32(1), auth.Sequence)
	constants := readConstantsTestMessage(t, connection).GetServerConstants()
	require.NotNil(t, constants)
	firstPong := readConstantsTestMessage(t, connection).GetPong()
	require.NotNil(t, firstPong.RuntimeSecondsTotal)
	require.Zero(t, *firstPong.RuntimeSecondsTotal)
	require.Equal(t, int64(123), firstPong.ClientTimeMs)
	for _, epoch := range []uint32{7, 9} {
		entry := readConstantsTestMessage(t, connection).GetPlayerEnterWorld()
		require.Equal(t, uint64(17), entry.EntityId)
		require.Equal(t, "calendar-test", entry.Name)
		require.Equal(t, epoch, entry.StreamEpoch)
		require.NotNil(t, entry.Audio)
	}
	require.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, readConstantsTestMessage(t, connection).GetError().Code)
	for _, clientTime := range []int64{456, 789} {
		pong := readConstantsTestMessage(t, connection).GetPong()
		require.NotNil(t, pong)
		require.Equal(t, clientTime, pong.ClientTimeMs)
		require.Equal(t, int64(5), *pong.RuntimeSecondsTotal)
	}
	require.Equal(t, types.EntityID(17), client.CharacterID)
	require.Zero(t, g.runtimeSecondsTotal, "network projection must not advance persisted runtime")
}

func TestUnauthenticatedPongDoesNotExposeRuntime(t *testing.T) {
	g := newConstantsTestGame(t)
	client, connection, resume := connectConstantsTestClient(t, g, 4)
	g.handlePing(client, 7, &netproto.C2S_Ping{ClientTimeMs: 1})
	resume()
	pong := readConstantsTestMessage(t, connection).GetPong()
	require.Nil(t, pong.RuntimeSecondsTotal)
	require.Equal(t, g.clock.WallNow().UnixMilli(), pong.ServerTimeMs)
}

func TestFailedAuthenticationDoesNotSendConstants(t *testing.T) {
	g := newConstantsTestGame(t)
	client, connection, resume := connectConstantsTestClient(t, g, 4)
	g.handleAuth(client, 1, &netproto.C2S_Auth{})
	g.handlePing(client, 2, &netproto.C2S_Ping{ClientTimeMs: 1})
	resume()
	require.False(t, readConstantsTestMessage(t, connection).GetAuthResult().Success)
	require.NotNil(t, readConstantsTestMessage(t, connection).GetPong(), "failed auth must not insert constants")
}

func TestMandatoryBootstrapFailureClearsOnlineAndAllowsRetry(t *testing.T) {
	for _, failure := range []string{"missing-constants", "auth-queue-full", "constants-queue-full", "close-before-commit"} {
		t.Run(failure, func(t *testing.T) {
			g := newConstantsTestGame(t)
			db := newAuthenticationTestDatabase(t)
			g.db = db
			g.shardManager = &ShardManager{shards: map[int]*Shard{0: {
				world: ecs.NewWorldForTesting(), logger: zap.NewNop(), Clients: make(map[types.EntityID]*network.Client),
				playerInbox: network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig()),
			}}}
			buffer := 1
			if failure == "missing-constants" {
				g.serverConstantsData = nil
			}
			for attempt := 0; attempt < 2; attempt++ {
				client, _, resume := connectConstantsTestClient(t, g, buffer)
				if failure == "auth-queue-full" {
					require.True(t, client.SendCritical([]byte{1}))
				}
				if failure == "close-before-commit" {
					db.beforeCommit = client.Close
				}
				g.handleAuth(client, uint32(attempt+1), &netproto.C2S_Auth{Token: "valid"})
				select {
				case <-client.Done():
				default:
					t.Fatal("failed bootstrap must close the connection")
				}
				require.False(t, db.isOnline())
				require.Equal(t, attempt+1, db.offlineCount(), "one normal disconnect cleanup per authentication")
				require.Equal(t, types.EntityID(17), client.CharacterID, "association is retained for normal cleanup")
				require.False(t, client.InWorld.Load())
				require.Empty(t, g.shardManager.GetShard(0).Clients)
				require.Zero(t, g.shardManager.GetShard(0).world.EntityCount(), "failed bootstrap must not spawn")
				resume()
			}
		})
	}
}

func TestAuthSerializationFailureClosesConnection(t *testing.T) {
	g := newConstantsTestGame(t)
	client, _, _ := connectConstantsTestClient(t, g, 4)
	require.False(t, g.sendAuthResult(client, 1, false, string([]byte{0xff})))
	select {
	case <-client.Done():
	default:
		t.Fatal("serialization failure must close the connection")
	}
}

// A small database/sql driver drives the actual generated auth/offline queries.
// It avoids a database service or added mocking dependency for lifecycle tests.
type authenticationTestDatabase struct {
	queries      *repository.Queries
	state        *authenticationTestState
	beforeCommit func()
}

type authenticationTestState struct {
	mu           sync.Mutex
	online       bool
	offlineCalls int
	attributes   []byte
}

func newAuthenticationTestDatabase(t *testing.T) *authenticationTestDatabase {
	t.Helper()
	attributes, err := characterattrs.Marshal(characterattrs.Default())
	require.NoError(t, err)
	state := &authenticationTestState{attributes: attributes}
	db := sql.OpenDB(authenticationTestConnector{state})
	t.Cleanup(func() { _ = db.Close() })
	return &authenticationTestDatabase{queries: repository.New(db), state: state}
}

func (d *authenticationTestDatabase) Queries() *repository.Queries { return d.queries }
func (d *authenticationTestDatabase) WithTx(_ context.Context, fn func(*repository.Queries) error) error {
	if err := fn(d.queries); err != nil {
		return err
	}
	if d.beforeCommit != nil {
		d.beforeCommit()
	}
	return nil
}
func (d *authenticationTestDatabase) isOnline() bool {
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	return d.state.online
}
func (d *authenticationTestDatabase) offlineCount() int {
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	return d.state.offlineCalls
}

type authenticationTestConnector struct{ state *authenticationTestState }
type authenticationTestDriver struct{}
type authenticationTestConn struct{ state *authenticationTestState }
type authenticationTestRows struct{ values []driver.Value }

func (c authenticationTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &authenticationTestConn{c.state}, nil
}
func (authenticationTestConnector) Driver() driver.Driver { return authenticationTestDriver{} }
func (authenticationTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}
func (*authenticationTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*authenticationTestConn) Close() error              { return nil }
func (*authenticationTestConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected begin") }
func (c *authenticationTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	switch {
	case strings.Contains(query, "name: SetCharacterOnline"):
		c.state.online = true
	case strings.Contains(query, "name: SetCharacterOffline"):
		c.state.online = false
		c.state.offlineCalls++
	case strings.Contains(query, "name: UpdateCharacterAttributes"):
	default:
		return nil, errors.New("unexpected auth exec: " + query)
	}
	return driver.RowsAffected(1), nil
}
func (c *authenticationTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "name: GetCharacterByTokenForUpdate") {
		return nil, errors.New("unexpected auth query: " + query)
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if args[0].Value != "valid" {
		return &authenticationTestRows{}, nil
	}
	return &authenticationTestRows{values: []driver.Value{
		int64(17), int64(1), "calendar-test", int64(1), int64(200), int64(200), int64(0), int64(0),
		float64(100), float64(100), float64(100), float64(100), false, c.state.attributes,
		[]byte("{}"), []byte("{}"), []byte("{}"), []byte("{}"), int64(0), "valid", time.Now().Add(time.Hour),
		c.state.online, nil, false, nil, nil, time.Now(), nil,
	}}, nil
}
func (*authenticationTestRows) Columns() []string { return make([]string, 28) }
func (*authenticationTestRows) Close() error      { return nil }
func (r *authenticationTestRows) Next(dest []driver.Value) error {
	if r.values == nil {
		return io.EOF
	}
	copy(dest, r.values)
	r.values = nil
	return nil
}
