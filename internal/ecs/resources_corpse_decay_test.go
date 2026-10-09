package ecs

import (
	"math/rand"
	"testing"

	"origin/internal/types"
)

func TestCorpseDecayScheduleRescheduleCancelAndOrdering(t *testing.T) {
	var schedule CorpseDecaySchedule
	for _, entry := range []CorpseDecayEntry{
		{Handle: 1, EntityID: 30, DueRuntimeSeconds: 100},
		{Handle: 2, EntityID: 20, DueRuntimeSeconds: 50},
		{Handle: 3, EntityID: 10, DueRuntimeSeconds: 50},
		{Handle: 4, EntityID: 40, DueRuntimeSeconds: 70},
	} {
		if !schedule.Schedule(entry.Handle, entry.EntityID, entry.DueRuntimeSeconds) {
			t.Fatal("schedule rejected a valid entry")
		}
	}
	for i := 0; i < 1000; i++ {
		schedule.Schedule(1, 30, 80+int64(i%10))
	}
	if len(schedule.queue) != 4 || len(schedule.positions) != 4 {
		t.Fatalf("rescheduling accumulated records: %+v", schedule)
	}
	schedule.Schedule(1, 30, 40)  // Move an existing entry to the root.
	schedule.Schedule(4, 40, 150) // Move an existing entry down the heap.
	if !schedule.Cancel(4) || schedule.Cancel(4) {
		t.Fatal("cancel must remove exactly one record")
	}
	if _, ok := schedule.PopDue(39); ok {
		t.Fatal("deadline fired early")
	}
	for _, want := range []types.EntityID{30, 10, 20} {
		entry, ok := schedule.PopDue(50)
		if !ok || entry.EntityID != want {
			t.Fatalf("unexpected due entry: %+v, %v; want entity %d", entry, ok, want)
		}
	}
	if _, ok := schedule.PopDue(1000); ok || schedule.PendingCount() != 0 {
		t.Fatal("removed entries fired again")
	}
	for _, entry := range []CorpseDecayEntry{{Handle: 0, EntityID: 1}, {Handle: 1, EntityID: 0}, {Handle: 1, EntityID: 1, DueRuntimeSeconds: -1}} {
		if schedule.Schedule(entry.Handle, entry.EntityID, entry.DueRuntimeSeconds) {
			t.Fatalf("accepted invalid entry: %+v", entry)
		}
	}
}

func TestCorpseDecayScheduleMatchesDeadlineSet(t *testing.T) {
	var schedule CorpseDecaySchedule
	entries := make(map[types.Handle]CorpseDecayEntry)
	random := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		handle := types.Handle(random.Intn(64) + 1)
		if random.Intn(3) == 0 {
			schedule.Cancel(handle)
			delete(entries, handle)
		} else {
			entry := CorpseDecayEntry{Handle: handle, EntityID: types.EntityID(handle), DueRuntimeSeconds: int64(random.Intn(100))}
			schedule.Schedule(entry.Handle, entry.EntityID, entry.DueRuntimeSeconds)
			entries[handle] = entry
		}
		if len(schedule.queue) != len(entries) || len(schedule.positions) != len(entries) {
			t.Fatal("heap retained canceled or superseded records")
		}
		for index, entry := range schedule.queue {
			if schedule.positions[entry.Handle] != index || entries[entry.Handle] != entry {
				t.Fatal("heap and index diverged")
			}
			if index > 0 && schedule.less(index, (index-1)/2) {
				t.Fatal("minimum-heap order broken")
			}
		}
	}
	previousDue := int64(-1)
	previousID := types.EntityID(0)
	for schedule.PendingCount() > 0 {
		entry, ok := schedule.PopDue(100)
		if !ok || entry.DueRuntimeSeconds < previousDue || (entry.DueRuntimeSeconds == previousDue && entry.EntityID < previousID) {
			t.Fatalf("invalid drain order: %+v", entry)
		}
		previousDue, previousID = entry.DueRuntimeSeconds, entry.EntityID
		delete(entries, entry.Handle)
	}
	if len(entries) != 0 {
		t.Fatal("not all current entries were drained")
	}
}

func TestCorpseDecayScheduleDespawnCleanupAndGeneration(t *testing.T) {
	w := NewWorldForTesting()
	schedule := EnsureCorpseDecaySchedule(w)
	observers := len(w.despawnObservers)
	if EnsureCorpseDecaySchedule(w) != schedule || len(w.despawnObservers) != observers {
		t.Fatal("schedule initialization must register cleanup once")
	}
	handle := w.Spawn(1, nil)
	schedule.Schedule(handle, 1, 21600)
	if !w.Despawn(handle) || schedule.PendingCount() != 0 {
		t.Fatal("despawn retained its deadline")
	}
	replacement := w.Spawn(2, nil)
	if replacement == handle {
		t.Fatal("expected a fresh handle generation")
	}
	schedule.Schedule(replacement, 2, 22000)
	if schedule.Cancel(handle) {
		t.Fatal("stale handle canceled a replacement")
	}
	entry, ok := schedule.PopDue(22000)
	if !ok || entry.Handle != replacement || entry.EntityID != 2 {
		t.Fatalf("unexpected replacement deadline: %+v", entry)
	}
}

func TestCorpseDecayScheduleHotOperationsDoNotAllocate(t *testing.T) {
	var schedule CorpseDecaySchedule
	schedule.Schedule(1, 1, 21600)
	if allocations := testing.AllocsPerRun(1000, func() {
		schedule.Schedule(1, 1, 21601)
		schedule.Schedule(1, 1, 21600)
		_, _ = schedule.PopDue(21600)
		schedule.Schedule(1, 1, 21600)
	}); allocations != 0 {
		t.Fatalf("hot schedule operations allocate: %v", allocations)
	}
}
