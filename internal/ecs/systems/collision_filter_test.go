package systems

import (
	"math"
	"testing"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestCollisionSystem_ObjectFilterPreservesOtherObstacles(t *testing.T) {
	for _, test := range []struct {
		name      string
		ownerID   types.EntityID
		blockedBy types.EntityID
		finalX    float64
	}{
		{name: "owner", ownerID: 1, blockedBy: 3, finalX: 122},
		{name: "other actor", ownerID: 99, blockedBy: 2, finalX: 102},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene := newSweepScene(t, 100, 100, []components.Transform{{X: 112, Y: 100}, {X: 132, Y: 100}})
			ignored := scene.world.GetHandleByEntityID(2)
			scene.system.SetObjectCollisionFilter(func(actorID types.EntityID, target types.Handle) bool {
				return actorID == test.ownerID && target == ignored
			})

			result := scene.runTick(t, 40, 0)
			if !result.HasCollision || result.CollidedWith != test.blockedBy || math.Abs(result.FinalX-test.finalX) > .05 {
				t.Fatalf("expected obstacle %d at x≈%.1f to stop actor, got %+v", test.blockedBy, test.finalX, result)
			}
		})
	}
}

func TestCollisionSystem_ObjectFilterAllowsInitialOverlap(t *testing.T) {
	for _, dy := range []float64{0, 12} {
		scene := newSweepScene(t, 100, 100, []components.Transform{{X: 106, Y: 100}})
		ignored := scene.world.GetHandleByEntityID(2)
		scene.system.SetObjectCollisionFilter(func(actorID types.EntityID, target types.Handle) bool {
			return actorID == 1 && target == ignored
		})

		result := scene.runTick(t, 12, dy)
		if result.HasCollision || math.Abs(result.FinalX-112) > .01 || math.Abs(result.FinalY-(100+dy)) > .01 {
			t.Fatalf("initial overlap blocked movement (12, %.1f): %+v", dy, result)
		}
	}
}

func TestCollisionSystem_ObjectFilterAppliesOnceAcrossSlide(t *testing.T) {
	scene := newSweepScene(t, 101, 110, []components.Transform{{X: 111, Y: 100}})
	scene.addStaticCandidate(3, 101, 123, 5, 5)
	ignored := scene.world.GetHandleByEntityID(3)
	calls := make(map[types.Handle]int)
	scene.system.SetObjectCollisionFilter(func(actorID types.EntityID, target types.Handle) bool {
		if actorID != 1 {
			t.Fatalf("expected mover entity ID 1, got %d", actorID)
		}
		calls[target]++
		return target == ignored
	})

	result := scene.runTick(t, 6.2, 1.5)
	if !result.HasCollision || result.CollidedWith != 2 || result.FinalX > 101.01 || result.FinalY <= 113.1 {
		t.Fatalf("expected slide through ignored object while retaining wall collision: %+v", result)
	}
	if len(calls) != 2 || calls[scene.world.GetHandleByEntityID(2)] != 1 || calls[ignored] != 1 {
		t.Fatalf("expected one filter call per candidate for the entire slide, got %v", calls)
	}
}

func TestCollisionSystem_ObjectFilterRunsAfterCandidateRejections(t *testing.T) {
	scene := newSweepScene(t, 100, 100, nil)
	scene.addStaticCandidate(2, 112, 100, 5, 5)
	scene.addStaticCandidate(3, 111, 100, 5, 5)
	scene.addStaticCandidate(4, 111, 100, 5, 5)
	scene.addStaticCandidate(5, 134, 100, 5, 5)
	ecs.WithComponent(scene.world, scene.world.GetHandleByEntityID(3), func(c *components.Collider) {
		c.Layer = 0
		c.Mask = 0
	})
	ecs.WithComponent(scene.world, scene.world.GetHandleByEntityID(4), func(c *components.Collider) {
		c.Phantom = &components.PhantomCollider{WorldX: 111, WorldY: 100, HalfWidth: 5, HalfHeight: 5}
	})

	var called []types.Handle
	scene.system.SetObjectCollisionFilter(func(actorID types.EntityID, target types.Handle) bool {
		if actorID != 1 {
			t.Fatalf("expected mover entity ID 1, got %d", actorID)
		}
		called = append(called, target)
		return true
	})

	result := scene.runTick(t, 12, 0)
	if result.HasCollision || math.Abs(result.FinalX-112) > .01 {
		t.Fatalf("expected unobstructed move, got %+v", result)
	}
	if len(called) != 1 || called[0] != scene.world.GetHandleByEntityID(2) {
		t.Fatalf("masked, phantom or out-of-path candidate reached filter: %v", called)
	}
}

func TestCollisionSystem_ObjectFilterPreservesTerrainAndBuildPhantom(t *testing.T) {
	t.Run("terrain", func(t *testing.T) {
		scene := newSweepScene(t, 100, 100, []components.Transform{{X: 106, Y: 100}}, func(chunk *core.Chunk) {
			paintTestTile(chunk, 110, 102, types.TileDeepWater)
		})
		scene.system.SetObjectCollisionFilter(func(types.EntityID, types.Handle) bool { return true })

		result := scene.runTick(t, 12, 0)
		if !result.HasCollision || result.CollidedWith != 0 || math.Abs(result.FinalX-103) > .05 {
			t.Fatalf("object filter bypassed terrain collision: %+v", result)
		}
	})

	t.Run("build phantom", func(t *testing.T) {
		scene := newSweepScene(t, 100, 100, []components.Transform{{X: 106, Y: 100}})
		ecs.WithComponent(scene.world, scene.mover, func(c *components.Collider) {
			c.Phantom = &components.PhantomCollider{WorldX: 112, WorldY: 100, HalfWidth: 5, HalfHeight: 60}
		})
		scene.system.SetObjectCollisionFilter(func(types.EntityID, types.Handle) bool { return true })

		result := scene.runTick(t, 12, 0)
		if !result.HasCollision || !result.IsPhantom || math.Abs(result.FinalX-102) > .05 {
			t.Fatalf("object filter bypassed the mover's build phantom: %+v", result)
		}
	})
}

func TestCollisionSystem_ObjectFilterDoesNotAllocate(t *testing.T) {
	scene := newSweepScene(t, 100, 100, []components.Transform{{X: 112, Y: 100}, {X: 132, Y: 100}})
	transform, _ := ecs.GetComponent[components.Transform](scene.world, scene.mover)
	collider, _ := ecs.GetComponent[components.Collider](scene.world, scene.mover)
	ignored := scene.world.GetHandleByEntityID(2)
	calls := 0
	scene.system.SetObjectCollisionFilter(func(actorID types.EntityID, target types.Handle) bool {
		calls++
		return actorID == 1 && target == ignored
	})

	allocs := testing.AllocsPerRun(1000, func() {
		result := scene.system.sweepCollision(scene.world, scene.mover, transform, collider, 40, 0, scene.chunk)
		if !result.HasCollision || result.CollidedWith != 3 {
			t.Fatalf("allocation workload must ignore one object and stop at another: %+v", result)
		}
	})
	if calls == 0 || allocs != 0 {
		t.Fatalf("expected active filter without allocations, got calls=%d allocs/sweep=%.2f", calls, allocs)
	}
}
