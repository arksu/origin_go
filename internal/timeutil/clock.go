package timeutil

import (
	"sync"
	"time"
)

// Clock provides a unified time source for the game server.
// GameNow returns monotonic game time (stable, no NTP jumps).
// WallNow returns real system time (for auth, tokens, DB).
type Clock interface {
	// GameNow returns monotonic game time: startWall + time.Since(startMono).
	// Use for all game simulation, network server_time_ms, and expiration timers.
	GameNow() time.Time

	// WallNow returns real system wall-clock time.
	// Use only for auth/token expiry and database timestamps.
	WallNow() time.Time

	// UnixMilli returns GameNow().UnixMilli() for network packets.
	UnixMilli() int64
}

// MonotonicClock is the production Clock implementation.
// It captures wall and monotonic baselines at creation and derives
// GameNow as startWall + elapsed(monotonic), so it never jumps on NTP adjustments.
type MonotonicClock struct {
	startWall time.Time
	startMono time.Time
}

// NewMonotonicClock creates a production clock anchored to the current instant.
func NewMonotonicClock() *MonotonicClock {
	now := time.Now()
	return &MonotonicClock{
		startWall: now,
		startMono: now,
	}
}

func (c *MonotonicClock) GameNow() time.Time {
	return c.startWall.Add(time.Since(c.startMono))
}

func (c *MonotonicClock) WallNow() time.Time {
	return time.Now()
}

func (c *MonotonicClock) UnixMilli() int64 {
	return c.GameNow().UnixMilli()
}

// ManualClock is a test Clock where time is advanced explicitly.
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewManualClock creates a test clock fixed at the given instant.
func NewManualClock(t time.Time) *ManualClock {
	return &ManualClock{now: t}
}

func (c *ManualClock) GameNow() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) WallNow() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) UnixMilli() int64 {
	return c.GameNow().UnixMilli()
}

// Advance moves the manual clock forward by d.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// Set sets the manual clock to an exact instant.
func (c *ManualClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}
