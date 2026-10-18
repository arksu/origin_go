package game

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/game/hearth"
	"origin/internal/timeutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCharacterDisconnectPostgresLifecycle(t *testing.T) {
	for _, layer := range []int{0, 1} {
		t.Run(fmt.Sprintf("layer_%d", layer), func(t *testing.T) {
			f := newHearthTerminalGameFixture(t)
			f.addCharacter(t, 10, layer)
			g, source := f.games[layer], f.shards[layer]
			g.shardManager.shards = map[int]*Shard{0: f.shards[0], 1: f.shards[1]}
			g.ctx = context.Background()
			clock := g.clock.(*timeutil.ManualClock)
			client, _ := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, layer
			client.InWorld.Store(true)
			source.Clients[10] = client
			body := spawnSoundLifecycleEntity(t, source, 10, 200, 200)
			require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
			g.handleDisconnect(client)
			row, err := f.db.Queries().GetCharacter(t.Context(), 10)
			require.NoError(t, err)
			require.False(t, row.IsOnline.Bool)
			require.True(t, row.DisconnectAt.Valid)
			require.True(t, row.DisconnectAt.Time.Equal(clock.WallNow()))
			require.Zero(t, f.runtime.Index.Snapshot(10).LastPingUnixMs, "disconnect is not a ping")
			require.True(t, source.world.Alive(body), "disconnect time precedes delayed body removal")
			require.True(t, ecs.GetResource[ecs.DetachedEntities](source.world).IsDetached(10))

			clock.Advance(time.Minute)
			g.handleDisconnect(client) // No second online->offline transition.
			again, err := f.db.Queries().GetCharacter(t.Context(), 10)
			require.NoError(t, err)
			require.True(t, again.DisconnectAt.Time.Equal(row.DisconnectAt.Time))
			replacement, _ := connectPlayerSpawnTestClient(t)
			replacement.CharacterID, replacement.Layer = 10, layer
			source.Clients[10] = replacement
			require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
			g.handleDisconnect(client) // The old callback cannot affect a new session.
			again, err = f.db.Queries().GetCharacter(t.Context(), 10)
			require.NoError(t, err)
			require.True(t, again.IsOnline.Bool)
			require.True(t, again.DisconnectAt.Time.Equal(row.DisconnectAt.Time))
			g.handleDisconnect(replacement)
			row, err = f.db.Queries().GetCharacter(t.Context(), 10)
			require.NoError(t, err)
			require.True(t, row.DisconnectAt.Time.Equal(clock.WallNow()))

			restarted := hearth.NewRuntime(nil, nil, f.db.Queries(), nil, nil, nil, nil, nil)
			require.NoError(t, restarted.LoadHome(10, f.hearth.DefID))
			require.Zero(t, restarted.Index.Snapshot(10).LastPingUnixMs)
		})
	}
}

func TestCharacterDisconnectPostgresClosedAuthentication(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	g := f.games[0]
	server := newHearthReconnectTestServer(t, g)
	client, _ := server.connect(t)
	require.True(t, client.BeginAuthentication())
	client.Close()
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	client.CharacterID = 10 // Commit publishes the identity before EndAuthentication.
	client.EndAuthentication()
	require.Same(t, client, waitHearthTerminal(t, server.disconnected))
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.False(t, row.IsOnline.Bool)
	require.True(t, row.DisconnectAt.Valid)
	require.True(t, row.DisconnectAt.Time.Equal(g.clock.WallNow()))
}

func TestCharacterDisconnectPostgresMonotonicFailureAndStartup(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	g := f.games[0]
	previous := time.Unix(100, 0)
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	g.recordCharacterDisconnect(10, previous)
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	g.recordCharacterDisconnect(10, previous.Add(-time.Minute))
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.True(t, row.DisconnectAt.Time.Equal(previous), "late writes keep the latest confirmed timestamp")
	require.Zero(t, f.runtime.Index.Snapshot(10).LastPingUnixMs)
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	require.NoError(t, f.db.Queries().ResetOnlinePlayers(t.Context(), 1))
	row, err = f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.True(t, row.DisconnectAt.Time.Equal(previous), "startup cannot infer the time of a prior crash")
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	_, err = f.db.Pool().Exec(t.Context(), `
		CREATE FUNCTION reject_disconnect() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test: disconnect write failed'; END $$;
		CREATE TRIGGER reject_disconnect BEFORE UPDATE OF disconnect_at ON character
		FOR EACH ROW EXECUTE FUNCTION reject_disconnect()`)
	require.NoError(t, err)
	g.recordCharacterDisconnect(10, previous.Add(time.Minute))
	row, err = f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.True(t, row.DisconnectAt.Time.Equal(previous))
	require.Zero(t, f.runtime.Index.Snapshot(10).LastPingUnixMs, "disconnect writes do not change last-seen metadata")
}

func TestCharacterDisconnectPostgresShutdownWaitsForAuthentication(t *testing.T) {
	f, g, stop := newHearthShutdownFixture(t, zap.NewNop())
	f.addCharacter(t, 10, 0)
	f.addCharacter(t, 11, 1)
	f.addCharacter(t, 12, 0)
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	g.wg.Add(1)
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	<-g.ctx.Done()
	// An authentication admitted before shutdown may still commit its online flag.
	err := f.db.Queries().SetCharacterOnline(t.Context(), 11)
	g.wg.Done()
	require.NoError(t, err)
	waitHearthTerminal(t, done)
	for _, id := range []int64{10, 11} {
		row, err := f.db.Queries().GetCharacter(t.Context(), id)
		require.NoError(t, err)
		require.False(t, row.IsOnline.Bool)
		require.True(t, row.DisconnectAt.Valid)
		require.True(t, row.DisconnectAt.Time.Equal(g.clock.WallNow()))
	}
	row, err := f.db.Queries().GetCharacter(t.Context(), 12)
	require.NoError(t, err)
	require.Equal(t, sql.NullTime{}, row.DisconnectAt, "already offline characters retain unknown history")
}
