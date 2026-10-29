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

func TestWakeBehaviorTick_ValidatesAndNormalizesKey(t *testing.T) {
	w := NewWorldForTesting()
	if WakeBehaviorTick(nil, 1, "npc", 1) || WakeBehaviorTick(w, 0, "npc", 1) || WakeBehaviorTick(w, 1, " \t", 1) {
		t.Fatal("wake accepted an invalid world or key")
	}
	if !WakeBehaviorTick(w, 1, " npc ", 4) || !WakeBehaviorTick(w, 1, "npc", 3) {
		t.Fatal("wake rejected a valid key")
	}
	schedule := GetResource[BehaviorTickSchedule](w)
	if schedule.PendingCount() != 1 {
		t.Fatalf("normalized key retained duplicate entries: %d", schedule.PendingCount())
	}
	if got := schedule.PopDue(3, 1, nil); len(got) != 1 || got[0] != (BehaviorTickKey{1, "npc"}) {
		t.Fatalf("normalized wake did not execute: %v", got)
	}
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_WakeAdvancesDeadline(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	schedule.Schedule(1, "npc", 20)
	schedule.Schedule(2, "npc", 10)
	schedule.Wake(1, "npc", 5)
	if schedule.PendingCount() != 2 {
		t.Fatalf("wake created duplicate work: %d", schedule.PendingCount())
	}
	if got := schedule.PopDue(4, 1, nil); len(got) != 0 {
		t.Fatalf("wake executed before its deadline: %v", got)
	}
	if got := schedule.PopDue(5, 1, nil); len(got) != 1 || got[0].EntityID != 1 {
		t.Fatalf("earlier wake did not move ahead of the heap root: %v", got)
	}
	if got := schedule.PopDue(10, 1, nil); len(got) != 1 || got[0].EntityID != 2 {
		t.Fatalf("wake changed another key's deadline: %v", got)
	}
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_WakePreservesOverdueOrder(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	schedule.Schedule(9, "npc", 2)
	schedule.Schedule(1, "npc", 3)
	for tick := uint64(2); tick < 100; tick++ {
		schedule.Wake(9, "npc", tick)
	}
	if schedule.PendingCount() != 2 {
		t.Fatalf("repeated wakes retained duplicate entries: %d", schedule.PendingCount())
	}
	if got := schedule.PopDue(100, 1, nil); len(got) != 1 || got[0].EntityID != 9 {
		t.Fatalf("repeated wakes postponed overdue work behind another entity: %v", got)
	}
	if got := schedule.PopDue(100, 1, nil); len(got) != 1 || got[0].EntityID != 1 {
		t.Fatalf("budgeted wake drain lost remaining work: %v", got)
	}
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_WakePreservesPrePoppedCallback(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	schedule.Schedule(1, "npc", 1)
	schedule.Schedule(2, "npc", 1)
	batch := schedule.PopDueForExecution(10, 2, nil)
	if len(batch) != 2 || !schedule.ShouldExecuteTick(batch[0]) {
		t.Fatalf("expected two ready callbacks: %v", batch)
	}
	// The first callback wakes the second after the entire batch was popped.
	for _, dueTick := range []uint64{0, 10, 20} {
		schedule.Wake(2, "npc", dueTick)
	}
	schedule.CompleteTick(batch[0])
	if schedule.PendingCount() != 0 || !schedule.ShouldExecuteTick(batch[1]) {
		t.Fatal("wake requeued or suppressed an already ready callback")
	}
	if schedule.ShouldExecuteTick(batch[1]) {
		t.Fatal("callback could be claimed twice")
	}
	schedule.CompleteTick(batch[1])
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_WakeDuringCallbackRearmsOnce(t *testing.T) {
	schedule := &BehaviorTickSchedule{}
	schedule.Wake(1, "npc", 5)
	batch := make([]BehaviorTickKey, 0, 2)
	for execution := 0; execution < 3; execution++ {
		batch = schedule.PopDueForExecution(5, 2, batch[:0])
		if len(batch) != 1 || !schedule.ShouldExecuteTick(batch[0]) {
			t.Fatalf("wake lost or duplicated callback %d: %v", execution, batch)
		}
		schedule.Wake(1, "npc", 8)
		schedule.Wake(1, "npc", 5)
		schedule.Wake(1, "npc", 6)
		schedule.CompleteTick(batch[0])
		if schedule.PendingCount() != 1 || len(schedule.latest) != 1 {
			t.Fatal("completion lost or duplicated a callback wake")
		}
	}
	schedule.Cancel(1, "npc")
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func TestBehaviorTickSchedule_WakeCancellationAndRecreation(t *testing.T) {
	for _, execute := range []bool{false, true} {
		schedule := &BehaviorTickSchedule{}
		schedule.Wake(1, "npc", 1)
		batch := schedule.PopDueForExecution(1, 1, nil)
		if execute && !schedule.ShouldExecuteTick(batch[0]) {
			t.Fatal("callback could not start")
		}
		if schedule.Cancel(1, "npc") {
			t.Fatal("cancel counted a popped callback as queued work")
		}
		if schedule.ShouldExecuteTick(batch[0]) {
			t.Fatal("canceled callback remained ready")
		}
		assertBehaviorTickScheduleEmpty(t, schedule)
		schedule.Wake(1, "npc", 3)
		schedule.CompleteTick(batch[0])
		if schedule.PendingCount() != 1 {
			t.Fatal("old callback completion removed a newly woken key")
		}
		if !schedule.Cancel(1, "npc") {
			t.Fatal("cancel failed to remove the new wake")
		}
		assertBehaviorTickScheduleEmpty(t, schedule)
	}
}

func TestBehaviorTickSchedule_RecurringWakeDoesNotAllocate(t *testing.T) {
	const population = 200
	schedule := &BehaviorTickSchedule{}
	for entityID := types.EntityID(1); entityID <= population; entityID++ {
		schedule.Wake(entityID, "npc", 0)
	}
	batch := make([]BehaviorTickKey, 0, population)
	var tick uint64
	calls := 0
	allocs := testing.AllocsPerRun(1000, func() {
		batch = schedule.PopDueForExecution(tick, population, batch[:0])
		for _, key := range batch {
			if schedule.ShouldExecuteTick(key) {
				calls++
				schedule.Wake(key.EntityID, key.BehaviorKey, tick+1)
				schedule.Wake(key.EntityID, key.BehaviorKey, tick+2)
			}
			schedule.CompleteTick(key)
		}
		tick++
	})
	if allocs != 0 {
		t.Fatalf("recurring wakes allocated after warmup: %g allocs/run", allocs)
	}
	if calls != 1001*population || schedule.PendingCount() != population {
		t.Fatalf("measurement did not retain recurring work: calls=%d pending=%d", calls, schedule.PendingCount())
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
	schedule.Wake(1, "tree", 1)
	schedule.Wake(1, "machine", 2)
	schedule.Wake(1, "npc", 1)
	due := schedule.PopDueForExecution(1, 2, nil)
	if len(due) != 2 || !schedule.ShouldExecuteTick(due[0]) {
		t.Fatalf("expected ready and executing callbacks: %v", due)
	}
	w.Despawn(handle)
	assertBehaviorTickScheduleEmpty(t, schedule)
	for _, key := range due {
		if schedule.ShouldExecuteTick(key) {
			t.Fatal("despawned entity's callback remained ready")
		}
		schedule.CompleteTick(key)
	}
	assertBehaviorTickScheduleEmpty(t, schedule)
}

func assertBehaviorTickScheduleEmpty(t *testing.T, schedule *BehaviorTickSchedule) {
	t.Helper()
	if len(schedule.queue) != 0 || len(schedule.latest) != 0 || len(schedule.byEntity) != 0 {
		t.Fatalf("scheduler retained completed work: queue=%d latest=%d entities=%d", len(schedule.queue), len(schedule.latest), len(schedule.byEntity))
	}
}
