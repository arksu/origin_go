package systems

import (
	"context"
	"math"
	"testing"

	"origin/internal/core"
	"origin/internal/types"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
)

func TestPointStopReportsActualPositionOnceAfterCollision(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "arrival", true: "blocked snap"}[blocked], func(t *testing.T) {
			var walls []components.Transform
			if blocked {
				walls = []components.Transform{{X: 130, Y: 100}}
			}
			scene := newSweepScene(t, 100, 100, walls)
			bus := eventbus.New(nil)
			t.Cleanup(func() { bus.Shutdown(context.Background()) })
			var stops []*ecs.PointMovementStoppedEvent
			bus.SubscribeSync(ecs.TopicGameplayPointMovementStopped, eventbus.PriorityHigh, func(_ context.Context, event eventbus.Event) error {
				stopped := event.(*ecs.PointMovementStoppedEvent)
				position, _ := ecs.GetComponent[components.Transform](scene.world, scene.mover)
				if position.X != stopped.X || position.Y != stopped.Y {
					t.Fatal("event preceded transform application")
				}
				stops = append(stops, stopped)
				return nil
			})
			movement := components.Movement{Mode: constt.Walk, Speed: 100}
			movement.SetTargetPoint(140, 100)
			ecs.AddComponent(scene.world, scene.mover, movement)
			move := NewMovementSystem(scene.world, &testChunkManager{scene.chunk}, zap.NewNop())
			transform := NewTransformUpdateSystem(scene.world, &testChunkManager{scene.chunk}, bus, zap.NewNop())
			move.Update(scene.world, 1)
			if len(stops) != 0 {
				t.Fatal("precollision arrival")
			}
			scene.system.Update(scene.world, 1)
			transform.Update(scene.world, 1)
			if len(stops) != 1 || stops[0].TargetX != 140 || stops[0].TargetY != 100 {
				t.Fatalf("stop events: %#v", stops)
			}
			if (stops[0].X == 140) == blocked {
				t.Fatalf("wrong actual arrival position: %#v", stops[0])
			}
			ecs.GetResource[ecs.MovedEntities](scene.world).Count = 0
			move.Update(scene.world, 1)
			scene.system.Update(scene.world, 1)
			transform.Update(scene.world, 1)
			if len(stops) != 1 {
				t.Fatal("stop event repeated")
			}
		})
	}
}

// Regression: a point target beyond impassable terrain (deep water) must stop
// the movement at the shoreline — one stop event at the collision-resolved
// position and a move batch entry with is_moving=false. Pre-fix the tile
// collision branch skipped the no-progress detection, the server stayed
// StateMoving forever, and clients walked in place at the shore.
func TestPointStopBlockedByDeepWaterBroadcastsStop(t *testing.T) {
	scene := newSweepScene(t, 100, 100, nil, func(chunk *core.Chunk) {
		paintTestTile(chunk, 110, 102, types.TileDeepWater)
	})
	bus := eventbus.New(nil)
	t.Cleanup(func() { bus.Shutdown(context.Background()) })

	var stops []*ecs.PointMovementStoppedEvent
	bus.SubscribeSync(ecs.TopicGameplayPointMovementStopped, eventbus.PriorityHigh, func(_ context.Context, event eventbus.Event) error {
		stops = append(stops, event.(*ecs.PointMovementStoppedEvent))
		return nil
	})

	var lastMove *ecs.MoveBatchEntry
	bus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		batch, ok := event.(*ecs.ObjectMoveBatchEvent)
		if !ok {
			return nil
		}
		for i := range batch.Entries {
			if batch.Entries[i].EntityID == types.EntityID(1) {
				lastMove = &batch.Entries[i]
			}
		}
		return nil
	})

	movement := components.Movement{Mode: constt.Walk, Speed: 12}
	movement.SetTargetPoint(140, 100)
	ecs.AddComponent(scene.world, scene.mover, movement)

	// The move batch skips entities with no observers; the mover watches itself.
	visState := ecs.GetResource[ecs.VisibilityState](scene.world)
	visState.Mu.Lock()
	visState.ObserversByVisibleTarget[scene.mover] = map[types.Handle]struct{}{
		scene.mover: {},
	}
	visState.Mu.Unlock()

	move := NewMovementSystem(scene.world, &testChunkManager{scene.chunk}, zap.NewNop())
	transform := NewTransformUpdateSystem(scene.world, &testChunkManager{scene.chunk}, bus, zap.NewNop())

	movedEntities := ecs.GetResource[ecs.MovedEntities](scene.world)
	stopped := false
	for tick := 0; tick < 5 && !stopped; tick++ {
		movedEntities.Count = 0
		move.Update(scene.world, 1)
		scene.system.Update(scene.world, 1)
		transform.Update(scene.world, 1)
		m, ok := ecs.GetComponent[components.Movement](scene.world, scene.mover)
		stopped = ok && m.State == constt.StateIdle
	}
	if !stopped {
		t.Fatal("movement never stopped against deep water")
	}

	// Shutdown drains the async queue inline, making batch assertions race-free.
	if err := bus.Shutdown(context.Background()); err != nil {
		t.Fatalf("event bus shutdown: %v", err)
	}

	if len(stops) != 1 {
		t.Fatalf("expected exactly one stop event, got %d", len(stops))
	}
	if math.Abs(stops[0].X-103) > 0.05 || math.Abs(stops[0].Y-100) > 0.01 {
		t.Fatalf("expected stop at the water line (x≈103), got (%.3f, %.3f)", stops[0].X, stops[0].Y)
	}
	if stops[0].TargetX != 140 || stops[0].TargetY != 100 {
		t.Fatalf("stop event lost the target: %#v", stops[0])
	}

	if lastMove == nil {
		t.Fatal("no move batch entry for the mover")
	}
	if lastMove.IsMoving {
		t.Fatalf("expected is_moving=false in the jam tick batch, got %+v", lastMove)
	}
	if math.Abs(float64(lastMove.X-103)) > 1 || math.Abs(float64(lastMove.Y-100)) > 1 {
		t.Fatalf("expected stop batch at the water line, got (%d, %d)", lastMove.X, lastMove.Y)
	}
	if lastMove.TargetX != nil || lastMove.TargetY != nil {
		t.Fatalf("stopped movement must not carry a target, got (%v, %v)", lastMove.TargetX, lastMove.TargetY)
	}
	if lastMove.VelocityX != 0 || lastMove.VelocityY != 0 {
		t.Fatalf("stopped movement must carry zero velocity, got (%d, %d)", lastMove.VelocityX, lastMove.VelocityY)
	}
}
