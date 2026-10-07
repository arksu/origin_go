package ecs

import (
	"math/rand"
	"reflect"
	"slices"
	"testing"

	"origin/internal/types"
)

func TestEntityStatsUpdateState_PreparedPlayerLifetime(t *testing.T) {
	state := &EntityStatsUpdateState{}
	id := types.EntityID(42)
	handle := types.MakeHandle(1, 1)
	replacement := types.MakeHandle(1, 2)
	if state.PreparePlayer(0, handle) || state.PreparePlayer(id, types.InvalidHandle) {
		t.Fatal("invalid identity must not create preparation")
	}
	if !state.PreparePlayer(id, handle) || !state.PreparePlayer(id, handle) {
		t.Fatal("preparation must be idempotent for the same handle")
	}
	if state.PreparePlayer(id, replacement) || state.IsPlayerPrepared(id, replacement) {
		t.Fatal("replacement must not inherit or overwrite an old preparation")
	}
	if !state.IsPlayerPrepared(id, handle) || state.PendingPlayerPushCount() != 0 {
		t.Fatal("preparation must not schedule a notification")
	}
	state.MarkPlayerStatsSent(id, PlayerStatsNetSnapshot{SHP: 10}, 1000)
	state.MarkPlayerDirty(id, 1100, 1000)
	if !state.ForgetPlayer(id) || !state.IsPlayerPrepared(id, handle) || len(state.pushQueue) != 0 {
		t.Fatal("disconnect must clear pending state but retain prepared storage")
	}
	if _, sent := state.GetLastSentPlayerStats(id); sent {
		t.Fatal("disconnect must clear last sent state")
	}
	state.MarkPlayerDirty(id, 1200, 0)
	if state.ReleasePlayer(id, replacement) || state.ForgetEntity(id, replacement) {
		t.Fatal("stale release must leave the current prepared entity intact")
	}
	if !state.IsPlayerPrepared(id, handle) || state.PendingPlayerPushCount() != 1 {
		t.Fatal("stale release changed current pending state")
	}
	if !state.ReleasePlayer(id, handle) || len(state.pushQueue) != 0 || len(state.pushLatest) != 0 {
		t.Fatal("release must remove the record and its real heap entry")
	}
	if state.ReleasePlayer(id, handle) || !state.PreparePlayer(id, replacement) {
		t.Fatal("released preparation must not affect a replacement generation")
	}
	state.MarkPlayerDirty(id, 1300, 0)
	if !state.ForgetEntity(id, replacement) || len(state.pushLatest) != 0 || len(state.pushQueue) != 0 {
		t.Fatal("despawn must retire preparation and pending state")
	}
}

func TestEntityStatsUpdateState_UrgentDeadlineCannotBePostponed(t *testing.T) {
	state := &EntityStatsUpdateState{}
	state.PreparePlayer(1, types.MakeHandle(1, 1))
	state.MarkPlayerSent(1, 1000)
	state.MarkPlayerDirty(1, 1100, 1000)
	state.MarkPlayerDirty(1, 1100, 0)
	for now := int64(1100); now < 2000; now++ {
		state.MarkPlayerDirty(1, now, 1000)
		state.MarkPlayerDirty(1, now, 0)
	}
	if len(state.pushQueue) != 1 || state.pushQueue[0].DueUnixMs != 1100 {
		t.Fatalf("urgent deadline postponed or duplicated: %+v", state.pushQueue)
	}
	if got := state.PopDuePlayerStatsPush(1100, nil); !reflect.DeepEqual(got, []types.EntityID{1}) {
		t.Fatalf("urgent notification did not become due immediately: %v", got)
	}
	if len(state.pushQueue) != 0 || state.pushLatest[1].HeapIndex != -1 {
		t.Fatal("drain left a superseded entry or pending index")
	}
}

func TestEntityStatsUpdateState_LegacyRecordCanBePrepared(t *testing.T) {
	state := &EntityStatsUpdateState{}
	state.MarkPlayerDirty(1, 1000, 0)
	if !state.PreparePlayer(1, types.MakeHandle(1, 1)) || state.PendingPlayerPushCount() != 1 {
		t.Fatal("preparing a legacy record must preserve its existing notification")
	}
	state.PopDuePlayerStatsPush(1000, nil)
	if !state.IsPlayerPrepared(1, types.MakeHandle(1, 1)) {
		t.Fatal("drain must retain preparation")
	}
	state.MarkPlayerDirty(2, 1000, 0)
	state.PopDuePlayerStatsPush(1000, nil)
	if !state.ForgetPlayer(2) || state.pushLatest[2] != nil {
		t.Fatal("legacy records must be retired by ForgetPlayer")
	}
}

