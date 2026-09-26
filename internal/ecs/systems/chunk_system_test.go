package systems

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

// positionUpdate records one UpdateEntityPosition call for assertions.
type positionUpdate struct {
	entityID types.EntityID
	coord    types.ChunkCoord
}

// migrationChunkManager serves a map of active chunks and records
// UpdateEntityPosition calls, so chunk-migration bookkeeping is assertable.
type migrationChunkManager struct {
	chunks  map[types.ChunkCoord]*core.Chunk
	updates []positionUpdate
}

func newMigrationChunkManager(coords ...types.ChunkCoord) *migrationChunkManager {
	m := &migrationChunkManager{chunks: make(map[types.ChunkCoord]*core.Chunk, len(coords))}
	for _, coord := range coords {
		chunk := newTestChunk(coord)
		chunk.RestoreTiles(chunk.Tiles, 0, 0)
		// Migration validates chunk state; test chunks must look active.
		chunk.SetState(types.ChunkStateActive)
		m.chunks[coord] = chunk
	}
	return m
}

func (m *migrationChunkManager) ActiveChunks() []*core.Chunk {
	result := make([]*core.Chunk, 0, len(m.chunks))
	for _, chunk := range m.chunks {
		result = append(result, chunk)
	}
	return result
}

func (m *migrationChunkManager) GetChunk(coord types.ChunkCoord) *core.Chunk {
	return m.chunks[coord]
}

func (m *migrationChunkManager) GetChunkFast(coord types.ChunkCoord) *core.Chunk {
	return m.chunks[coord]
}

func (m *migrationChunkManager) UpdateEntityPosition(entityID types.EntityID, newCenter types.ChunkCoord) {
	m.updates = append(m.updates, positionUpdate{entityID: entityID, coord: newCenter})
}

func (m *migrationChunkManager) chunk(coord types.ChunkCoord) *core.Chunk {
	return m.chunks[coord]
}

// migrationScene wires a mover into the TransformUpdateSystem -> ChunkSystem
// tail of the pipeline. Tests set the collision result (or run
// CollisionSystem first for full-pipeline coverage) and an intent, then run
// the two systems in pipeline order.
type migrationScene struct {
	cm        *migrationChunkManager
	world     *ecs.World
	transform *TransformUpdateSystem
	chunkSys  *ChunkSystem
	mover     types.Handle
}

func newMigrationScene(t *testing.T, cm *migrationChunkManager, moverX, moverY float64, moverChunk types.ChunkCoord) *migrationScene {
	t.Helper()

	world := ecs.NewWorldForTesting()
	mover := world.Spawn(types.EntityID(1), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: moverX, Y: moverY})
		ecs.AddComponent(w, h, components.Collider{
			HalfWidth:  5,
			HalfHeight: 5,
			Layer:      constt.PlayerLayer,
			Mask:       constt.PlayerMask,
		})
		ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: moverChunk.X, CurrentChunkY: moverChunk.Y})
		ecs.AddComponent(w, h, components.CollisionResult{})
	})
	cm.chunk(moverChunk).Spatial().AddDynamic(mover, int(moverX), int(moverY))

	bus := eventbus.New(nil)
	return &migrationScene{
		cm:        cm,
		world:     world,
		transform: NewTransformUpdateSystem(world, cm, bus, zap.NewNop()),
		chunkSys:  NewChunkSystem(cm, zap.NewNop()),
		mover:     mover,
	}
}

// setFinal writes the collision-adjusted position the mover ended up at.
func (s *migrationScene) setFinal(finalX, finalY float64) {
	ecs.WithComponent(s.world, s.mover, func(cr *components.CollisionResult) {
		cr.FinalX = finalX
		cr.FinalY = finalY
		cr.HasCollision = true
	})
}

// setIntent feeds one intent into MovedEntities (as a target position, the
// same convention MovementSystem uses).
func (s *migrationScene) setIntent(intentX, intentY float64) {
	moved := ecs.GetResource[ecs.MovedEntities](s.world)
	moved.Count = 0
	moved.Add(s.mover, intentX, intentY)
}

func (s *migrationScene) runTransformAndChunk() {
	s.transform.Update(s.world, 0.1)
	s.chunkSys.Update(s.world, 0.1)
}

func (s *migrationScene) runTick(finalX, finalY, intentX, intentY float64) {
	s.setFinal(finalX, finalY)
	s.setIntent(intentX, intentY)
	s.runTransformAndChunk()
}

func chunkRefOf(t *testing.T, w *ecs.World, h types.Handle) components.ChunkRef {
	t.Helper()
	ref, ok := ecs.GetComponent[components.ChunkRef](w, h)
	if !ok {
		t.Fatalf("expected chunk ref component")
	}
	return ref
}

func querySpatial(t *testing.T, chunk *core.Chunk, minX, minY, maxX, maxY int) []types.Handle {
	t.Helper()
	var found []types.Handle
	chunk.Spatial().QueryAABB(minX, minY, maxX, maxY, &found)
	return found
}

func handleCount(handles []types.Handle, h types.Handle) int {
	count := 0
	for _, candidate := range handles {
		if candidate == h {
			count++
		}
	}
	return count
}

