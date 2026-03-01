package systems

import (
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

func TestMovementSystem_OverstuffedEnergyForcesCrawl(t *testing.T) {
	world := ecs.NewWorldForTesting()
	handle := world.Spawn(types.EntityID(7001), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{
			Mode:       constt.Run,
			State:      constt.StateMoving,
			Speed:      10,
			TargetType: constt.TargetPoint,
			TargetX:    100,
			TargetY:    0,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1001,
		})
	})

	system := NewMovementSystem(world, nil, zap.NewNop())
	system.Update(world, 1.0)

	movement, ok := ecs.GetComponent[components.Movement](world, handle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Crawl {
		t.Fatalf("expected forced crawl mode for overstuffed entity, got %v", movement.Mode)
	}
	if math.Abs(movement.VelocityX-5.0) > 1e-9 {
		t.Fatalf("expected crawl velocity 5.0, got %v", movement.VelocityX)
	}
}

func TestMovementSystem_EnergyAtThresholdKeepsAllowedMode(t *testing.T) {
	world := ecs.NewWorldForTesting()
	handle := world.Spawn(types.EntityID(7002), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{
			Mode:       constt.Run,
			State:      constt.StateMoving,
			Speed:      10,
			TargetType: constt.TargetPoint,
			TargetX:    100,
			TargetY:    0,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1000,
		})
	})

	system := NewMovementSystem(world, nil, zap.NewNop())
	system.Update(world, 1.0)

	movement, ok := ecs.GetComponent[components.Movement](world, handle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Run {
		t.Fatalf("expected run mode at threshold energy, got %v", movement.Mode)
	}
	if math.Abs(movement.VelocityX-15.0) > 1e-9 {
		t.Fatalf("expected run velocity 15.0, got %v", movement.VelocityX)
	}
}

func TestMovementSystem_CarryingRunDowngradesToWalkSpeed(t *testing.T) {
	world := ecs.NewWorldForTesting()
	handle := world.Spawn(types.EntityID(7003), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{
			Mode:       constt.Run,
			State:      constt.StateMoving,
			Speed:      10,
			TargetType: constt.TargetPoint,
			TargetX:    100,
			TargetY:    0,
		})
		ecs.AddComponent(w, h, components.LiftCarryState{
			ObjectEntityID: types.EntityID(9001),
			ObjectHandle:   types.InvalidHandle,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1000,
		})
	})

	system := NewMovementSystem(world, nil, zap.NewNop())
	system.Update(world, 1.0)

	movement, ok := ecs.GetComponent[components.Movement](world, handle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Walk {
		t.Fatalf("expected carry mode to downgrade run to walk, got %v", movement.Mode)
	}
	if math.Abs(movement.VelocityX-10.0) > 1e-9 {
		t.Fatalf("expected walk velocity 10.0, got %v", movement.VelocityX)
	}
}

func TestMovementSystem_CarryingOverstuffedStillForcesCrawl(t *testing.T) {
	world := ecs.NewWorldForTesting()
	handle := world.Spawn(types.EntityID(7004), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{
			Mode:       constt.FastRun,
			State:      constt.StateMoving,
			Speed:      10,
			TargetType: constt.TargetPoint,
			TargetX:    100,
			TargetY:    0,
		})
		ecs.AddComponent(w, h, components.LiftCarryState{
			ObjectEntityID: types.EntityID(9002),
			ObjectHandle:   types.InvalidHandle,
		})
		ecs.AddComponent(w, h, components.EntityStats{
			Stamina: 500,
			Energy:  1001,
		})
	})

	system := NewMovementSystem(world, nil, zap.NewNop())
	system.Update(world, 1.0)

	movement, ok := ecs.GetComponent[components.Movement](world, handle)
	if !ok {
		t.Fatalf("expected movement component")
	}
	if movement.Mode != constt.Crawl {
		t.Fatalf("expected overstuffed carry mode to force crawl, got %v", movement.Mode)
	}
	if math.Abs(movement.VelocityX-5.0) > 1e-9 {
		t.Fatalf("expected crawl velocity 5.0, got %v", movement.VelocityX)
	}
}
