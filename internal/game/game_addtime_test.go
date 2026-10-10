package game

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"origin/internal/ecs"
	gameworld "origin/internal/game/world"
	"origin/internal/timeutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Advance changes runtime only, matching the production clock. Wall time can
// be controlled independently so tests never attribute an admin jump to a frame.
type adminRuntimeClock struct {
	mu      sync.RWMutex
	gameNow time.Time
	wallNow time.Time
}

func (c *adminRuntimeClock) GameNow() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.gameNow
}

func (c *adminRuntimeClock) WallNow() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.wallNow
}

func (c *adminRuntimeClock) UnixMilli() int64 { return c.WallNow().UnixMilli() }

func (c *adminRuntimeClock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gameNow = c.gameNow.Add(d)
}

func TestAdminAddTimeAppliesOnceAndPreservesClockDomains(t *testing.T) {
	wall := time.Unix(10000, 0)
	clock := &adminRuntimeClock{gameNow: time.Unix(1000, 900000000), wallNow: wall}
	g := &Game{clock: clock, currentTick: 7, runtimeSecondsTotal: 1000,
		runtimeRemainder: 900 * time.Millisecond, runtimeSampledAt: wall, startTime: time.Unix(900, 0)}
	before := g.serverTimeSnapshot()
	gameNow := clock.GameNow()
	uptime := gameNow.Sub(g.startTime)

	require.NoError(t, g.RequestAdminAddTime(60))
	require.Equal(t, int64(60), g.pendingAdminSeconds)
	require.Equal(t, before, g.serverTimeSnapshot(), "pending time must not be persisted")
	pongWall, pongRuntime := g.runtimePongSample()
	require.Equal(t, wall.UnixMilli(), pongWall)
	require.Equal(t, int64(1000), pongRuntime, "Pong must not publish a pending jump")
	require.Equal(t, gameNow, clock.GameNow())

	g.applyPendingAdminTime()
	require.Zero(t, g.pendingAdminSeconds)
	require.Equal(t, gameNow.Add(time.Minute), clock.GameNow())
	require.Equal(t, uptime, clock.GameNow().Sub(g.startTime))
	require.Equal(t, 900*time.Millisecond, g.runtimeRemainder)
	require.Equal(t, wall, g.runtimeSampledAt)
	require.Equal(t, wall, clock.WallNow())
	require.Equal(t, uint64(7), g.CurrentTick())
	require.Equal(t, int64(1060), g.serverTimeSnapshot().RuntimeSecondsTotal)

	g.applyPendingAdminTime()
	require.Equal(t, int64(1060), g.serverTimeSnapshot().RuntimeSecondsTotal)
	require.Equal(t, gameNow.Add(time.Minute), clock.GameNow(), "empty drain must not apply twice")

	clock.wallNow = wall.Add(250 * time.Millisecond)
	pongWall, pongRuntime = g.runtimePongSample()
	require.Equal(t, clock.WallNow().UnixMilli(), pongWall)
	require.Equal(t, int64(1061), pongRuntime, "project elapsed wall time from the original anchor")
	require.Equal(t, int64(1060), g.serverTimeSnapshot().RuntimeSecondsTotal)
	clock.Advance(250 * time.Millisecond)
	g.accumulateRuntime(clock.WallNow(), 250*time.Millisecond)
	require.Equal(t, int64(1061), g.runtimeSecondsTotal)
	require.Equal(t, 150*time.Millisecond, g.runtimeRemainder)
	_, pongRuntime = g.runtimePongSample()
	require.Equal(t, int64(1061), pongRuntime, "normal accumulation must not double count the jump")
}

