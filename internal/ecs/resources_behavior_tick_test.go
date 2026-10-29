package ecs

import (
	"reflect"
	"testing"

	"origin/internal/types"
)

func TestBehaviorTickSchedule_RescheduleAndDueDrain(t *testing.T) {
	schedule := &BehaviorTickSchedule{}

	if !schedule.Schedule(types.EntityID(1), "tree", 10) {
		t.Fatalf("expected initial schedule to succeed")
	}
	if !schedule.Schedule(types.EntityID(1), "tree", 20) {
		t.Fatalf("expected reschedule to succeed")
	}
	if !schedule.Schedule(types.EntityID(2), "tree", 15) {
		t.Fatalf("expected second entity schedule to succeed")
	}

	due := schedule.PopDue(12, 10, nil)
	if len(due) != 0 {
		t.Fatalf("expected no due entries at tick 12, got %d", len(due))
	}

	due = schedule.PopDue(16, 10, due[:0])
	if len(due) != 1 {
		t.Fatalf("expected one due entry at tick 16, got %d", len(due))
	}
	if due[0].EntityID != types.EntityID(2) || due[0].BehaviorKey != "tree" {
		t.Fatalf("unexpected due entry at tick 16: %+v", due[0])
	}

	due = schedule.PopDue(21, 10, due[:0])
	if len(due) != 1 {
		t.Fatalf("expected one due entry at tick 21, got %d", len(due))
	}
	if due[0].EntityID != types.EntityID(1) || due[0].BehaviorKey != "tree" {
		t.Fatalf("unexpected due entry at tick 21: %+v", due[0])
	}
}

func TestBehaviorTickSchedule_CancelAndCancelAll(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	_ = schedule.Schedule(types.EntityID(1), "tree", 10)
	_ = schedule.Schedule(types.EntityID(1), "machine", 10)
	_ = schedule.Schedule(types.EntityID(2), "tree", 10)

	if !schedule.Cancel(types.EntityID(1), "tree") {
		t.Fatalf("expected cancel to return true")
	}
	if schedule.PendingCount() != 2 {
		t.Fatalf("unexpected pending count after cancel: %d", schedule.PendingCount())
	}

	canceled := schedule.CancelAll(types.EntityID(1))
	if canceled != 1 {
		t.Fatalf("expected one cancel from CancelAll, got %d", canceled)
	}
	if schedule.PendingCount() != 1 {
		t.Fatalf("unexpected pending count after CancelAll: %d", schedule.PendingCount())
	}

	due := schedule.PopDue(10, 10, nil)
	if len(due) != 1 {
		t.Fatalf("expected one due entry, got %d", len(due))
	}
	if due[0].EntityID != types.EntityID(2) || due[0].BehaviorKey != "tree" {
		t.Fatalf("unexpected due entry: %+v", due[0])
	}
}

func TestBehaviorTickSchedule_RescheduleKeepsOnePhysicalEntry(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	for tick := uint64(1); tick <= 10000; tick++ {
		schedule.Schedule(1, "tree", tick)
	}
	if len(schedule.queue) != 1 || schedule.PendingCount() != 1 {
		t.Fatalf("rescheduling retained obsolete deadlines: queue=%d pending=%d", len(schedule.queue), schedule.PendingCount())
	}
	if due := schedule.PopDue(9999, 1, nil); len(due) != 0 {
		t.Fatalf("replaced deadline fired early: %v", due)
	}
	if due := schedule.PopDue(10000, 1, nil); len(due) != 1 || due[0].EntityID != 1 {
		t.Fatalf("latest deadline did not fire: %v", due)
	}
}

func TestBehaviorTickSchedule_MassCancellationReleasesQueueAndIndices(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	for entityID := types.EntityID(1); entityID <= 5000; entityID++ {
		for _, key := range []string{"tree", "machine"} {
			schedule.Schedule(entityID, key, 1)
			schedule.Schedule(entityID, key, 2)
		}
	}
	for entityID := types.EntityID(1); entityID <= 5000; entityID++ {
		if !schedule.Cancel(entityID, "tree") || schedule.CancelAll(entityID) != 1 {
			t.Fatalf("cancellation failed for entity %d", entityID)
		}
	}
	if len(schedule.queue) != 0 || len(schedule.latest) != 0 || len(schedule.byEntity) != 0 {
		t.Fatalf("cancellation retained scheduler state: queue=%d latest=%d entities=%d", len(schedule.queue), len(schedule.latest), len(schedule.byEntity))
	}
	if due := schedule.PopDue(2, 1, nil); len(due) != 0 {
		t.Fatalf("canceled deadline executed: %v", due)
	}
}

