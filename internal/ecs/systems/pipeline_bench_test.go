package systems

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

// pipelineMover holds the reset state for one bench mover: every iteration
// restarts it from the same spot with a fresh target and stamina, so the
// pipeline always processes real ~10-unit sweeps inside the loaded chunk.
type pipelineMover struct {
	handle   types.Handle
	startX   float64
	startY   float64
	movement components.Movement
	stats    components.EntityStats
}

// BenchmarkMovementPipeline measures the per-tick cost of the movement hot
// path (MovementSystem -> CollisionSystem -> TransformUpdateSystem) for 200
// concurrently moving entities, including the per-moved-entity component
// access overhead the systems pay.
func BenchmarkMovementPipeline(b *testing.B) {
	runPipelineBench(b, 0, false, false)
}

// BenchmarkMovementPipelineDense adds static pillars inside every mover's
// swept corridor so the collision candidate loop, hit handling and slide
// iterations run each tick, as they would around built structures.
func BenchmarkMovementPipelineDense(b *testing.B) {
	runPipelineBench(b, 3, false, false)
}

func BenchmarkDirectionalMovementPipeline(b *testing.B) {
	b.Run("active", func(b *testing.B) { runPipelineBench(b, 0, true, false) })
	b.Run("sliding", func(b *testing.B) { runPipelineBench(b, 3, true, false) })
	b.Run("blocked", func(b *testing.B) { runPipelineBench(b, 1, true, true) })
}

func BenchmarkClickMovementPipelineBlocked(b *testing.B) { runPipelineBench(b, 1, false, true) }

func BenchmarkMovementPipelineColliderIndex(b *testing.B) {
	for _, dense := range []bool{false, true} {
		for _, indexed := range []bool{false, true} {
			b.Run(fmt.Sprintf("dense=%t/indexed=%t", dense, indexed), func(b *testing.B) {
				pillars := 0
				if dense {
					pillars = 3
				}
				runPipelineBench(b, pillars, false, false, indexed)
			})
		}
	}
}