func TestAdminAddTimeRejectsInvalidAndOverflowRequests(t *testing.T) {
	for _, seconds := range []int64{0, -1, math.MinInt64, math.MaxInt64, maxAdminRuntimeSeconds - 104} {
		t.Run(time.Duration(seconds).String(), func(t *testing.T) {
			g := &Game{currentTick: 7, runtimeSecondsTotal: 100, pendingAdminSeconds: 5}
			before := g.serverTimeSnapshot()
			require.Error(t, g.RequestAdminAddTime(seconds))
			require.Equal(t, int64(5), g.pendingAdminSeconds)
			require.Equal(t, before, g.serverTimeSnapshot())
		})
	}
	for _, runtime := range []int64{-1, maxAdminRuntimeSeconds + 1} {
		g := &Game{runtimeSecondsTotal: runtime}
		require.Error(t, g.RequestAdminAddTime(1))
		require.Zero(t, g.pendingAdminSeconds)
		require.Equal(t, runtime, g.runtimeSecondsTotal)
	}

	clock := &adminRuntimeClock{gameNow: time.Unix(0, 0), wallNow: time.Unix(10000, 0)}
	g := &Game{clock: clock, startTime: clock.GameNow()}
	require.NoError(t, g.RequestAdminAddTime(maxAdminRuntimeSeconds-1))
	require.NoError(t, g.RequestAdminAddTime(1))
	require.Error(t, g.RequestAdminAddTime(1), "include previously queued requests in the bound")
	require.Equal(t, maxAdminRuntimeSeconds, g.pendingAdminSeconds)
	g.applyPendingAdminTime()
	require.Equal(t, time.Unix(maxAdminRuntimeSeconds, 0), clock.GameNow(), "duration conversion must not wrap")
	require.Equal(t, maxAdminRuntimeSeconds, g.runtimeSecondsTotal)
	require.Zero(t, clock.GameNow().Sub(g.startTime))
	require.Error(t, g.RequestAdminAddTime(1))
}

func TestAdminAddTimeConcurrentRequestsAndPublication(t *testing.T) {
	wall := time.Unix(10000, 0)
	clock := &adminRuntimeClock{gameNow: time.Unix(0, 0), wallNow: wall}
	g := &Game{clock: clock, runtimeSampledAt: wall, startTime: clock.GameNow(), currentTick: 42}
	const workers, requests = 16, 100
	var wg sync.WaitGroup
	failures := make(chan error, workers*requests)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for request := 0; request < requests; request++ {
				if err := g.RequestAdminAddTime(1); err != nil {
					failures <- err
				}
			}
		}()
	}
	// Publication readers race with request aggregation, but pending seconds
	// are not part of either the network sample or persistence checkpoint.
	for i := 0; i < requests; i++ {
		require.Zero(t, g.serverTimeSnapshot().RuntimeSecondsTotal)
		_, runtime := g.runtimePongSample()
		require.Zero(t, runtime)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	require.Equal(t, int64(workers*requests), g.pendingAdminSeconds)

	// Readers must see either side of the whole applied jump, with the same
	// tick and wall anchor, never a partial sum or duplicated increment.
	wg.Add(1)
	go func() {
		defer wg.Done()
		g.applyPendingAdminTime()
	}()
	for i := 0; i < requests; i++ {
		snapshot := g.serverTimeSnapshot()
		require.Equal(t, uint64(42), snapshot.InitialTick)
		require.Contains(t, []int64{0, workers * requests}, snapshot.RuntimeSecondsTotal)
		pongWall, runtime := g.runtimePongSample()
		require.Equal(t, wall.UnixMilli(), pongWall)
		require.Contains(t, []int64{0, workers * requests}, runtime)
	}
	wg.Wait()
	require.Equal(t, int64(workers*requests), g.runtimeSecondsTotal)
	require.Equal(t, time.Unix(workers*requests, 0), clock.GameNow())
	require.Zero(t, clock.GameNow().Sub(g.startTime))
	g.applyPendingAdminTime()
	require.Equal(t, int64(workers*requests), g.runtimeSecondsTotal)
}

type adminTimeLoopRecorder struct {
	ecs.BaseSystem
	times  []ecs.TimeState
	onTick func(ecs.TimeState)
}

func (r *adminTimeLoopRecorder) Update(w *ecs.World, _ float64) {
	ts := *ecs.GetResource[ecs.TimeState](w)
	r.times = append(r.times, ts)
	r.onTick(ts)
}

