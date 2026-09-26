package systems

import (
	"context"
	"testing"

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
