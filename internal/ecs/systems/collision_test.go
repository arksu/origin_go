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

// newTestChunk returns a chunk at the given coord filled with grass in its
// tile array. Bitsets are NOT populated yet; runCollisionSweep finalizes the
// chunk with a single RestoreTiles call after all paint callbacks ran, matching
// the production one-shot load flow.
func newTestChunk(coord types.ChunkCoord) *core.Chunk {
	chunk := core.NewChunk(coord, 0, 0, constt.ChunkSize)
	for i := range chunk.Tiles {
		chunk.Tiles[i] = types.TileGrass
	}
	return chunk
}

// paintTestTile sets the tile containing the world point to tileID. Must run
// before the chunk's RestoreTiles call that populates the passability bitsets.
func paintTestTile(chunk *core.Chunk, worldX, worldY float64, tileID byte) {
	tileSize := float64(constt.CoordPerTile)
	localX := int(math.Floor(worldX/tileSize)) - chunk.Coord.X*constt.ChunkSize
	localY := int(math.Floor(worldY/tileSize)) - chunk.Coord.Y*constt.ChunkSize
	chunk.Tiles[localY*constt.ChunkSize+localX] = tileID
}

// sweepScene bundles a prepared collision world: a grass chunk, a mover with
// half-extents 5, and obstacle walls with half-extents 5x60. Tests that only
// need one sweep use runCollisionSweep; tests needing custom candidates or
// multiple ticks drive the scene directly.
type sweepScene struct {
	world  *ecs.World
	chunk  *core.Chunk
	system *CollisionSystem
	mover  types.Handle
}

func newSweepScene(t *testing.T, moverX, moverY float64, walls []components.Transform, paint ...func(*core.Chunk)) *sweepScene {
	t.Helper()

	chunk := newTestChunk(types.ChunkCoord{X: 0, Y: 0})
	for _, p := range paint {
		p(chunk)
	}
	chunk.RestoreTiles(chunk.Tiles, 0, 0)
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
	return &sweepScene{world: world, chunk: chunk, system: system, mover: mover}
}

// addMovingCandidate spawns an entity that counts as dynamic for collision
// (Movement in StateMoving), like a walking player would.
func (s *sweepScene) addMovingCandidate(id uint64, x, y float64) {
	handle := s.world.Spawn(types.EntityID(id), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: x, Y: y})
		ecs.AddComponent(w, h, components.Collider{
			HalfWidth:  5,
			HalfHeight: 60,
			Layer:      constt.PlayerLayer,
			Mask:       constt.PlayerMask,
		})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving})
	})
	s.chunk.Spatial().AddDynamic(handle, int(x), int(y))
}

func (s *sweepScene) addStaticCandidate(id uint64, x, y, halfWidth, halfHeight float64) {
	handle := s.world.Spawn(types.EntityID(id), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: x, Y: y})
		ecs.AddComponent(w, h, components.Collider{
			HalfWidth:  halfWidth,
			HalfHeight: halfHeight,
			Layer:      constt.PlayerLayer,
			Mask:       constt.PlayerMask,
		})
	})
	s.chunk.Spatial().AddStatic(handle, int(x), int(y))
}

// runTick feeds one intent (as a delta from the mover's current position) into
// MovedEntities and runs the collision system. Between ticks the collision
// result is applied to the mover's transform, mirroring TransformUpdateSystem,
// and the buffer is cleared, mirroring ResetSystem.
func (s *sweepScene) runTick(t *testing.T, dx, dy float64) components.CollisionResult {
	t.Helper()

	transform, ok := ecs.GetComponent[components.Transform](s.world, s.mover)
	if !ok {
		t.Fatalf("expected transform component")
	}
	moved := ecs.GetResource[ecs.MovedEntities](s.world)
	moved.Count = 0
	moved.Add(s.mover, transform.X+dx, transform.Y+dy)
	s.system.Update(s.world, 0.1)

	result, ok := ecs.GetComponent[components.CollisionResult](s.world, s.mover)
	if !ok {
		t.Fatalf("expected collision result component")
	}
	ecs.WithComponent(s.world, s.mover, func(m *components.Transform) {
		m.X = result.FinalX
		m.Y = result.FinalY
	})
	return result
}

