package timeutil

import (
	"testing"
	"time"
)

func TestMonotonicClock_DoesNotFollowWallTime(t *testing.T) {
	start := time.Unix(1700000000, 0)
	clock := NewMonotonicClockAt(start)

	first := clock.GameNow()
	time.Sleep(5 * time.Millisecond)
	second := clock.GameNow()

	if !second.Equal(first) {
		t.Fatalf("expected game time to stay constant without ticks: got %v -> %v", first, second)
	}
}

func TestMonotonicClock_Advance(t *testing.T) {
	start := time.Unix(1700000000, 0)
	clock := NewMonotonicClockAt(start)

	clock.Advance(100 * time.Millisecond)
	clock.Advance(250 * time.Millisecond)

	got := clock.GameNow()
	want := start.Add(350 * time.Millisecond)

	if !got.Equal(want) {
		t.Fatalf("unexpected advanced time: got %v, want %v", got, want)
	}
}

func TestMonotonicClock_AdvanceNonPositiveNoop(t *testing.T) {
	start := time.Unix(1700000000, 0)
	clock := NewMonotonicClockAt(start)

	clock.Advance(0)
	clock.Advance(-time.Second)

	got := clock.GameNow()
	if !got.Equal(start) {
		t.Fatalf("expected no-op advance for non-positive duration: got %v, want %v", got, start)
	}
}
