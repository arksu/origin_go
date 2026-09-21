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
	const moverCount = 200
	const targetX = 10000 // far beyond one tick's step; movers reset each iteration

	chunk := newTestChunk(types.ChunkCoord{X: 0, Y: 0})
	chunk.SetTiles(chunk.Tiles, 0)
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
			ecs.AddComponent(world, m.handle, components.Transform{X: m.startX, Y: m.startY})
			ecs.AddComponent(world, m.handle, m.movement)
			ecs.AddComponent(world, m.handle, m.stats)
		}

		movementSystem.Update(world, 0.1)
		collisionSystem.Update(world, 0.1)
		transformSystem.Update(world, 0.1)
	}
}
