package systems

import (
	"testing"

	constt "origin/internal/const"
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
	runPipelineBench(b, 0)
}

// BenchmarkMovementPipelineDense adds static pillars inside every mover's
// swept corridor so the collision candidate loop, hit handling and slide
// iterations run each tick, as they would around built structures.
func BenchmarkMovementPipelineDense(b *testing.B) {
	runPipelineBench(b, 3)
}

func runPipelineBench(b *testing.B, pillarsPerMover int) {
	const moverCount = 200
	const targetX = 10000 // far beyond one tick's step; movers reset each iteration

	chunk := newTestChunk(types.ChunkCoord{X: 0, Y: 0})
	chunk.RestoreTiles(chunk.Tiles, 0, 0)
	cm := &testChunkManager{chunk: chunk}
	world := ecs.NewWorldForTesting()

	movers := make([]pipelineMover, 0, moverCount)
	observer := types.Handle(0)
	for i := 0; i < moverCount; i++ {
		startX := 100 + float64(i%20)*40
		startY := 100 + float64(i/20)*40

		var movement components.Movement
		movement.Mode = constt.Walk
		movement.State = constt.StateMoving
		movement.Speed = 100
		movement.SetTargetPoint(int(targetX), int(startY))

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
	transformSystem := NewTransformUpdateSystem(world, cm, eventbus.New(nil), zap.NewNop())

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
	}
}
