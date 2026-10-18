package game

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"origin/internal/game/hearth"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/timeutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPlayerActivityPostgresPingDisconnectAndRestart(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 1)
	g := f.games[1]
	g.shardManager.shards[0] = f.shards[0]
	g.playerActivity = newPlayerActivity(f.db.Queries().UpdateCharacterActivity, f.runtime.Index.RecordPing, g.logger)
	client, _ := connectPlayerSpawnTestClient(t)
	client.CharacterID, client.Layer = 10, 1
	clock := g.clock.(*timeutil.ManualClock)
	g.playerActivity.beginSession(10, client.ID, 42)
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	// Authenticated pings count even before world entry and during transfers.
	require.False(t, client.InWorld.Load())
	g.handlePing(client, 1, &netproto.C2S_Ping{ClientTimeMs: 1 << 60})
	clock.Advance(5500 * time.Millisecond)
	g.handlePing(client, 2, &netproto.C2S_Ping{ClientTimeMs: -1 << 60})
	require.Equal(t, clock.WallNow().UnixMilli(), f.runtime.Index.Snapshot(10).LastPingUnixMs)
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.False(t, row.LastPingAt.Valid, "pings update memory without synchronous SQL")
	require.Zero(t, row.OnlineTime)
	lastPing := clock.WallNow()
	clock.Advance(2 * time.Second)
	g.handleDisconnect(client)
	row, err = f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(47), row.OnlineTime)
	require.True(t, row.LastPingAt.Valid)
	require.True(t, row.LastPingAt.Time.Equal(lastPing), "disconnect does not extend last-seen time")
	require.True(t, row.DisconnectAt.Time.Equal(clock.WallNow()))

	restarted := hearth.NewRuntime(nil, nil, f.db.Queries(), nil, nil, nil, nil, nil)
	require.NoError(t, restarted.LoadHome(10, f.hearth.DefID))
	require.Equal(t, lastPing.UnixMilli(), restarted.Index.Snapshot(10).LastPingUnixMs)
	// A replacement connection keeps the fractional second but excludes its gap.
	next, _ := connectPlayerSpawnTestClient(t)
	next.CharacterID, next.Layer = 10, 1
	g.playerActivity.beginSession(10, next.ID, row.OnlineTime)
	clock.Advance(time.Hour)
	g.handlePing(next, 3, &netproto.C2S_Ping{})
	clock.Advance(500 * time.Millisecond)
	g.handlePing(next, 4, &netproto.C2S_Ping{})
	require.NoError(t, g.playerActivity.flush(t.Context(), 10))
	row, err = f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(48), row.OnlineTime)
}

func TestPlayerActivityPostgresIdempotentAndOlderCheckpoints(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	at := time.Unix(100, 123000000)
	committed := false
	a := newPlayerActivity(func(ctx context.Context, params repository.UpdateCharacterActivityParams) error {
		if err := f.db.Queries().UpdateCharacterActivity(ctx, params); err != nil {
			return err
		}
		if !committed {
			committed = true
			return errors.New("test: commit succeeded but acknowledgement was lost")
		}
		return nil
	}, nil, zap.NewNop())
	a.beginSession(10, 1, 12)
	a.recordPing(10, 1, at)
	a.recordPing(10, 1, at.Add(5*time.Second))
	require.Error(t, a.flush(t.Context(), 10))
	require.NoError(t, a.flush(t.Context(), 10))
	require.NoError(t, f.db.Queries().UpdateCharacterActivity(t.Context(), repository.UpdateCharacterActivityParams{
		Ids: []int64{10}, OnlineTimes: []int64{13}, LastPingMs: []int64{at.UnixMilli()},
	}))
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(17), row.OnlineTime)
	require.Equal(t, at.Add(5*time.Second).UnixMilli(), row.LastPingAt.Time.UnixMilli())
	// A ping received while alive still belongs in historical metadata after death.
	a.recordPing(10, 1, at.Add(6*time.Second))
	require.NoError(t, f.db.Queries().DeleteCharacterByID(t.Context(), 10))
	require.NoError(t, a.flush(t.Context(), 10))
	var total int64
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), "SELECT online_time FROM character WHERE id=10").Scan(&total))
	require.Equal(t, int64(18), total)
}

func TestPlayerActivityPostgresShutdownFlushesPendingPings(t *testing.T) {
	f, g, stop := newHearthShutdownFixture(t, zap.NewNop())
	f.addCharacter(t, 10, 1)
	g.playerActivity = newPlayerActivity(f.db.Queries().UpdateCharacterActivity, f.runtime.Index.RecordPing, g.logger)
	g.playerActivity.beginSession(10, 7, 0)
	g.playerActivity.recordPing(10, 7, time.Unix(2, 0))
	g.playerActivity.recordPing(10, 7, time.Unix(7, 700000000))
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))
	stop() // Cancels g.ctx first; the final checkpoint needs an independent context.
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(5), row.OnlineTime)
	require.Equal(t, int64(7700), row.LastPingAt.Time.UnixMilli())
	require.True(t, row.DisconnectAt.Time.Equal(g.clock.WallNow()))
}

func TestPlayerActivityPostgresMigrationPreservesExistingTotals(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	_, err := f.db.Pool().Exec(t.Context(), "UPDATE character SET online_time=123 WHERE id=10; ALTER TABLE character DROP COLUMN last_ping_at")
	require.NoError(t, err)
	source, err := os.ReadFile(filepath.Join("..", "..", "migrations", "20261017_character_last_ping.sql"))
	require.NoError(t, err)
	for range 2 { // Deployment migration is safe to rerun.
		_, err = f.db.Pool().Exec(t.Context(), strings.ReplaceAll(string(source), "origin.", ""))
		require.NoError(t, err)
	}
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, int64(123), row.OnlineTime)
	require.Equal(t, sql.NullTime{}, row.LastPingAt)
}
