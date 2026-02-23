package systems

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type testBuildPlacementFinalizer struct {
	cancelCalls   int
	finalizeCalls int
	lastPlayerID  types.EntityID
	lastHandle    types.Handle
}

func (f *testBuildPlacementFinalizer) FinalizePendingBuildPlacement(_ *ecs.World, playerID types.EntityID, playerHandle types.Handle, _ components.PendingBuildPlacement) {
	f.finalizeCalls++
	f.lastPlayerID = playerID
	f.lastHandle = playerHandle
}

func (f *testBuildPlacementFinalizer) CancelPendingBuildPlacement(_ *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	f.cancelCalls++
	f.lastPlayerID = playerID
	f.lastHandle = playerHandle
}

func TestBuildPlacementSystem_TTLCancelsWithoutCollisionResult(t *testing.T) {
	world := ecs.NewWorldForTesting()
	ecs.SetResource(world, ecs.TimeState{UnixMs: 1000})

	playerID := types.EntityID(101)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.PendingBuildPlacement{
			BuildKey:       "campfire",
			TargetX:        10,
			TargetY:        20,
			ExpireAtUnixMs: 999,
		})
		// Intentionally no CollisionResult component: TTL cleanup must still work.
	})
	if playerHandle == types.InvalidHandle {
		t.Fatal("failed to spawn player")
	}

	finalizer := &testBuildPlacementFinalizer{}
	system := NewBuildPlacementSystem(world, finalizer, nil)
	system.Update(world, 0.1)

	if finalizer.cancelCalls != 1 {
		t.Fatalf("expected 1 cancel call, got %d", finalizer.cancelCalls)
	}
	if finalizer.finalizeCalls != 0 {
		t.Fatalf("expected 0 finalize calls, got %d", finalizer.finalizeCalls)
	}
	if finalizer.lastPlayerID != playerID {
		t.Fatalf("expected playerID %d, got %d", playerID, finalizer.lastPlayerID)
	}
}

func TestBuildPlacementSystem_FinalizesOnPhantomCollision(t *testing.T) {
	world := ecs.NewWorldForTesting()
	ecs.SetResource(world, ecs.TimeState{UnixMs: 1000})

	playerID := types.EntityID(202)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.PendingBuildPlacement{
			BuildKey:       "campfire",
			TargetX:        30,
			TargetY:        40,
			ExpireAtUnixMs: 2000,
		})
		ecs.AddComponent(w, h, components.CollisionResult{
			IsPhantom: true,
		})
	})
	if playerHandle == types.InvalidHandle {
		t.Fatal("failed to spawn player")
	}

	finalizer := &testBuildPlacementFinalizer{}
	system := NewBuildPlacementSystem(world, finalizer, nil)
	system.Update(world, 0.1)

	if finalizer.finalizeCalls != 1 {
		t.Fatalf("expected 1 finalize call, got %d", finalizer.finalizeCalls)
	}
	if finalizer.cancelCalls != 0 {
		t.Fatalf("expected 0 cancel calls, got %d", finalizer.cancelCalls)
	}
	if finalizer.lastPlayerID != playerID {
		t.Fatalf("expected playerID %d, got %d", playerID, finalizer.lastPlayerID)
	}
}
