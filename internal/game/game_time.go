package game

import (
	"math"
	"time"
)

// Start counting only when the loop starts, after world/bootstrap loading.
// The returned instant is also the first loop frame's elapsed-time origin.
func (g *Game) initializeRuntimeAccumulator() time.Time {
	g.timeStateMu.Lock()
	defer g.timeStateMu.Unlock()
	g.runtimeSampledAt = g.clock.WallNow()
	return g.runtimeSampledAt
}

func (g *Game) accumulateRuntime(sampledAt time.Time, elapsed time.Duration) {
	if elapsed < 0 {
		elapsed = 0
	}
	g.timeStateMu.Lock()
	defer g.timeStateMu.Unlock()
	// Split before adding: even a large elapsed duration cannot overflow the
	// bounded subsecond remainder. Accumulated seconds never become a Duration.
	g.runtimeSecondsTotal += int64(elapsed / time.Second)
	g.runtimeRemainder += elapsed % time.Second
	if g.runtimeRemainder >= time.Second {
		g.runtimeSecondsTotal++
		g.runtimeRemainder -= time.Second
	}
	g.runtimeSampledAt = sampledAt
}

// runtimePongSample pairs response wall time with a read-only projection of the
// published runtime accumulator. It never advances the clock or gameplay state.
func (g *Game) runtimePongSample() (wallUnixMs, runtimeSeconds int64) {
	g.timeStateMu.RLock()
	seconds := g.runtimeSecondsTotal
	remainder := g.runtimeRemainder
	sampledAt := g.runtimeSampledAt
	now := g.clock.WallNow()
	g.timeStateMu.RUnlock()
	return now.UnixMilli(), projectRuntimeSeconds(seconds, remainder, sampledAt, now)
}

func projectRuntimeSeconds(seconds int64, remainder time.Duration, sampledAt, now time.Time) int64 {
	if sampledAt.IsZero() {
		return seconds
	}
	// Both times retain the production clock's monotonic component. A clock
	// implementation without one must not turn a backward wall jump into runtime.
	elapsed := now.Sub(sampledAt)
	if elapsed <= 0 {
		return seconds
	}
	addedSeconds := int64(elapsed / time.Second)
	if remainder+elapsed%time.Second >= time.Second {
		addedSeconds++
	}
	if seconds > math.MaxInt64-addedSeconds {
		return math.MaxInt64
	}
	return seconds + addedSeconds
}