func TestBehaviorTickSchedule_BudgetAndOrderAfterDeadlineChanges(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	schedule.Schedule(3, "tree", 1)
	schedule.Schedule(1, "tree", 50)
	schedule.Schedule(2, "tree", 30)
	schedule.Schedule(1, "machine", 20)
	schedule.Schedule(3, "tree", 20) // Move a root later.
	schedule.Schedule(1, "tree", 20) // Move a leaf earlier.
	schedule.Schedule(2, "tree", 20)
	schedule.Schedule(4, "tree", 19)
	schedule.Cancel(4, "tree")
	want := []BehaviorTickKey{{1, "machine"}, {1, "tree"}, {2, "tree"}, {3, "tree"}}
	for i := 0; i < len(want); i += 2 {
		due := schedule.PopDue(20, 2, nil)
		if !reflect.DeepEqual(due, want[i:i+2]) {
			t.Fatalf("budget or deadline ordering changed: got %v, want %v", due, want[i:i+2])
		}
		if schedule.PendingCount() != len(want)-i-2 || len(schedule.queue) != schedule.PendingCount() {
			t.Fatalf("pop exceeded physical work budget: pending=%d queue=%d", schedule.PendingCount(), len(schedule.queue))
		}
	}
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_ExecutionCompletionReleasesOneShotRegistration(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	schedule.Schedule(1, "tree", 1)
	due := schedule.PopDueForExecution(1, 1, nil)
	if len(due) != 1 || schedule.PendingCount() != 0 || len(schedule.latest) != 1 || len(schedule.byEntity) != 1 {
		t.Fatal("due callback did not retain its reusable registration")
	}
	schedule.CompleteTick(due[0])
	assertBehaviorTickScheduleEmpty(t, schedule)
	schedule.CompleteTick(due[0]) // Completing an already removed key is harmless.
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_ExecutionCompletionPreservesRearmedDeadline(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	schedule.Schedule(1, "tree", 1)
	due := schedule.PopDueForExecution(1, 1, nil)
	key := due[0]
	schedule.Schedule(1, "tree", 3)
	schedule.Schedule(1, "tree", 2)
	schedule.CompleteTick(due[0])
	if schedule.PendingCount() != 1 || len(schedule.latest) != 1 || len(schedule.queue) != 1 {
		t.Fatal("completion lost or duplicated the rearmed deadline")
	}
	if got := schedule.PopDue(2, 1, due[:0]); len(got) != 1 || got[0] != key {
		t.Fatalf("rearmed deadline did not execute: %v", got)
	}
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_CancelDuringExecutionReleasesRegistration(t *testing.T) {
	for _, cancelAll := range []bool{false, true} {
		schedule := &BehaviorTickSchedule{}
		schedule.Schedule(1, "tree", 1)
		schedule.Schedule(1, "machine", 2)
		due := schedule.PopDueForExecution(1, 1, nil)
		if cancelAll {
			if canceled := schedule.CancelAll(1); canceled != 1 {
				t.Fatalf("CancelAll counted an in-flight callback as pending: %d", canceled)
			}
		} else {
			if schedule.Cancel(1, "tree") {
				t.Fatal("Cancel counted an in-flight callback as pending")
			}
			schedule.Cancel(1, "machine")
		}
		assertBehaviorTickScheduleEmpty(t, schedule)
		// A new schedule with the same key must survive the old completion.
		schedule.Schedule(1, "tree", 3)
		schedule.CompleteTick(due[0])
		if schedule.PendingCount() != 1 {
			t.Fatal("old callback completion removed the new deadline")
		}
	}
}

func TestBehaviorTickSchedule_DespawnReleasesQueuedAndInFlightKeys(t *testing.T) {
	w := NewWorldForTesting()
	handle := w.Spawn(1, nil)
	schedule := GetResource[BehaviorTickSchedule](w)
	schedule.Schedule(1, "tree", 1)
	schedule.Schedule(1, "machine", 2)
	due := schedule.PopDueForExecution(1, 1, nil)
	w.Despawn(handle)
	assertBehaviorTickScheduleEmpty(t, schedule)
	schedule.CompleteTick(due[0])
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func assertBehaviorTickScheduleEmpty(t *testing.T, schedule *BehaviorTickSchedule) {
	t.Helper()
	if len(schedule.queue) != 0 || len(schedule.latest) != 0 || len(schedule.byEntity) != 0 {
		t.Fatalf("scheduler retained completed work: queue=%d latest=%d entities=%d", len(schedule.queue), len(schedule.latest), len(schedule.byEntity))
	}
}