// runCollisionSweep spawns a mover plus obstacle walls, feeds the intent into
// MovedEntities and returns the collision result for the mover.
// Mover and walls use half-extents of 5 (walls 60 tall).
// Optional paint callbacks run after chunk creation to customize tiles.
func runCollisionSweep(t *testing.T, moverX, moverY, dx, dy float64, walls []components.Transform, paint ...func(*core.Chunk)) components.CollisionResult {
	t.Helper()
	scene := newSweepScene(t, moverX, moverY, walls, paint...)
	return scene.runTick(t, dx, dy)
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

// C3/H1 regression: a slide must not push the mover into impassable terrain,
// and tile collision must respect the collider box, not just the center.
// The intent path keeps the box clear of the deep-water tile (col 9, row 10)
// while the slide segment would push the box edge past its y=120 boundary.
func TestCollisionSystem_SlideStopsBeforeImpassableTerrain(t *testing.T) {
	paintWater := func(chunk *core.Chunk) {
		paintTestTile(chunk, 110, 120.5, types.TileDeepWater)
	}
	wall := components.Transform{X: 120, Y: 100}
	result := runCollisionSweep(t, 100, 112, 12, 2, []components.Transform{wall}, paintWater)

	// Box-aware stopping: slide ends when the box edge (center+5) reaches
	// the water line at y=120, i.e. center y ≈ 115.
	if math.Abs(result.FinalY-115) > 0.05 {
		t.Fatalf("expected slide to stop with box edge at water line (y≈115), got %.3f", result.FinalY)
	}
	if result.FinalX > 110.01 {
		t.Fatalf("expected stop at wall face, final x %.3f", result.FinalX)
	}
	if !result.HasCollision {
		t.Fatalf("expected collision to be reported")
	}
}

func TestCollisionSystem_SlideStopsAtObjectBeforeTile(t *testing.T) {
	paintWater := func(chunk *core.Chunk) {
		paintTestTile(chunk, 101, 125, types.TileDeepWater)
	}
	scene := newSweepScene(t, 101, 110, []components.Transform{{X: 111, Y: 100}}, paintWater)
	scene.addStaticCandidate(3, 101, 123, 5, 5)

	result := scene.runTick(t, 6.2, 1.5)
	if result.FinalY > 113.01 {
		t.Fatalf("slide passed object before reaching water: final (%.3f, %.3f)", result.FinalX, result.FinalY)
	}
	if result.CollidedWith != 3 {
		t.Fatalf("expected nearer object collision, got %d", result.CollidedWith)
	}
}

func TestCollisionSystem_DirectMoveStopsAtObjectBeforeTile(t *testing.T) {
	paintWater := func(chunk *core.Chunk) {
		paintTestTile(chunk, 101, 125, types.TileDeepWater)
	}
	scene := newSweepScene(t, 101, 110, nil, paintWater)
	scene.addStaticCandidate(2, 101, 123, 5, 5)

	result := scene.runTick(t, 0, 6.2)
	if result.FinalY > 113.01 {
		t.Fatalf("move passed object before reaching water: final y %.3f", result.FinalY)
	}
	if result.CollidedWith != 2 {
		t.Fatalf("expected nearer object collision, got %d", result.CollidedWith)
	}
}

func TestCollisionSystem_TileStopsBeforeFartherObject(t *testing.T) {
	paintWater := func(chunk *core.Chunk) {
		paintTestTile(chunk, 101, 125, types.TileDeepWater)
	}
	scene := newSweepScene(t, 101, 110, nil, paintWater)
	scene.addStaticCandidate(2, 101, 126, 5, 5)

	result := scene.runTick(t, 0, 6.2)
	if math.Abs(result.FinalY-115) > 0.05 {
		t.Fatalf("expected stop at water before object, final y %.3f", result.FinalY)
	}
	if !result.HasCollision || result.CollidedWith != 0 {
		t.Fatalf("expected tile collision before object, got hasCollision=%v collidedWith=%d",
			result.HasCollision, result.CollidedWith)
	}
}

// H1 regression: the collider box must not cut diagonally between two blocked
// tiles even when the destination tile itself is passable.
func TestCollisionSystem_BoxCannotCutDiagonalCorner(t *testing.T) {
	// Water at (col 9, row 8) and (col 8, row 9); the diagonal tile
	// (col 9, row 9) stays grass, so center-only checks would pass through.
	paint := func(chunk *core.Chunk) {
		paintTestTile(chunk, 110, 102, types.TileDeepWater)
		paintTestTile(chunk, 102, 110, types.TileDeepWater)
	}
	result := runCollisionSweep(t, 100, 100, 12, 12, nil, paint)

	// Box contact happens at t≈0.25 of the intent: center stops at ≈(103,103).
	if result.FinalX > 104 || result.FinalY > 104 {
		t.Fatalf("box cut the diagonal corner: final (%.3f, %.3f)", result.FinalX, result.FinalY)
	}
	if !result.HasCollision {
		t.Fatalf("expected collision to be reported")
	}
}

// H1 regression: straight walk into deep water must stop when the box edge
// reaches the water line, not when the center crosses the tile boundary.
func TestCollisionSystem_WalkStopsBeforeDeepWater(t *testing.T) {
	// Deep water tile (col 9, row 8): western edge at x=108.
	paint := func(chunk *core.Chunk) {
		paintTestTile(chunk, 110, 102, types.TileDeepWater)
	}
	result := runCollisionSweep(t, 100, 100, 12, 0, nil, paint)

	// Box edge (center+5) at 108 means the center stops at ≈103.
	if math.Abs(result.FinalX-103) > 0.05 {
		t.Fatalf("expected stop with box edge at water line (x≈103), got %.3f", result.FinalX)
	}
	if !result.HasCollision {
		t.Fatalf("expected collision to be reported")
	}
}

func TestCollisionSystem_InitialTerrainOverlap(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		startX       float64
		startY       float64
		dx, dy       float64
		finalX       float64
		finalY       float64
		hasCollision bool
	}{
		{"deepening", 105, 102, 6, 0, 105, 102, true},
		{"diagonal deepening", 105, 102, 3, 3, 105, 102, true},
		{"escaping", 105, 102, -6, 0, 99, 102, false},
		{"along shoreline across tile seam", 105, 102, 0, 6, 105, 108, false},
		{"deepening at internal seam", 110, 108, 6, 0, 110, 108, true},
		{"escaping at internal seam", 110, 108, -6, 0, 104, 108, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			scene := newSweepScene(t, scenario.startX, scenario.startY, nil, func(chunk *core.Chunk) {
				paintTestTile(chunk, 110, 102, types.TileDeepWater)
				paintTestTile(chunk, 110, 114, types.TileDeepWater)
			})
			ecs.AddComponent(scene.world, scene.mover, components.Movement{Mode: constt.Walk})
			result := scene.runTick(t, scenario.dx, scenario.dy)
			if math.Abs(result.FinalX-scenario.finalX) > 0.01 || math.Abs(result.FinalY-scenario.finalY) > 0.01 {
				t.Fatalf("expected (%.3f, %.3f), got (%.3f, %.3f)",
					scenario.finalX, scenario.finalY, result.FinalX, result.FinalY)
			}
			if result.HasCollision != scenario.hasCollision {
				t.Fatalf("expected hasCollision=%v, got %v", scenario.hasCollision, result.HasCollision)
			}
		})
	}
}

