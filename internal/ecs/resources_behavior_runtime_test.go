package ecs

import (
	"math/rand"
	"testing"

	"origin/internal/types"
)

func TestBehaviorRuntimeScheduleIndexedReplacementAndCancellation(t *testing.T) {
	var schedule BehaviorRuntimeSchedule
	for _, entry := range []BehaviorRuntimeEntry{
		{Handle: 1, EntityID: 30, BehaviorKey: "drying", DueRuntimeSeconds: 100},
		{Handle: 2, EntityID: 20, BehaviorKey: "drying", DueRuntimeSeconds: 50},
		{Handle: 3, EntityID: 10, BehaviorKey: "regrow", DueRuntimeSeconds: 50},
		{Handle: 3, EntityID: 10, BehaviorKey: "drying", DueRuntimeSeconds: 50},
		{Handle: 4, EntityID: 10, BehaviorKey: "drying", DueRuntimeSeconds: 50},
	} {
		if !schedule.Schedule(entry.Handle, entry.EntityID, entry.BehaviorKey, entry.DueRuntimeSeconds) {
			t.Fatal("schedule rejected valid entry")
		}
	}
	for i := 0; i < 1000; i++ {
		schedule.Schedule(1, 30, "drying", 80+int64(i%10))
	}
	if len(schedule.queue) != 5 || len(schedule.positions) != 5 {
		t.Fatal("rescheduling retained superseded heap records")
	}
	schedule.Schedule(1, 30, "drying", 40)
	schedule.Schedule(2, 20, "drying", 150)
	if !schedule.Cancel(2, "drying") || schedule.Cancel(2, "drying") || len(schedule.queue) != 4 {
		t.Fatal("cancel did not physically remove exactly one entry")
	}
	if _, due := schedule.PopDue(39); due {
		t.Fatal("deadline fired early")
	}
	for _, want := range []BehaviorRuntimeKey{{Handle: 1, BehaviorKey: "drying"}, {Handle: 3, BehaviorKey: "drying"}, {Handle: 4, BehaviorKey: "drying"}, {Handle: 3, BehaviorKey: "regrow"}} {
		entry, due := schedule.PopDue(50)
		if !due || entry.key() != want {
			t.Fatalf("unexpected due entry: %+v, due=%v; want %+v", entry, due, want)
		}
	}
	if schedule.PendingCount() != 0 || len(schedule.positions) != 0 || len(schedule.byHandle) != 0 {
		t.Fatal("drain retained scheduler entries")
	}

	schedule.Schedule(1, 1, "drying", 10)
	schedule.Schedule(1, 1, "regrow", 10)
	schedule.Schedule(2, 2, "drying", 10)
	if canceled := schedule.CancelAll(1); canceled != 2 || schedule.PendingCount() != 1 {
		t.Fatalf("CancelAll did not remove exact handle entries: canceled=%d, pending=%d", canceled, schedule.PendingCount())
	}
	if canceled := schedule.CancelAll(1); canceled != 0 {
		t.Fatal("CancelAll counted already removed entries")
	}
	entry, due := schedule.PopDue(10)
	if !due || entry.Handle != 2 {
		t.Fatalf("CancelAll interfered with another handle: %+v", entry)
	}
}

