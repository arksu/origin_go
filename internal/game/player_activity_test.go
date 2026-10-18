package game

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/timeutil"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func activityStateForTest(a *playerActivity, id types.EntityID) characterActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	return *a.characters[id]
}

func TestPlayerActivityFractionalSecondsReconnectAndRestart(t *testing.T) {
	var saved repository.UpdateCharacterActivityParams
	a := newPlayerActivity(func(_ context.Context, params repository.UpdateCharacterActivityParams) error {
		saved = params
		return nil
	}, nil, zap.NewNop())
	at := time.Unix(100, 0)
	a.beginSession(7, 1, 100)
	require.True(t, a.recordPing(7, 1, at))
	require.True(t, a.recordPing(7, 1, at.Add(600*time.Millisecond)))
	require.Equal(t, int64(100), activityStateForTest(a, 7).onlineSeconds)
	a.recordPing(7, 1, at.Add(1200*time.Millisecond))
	require.Equal(t, int64(101), activityStateForTest(a, 7).onlineSeconds)
	require.Equal(t, 200*time.Millisecond, activityStateForTest(a, 7).remainder)
	require.True(t, a.endSession(7, 1))
	a.beginSession(7, 2, 100) // A login can read an older, not yet flushed row.
	a.recordPing(7, 2, at.Add(time.Hour))
	a.recordPing(7, 2, at.Add(time.Hour+900*time.Millisecond))
	state := activityStateForTest(a, 7)
	require.Equal(t, int64(102), state.onlineSeconds)
	require.Equal(t, 100*time.Millisecond, state.remainder)
	require.NoError(t, a.flush(t.Context(), 7))
	require.Equal(t, []int64{102}, saved.OnlineTimes)
	require.Equal(t, []int64{at.Add(time.Hour + 900*time.Millisecond).UnixMilli()}, saved.LastPingMs)

	restarted := newPlayerActivity(nil, nil, zap.NewNop())
	restarted.beginSession(7, 3, saved.OnlineTimes[0])
	restarted.recordPing(7, 3, at.Add(24*time.Hour))
	restarted.recordPing(7, 3, at.Add(24*time.Hour+900*time.Millisecond))
	state = activityStateForTest(restarted, 7)
	require.Equal(t, int64(102), state.onlineSeconds, "restart drops only the in-memory fractional remainder and adds no offline gap")
	require.Equal(t, 900*time.Millisecond, state.remainder)
}

func TestPlayerActivityRejectsStaleSessionsAndBackwardClock(t *testing.T) {
	a := newPlayerActivity(nil, nil, zap.NewNop())
	at := time.Unix(100, 0)
	require.False(t, a.recordPing(7, 1, at))
	a.beginSession(7, 1, 0)
	a.recordPing(7, 1, at)
	a.beginSession(7, 2, 0)
	require.False(t, a.recordPing(7, 1, at.Add(time.Hour)))
	require.False(t, a.endSession(7, 1))
	a.recordPing(7, 2, at)
	a.recordPing(7, 2, at.Add(-time.Hour))
	a.recordPing(7, 2, at.Add(5*time.Second))
	require.Equal(t, int64(5), activityStateForTest(a, 7).onlineSeconds)
	require.Equal(t, at.Add(5*time.Second).UnixMilli(), activityStateForTest(a, 7).lastPingMs)
	require.True(t, a.endSession(7, 2))
	require.False(t, a.endSession(7, 2))
	require.False(t, a.recordPing(7, 2, at.Add(6*time.Second)))
	a.beginSession(8, 3, math.MaxInt64-1)
	a.recordPing(8, 3, at)
	a.recordPing(8, 3, at.Add(5*time.Second))
	require.Equal(t, int64(math.MaxInt64), activityStateForTest(a, 8).onlineSeconds)
}

func TestPlayerActivityFailedAndSupersededCheckpoints(t *testing.T) {
	at := time.Unix(100, 0)
	var a *playerActivity
	calls := 0
	a = newPlayerActivity(func(_ context.Context, params repository.UpdateCharacterActivityParams) error {
		calls++
		switch calls {
		case 1:
			return errors.New("write failed")
		case 2:
			require.Equal(t, []int64{5}, params.OnlineTimes)
			a.recordPing(7, 1, at.Add(6*time.Second)) // Arrives during SQL.
		case 3:
			require.Equal(t, []int64{6}, params.OnlineTimes)
		}
		return nil
	}, nil, zap.NewNop())
	a.beginSession(7, 1, 0)
	a.recordPing(7, 1, at)
	a.recordPing(7, 1, at.Add(5*time.Second))
	require.Error(t, a.flush(t.Context(), 7))
	require.Len(t, a.dirty, 1)
	require.NoError(t, a.flush(t.Context(), 7))
	require.Len(t, a.dirty, 1, "acknowledging an older write must retain the new ping")
	require.NoError(t, a.flush(t.Context(), 7))
	require.Empty(t, a.dirty)
	require.Equal(t, 3, calls)
}