// H2 regression: a mover already overlapping a static object must not pass
// through it — deepening movement hits the overlap boundary. Pre-fix the
// swept test rejected the pair (entryTime < 0) and the mover walked through.
func TestCollisionSystem_OverlappedMoverCannotDeepenIntoStatic(t *testing.T) {
	// Wall center 6 units right of the mover: boxes overlap by 4 on x.
	wall := components.Transform{X: 106, Y: 100}
	result := runCollisionSweep(t, 100, 100, 12, 0, []components.Transform{wall})

	// The wall face sits at 101; the mover must stay left of it.
	if result.FinalX > 101 {
		t.Fatalf("overlapped mover passed through the wall: final x %.3f", result.FinalX)
	}
	if !result.HasCollision || result.CollidedWith != 2 {
		t.Fatalf("expected collision with wall 2, got hasCollision=%v collidedWith=%v",
			result.HasCollision, result.CollidedWith)
	}
}

// H2: overlapped movers must keep a way out — movement away from the object
// is never blocked. Force-drop and admin-teleport spawns rely on this.
func TestCollisionSystem_OverlappedMoverEscapesStatic(t *testing.T) {
	wall := components.Transform{X: 106, Y: 100}
	result := runCollisionSweep(t, 100, 100, -12, 0, []components.Transform{wall})

	if math.Abs(result.FinalX-88) > 0.01 || math.Abs(result.FinalY-100) > 0.01 {
		t.Fatalf("expected full escape move to (88, 100), got (%.3f, %.3f)",
			result.FinalX, result.FinalY)
	}
}

