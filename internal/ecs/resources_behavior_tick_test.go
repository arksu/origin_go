package ecs

import (
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
