package entitystats

import (
	"math"
	"testing"

	constt "origin/internal/const"
)

func TestResolveAllowedMoveMode(t *testing.T) {
	max := 100.0

	mode, canMove := ResolveAllowedMoveMode(constt.FastRun, 49, max, 1000)
	if !canMove || mode != constt.Run {
		t.Fatalf("expected fast run downgrade to run at 49%%, got mode=%v canMove=%v", mode, canMove)
	}

	mode, canMove = ResolveAllowedMoveMode(constt.Run, 24, max, 1000)
	if !canMove || mode != constt.Walk {
		t.Fatalf("expected run downgrade to walk at 24%%, got mode=%v canMove=%v", mode, canMove)
	}

	mode, canMove = ResolveAllowedMoveMode(constt.Swim, 9, max, 1000)
	if !canMove || mode != constt.Crawl {
		t.Fatalf("expected crawl-only below 10%%, got mode=%v canMove=%v", mode, canMove)
	}

	mode, canMove = ResolveAllowedMoveMode(constt.Walk, 4, max, 1000)
	if canMove || mode != constt.Crawl {
		t.Fatalf("expected no movement below 5%%, got mode=%v canMove=%v", mode, canMove)
	}
}

func TestResolveAllowedMoveMode_OverstuffedForcesCrawl(t *testing.T) {
	max := 100.0

	mode, canMove := ResolveAllowedMoveMode(constt.FastRun, 90, max, 1001)
	if !canMove || mode != constt.Crawl {
		t.Fatalf("expected overstuffed entity to be forced to crawl, got mode=%v canMove=%v", mode, canMove)
	}

	mode, canMove = ResolveAllowedMoveMode(constt.Run, 90, max, 1000)
	if !canMove || mode != constt.Run {
		t.Fatalf("expected threshold energy to keep allowed mode, got mode=%v canMove=%v", mode, canMove)
	}
}

func TestResolveMovementStaminaCostPerTick_Swim(t *testing.T) {
	cost := ResolveMovementStaminaCostPerTick(constt.Swim, 10, MovementTileContext{})
	if math.Abs(cost-1.0) > 1e-9 {
		t.Fatalf("expected swim cost 1.0 for CON=10, got %.8f", cost)
	}

	cost = ResolveMovementStaminaCostPerTick(constt.Swim, 40, MovementTileContext{})
	if math.Abs(cost-0.5) > 1e-9 {
		t.Fatalf("expected swim cost 0.5 for CON=40, got %.8f", cost)
	}
}

func TestCanConsumeLongActionStamina(t *testing.T) {
	max := 100.0
	if CanConsumeLongActionStamina(20, max, 11) {
		t.Fatalf("expected consume to fail when it drops below floor")
	}
	if !CanConsumeLongActionStamina(20, max, 10) {
		t.Fatalf("expected consume to pass at exact floor")
	}
}
