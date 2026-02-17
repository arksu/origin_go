package timeutil

import (
	"sync"
	"time"
)

// Clock provides a unified time source for the game server.
// GameNow returns simulation game time.
// WallNow returns real system time (for auth, tokens, DB).
type Clock interface {
	// GameNow returns simulation game time.
	// Use for all game simulation and expiration timers.
	GameNow() time.Time

	// WallNow returns real system wall-clock time.
	// Use for auth/token expiry, DB timestamps, and network server_time_ms.
	WallNow() time.Time

	// UnixMilli returns WallNow().UnixMilli() for network packets.
	UnixMilli() int64

	// Advance moves simulation time forward.
	Advance(d time.Duration)
}

// MonotonicClock is the production simulation clock implementation.
// Game time is advanced explicitly by ticks, independent from wall-clock speed.
type MonotonicClock struct {
	mu      sync.RWMutex
	gameNow time.Time
}

// NewMonotonicClockAt creates a production clock anchored to a specific wall time.
func NewMonotonicClockAt(startWall time.Time) *MonotonicClock {
	return &MonotonicClock{
		gameNow: startWall,
	}
}

func (c *MonotonicClock) GameNow() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.gameNow
}

func (c *MonotonicClock) WallNow() time.Time {
	return time.Now()
}

func (c *MonotonicClock) UnixMilli() int64 {
	return c.WallNow().UnixMilli()
}

func (c *MonotonicClock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gameNow = c.gameNow.Add(d)
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
	return c.WallNow().UnixMilli()
}

// Advance moves the manual clock forward by d.
func (c *ManualClock) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
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