// C4 regression: chunk migration must follow the collision-adjusted final
// position, not the pre-collision intent. A slide that carries the box across
// a border while the intent target stayed behind must still migrate.
func TestChunkSystem_MigratesOnFinalPositionWhenIntentStays(t *testing.T) {
	cm := newMigrationChunkManager(types.ChunkCoord{X: 0, Y: 0}, types.ChunkCoord{X: 1, Y: 0})
	scene := newMigrationScene(t, cm, 1500, 100, types.ChunkCoord{X: 0, Y: 0})

	// Final position crossed the border at x=1536, intent target did not.
	scene.runTick(1545, 100, 1532, 100)

	ref := chunkRefOf(t, scene.world, scene.mover)
	if ref.CurrentChunkX != 1 || ref.CurrentChunkY != 0 {
		t.Fatalf("expected migration to chunk (1,0), got (%d,%d)", ref.CurrentChunkX, ref.CurrentChunkY)
	}
	if ref.PrevChunkX != 0 || ref.PrevChunkY != 0 {
		t.Fatalf("expected prev chunk (0,0), got (%d,%d)", ref.PrevChunkX, ref.PrevChunkY)
	}
	if len(cm.updates) != 1 || cm.updates[0].coord != (types.ChunkCoord{X: 1, Y: 0}) || cm.updates[0].entityID != 1 {
		t.Fatalf("expected exactly one UpdateEntityPosition to (1,0) for entity 1, got %+v", cm.updates)
	}

	oldGrid := querySpatial(t, cm.chunk(types.ChunkCoord{X: 0, Y: 0}), 1536, 90, 1560, 110)
	if handleCount(oldGrid, scene.mover) != 0 {
		t.Fatalf("stale spatial entry left in the old chunk grid")
	}
	newGrid := querySpatial(t, cm.chunk(types.ChunkCoord{X: 1, Y: 0}), 1536, 90, 1560, 110)
	if handleCount(newGrid, scene.mover) != 1 {
		t.Fatalf("mover missing from the new chunk grid, got %d entries", handleCount(newGrid, scene.mover))
	}
}

// C4 regression, full pipeline: collision stops the mover short of the border
// while the intent target lies beyond it — no migration may happen.
func TestChunkSystem_NoMigrationWhenCollisionStopsAtBorder(t *testing.T) {
	cm := newMigrationChunkManager(types.ChunkCoord{X: 0, Y: 0}, types.ChunkCoord{X: 1, Y: 0})
	scene := newMigrationScene(t, cm, 1500, 100, types.ChunkCoord{X: 0, Y: 0})

	// Wall face at x=1540 stops the box with its center at ~1535, short of
	// the border at 1536.
	wall := scene.world.Spawn(types.EntityID(2), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 1545, Y: 100})
		ecs.AddComponent(w, h, components.Collider{
			HalfWidth:  5,
			HalfHeight: 60,
			Layer:      constt.PlayerLayer,
			Mask:       constt.PlayerMask,
		})
	})
	cm.chunk(types.ChunkCoord{X: 0, Y: 0}).Spatial().AddStatic(wall, 1545, 100)

	collision := NewCollisionSystem(scene.world, cm, zap.NewNop(), 0, constt.ChunkWorldSize, 0, constt.ChunkWorldSize, 0)
	scene.setIntent(1600, 100)
	collision.Update(scene.world, 0.1)
	scene.runTransformAndChunk()

	result, ok := ecs.GetComponent[components.CollisionResult](scene.world, scene.mover)
	if !ok {
		t.Fatalf("expected collision result component")
	}
	if result.FinalX >= 1536 {
		t.Fatalf("expected collision to stop the mover before the border, final x %.3f", result.FinalX)
	}

	ref := chunkRefOf(t, scene.world, scene.mover)
	if ref.CurrentChunkX != 0 || ref.CurrentChunkY != 0 {
		t.Fatalf("expected no migration, got chunk (%d,%d)", ref.CurrentChunkX, ref.CurrentChunkY)
	}
	if len(cm.updates) != 0 {
		t.Fatalf("expected no UpdateEntityPosition calls, got %+v", cm.updates)
	}
	grid := querySpatial(t, cm.chunk(types.ChunkCoord{X: 0, Y: 0}), 1500, 90, 1536, 110)
	if handleCount(grid, scene.mover) != 1 {
		t.Fatalf("mover spatial entry missing from its own chunk grid")
	}
}

