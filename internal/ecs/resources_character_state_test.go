package ecs

import (
	"testing"
	"time"

	"origin/internal/types"
)

func TestCharacterEntitiesPopDueIncludesExactDeadline(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_000, 0).UTC()
	entities := CharacterEntities{
		Map: make(map[types.EntityID]CharacterEntity),
	}

	entities.Add(types.EntityID(11), types.Handle(101), base)
	entities.Add(types.EntityID(22), types.Handle(202), base.Add(time.Second))

	due := entities.PopDue(base, nil)
	if len(due) != 1 || due[0] != types.EntityID(11) {
		t.Fatalf("expected entity 11 due at exact deadline, got %v", due)
	}
}

func TestCharacterEntitiesRescheduleDropsStaleEntry(t *testing.T) {
	t.Parallel()

	base := time.Unix(2_000, 0).UTC()
	entities := CharacterEntities{
		Map: make(map[types.EntityID]CharacterEntity),
	}

	entityID := types.EntityID(33)
	entities.Add(entityID, types.Handle(303), base.Add(time.Second))
	entities.UpdateSaveTime(entityID, base, base.Add(3*time.Second))

	dueAtOldDeadline := entities.PopDue(base.Add(time.Second), nil)
	if len(dueAtOldDeadline) != 0 {
		t.Fatalf("expected stale item to be ignored, got %v", dueAtOldDeadline)
	}

	dueAtNewDeadline := entities.PopDue(base.Add(3*time.Second), nil)
	if len(dueAtNewDeadline) != 1 || dueAtNewDeadline[0] != entityID {
		t.Fatalf("expected entity %d at new deadline, got %v", entityID, dueAtNewDeadline)
	}
}

func TestCharacterEntitiesRemoveCancelsPendingSchedule(t *testing.T) {
	t.Parallel()

	base := time.Unix(3_000, 0).UTC()
	entities := CharacterEntities{
		Map: make(map[types.EntityID]CharacterEntity),
	}

	entityID := types.EntityID(44)
	entities.Add(entityID, types.Handle(404), base)
	entities.Remove(entityID)

	due := entities.PopDue(base, nil)
	if len(due) != 0 {
		t.Fatalf("expected no due entries after remove, got %v", due)
	}
	if entities.PendingSaveCount() != 0 {
		t.Fatalf("expected zero pending saves, got %d", entities.PendingSaveCount())
	}
}

func TestCharacterEntitiesRescheduleSavePreservesAcceptedCaptureState(t *testing.T) {
	t.Parallel()

	base := time.Unix(4_000, 0).UTC()
	entities := CharacterEntities{Map: make(map[types.EntityID]CharacterEntity)}
	entityID := types.EntityID(55)
	entities.Add(entityID, types.Handle(505), base)
	entities.UpdateSaveTime(entityID, base, base.Add(time.Second))
	before := entities.Map[entityID]
	if due := entities.PopDue(base.Add(time.Second), nil); len(due) != 1 {
		t.Fatalf("expected due capture, got %v", due)
	}

	retryAt := base.Add(6 * time.Second)
	entities.RescheduleSave(entityID, retryAt)
	after := entities.Map[entityID]
	if after.LastSaveAt != before.LastSaveAt || after.SavesCount != before.SavesCount || after.Handle != before.Handle {
		t.Fatalf("retry changed accepted capture state: before=%+v after=%+v", before, after)
	}
	if after.NextSaveAt != retryAt || entities.PendingSaveCount() != 1 {
		t.Fatalf("retry not scheduled: entity=%+v pending=%d", after, entities.PendingSaveCount())
	}
	if due := entities.PopDue(retryAt.Add(-time.Nanosecond), nil); len(due) != 0 {
		t.Fatalf("retried before deadline: %v", due)
	}
	if due := entities.PopDue(retryAt, nil); len(due) != 1 || due[0] != entityID {
		t.Fatalf("retry lost after consumed schedule: %v", due)
	}

	entities.Remove(entityID)
	entities.RescheduleSave(entityID, retryAt.Add(time.Second))
	if entities.PendingSaveCount() != 0 || len(entities.Map) != 0 {
		t.Fatal("retry recreated a removed character")
	}
}
