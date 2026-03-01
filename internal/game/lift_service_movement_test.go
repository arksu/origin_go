package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

func TestLiftService_ReconcileMovementModeForCarry_DowngradesRunToWalk(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(9101)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 20})
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.Run,
			State: constt.StateIdle,
			Speed: 10,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1000,
		})
		ecs.AddComponent(w, h, components.LiftCarryState{
			ObjectEntityID: types.EntityID(9901),
		})
	})

	service := NewLiftService(world, nil, nil, nil, zap.NewNop())
	service.reconcileMovementModeForCarry(world, playerHandle)

	movement, ok := ecs.GetComponent[components.Movement](world, playerHandle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Walk {
		t.Fatalf("expected carry mode to downgrade run to walk, got %v", movement.Mode)
	}
	updateState := ecs.GetResource[ecs.EntityStatsUpdateState](world)
	pendingIDs := updateState.PopDueMovementModePush(nil)
	if len(pendingIDs) != 1 || pendingIDs[0] != playerID {
		t.Fatalf("expected movement mode dirty enqueue for player %d, got %v", playerID, pendingIDs)
	}
}

func TestLiftService_ReconcileMovementModeForCarry_OverstuffedForcesCrawl(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(9102)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 20})
		ecs.AddComponent(w, h, components.Movement{
			Mode:  constt.FastRun,
			State: constt.StateIdle,
			Speed: 10,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1001,
		})
		ecs.AddComponent(w, h, components.LiftCarryState{
			ObjectEntityID: types.EntityID(9902),
		})
	})

	service := NewLiftService(world, nil, nil, nil, zap.NewNop())
	service.reconcileMovementModeForCarry(world, playerHandle)

	movement, ok := ecs.GetComponent[components.Movement](world, playerHandle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Crawl {
		t.Fatalf("expected carry overstuffed to force crawl, got %v", movement.Mode)
	}
}

func TestLiftService_ReconcileMovementModeForCarry_NoMoveStopsMovement(t *testing.T) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(9103)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 30, Y: 40})
		ecs.AddComponent(w, h, components.Movement{
			Mode:       constt.Run,
			State:      constt.StateMoving,
			Speed:      10,
			TargetType: constt.TargetPoint,
			TargetX:    80,
			TargetY:    40,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 4,
			Energy:  1000,
		})
		ecs.AddComponent(w, h, components.LiftCarryState{
			ObjectEntityID: types.EntityID(9903),
		})
	})

	service := NewLiftService(world, nil, nil, nil, zap.NewNop())
	service.reconcileMovementModeForCarry(world, playerHandle)

	movement, ok := ecs.GetComponent[components.Movement](world, playerHandle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Crawl {
		t.Fatalf("expected carry no-move threshold to force crawl, got %v", movement.Mode)
	}
	if movement.State != constt.StateIdle || movement.TargetType != constt.TargetNone {
		t.Fatalf("expected movement to be stopped, got state=%v targetType=%v", movement.State, movement.TargetType)
	}
	moved := ecs.GetResource[ecs.MovedEntities](world)
	if moved.Count != 1 {
		t.Fatalf("expected one forced movement update, got %d", moved.Count)
	}
}