// H2: deepening movement into an overlapped object slides along it instead of
// stopping dead, matching ordinary wall-contact behavior.
func TestCollisionSystem_OverlappedMoverSlidesAlongStatic(t *testing.T) {
	wall := components.Transform{X: 106, Y: 100}
	result := runCollisionSweep(t, 100, 100, 12, 12, []components.Transform{wall})

	if result.FinalX > 101 {
		t.Fatalf("slide pushed the mover deeper into the wall: final x %.3f", result.FinalX)
	}
	if result.FinalY <= 105 {
		t.Fatalf("expected slide progress along wall, final y %.3f", result.FinalY)
	}
}

// H2: deepening movement into an overlapping moving candidate (a walking
// player) stops completely, matching the existing dynamic-dynamic hard-stop.
func TestCollisionSystem_OverlappedMovingCandidateStopsMover(t *testing.T) {
	scene := newSweepScene(t, 100, 100, nil)
	// Candidate center 6 units right of the mover: boxes overlap by 4 on x.
	scene.addMovingCandidate(2, 106, 100)

	result := scene.runTick(t, 12, 0)

	if result.FinalX > 101 {
		t.Fatalf("mover passed through the moving candidate: final x %.3f", result.FinalX)
	}
	if !result.HasCollision {
		t.Fatalf("expected collision to be reported")
	}
}

// H2 guard: normal wall contact stops a fraction short of the surface, so the
// next tick's diagonal push must still slide via the regular sweep. The
// overlap path must never turn near-contact movement into a hard stop.
func TestCollisionSystem_EpsilonContactThenDiagonalStillSlides(t *testing.T) {
	wall := components.Transform{X: 120, Y: 100}
	scene := newSweepScene(t, 100, 100, []components.Transform{wall})

	first := scene.runTick(t, 12, 0)
	if !first.HasCollision || first.FinalX > 110.01 {
		t.Fatalf("expected first tick to stop at wall face, got (%.3f, %.3f)",
			first.FinalX, first.FinalY)
	}

	second := scene.runTick(t, 2, 12)
	if second.FinalX > 110.01 {
		t.Fatalf("diagonal push pushed the mover deeper into the wall: final x %.3f",
			second.FinalX)
	}
	if second.FinalY <= 105 {
		t.Fatalf("expected slide progress along wall, final y %.3f", second.FinalY)
	}
}