func TestPlayerActivityBatchesAndFinalFlush(t *testing.T) {
	count, batches := 0, 0
	a := newPlayerActivity(func(ctx context.Context, params repository.UpdateCharacterActivityParams) error {
		require.NoError(t, ctx.Err())
		require.LessOrEqual(t, len(params.Ids), playerActivityBatchSize)
		count += len(params.Ids)
		batches++
		return nil
	}, nil, zap.NewNop())
	for id := types.EntityID(1); id <= 1100; id++ {
		a.beginSession(id, uint64(id), 0)
		a.recordPing(id, uint64(id), time.Unix(100, 0))
	}
	a.stop()
	require.Equal(t, 1100, count)
	require.Equal(t, 3, batches)
	require.False(t, a.recordPing(1, 1, time.Unix(101, 0)))
	a.stop() // Idempotent shutdown.
	require.Equal(t, 3, batches)
}

func TestPlayerActivityPeriodicFlushAndConcurrentPings(t *testing.T) {
	saved := make(chan struct{}, 1)
	a := newPlayerActivity(func(context.Context, repository.UpdateCharacterActivityParams) error {
		select {
		case saved <- struct{}{}:
		default:
		}
		return nil
	}, nil, zap.NewNop())
	a.start()
	t.Cleanup(a.stop)
	var workers sync.WaitGroup
	for id := types.EntityID(1); id <= 8; id++ {
		a.beginSession(id, uint64(id), 0)
		workers.Go(func() {
			for n := range 100 {
				a.recordPing(id, uint64(id), time.Unix(100, int64(n)*int64(time.Millisecond)))
			}
		})
	}
	workers.Wait()
	select {
	case <-saved:
	case <-time.After(3 * time.Second):
		t.Fatal("periodic player activity checkpoint did not run")
	}
}

func TestPlayerActivityPingDoesNotAllocate(t *testing.T) {
	a := newPlayerActivity(nil, nil, zap.NewNop())
	a.beginSession(1, 1, 0)
	at := time.Unix(100, 0)
	a.recordPing(1, 1, at)
	require.Zero(t, testing.AllocsPerRun(100, func() { a.recordPing(1, 1, at) }))
}

func TestPlayerActivityPingRequiresCurrentLivingSession(t *testing.T) {
	g := newConstantsTestGame(t)
	g.state.Store(int32(GameStateStarting)) // This unit fixture has no real database.
	g.playerActivity = newPlayerActivity(nil, nil, zap.NewNop())
	client, _, _ := connectConstantsTestClient(t, g, 16)
	t.Cleanup(func() { client.CharacterID = 0; client.ClearDeadObserverMode() })
	clock := g.clock.(*timeutil.ManualClock)
	g.handlePing(client, 1, &netproto.C2S_Ping{})
	require.Empty(t, g.playerActivity.characters)
	client.CharacterID = 17
	g.playerActivity.beginSession(17, client.ID, 0)
	g.handlePing(client, 2, &netproto.C2S_Ping{ClientTimeMs: math.MaxInt64})
	clock.Advance(1500 * time.Millisecond)
	g.handlePing(client, 3, &netproto.C2S_Ping{ClientTimeMs: math.MinInt64})
	before := activityStateForTest(g.playerActivity, 17)
	require.Equal(t, int64(1), before.onlineSeconds)
	client.DeadObserverDeadlineUnixMs.Store(1)
	clock.Advance(time.Hour)
	g.handlePing(client, 4, &netproto.C2S_Ping{})
	require.Equal(t, before, activityStateForTest(g.playerActivity, 17))
	client.ClearDeadObserverMode()
	g.playerActivity.beginSession(17, client.ID+1, 0)
	before = activityStateForTest(g.playerActivity, 17)
	g.handlePing(client, 5, &netproto.C2S_Ping{})
	require.Equal(t, before, activityStateForTest(g.playerActivity, 17), "old connections cannot accrue time for a replacement session")
}