func TestAdminAddTimeGameLoopKeepsLayersAndCatchUpCoherent(t *testing.T) {
	wall := time.Unix(10000, 0)
	gameOrigin := time.Unix(28794, 0)
	clock := &adminRuntimeClock{gameNow: gameOrigin, wallNow: wall.Add(5500 * time.Millisecond)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := NewWorkerPool(2)
	defer pool.Stop()
	g := &Game{clock: clock, startTime: gameOrigin, runtimeSecondsTotal: 28794,
		tickRate: 1, tickPeriod: time.Second, ctx: ctx, logger: zap.NewNop(),
		shardManager: &ShardManager{shards: make(map[int]*Shard), workerPool: pool},
		tickStats:    tickStats{lastLog: time.Now(), systemStats: make(map[string]ecs.SystemTimingStat)}}
	g.state.Store(int32(GameStateRunning))
	firstLayerQueued := make(chan struct{})
	var recorders [2]*adminTimeLoopRecorder
	var requestErrors [2]error
	var pendingClock time.Time
	var pendingRuntime int64
	for layer := range recorders {
		w := ecs.NewWorldWithCapacity(16, nil, layer)
		handler := NewChatAdminCommandHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop())
		shard := &Shard{world: w, layer: layer, state: ShardStateRunning,
			chunkManager: &gameworld.ChunkManager{}, adminHandler: handler}
		shard.SetAdminTimeExecutor(g)
		g.shardManager.shards[layer] = shard
		r := &adminTimeLoopRecorder{BaseSystem: ecs.NewBaseSystem("AdminTimeRecorder", 1)}
		r.onTick = func(ts ecs.TimeState) {
			if ts.Tick == 1 {
				if layer == 0 {
					requestErrors[layer] = handler.timeExecutor.RequestAdminAddTime(20)
					close(firstLayerQueued)
				} else {
					<-firstLayerQueued
					pendingClock = clock.GameNow()
					pendingRuntime = g.serverTimeSnapshot().RuntimeSecondsTotal
					requestErrors[layer] = handler.timeExecutor.RequestAdminAddTime(40)
				}
			}
			if ts.Tick == maxCatchUpTicks {
				cancel()
			}
		}
		w.AddSystem(r)
		recorders[layer] = r
	}

	done := make(chan struct{})
	g.wg.Add(1)
	go func() {
		g.gameLoop(wall)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		g.wg.Wait()
		t.Fatal("game loop did not complete its bounded catch-up frame")
	}
	for _, err := range requestErrors {
		require.NoError(t, err)
	}
	initialNow := gameOrigin.Add(5500 * time.Millisecond)
	require.Equal(t, initialNow, pendingClock, "a shard request must not advance the clock mid-tick")
	require.Equal(t, int64(28799), pendingRuntime)
	require.Equal(t, recorders[0].times, recorders[1].times)
	for _, recorder := range recorders {
		require.Len(t, recorder.times, maxCatchUpTicks, "runtime jumps must not add simulation catch-up work")
		for i, ts := range recorder.times {
			require.Equal(t, uint64(i+1), ts.Tick)
			require.Equal(t, 1, ts.TickRate)
			require.Equal(t, time.Second, ts.TickPeriod)
			require.Equal(t, float64(1), ts.Delta)
			require.Equal(t, clock.WallNow(), ts.WallNow)
			require.Equal(t, clock.WallNow().UnixMilli(), ts.UnixMs)
			require.Equal(t, 5500*time.Millisecond, ts.Uptime)
			wantNow, wantRuntime := initialNow, int64(28799)
			if i > 0 {
				wantNow, wantRuntime = initialNow.Add(time.Minute), 28859
			}
			require.Equal(t, wantNow, ts.Now)
			require.Equal(t, wantRuntime, ts.RuntimeSecondsTotal)
			calendar, err := timeutil.GameCalendarFromRuntime(ts.RuntimeSecondsTotal)
			require.NoError(t, err)
			wantDay := int64(1)
			if i > 0 {
				wantDay = 2
			}
			require.Equal(t, wantDay, calendar.Day)
		}
	}
	require.Equal(t, 500*time.Millisecond, g.runtimeRemainder)
	require.Equal(t, clock.WallNow(), g.runtimeSampledAt)
	require.Equal(t, int64(28859), g.serverTimeSnapshot().RuntimeSecondsTotal)
	pongWall, pongRuntime := g.runtimePongSample()
	require.Equal(t, clock.WallNow().UnixMilli(), pongWall)
	require.Equal(t, int64(28859), pongRuntime)
	require.Zero(t, g.pendingAdminSeconds)
}