func TestBehaviorRuntimeScheduleMatchesDeadlineSet(t *testing.T) {
	var schedule BehaviorRuntimeSchedule
	entries := make(map[BehaviorRuntimeKey]BehaviorRuntimeEntry)
	random := rand.New(rand.NewSource(1))
	behaviorKeys := []string{"drying", "regrow", "other"}
	for i := 0; i < 2000; i++ {
		handle := types.Handle(random.Intn(64) + 1)
		key := BehaviorRuntimeKey{Handle: handle, BehaviorKey: behaviorKeys[random.Intn(len(behaviorKeys))]}
		switch random.Intn(4) {
		case 0:
			schedule.Cancel(key.Handle, key.BehaviorKey)
			delete(entries, key)
		case 1:
			schedule.CancelAll(handle)
			for existing := range entries {
				if existing.Handle == handle {
					delete(entries, existing)
				}
			}
		default:
			entry := BehaviorRuntimeEntry{Handle: handle, EntityID: types.EntityID(handle), BehaviorKey: key.BehaviorKey, DueRuntimeSeconds: int64(random.Intn(100))}
			schedule.Schedule(entry.Handle, entry.EntityID, entry.BehaviorKey, entry.DueRuntimeSeconds)
			entries[key] = entry
		}
		if len(schedule.queue) != len(entries) || len(schedule.positions) != len(entries) {
			t.Fatal("heap retained canceled or superseded entries")
		}
		for index, entry := range schedule.queue {
			if schedule.positions[entry.key()] != index || entries[entry.key()] != entry {
				t.Fatal("heap and position index diverged")
			}
			if index > 0 && schedule.less(index, (index-1)/2) {
				t.Fatal("minimum-heap order broken")
			}
		}
		memberships := 0
		for memberHandle, keys := range schedule.byHandle {
			for _, behaviorKey := range keys {
				if _, exists := entries[BehaviorRuntimeKey{Handle: memberHandle, BehaviorKey: behaviorKey}]; !exists {
					t.Fatal("handle index retained removed behavior")
				}
				memberships++
			}
		}
		if memberships != len(entries) {
			t.Fatal("handle index lost a pending behavior")
		}
	}
	var previous BehaviorRuntimeEntry
	first := true
	for schedule.PendingCount() > 0 {
		entry, due := schedule.PopDue(100)
		if !due || entry.DueRuntimeSeconds < previous.DueRuntimeSeconds || (!first && entry.DueRuntimeSeconds == previous.DueRuntimeSeconds && entry.EntityID < previous.EntityID) {
			t.Fatalf("invalid due ordering: %+v after %+v", entry, previous)
		}
		previous, first = entry, false
		delete(entries, entry.key())
	}
	if len(entries) != 0 {
		t.Fatal("not all current deadlines drained")
	}
}

func TestBehaviorRuntimeScheduleDespawnAndGeneration(t *testing.T) {
	w := NewWorldForTesting()
	schedule := EnsureBehaviorRuntimeSchedule(w)
	observerCount := len(w.despawnObservers)
	if EnsureBehaviorRuntimeSchedule(w) != schedule || len(w.despawnObservers) != observerCount {
		t.Fatal("schedule initialization registered duplicate cleanup")
	}
	handle := w.Spawn(1, nil)
	schedule.Schedule(handle, 1, "drying", 300)
	schedule.Schedule(handle, 1, "regrow", 500)
	if !w.Despawn(handle) || schedule.PendingCount() != 0 {
		t.Fatal("despawn retained runtime deadlines")
	}
	replacement := w.Spawn(2, nil)
	if replacement == handle {
		t.Fatal("expected new handle generation")
	}
	schedule.Schedule(replacement, 2, "drying", 300)
	if schedule.Cancel(handle, "drying") || schedule.CancelAll(handle) != 0 {
		t.Fatal("stale handle canceled replacement generation")
	}
	entry, due := schedule.PopDue(300)
	if !due || entry.Handle != replacement || entry.EntityID != 2 {
		t.Fatalf("unexpected replacement deadline: %+v", entry)
	}
}

func TestBehaviorRuntimeScheduleRejectsInvalidEntries(t *testing.T) {
	var schedule BehaviorRuntimeSchedule
	for _, entry := range []BehaviorRuntimeEntry{
		{Handle: 0, EntityID: 1, BehaviorKey: "drying"},
		{Handle: 1, EntityID: 0, BehaviorKey: "drying"},
		{Handle: 1, EntityID: 1, BehaviorKey: " "},
		{Handle: 1, EntityID: 1, BehaviorKey: "drying", DueRuntimeSeconds: -1},
	} {
		if schedule.Schedule(entry.Handle, entry.EntityID, entry.BehaviorKey, entry.DueRuntimeSeconds) {
			t.Fatalf("accepted invalid schedule: %+v", entry)
		}
	}
	if EnsureBehaviorRuntimeSchedule(nil) != nil || ScheduleBehaviorRuntime(nil, 1, 1, "drying", 1) || CancelBehaviorRuntime(nil, 1, "drying") || CancelBehaviorRuntimeByHandle(nil, 1) != 0 {
		t.Fatal("nil-world helpers must be safe")
	}
}

func TestBehaviorRuntimeScheduleReschedulingDoesNotAllocate(t *testing.T) {
	var schedule BehaviorRuntimeSchedule
	for handle := types.Handle(1); handle <= 64; handle++ {
		schedule.Schedule(handle, types.EntityID(handle), "drying", int64(handle))
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		schedule.Schedule(1, 1, "drying", 100)
		schedule.Schedule(1, 1, "drying", 1)
	}); allocations != 0 {
		t.Fatalf("rescheduling allocated: %v", allocations)
	}
}
