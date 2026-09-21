package systems

import (
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

// testChunkManager serves a single chunk to collision tests.
type testChunkManager struct {
	chunk *core.Chunk
}

func (m *testChunkManager) ActiveChunks() []*core.Chunk {
	if m.chunk == nil {
		return nil
	}
	return []*core.Chunk{m.chunk}
}

func (m *testChunkManager) GetChunk(coord types.ChunkCoord) *core.Chunk {
	if m.chunk != nil && m.chunk.Coord == coord {
		return m.chunk
	}
	return nil
}

func (m *testChunkManager) GetChunkFast(coord types.ChunkCoord) *core.Chunk {
	return m.GetChunk(coord)
}

func (m *testChunkManager) UpdateEntityPosition(types.EntityID, types.ChunkCoord) {}

// newTestChunk returns an all-grass (passable) chunk at the given coord.
func newTestChunk(coord types.ChunkCoord) *core.Chunk {
	chunk := core.NewChunk(coord, 0, 0, constt.ChunkSize)
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for i := range tiles {
		tiles[i] = types.TileGrass
	}
	chunk.SetTiles(tiles, 0)
	return chunk
}

// runCollisionSweep spawns a mover plus obstacle walls, feeds the intent into
// MovedEntities and returns the collision result for the mover.
// Mover and walls use half-extents of 5 (walls 60 tall).
func runCollisionSweep(t *testing.T, moverX, moverY, dx, dy float64, walls []components.Transform) components.CollisionResult {
	t.Helper()

	chunk := newTestChunk(types.ChunkCoord{X: 0, Y: 0})
	cm := &testChunkManager{chunk: chunk}
	world := ecs.NewWorldForTesting()

	mover := world.Spawn(types.EntityID(1), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: moverX, Y: moverY})
		ecs.AddComponent(w, h, components.Collider{
			HalfWidth:  5,
			HalfHeight: 5,
			Layer:      constt.PlayerLayer,
			Mask:       constt.PlayerMask,
		})
		ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: 0, CurrentChunkY: 0})
		ecs.AddComponent(w, h, components.CollisionResult{})
	})
	chunk.Spatial().AddDynamic(mover, int(moverX), int(moverY))

	for i, wall := range walls {
		handle := world.Spawn(types.EntityID(uint64(i+2)), func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, wall)
			ecs.AddComponent(w, h, components.Collider{
				HalfWidth:  5,
				HalfHeight: 60,
				Layer:      constt.PlayerLayer,
				Mask:       constt.PlayerMask,
			})
		})
		chunk.Spatial().AddStatic(handle, int(wall.X), int(wall.Y))
	}

	system := NewCollisionSystem(world, cm, zap.NewNop(), 0, constt.ChunkWorldSize, 0, constt.ChunkWorldSize, 0)
	ecs.GetResource[ecs.MovedEntities](world).Add(mover, moverX+dx, moverY+dy)
	system.Update(world, 0.1)

	result, ok := ecs.GetComponent[components.CollisionResult](world, mover)
	if !ok {
		t.Fatalf("expected collision result component")
	}
	return result
}

// C2 regression: sliding along a wall must stay inside the original per-tick
// distance. The old code re-applied the full per-tick distance after every
// slide hit, letting wall-huggers move up to maxIterations times faster.
func TestCollisionSystem_SlideStaysWithinTickBudget(t *testing.T) {
	// Vertical wall the mover clips at 45 degrees.
	wall := components.Transform{X: 120, Y: 100}
	result := runCollisionSweep(t, 100, 100, 12, 12, []components.Transform{wall})

	intentDist := math.Hypot(12, 12)
	movedDist := math.Hypot(result.FinalX-100, result.FinalY-100)

	if movedDist > intentDist+0.01 {
		t.Fatalf("slide exceeded per-tick budget: moved %.3f, intent %.3f (final %.3f, %.3f)",
			movedDist, intentDist, result.FinalX, result.FinalY)
	}
	// The mover must still slide along the wall, not just stop dead.
	if result.FinalY <= 101 {
		t.Fatalf("expected slide progress along wall, final y %.3f", result.FinalY)
	}
	if !result.HasCollision {
		t.Fatalf("expected collision to be reported")
	}
}

func TestCollisionSystem_UnobstructedMoveKeepsFullDistance(t *testing.T) {
	result := runCollisionSweep(t, 100, 100, -12, 0, nil)

	if math.Abs(result.FinalX-88) > 0.01 || math.Abs(result.FinalY-100) > 0.01 {
		t.Fatalf("expected full unobstructed move to (88, 100), got (%.3f, %.3f)", result.FinalX, result.FinalY)
	}
	if result.HasCollision {
		t.Fatalf("expected no collision")
	}
}

func TestCollisionSystem_PerpendicularHitStopsAtWall(t *testing.T) {
	wall := components.Transform{X: 120, Y: 100}
	result := runCollisionSweep(t, 100, 100, 12, 0, []components.Transform{wall})

	// Contact point: wall face at 115 minus mover half 5 -> center stops at 110.
	if result.FinalX > 110.01 {
		t.Fatalf("expected stop at wall face, final x %.3f", result.FinalX)
	}
	if !result.PerpendicularOscillation {
		t.Fatalf("expected perpendicular oscillation to be flagged")
	}
}
