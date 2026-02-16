package systems

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestEntityStatsRegenSystem_RegenAndReschedule(t *testing.T) {
	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(201)
	handle := world.Spawn(entityID, nil)
	ecs.AddComponent(world, handle, components.EntityStats{
		Stamina: 900,
		Energy:  10,
	})

	timeState := ecs.GetResource[ecs.TimeState](world)
	timeState.Tick = 0
	timeState.UnixMs = 1_000

	if !ecs.UpdateEntityStatsRegenSchedule(world, handle, 900, 10, 1000) {
		t.Fatalf("expected initial regen scheduling to succeed")
	}

	system := NewEntityStatsRegenSystem()
	system.Update(world, 0)

	stats, ok := ecs.GetComponent[components.EntityStats](world, handle)
	if !ok {
		t.Fatalf("expected EntityStats component")
	}
	if stats.Stamina != 900 || stats.Energy != 10 {
		t.Fatalf("expected no regen before due tick, got stamina=%v energy=%v", stats.Stamina, stats.Energy)
	}

	timeState.Tick = 500
	timeState.UnixMs = 1_500
	system.Update(world, 0)

	stats, _ = ecs.GetComponent[components.EntityStats](world, handle)
	if stats.Stamina != 905 || stats.Energy != 9 {
		t.Fatalf("unexpected stats after regen: stamina=%v energy=%v", stats.Stamina, stats.Energy)
	}

	state := ecs.GetResource[ecs.EntityStatsUpdateState](world)
	if state.PendingPlayerPushCount() != 1 {
		t.Fatalf("expected one pending player stats push, got %d", state.PendingPlayerPushCount())
	}
	if state.PendingRegenCount() != 1 {
		t.Fatalf("expected regen to be rescheduled, got %d", state.PendingRegenCount())
	}
}

func TestEntityStatsRegenSystem_StopsWhenEnergyDepletedOrMaxReached(t *testing.T) {
	world := ecs.NewWorldForTesting()
	entityID := types.EntityID(202)
	handle := world.Spawn(entityID, nil)
	ecs.AddComponent(world, handle, components.EntityStats{
		Stamina: 999,
		Energy:  1,
	})

	timeState := ecs.GetResource[ecs.TimeState](world)
	timeState.Tick = 0
	timeState.UnixMs = 2_000

	if !ecs.UpdateEntityStatsRegenSchedule(world, handle, 999, 1, 1000) {
		t.Fatalf("expected initial regen scheduling to succeed")
	}

	system := NewEntityStatsRegenSystem()
	timeState.Tick = 500
	timeState.UnixMs = 2_500
	system.Update(world, 0)

	stats, ok := ecs.GetComponent[components.EntityStats](world, handle)
	if !ok {
		t.Fatalf("expected EntityStats component")
	}
	if stats.Stamina != 1000 || stats.Energy != 0 {
		t.Fatalf("unexpected stats after terminal regen: stamina=%v energy=%v", stats.Stamina, stats.Energy)
	}

	state := ecs.GetResource[ecs.EntityStatsUpdateState](world)
	if state.PendingRegenCount() != 0 {
		t.Fatalf("expected regen queue to stop after terminal regen, got %d", state.PendingRegenCount())
	}
}