func TestEntityStatsUpdateState_PreparedCyclesDoNotAllocate(t *testing.T) {
	state := &EntityStatsUpdateState{}
	const players = 300 // Cross both the old heap and initial map capacities.
	for index := 1; index <= players; index++ {
		if !state.PreparePlayer(types.EntityID(index), types.MakeHandle(uint32(index), 1)) {
			t.Fatal("preparation failed")
		}
	}
	due := make([]types.EntityID, 0, players)
	allocs := testing.AllocsPerRun(100, func() {
		for index := players; index >= 1; index-- {
			id := types.EntityID(index)
			state.MarkPlayerDirty(id, 1000+int64(index), 0)
			state.MarkPlayerDirty(id, 900, 0) // Earlier deadline exercises fix.
			state.MarkPlayerDirty(id, 2000, 1000)
		}
		state.ForgetPlayer(150) // Remove a non-root entry, then reuse preparation.
		state.MarkPlayerDirty(150, 900, 0)
		due = state.PopDuePlayerStatsPush(3000, due[:0])
	})
	if allocs != 0 || len(due) != players || len(state.pushLatest) != players {
		t.Fatalf("prepared cycle allocated or lost records: allocs=%g due=%d records=%d", allocs, len(due), len(state.pushLatest))
	}
	for index, id := range due {
		if id != types.EntityID(index+1) {
			t.Fatalf("equal deadlines must drain in EntityID order: %v", due)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		state.PreparePlayer(1, types.MakeHandle(1, 1))
		state.PreparePlayer(0, types.MakeHandle(1, 1))
		state.PreparePlayer(1, types.InvalidHandle)
		state.PreparePlayer(1, types.MakeHandle(1, 2))
		state.ReleasePlayer(1, types.MakeHandle(1, 2))
		state.MarkPlayerDirty(0, 1000, 0)
	}); allocs != 0 {
		t.Fatalf("idempotent preparation and invalid requests allocated: %g", allocs)
	}
}

func TestEntityStatsUpdateState_IndexedHeapMatchesDeadlineModel(t *testing.T) {
	state := &EntityStatsUpdateState{}
	model := make(map[types.EntityID]int64)
	random := rand.New(rand.NewSource(42))
	buffer := make([]types.EntityID, 0, 100)
	for step := 0; step < 5000; step++ {
		id := types.EntityID(random.Intn(100) + 1)
		switch random.Intn(4) {
		case 0, 1:
			due := random.Int63n(1000)
			state.MarkPlayerDirty(id, due, 0)
			if previous, exists := model[id]; !exists || due < previous {
				model[id] = due
			}
		case 2:
			state.ForgetPlayer(id)
			delete(model, id)
		case 3:
			now := random.Int63n(1000)
			want := make([]types.EntityID, 0, len(model))
			for candidate, due := range model {
				if due <= now {
					want = append(want, candidate)
				}
			}
			slices.SortFunc(want, func(first, second types.EntityID) int {
				if model[first] < model[second] || (model[first] == model[second] && first < second) {
					return -1
				}
				if first == second {
					return 0
				}
				return 1
			})
			got := state.PopDuePlayerStatsPush(now, buffer[:0])
			if !slices.Equal(got, want) {
				t.Fatalf("step %d: due order got %v want %v", step, got, want)
			}
			for _, candidate := range want {
				delete(model, candidate)
			}
		}
		if len(state.pushQueue) != len(model) || len(state.pushQueue) > len(state.pushLatest) {
			t.Fatalf("step %d: heap contains stale entries", step)
		}
		for index, record := range state.pushQueue {
			if record.HeapIndex != index || state.pushLatest[record.EntityID] != record {
				t.Fatalf("step %d: indexed heap points to wrong record", step)
			}
			if index > 0 && state.pushQueue.Less(index, (index-1)/2) {
				t.Fatalf("step %d: heap priority violated", step)
			}
		}
	}
}

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

