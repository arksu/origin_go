package systems

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type testLiftPlacementFinalizer struct {
	finalizeCalls int
	cancelCalls   int
}

func (f *testLiftPlacementFinalizer) FinalizePendingLiftTransition(_ *ecs.World, _ types.EntityID, _ types.Handle, _ components.PendingLiftTransition) {
	f.finalizeCalls++
}

func (f *testLiftPlacementFinalizer) CancelPendingLiftTransition(_ *ecs.World, _ types.EntityID, _ types.Handle) {
	f.cancelCalls++
}

func TestLiftPlacementSystem_FinalizesNoColliderPickupAtArrivalDistance(t *testing.T) {
	world := ecs.NewWorldForTesting()
	ecs.SetResource(world, ecs.TimeState{UnixMs: 1000})

	playerID := types.EntityID(301)
	world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: constt.StopDistance})
		ecs.AddComponent(w, h, components.PendingLiftTransition{
			Mode:           components.LiftTransitionModePickupNoCollider,
			ExpireAtUnixMs: 2000,
		})
	})

	finalizer := &testLiftPlacementFinalizer{}
	NewLiftPlacementSystem(world, finalizer, nil).Update(world, 0.1)

	if finalizer.finalizeCalls != 1 {
		t.Fatalf("expected one nearby pickup finalization, got %d", finalizer.finalizeCalls)
	}
	if finalizer.cancelCalls != 0 {
		t.Fatalf("expected no cancellation, got %d", finalizer.cancelCalls)
	}
}

func TestLiftPlacementSystem_DoesNotFinalizePutDownAtArrivalDistance(t *testing.T) {
	world := ecs.NewWorldForTesting()
	ecs.SetResource(world, ecs.TimeState{UnixMs: 1000})

	playerID := types.EntityID(302)
	world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 200})
		ecs.AddComponent(w, h, components.PendingLiftTransition{
			Mode:           components.LiftTransitionModePutDown,
			TargetX:        100,
			TargetY:        200,
			ExpireAtUnixMs: 2000,
		})
	})

	finalizer := &testLiftPlacementFinalizer{}
	NewLiftPlacementSystem(world, finalizer, nil).Update(world, 0.1)

	if finalizer.finalizeCalls != 0 {
		t.Fatalf("expected no put-down finalization without phantom collision, got %d", finalizer.finalizeCalls)
	}
}
