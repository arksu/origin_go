package ecs

import (
	"testing"

	"origin/internal/types"
)

func TestEntityStatsUpdateState_RegenScheduleAndReschedule(t *testing.T) {
	state := &EntityStatsUpdateState{}
	handleOne := types.MakeHandle(1, 1)
	handleTwo := types.MakeHandle(2, 1)

	if !state.ScheduleRegen(handleOne, 10) {
		t.Fatalf("expected initial regen schedule to succeed")
	}
	if !state.ScheduleRegen(handleOne, 20) {
		t.Fatalf("expected regen reschedule to succeed")
	}
	if !state.ScheduleRegen(handleTwo, 15) {
		t.Fatalf("expected second regen schedule to succeed")
	}
	if state.PendingRegenCount() != 2 {
		t.Fatalf("unexpected pending regen count: %d", state.PendingRegenCount())
	}

	due := state.PopDueRegen(14, nil)
	if len(due) != 0 {
		t.Fatalf("expected no due regen at tick 14, got %d", len(due))
	}

	due = state.PopDueRegen(15, due[:0])
	if len(due) != 1 || due[0] != handleTwo {
		t.Fatalf("unexpected due regen at tick 15: %+v", due)
	}

	due = state.PopDueRegen(20, due[:0])
	if len(due) != 1 || due[0] != handleOne {
		t.Fatalf("unexpected due regen at tick 20: %+v", due)
	}
}

func TestEntityStatsUpdateState_PlayerPushTTLAndCoalescing(t *testing.T) {
	state := &EntityStatsUpdateState{}
	const ttlMs uint32 = 1000
	playerOne := types.EntityID(1)
	playerTwo := types.EntityID(2)

	if !state.MarkPlayerSent(playerOne, 1000) {
		t.Fatalf("expected MarkPlayerSent to succeed")
	}
	if !state.MarkPlayerDirty(playerOne, 1200, ttlMs) {
		t.Fatalf("expected MarkPlayerDirty to succeed")
	}
	// Re-mark inside the same ttl window should coalesce into one latest schedule.
	if !state.MarkPlayerDirty(playerOne, 1300, ttlMs) {
		t.Fatalf("expected second MarkPlayerDirty to succeed")
	}
	if state.PendingPlayerPushCount() != 1 {
		t.Fatalf("expected one pending player push, got %d", state.PendingPlayerPushCount())
	}

	due := state.PopDuePlayerStatsPush(1999, nil)
	if len(due) != 0 {
		t.Fatalf("expected no due player push before ttl boundary, got %d", len(due))
	}

	due = state.PopDuePlayerStatsPush(2000, due[:0])
	if len(due) != 1 || due[0] != playerOne {
		t.Fatalf("unexpected due player push at ttl boundary: %+v", due)
	}

	if !state.MarkPlayerSent(playerOne, 2000) {
		t.Fatalf("expected second MarkPlayerSent to succeed")
	}
	if !state.MarkPlayerDirty(playerOne, 2500, ttlMs) {
		t.Fatalf("expected third MarkPlayerDirty to succeed")
	}
	if !state.MarkPlayerDirty(playerTwo, 2500, ttlMs) {
		t.Fatalf("expected second player dirty mark to succeed")
	}

	due = state.PopDuePlayerStatsPush(2600, due[:0])
	if len(due) != 1 || due[0] != playerTwo {
		t.Fatalf("unexpected due player push at 2600ms: %+v", due)
	}

	due = state.PopDuePlayerStatsPush(3000, due[:0])
	if len(due) != 1 || due[0] != playerOne {
		t.Fatalf("unexpected due player push at 3000ms: %+v", due)
	}
}

func TestEntityStatsUpdateState_ForgetOnDespawn(t *testing.T) {
	world := NewWorldForTesting()
	entityID := types.EntityID(42)
	handle := world.Spawn(entityID, nil)

	state := GetResource[EntityStatsUpdateState](world)
	if !state.ScheduleRegen(handle, 5) {
		t.Fatalf("expected regen schedule to succeed")
	}
	if !state.MarkPlayerSent(entityID, 100) {
		t.Fatalf("expected MarkPlayerSent to succeed")
	}
	if !state.MarkPlayerDirty(entityID, 200, 1000) {
		t.Fatalf("expected MarkPlayerDirty to succeed")
	}
	if state.PendingRegenCount() != 1 {
		t.Fatalf("unexpected pending regen count before despawn: %d", state.PendingRegenCount())
	}
	if state.PendingPlayerPushCount() != 1 {
		t.Fatalf("unexpected pending push count before despawn: %d", state.PendingPlayerPushCount())
	}

	if !world.Despawn(handle) {
		t.Fatalf("expected despawn to succeed")
	}

	if state.PendingRegenCount() != 0 {
		t.Fatalf("expected no pending regen after despawn, got %d", state.PendingRegenCount())
	}
	if state.PendingPlayerPushCount() != 0 {
		t.Fatalf("expected no pending player push after despawn, got %d", state.PendingPlayerPushCount())
	}
	if _, exists := state.lastSentUnixMs[entityID]; exists {
		t.Fatalf("expected last sent record to be removed on despawn")
	}

	dueRegen := state.PopDueRegen(100, nil)
	if len(dueRegen) != 0 {
		t.Fatalf("expected no due regen after despawn, got %+v", dueRegen)
	}
	duePush := state.PopDuePlayerStatsPush(10_000, nil)
	if len(duePush) != 0 {
		t.Fatalf("expected no due push after despawn, got %+v", duePush)
	}
}