// Both intent and final cross the border: exactly one clean migration, no
// duplicate spatial entries anywhere.
func TestChunkSystem_BothCrossMigratesCleanly(t *testing.T) {
	cm := newMigrationChunkManager(types.ChunkCoord{X: 0, Y: 0}, types.ChunkCoord{X: 1, Y: 0})
	scene := newMigrationScene(t, cm, 1500, 100, types.ChunkCoord{X: 0, Y: 0})

	scene.runTick(1560, 100, 1570, 100)

	ref := chunkRefOf(t, scene.world, scene.mover)
	if ref.CurrentChunkX != 1 || ref.CurrentChunkY != 0 {
		t.Fatalf("expected migration to chunk (1,0), got (%d,%d)", ref.CurrentChunkX, ref.CurrentChunkY)
	}
	if len(cm.updates) != 1 {
		t.Fatalf("expected exactly one UpdateEntityPosition, got %+v", cm.updates)
	}
	oldGrid := querySpatial(t, cm.chunk(types.ChunkCoord{X: 0, Y: 0}), 1400, 90, 1536, 110)
	if handleCount(oldGrid, scene.mover) != 0 {
		t.Fatalf("stale spatial entry left in the old chunk grid")
	}
	newGrid := querySpatial(t, cm.chunk(types.ChunkCoord{X: 1, Y: 0}), 1536, 90, 1600, 110)
	if handleCount(newGrid, scene.mover) != 1 {
		t.Fatalf("expected exactly one entry in the new chunk grid, got %d", handleCount(newGrid, scene.mover))
	}
}

// A move that stays inside the chunk must not migrate, and the spatial entry
// must follow the position within the same grid.
func TestChunkSystem_SameChunkMoveKeepsSpatialInPlace(t *testing.T) {
	cm := newMigrationChunkManager(types.ChunkCoord{X: 0, Y: 0}, types.ChunkCoord{X: 1, Y: 0})
	scene := newMigrationScene(t, cm, 1500, 100, types.ChunkCoord{X: 0, Y: 0})

	scene.runTick(1520, 100, 1530, 100)

	ref := chunkRefOf(t, scene.world, scene.mover)
	if ref.CurrentChunkX != 0 || ref.CurrentChunkY != 0 {
		t.Fatalf("expected no migration, got chunk (%d,%d)", ref.CurrentChunkX, ref.CurrentChunkY)
	}
	if len(cm.updates) != 0 {
		t.Fatalf("expected no UpdateEntityPosition calls, got %+v", cm.updates)
	}
	oldCell := querySpatial(t, cm.chunk(types.ChunkCoord{X: 0, Y: 0}), 1488, 90, 1504, 110)
	if handleCount(oldCell, scene.mover) != 0 {
		t.Fatalf("spatial entry still at the pre-move cell")
	}
	newCell := querySpatial(t, cm.chunk(types.ChunkCoord{X: 0, Y: 0}), 1512, 90, 1536, 110)
	if handleCount(newCell, scene.mover) != 1 {
		t.Fatalf("spatial entry missing at the new position")
	}
}

// When the target chunk is not active, migration must abort without mutating
// anything: the entity keeps its ChunkRef and stays queryable in the old grid.
func TestChunkSystem_InactiveTargetChunkAbortsCleanly(t *testing.T) {
	// Only chunk (0,0) exists — the migration target (1,0) is missing.
	cm := newMigrationChunkManager(types.ChunkCoord{X: 0, Y: 0})
	scene := newMigrationScene(t, cm, 1500, 100, types.ChunkCoord{X: 0, Y: 0})

	scene.runTick(1545, 100, 1560, 100)

	ref := chunkRefOf(t, scene.world, scene.mover)
	if ref.CurrentChunkX != 0 || ref.CurrentChunkY != 0 {
		t.Fatalf("expected aborted migration to keep chunk (0,0), got (%d,%d)", ref.CurrentChunkX, ref.CurrentChunkY)
	}
	if len(cm.updates) != 0 {
		t.Fatalf("expected no UpdateEntityPosition calls, got %+v", cm.updates)
	}
	grid := querySpatial(t, cm.chunk(types.ChunkCoord{X: 0, Y: 0}), 1536, 90, 1560, 110)
	if handleCount(grid, scene.mover) != 1 {
		t.Fatalf("spatial entry vanished during aborted migration")
	}
}

// Chunk coordinates must use floor semantics: a mover in a negative chunk
// stays there even when the (truncating) intent math would say otherwise.
func TestChunkSystem_NegativeCoordsStayInTheirChunk(t *testing.T) {
	cm := newMigrationChunkManager(types.ChunkCoord{X: -1, Y: 0}, types.ChunkCoord{X: 0, Y: 0})
	scene := newMigrationScene(t, cm, -1500, 100, types.ChunkCoord{X: -1, Y: 0})

	// Final position stays in chunk (-1,0); intent -500 would truncate to
	// chunk (0,0) with plain int division.
	scene.runTick(-1520, 100, -500, 100)

	ref := chunkRefOf(t, scene.world, scene.mover)
	if ref.CurrentChunkX != -1 || ref.CurrentChunkY != 0 {
		t.Fatalf("expected no migration for negative coords, got chunk (%d,%d)", ref.CurrentChunkX, ref.CurrentChunkY)
	}
	if len(cm.updates) != 0 {
		t.Fatalf("expected no UpdateEntityPosition calls, got %+v", cm.updates)
	}
	grid := querySpatial(t, cm.chunk(types.ChunkCoord{X: -1, Y: 0}), -1536, 90, -1504, 110)
	if handleCount(grid, scene.mover) != 1 {
		t.Fatalf("mover spatial entry missing from its own chunk grid")
	}
}
