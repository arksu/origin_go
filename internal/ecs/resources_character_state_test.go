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