func TestEntityStatsUpdateState_PlayerStatsNetDiff(t *testing.T) {
	state := &EntityStatsUpdateState{}
	entityID := types.EntityID(700)
	first := PlayerStatsNetSnapshot{Stamina: 100, Energy: 1000, StaminaMax: 1500, EnergyMax: 1000, SHP: 90, HHP: 100, MHP: 120, IsKnockedOut: false}

	if !state.ShouldSendPlayerStats(entityID, first, false) {
		t.Fatalf("expected first snapshot to be sent")
	}
	if !state.MarkPlayerStatsSent(entityID, first, 1000) {
		t.Fatalf("expected MarkPlayerStatsSent to succeed")
	}

	if state.ShouldSendPlayerStats(entityID, first, false) {
		t.Fatalf("expected unchanged snapshot to be skipped")
	}

	staminaChanged := PlayerStatsNetSnapshot{Stamina: 101, Energy: 1000, StaminaMax: 1500, EnergyMax: 1000, SHP: 90, HHP: 100, MHP: 120, IsKnockedOut: false}
	if !state.ShouldSendPlayerStats(entityID, staminaChanged, false) {
		t.Fatalf("expected changed stamina snapshot to be sent")
	}

	energyChanged := PlayerStatsNetSnapshot{Stamina: 101, Energy: 999, StaminaMax: 1500, EnergyMax: 1000, SHP: 90, HHP: 100, MHP: 120, IsKnockedOut: false}
	if !state.ShouldSendPlayerStats(entityID, energyChanged, false) {
		t.Fatalf("expected changed energy snapshot to be sent")
	}

	maxChanged := PlayerStatsNetSnapshot{Stamina: 100, Energy: 1000, StaminaMax: 1600, EnergyMax: 1000, SHP: 90, HHP: 100, MHP: 120, IsKnockedOut: false}
	if !state.ShouldSendPlayerStats(entityID, maxChanged, false) {
		t.Fatalf("expected changed max snapshot to be sent")
	}

	healthChanged := PlayerStatsNetSnapshot{Stamina: 100, Energy: 1000, StaminaMax: 1500, EnergyMax: 1000, SHP: 80, HHP: 100, MHP: 120, IsKnockedOut: false}
	if !state.ShouldSendPlayerStats(entityID, healthChanged, false) {
		t.Fatalf("expected changed health snapshot to be sent")
	}

	koChanged := PlayerStatsNetSnapshot{Stamina: 100, Energy: 1000, StaminaMax: 1500, EnergyMax: 1000, SHP: 90, HHP: 100, MHP: 120, IsKnockedOut: true}
	if !state.ShouldSendPlayerStats(entityID, koChanged, false) {
		t.Fatalf("expected changed KO snapshot to be sent")
	}

	if !state.ShouldSendPlayerStats(entityID, first, true) {
		t.Fatalf("expected force=true to always send")
	}
}

func TestUpdateEntityStatsRegenSchedule(t *testing.T) {
	world := NewWorldForTesting()
	entityID := types.EntityID(99)
	handle := world.Spawn(entityID, nil)
	state := GetResource[EntityStatsUpdateState](world)
	timeState := GetResource[TimeState](world)
	timeState.Tick = 100
	regenIntervalTicks := uint64(50)
	SetResource(world, EntityStatsRuntimeConfig{
		PlayerStatsTTLms:          ResolvePlayerStatsTTLms(world),
		StaminaRegenIntervalTicks: regenIntervalTicks,
	})

	if !UpdateEntityStatsRegenSchedule(world, handle, 100, 10, 200) {
		t.Fatalf("expected regen schedule to be created")
	}
	if state.PendingRegenCount() != 1 {
		t.Fatalf("expected one pending regen, got %d", state.PendingRegenCount())
	}

	due := state.PopDueRegen(timeState.Tick+regenIntervalTicks-1, nil)
	if len(due) != 0 {
		t.Fatalf("expected no due regen before interval boundary, got %+v", due)
	}

	due = state.PopDueRegen(timeState.Tick+regenIntervalTicks, nil)
	if len(due) != 1 || due[0] != handle {
		t.Fatalf("unexpected due regen at interval boundary: %+v", due)
	}

	// Re-schedule first, then verify cancel path.
	if !UpdateEntityStatsRegenSchedule(world, handle, 50, 1, 100) {
		t.Fatalf("expected regen re-schedule to succeed")
	}
	if !UpdateEntityStatsRegenSchedule(world, handle, 100, 1, 100) {
		t.Fatalf("expected regen cancel to report state change")
	}
	if state.PendingRegenCount() != 0 {
		t.Fatalf("expected no pending regen after cancel, got %d", state.PendingRegenCount())
	}
}
