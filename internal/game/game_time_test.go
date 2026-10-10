package game

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"origin/internal/timeutil"

	"github.com/stretchr/testify/require"
)

func TestRuntimeAccumulatorStartsAfterLoading(t *testing.T) {
	clock := timeutil.NewManualClock(time.Unix(1000, 0))
	g := &Game{clock: clock, currentTick: 123, runtimeSecondsTotal: 456}
	clock.Advance(2 * time.Hour) // World loading is outside runtime accumulation.
	origin := g.initializeRuntimeAccumulator()
	require.Equal(t, clock.WallNow(), origin)
	require.Equal(t, origin, g.runtimeSampledAt)
	require.Equal(t, int64(456), g.runtimeSecondsTotal)
	clock.Advance(900 * time.Millisecond)
	g.accumulateRuntime(clock.WallNow(), clock.WallNow().Sub(origin))
	require.Equal(t, int64(456), g.runtimeSecondsTotal)
	require.Equal(t, 900*time.Millisecond, g.runtimeRemainder)
	clock.Advance(200 * time.Millisecond)
	g.accumulateRuntime(clock.WallNow(), 200*time.Millisecond)
	require.Equal(t, int64(457), g.runtimeSecondsTotal)
	require.Equal(t, 100*time.Millisecond, g.runtimeRemainder)
	require.Equal(t, uint64(123), g.serverTimeSnapshot().InitialTick)
}

func TestRuntimeAccumulatorKeepsLargeElapsedRemainderBounded(t *testing.T) {
	g := &Game{runtimeSecondsTotal: 7, runtimeRemainder: 999 * time.Millisecond}
	elapsed := time.Duration(math.MaxInt64)
	now := time.Now()
	g.accumulateRuntime(now, elapsed)
	require.Equal(t, int64(7)+int64(elapsed/time.Second)+1, g.runtimeSecondsTotal)
	require.Equal(t, 999*time.Millisecond+elapsed%time.Second-time.Second, g.runtimeRemainder)
	require.Equal(t, now, g.runtimeSampledAt)
}

func TestRuntimePongSampleProjectsWithoutMutatingAccumulator(t *testing.T) {
	origin := time.Now() // Retain its monotonic component in the published anchor.
	clock := timeutil.NewManualClock(origin)
	g := &Game{clock: clock, currentTick: 7, runtimeSecondsTotal: 1000, runtimeRemainder: 900 * time.Millisecond, runtimeSampledAt: origin}
	before := g.serverTimeSnapshot()
	for _, tc := range []struct {
		elapsed time.Duration
		seconds int64
	}{
		{0, 1000},
		{99 * time.Millisecond, 1000},
		{100 * time.Millisecond, 1001},
		{2200 * time.Millisecond, 1003},
		{-time.Second, 1000},
	} {
		t.Run(tc.elapsed.String(), func(t *testing.T) {
			clock.Set(origin.Add(tc.elapsed))
			wall, runtime := g.runtimePongSample()
			require.Equal(t, clock.WallNow().UnixMilli(), wall)
			require.Equal(t, tc.seconds, runtime)
			require.Equal(t, before, g.serverTimeSnapshot())
			require.Equal(t, 900*time.Millisecond, g.runtimeRemainder)
			require.Equal(t, origin, g.runtimeSampledAt)
		})
	}
}

func TestRuntimePongSampleZeroAndLargeCounter(t *testing.T) {
	origin := time.Now()
	require.Zero(t, projectRuntimeSeconds(0, 0, origin, origin.Add(999*time.Millisecond)))
	require.Equal(t, int64(math.MaxInt64), projectRuntimeSeconds(math.MaxInt64, 0, origin, origin))
	require.Equal(t, int64(math.MaxInt64), projectRuntimeSeconds(math.MaxInt64-1, 900*time.Millisecond, origin, origin.Add(2200*time.Millisecond)))
}

func TestRuntimePublicationConcurrentReaders(t *testing.T) {
	origin := time.Now()
	const iterations = 10000
	// Every coherent publication projects to the same response instant and value,
	// so torn copies of seconds/remainder/sampledAt are observable, not just races.
	clock := timeutil.NewManualClock(origin.Add(10000 * time.Second))
	g := &Game{clock: clock, currentTick: 42, runtimeSecondsTotal: 100, runtimeSampledAt: origin}
	var wg sync.WaitGroup
	errors := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= iterations; i++ {
			g.accumulateRuntime(origin.Add(time.Duration(i)*100*time.Millisecond), 100*time.Millisecond)
		}
	}()
	for i := 0; i < iterations; i++ {
		wall, runtime := g.runtimePongSample()
		if runtime != 10100 || wall != clock.WallNow().UnixMilli() {
			select {
			case errors <- fmt.Errorf("incoherent sample: wall=%d runtime=%d", wall, runtime):
			default:
			}
		}
		_ = g.serverTimeSnapshot()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	snapshot := g.serverTimeSnapshot()
	require.Equal(t, uint64(42), snapshot.InitialTick)
	require.Equal(t, int64(1100), snapshot.RuntimeSecondsTotal)
	require.Zero(t, g.runtimeRemainder)
}