func runPipelineBench(b *testing.B, pillarsPerMover int, directional, blocked bool, colliderIndexed ...bool) {
	const moverCount = 200
	const targetX = 10000 // far beyond one tick's step; movers reset each iteration

	chunk := newTestChunk(types.ChunkCoord{X: 0, Y: 0})
	chunk.RestoreTiles(chunk.Tiles, 0, 0)
	cm := &testChunkManager{chunk: chunk}
	world := ecs.NewWorldForTesting()
	var colliderSpatial *core.WorldColliderSpatial
	if len(colliderIndexed) > 0 && colliderIndexed[0] {
		colliderSpatial = core.AttachColliderSpatial(world)
	}

	movers := make([]pipelineMover, 0, moverCount)
	observer := types.Handle(0)
	for i := 0; i < moverCount; i++ {
		startX := 100 + float64(i%20)*40
		startY := 100 + float64(i/20)*40

		var movement components.Movement
		movement.Mode = constt.Walk
		movement.State = constt.StateMoving
		movement.Speed = 100
		targetY := startY
		if pillarsPerMover > 0 && !blocked {
			// An oblique approach leaves movement along the pillar's face
			// after contact, exercising the subsequent slide sweep.
			targetY += (targetX - startX) / 2
		}
		movement.SetTargetPoint(int(targetX), int(targetY))
		if directional {
			dx, dy := float64(targetX)-startX, targetY-startY
			distance := math.Hypot(dx, dy)
			movement.SetDirection(dx/distance, dy/distance, 1, time.Time{}.Add(time.Hour))
			if blocked {
				movement.State = constt.StateIdle
				movement.Direction.Blocked = true
				movement.Direction.BlockedMode = movement.Mode
				movement.Direction.UpdatePending = false
			}
		}

		handle := world.Spawn(types.EntityID(i+1), func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Transform{X: startX, Y: startY})
			ecs.AddComponent(w, h, components.Collider{
				HalfWidth:  5,
				HalfHeight: 5,
				Layer:      constt.PlayerLayer,
				Mask:       constt.PlayerMask,
			})
			ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: 0, CurrentChunkY: 0})
			ecs.AddComponent(w, h, components.CollisionResult{})
			ecs.AddComponent(w, h, components.EntityStats{Stamina: 100, Energy: 100})
			ecs.AddComponent(w, h, movement)
		})
		chunk.Spatial().AddDynamic(handle, int(startX), int(startY))
		if i == 0 {
			observer = handle
		}
		movers = append(movers, pipelineMover{
			handle:   handle,
			startX:   startX,
			startY:   startY,
			movement: movement,
			stats:    components.EntityStats{Stamina: 100, Energy: 100},
		})
	}

	// Place static pillars across each mover's swept corridor so collisions
	// and slides happen every tick. The reset restarts movers from the same
	// spot, giving a stable steady state.
	for i := range movers {
		m := &movers[i]
		for k := 0; k < pillarsPerMover; k++ {
			pillarX := m.startX + 10 + float64(k)*3
			pillarY := m.startY - 6
			if k%2 == 1 {
				pillarY = m.startY + 6
			}
			if blocked {
				pillarX = m.startX + 8
				pillarY = m.startY
			}
			pillarID := types.EntityID(100000 + i*pillarsPerMover + k)
			pillar := world.Spawn(pillarID, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{X: pillarX, Y: pillarY})
				ecs.AddComponent(w, h, components.Collider{
					HalfWidth:  3,
					HalfHeight: 3,
					Layer:      constt.PlayerLayer,
					Mask:       constt.PlayerMask,
				})
			})
			chunk.Spatial().AddStatic(pillar, int(pillarX), int(pillarY))
		}
	}

	// Make every mover visible to one observer so the movement-batch block in
	// TransformUpdateSystem runs; in production observers are always nearby.
	visState := ecs.GetResource[ecs.VisibilityState](world)
	observerSet := map[types.Handle]struct{}{observer: {}}
	for _, m := range movers {
		visState.ObserversByVisibleTarget[m.handle] = observerSet
	}

	movementSystem := NewMovementSystem(world, cm, zap.NewNop())
	collisionSystem := NewCollisionSystem(world, cm, zap.NewNop(), 0, constt.ChunkWorldSize, 0, constt.ChunkWorldSize, 0)
	bus := eventbus.New(nil)
	b.Cleanup(func() {
		if err := bus.Shutdown(context.Background()); err != nil {
			b.Error(err)
		}
	})
	transformSystem := NewTransformUpdateSystem(world, cm, bus, zap.NewNop())
	if colliderSpatial != nil {
		transformSystem.SetPositionObserver(colliderSpatial)
	}

	if pillarsPerMover > 0 && !blocked {
		// Validate the workload before timing so a geometry change cannot
		// silently turn this into a benchmark of perpendicular stops.
		movementSystem.Update(world, 0.1)
		collisionSystem.Update(world, 0.1)
		movedEntities := ecs.GetResource[ecs.MovedEntities](world)
		if movedEntities.Count != moverCount {
			b.Fatalf("expected %d dense movers, got %d", moverCount, movedEntities.Count)
		}
		for i := 0; i < movedEntities.Count; i++ {
			result, ok := ecs.GetComponent[components.CollisionResult](world, movedEntities.Handles[i])
			if !ok || !result.HasCollision || result.PerpendicularOscillation || result.FinalY <= movedEntities.IntentY[i] {
				b.Fatalf("expected dense mover to slide past its intended Y=%.3f, got %+v", movedEntities.IntentY[i], result)
			}
		}
		transformSystem.Update(world, 0.1)
	}

	if blocked {
		for tick := 0; tick < 3; tick++ {
			ecs.GetResource[ecs.MovedEntities](world).Count = 0
			if !directional {
				for _, m := range movers {
					ecs.AddComponent(world, m.handle, m.movement)
				}
			}
			movementSystem.Update(world, .1)
			collisionSystem.Update(world, .1)
			transformSystem.Update(world, .1)
		}
		for i := range movers {
			m := &movers[i]
			position, _ := ecs.GetComponent[components.Transform](world, m.handle)
			current, _ := ecs.GetComponent[components.Movement](world, m.handle)
			if current.State != constt.StateIdle {
				b.Fatalf("blocked workload still moves: %+v", current)
			}
			m.startX, m.startY = position.X, position.Y
			if directional {
				m.movement = current
			}
		}
	}

	var packetEntries int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		movedEntities := ecs.GetResource[ecs.MovedEntities](world)
		movedEntities.Count = 0

		// Constant per-iteration setup, identical on both sides of any
		// before/after comparison of the systems themselves.
		for j := range movers {
			m := &movers[j]
			transform, ok := ecs.GetComponent[components.Transform](world, m.handle)
			if !ok {
				b.Fatal("benchmark mover is missing its transform")
			}
			// Keep the spatial entry in sync with the reset position so each
			// tick performs the same cell transitions as the first tick.
			chunk.Spatial().UpdateDynamic(m.handle, int(transform.X), int(transform.Y), int(m.startX), int(m.startY))
			ecs.AddComponent(world, m.handle, components.Transform{X: m.startX, Y: m.startY})
			ecs.AddComponent(world, m.handle, m.movement)
			ecs.AddComponent(world, m.handle, m.stats)
		}

		movementSystem.Update(world, 0.1)
		collisionSystem.Update(world, 0.1)
		transformSystem.Update(world, 0.1)
		packetEntries += len(transformSystem.moveBatch)
	}
	b.ReportMetric(float64(packetEntries)/float64(b.N), "entries/tick")
	if blocked && directional && packetEntries != 0 {
		b.Fatal("steady blocked direction broadcasts")
	}
}
