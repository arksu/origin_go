package game

import (
	"math"
	"strconv"
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type liftPlacementFixture struct {
	w       *ecs.World
	spatial *core.WorldColliderSpatial
	service *LiftService
	player  types.Handle
	object  types.Handle
	lifted  components.LiftedObjectState
}

func newLiftPlacementFixture(layer int) *liftPlacementFixture {
	w := ecs.NewWorldWithCapacity(5000, nil, layer)
	return &liftPlacementFixture{
		w:       w,
		spatial: core.AttachColliderSpatial(w),
		service: NewLiftService(w, nil, nil, nil, nil),
		player:  w.Spawn(1, nil),
		object:  w.Spawn(2, nil),
		lifted: components.LiftedObjectState{
			HadCollider:      true,
			OriginalCollider: components.Collider{HalfWidth: 1, HalfHeight: 1, Layer: 1, Mask: 1},
		},
	}
}

func (f *liftPlacementFixture) allowed(x, y float64) bool {
	return f.service.placementAllowedAt(f.w, f.player, f.object, f.lifted, x, y)
}

func spawnLiftPlacementCollider(w *ecs.World, id types.EntityID, x, y float64, collider components.Collider) types.Handle {
	return w.Spawn(id, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.Transform{X: x, Y: y})
		ecs.AddComponent(w, handle, collider)
	})
}

func TestLiftPlacementBlocksOrdinaryCollidersOnEveryWorldLayer(t *testing.T) {
	for _, layer := range []int{-1, 0, 1, 2} {
		t.Run(strconv.Itoa(layer), func(t *testing.T) {
			for _, obstacle := range []struct {
				name                  string
				halfWidth, halfHeight float64
			}{
				{"tree", 3, 3},
				{"box", 6, 4},
				{"campfire", 6, 6},
				{"player", 2, 2},
				{"animal", 4, 3},
			} {
				t.Run(obstacle.name, func(t *testing.T) {
					f := newLiftPlacementFixture(layer)
					handle := spawnLiftPlacementCollider(f.w, 100, 100, 100, components.Collider{
						HalfWidth: obstacle.halfWidth, HalfHeight: obstacle.halfHeight, Layer: 1, Mask: 1,
					})
					_, hasInfo := ecs.GetComponent[components.EntityInfo](f.w, handle)
					require.False(t, hasInfo, "ordinary collision must not require object definitions or behaviors")
					require.False(t, f.allowed(100, 100))
					require.True(t, f.allowed(120, 120))
				})
			}
		})
	}
}

func TestLiftPlacementRequiresPositiveAreaOverlap(t *testing.T) {
	f := newLiftPlacementFixture(0)
	spawnLiftPlacementCollider(f.w, 100, 100, 100, components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: 1, Mask: 1})
	for _, tc := range []struct {
		name    string
		x, y    float64
		allowed bool
	}{
		{"inside", 100, 100, false},
		{"anchor outside", 106.5, 100, false},
		{"corner overlap", 106.5, 106.5, false},
		{"left edge", 93, 100, true},
		{"right edge", 107, 100, true},
		{"top edge", 100, 93, true},
		{"bottom edge", 100, 107, true},
		{"corner contact", 107, 107, true},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.allowed, f.allowed(tc.x, tc.y)) })
	}
}

func TestLiftPlacementUsesOriginalAndColliderlessFootprints(t *testing.T) {
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 100, Key: "log_x", LiftConfig: &objectdefs.LiftBehaviorConfig{PlacementSize: &objectdefs.LiftPlacementSize{W: 24, H: 6}}},
		{DefID: 101, Key: "log_y", LiftConfig: &objectdefs.LiftBehaviorConfig{PlacementSize: &objectdefs.LiftPlacementSize{W: 6, H: 24}}},
		{DefID: 102, Key: "default", LiftConfig: &objectdefs.LiftBehaviorConfig{}},
		{DefID: 103, Key: "no_lift_config"},
	}))
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	for _, tc := range []struct {
		name        string
		defID       uint32
		hadCollider bool
		x, y        float64
		allowed     bool
	}{
		{"horizontal log overlap", 100, false, 117, 100, false},
		{"horizontal log edge", 100, false, 118, 100, true},
		{"horizontal log narrow side", 100, false, 100, 109, true},
		{"vertical log overlap", 101, false, 100, 117, false},
		{"vertical log edge", 101, false, 100, 118, true},
		{"vertical log narrow side", 101, false, 109, 100, true},
		{"original collider overrides placement size", 100, true, 107, 100, true},
		{"default overlap", 102, false, 106.5, 100, false},
		{"default edge", 102, false, 107, 100, true},
		{"missing lift config", 103, false, 107, 100, true},
		{"missing definition", 104, false, 107, 100, true},
		{"missing entity info", 0, false, 106.5, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLiftPlacementFixture(0)
			f.lifted.HadCollider = tc.hadCollider
			if tc.defID != 0 {
				ecs.AddComponent(f.w, f.object, components.EntityInfo{TypeID: tc.defID})
			}
			spawnLiftPlacementCollider(f.w, 200, 100, 100, components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: 1, Mask: 1})
			require.Equal(t, tc.allowed, f.allowed(tc.x, tc.y))
			_, hasCollider := ecs.GetComponent[components.Collider](f.w, f.object)
			require.False(t, hasCollider, "checking a placement footprint must not restore a carried collider")
		})
	}
}

func TestLiftPlacementUsesMovementMaskCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		sourceLayer, sourceMask uint64
		targetLayer, targetMask uint64
		allowed                 bool
	}{
		{"both directions", 1, 2, 2, 1, false},
		{"source mask only", 1, 2, 2, 0, false},
		{"target mask only", 1, 0, 2, 1, false},
		{"neither direction", 1, 1, 2, 2, true},
		{"inactive target", 1, 1, 0, 0, true},
		{"incompatible zero mask", 1, 1, 2, 0, true},
		{"inactive source", 0, 0, 1, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLiftPlacementFixture(0)
			f.lifted.OriginalCollider.Layer, f.lifted.OriginalCollider.Mask = tc.sourceLayer, tc.sourceMask
			spawnLiftPlacementCollider(f.w, 100, 100, 100, components.Collider{
				HalfWidth: 6, HalfHeight: 6, Layer: tc.targetLayer, Mask: tc.targetMask,
			})
			require.Equal(t, tc.allowed, f.allowed(100, 100))
		})
	}
	t.Run("colliderless source uses layer and mask one", func(t *testing.T) {
		f := newLiftPlacementFixture(0)
		f.lifted = components.LiftedObjectState{OriginalCollider: components.Collider{Layer: 2, Mask: 2}}
		handle := spawnLiftPlacementCollider(f.w, 100, 100, 100, components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: 2, Mask: 2})
		require.True(t, f.allowed(100, 100))
		ecs.WithComponent(f.w, handle, func(c *components.Collider) { c.Layer = 1 })
		require.False(t, f.allowed(100, 100))
	})
}

func TestLiftPlacementExcludesCarrierAndCarriedObject(t *testing.T) {
	f := newLiftPlacementFixture(0)
	collider := components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: 1, Mask: 1}
	for _, handle := range []types.Handle{f.player, f.object} {
		ecs.AddComponent(f.w, handle, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(f.w, handle, collider)
	}
	require.True(t, f.allowed(100, 100))
	spawnLiftPlacementCollider(f.w, 100, 100, 100, collider)
	require.False(t, f.allowed(100, 100), "another entity at the carrier's position still blocks placement")
}

func TestLiftPlacementIgnoresNonBlockingCandidates(t *testing.T) {
	for _, scenario := range []string{"phantom", "stale handle", "missing transform", "missing collider"} {
		t.Run(scenario, func(t *testing.T) {
			f := newLiftPlacementFixture(0)
			handle := spawnLiftPlacementCollider(f.w, 100, 100, 100, components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: 1, Mask: 1})
			switch scenario {
			case "phantom":
				ecs.WithComponent(f.w, handle, func(c *components.Collider) {
					c.Phantom = &components.PhantomCollider{WorldX: 100, WorldY: 100, HalfWidth: 6, HalfHeight: 6}
				})
			case "stale handle":
				require.True(t, f.w.Despawn(handle))
				replacement := f.w.Spawn(101, nil)
				require.NotEqual(t, handle, replacement)
				require.False(t, f.w.Alive(handle))
			case "missing transform":
				ecs.RemoveComponent[components.Transform](f.w, handle)
			case "missing collider":
				ecs.RemoveComponent[components.Collider](f.w, handle)
			}
			require.NoError(t, f.spatial.Index.Upsert(handle, core.ColliderBounds{MinX: 94, MinY: 94, MaxX: 106, MaxY: 106}), "exercise candidate validation even with stale spatial data")
			require.True(t, f.allowed(100, 100))
			require.Equal(t, 1, f.spatial.Index.LastQueryStats().Candidates)
		})
	}
}

func TestLiftPlacementFindsLargeColliderAnchoredOutsidePlacement(t *testing.T) {
	f := newLiftPlacementFixture(0)
	anchorX := float64(constt.ChunkWorldSize) + 10
	spawnLiftPlacementCollider(f.w, 100, anchorX, 100, components.Collider{HalfWidth: anchorX - 99, HalfHeight: 10, Layer: 1, Mask: 1})
	require.False(t, f.allowed(100, 100), "the obstacle's bounds overlap even though its anchor is far outside the query")
	require.True(t, f.allowed(98, 100), "exact contact with a large collider remains allowed")
}

func TestLiftPlacementFailsClosedWithoutSpatialIndex(t *testing.T) {
	for _, scenario := range []string{"missing resource", "nil resource", "nil index"} {
		t.Run(scenario, func(t *testing.T) {
			f := newLiftPlacementFixture(0)
			switch scenario {
			case "missing resource":
				f.w = ecs.NewWorldWithCapacity(5000, nil, 0)
				f.service = NewLiftService(f.w, nil, nil, nil, nil)
			case "nil resource":
				ecs.SetResource(f.w, (*core.WorldColliderSpatial)(nil))
			case "nil index":
				f.spatial.Index = nil
			}
			require.False(t, f.allowed(100, 100))
		})
	}
}

func TestLiftPlacementRejectsInvalidBounds(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		x, y, halfWidth, halfHeight float64
	}{
		{"zero width", 100, 100, 0, 1},
		{"negative height", 100, 100, 1, -1},
		{"nan width", 100, 100, math.NaN(), 1},
		{"infinite height", 100, 100, 1, math.Inf(1)},
		{"nan position", math.NaN(), 100, 1, 1},
		{"infinite position", 100, math.Inf(1), 1, 1},
		{"negative infinite position", math.Inf(-1), 100, 1, 1},
		{"collapsed bounds", 1e100, 100, 1, 1},
		{"overflowing bounds", 1e308, 100, 1e308, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLiftPlacementFixture(0)
			f.lifted.OriginalCollider.HalfWidth, f.lifted.OriginalCollider.HalfHeight = tc.halfWidth, tc.halfHeight
			require.False(t, f.allowed(tc.x, tc.y))
		})
	}
}

func TestLiftPlacementRejectsIncompleteSpatialQueries(t *testing.T) {
	t.Run("visit budget", func(t *testing.T) {
		f := newLiftPlacementFixture(0)
		collider := components.Collider{HalfWidth: 1, HalfHeight: 1}
		for i := 0; i < liftPlacementMaxVisits; i++ {
			spawnLiftPlacementCollider(f.w, types.EntityID(i+100), 100, 100, collider)
		}
		require.True(t, f.allowed(100, 100), "a complete query with only inactive colliders is allowed")
		require.Equal(t, liftPlacementMaxVisits, f.spatial.Index.LastQueryStats().Visits)
		spawnLiftPlacementCollider(f.w, types.EntityID(liftPlacementMaxVisits+100), 100, 100, collider)
		require.False(t, f.allowed(100, 100), "an incomplete query must not allow placement based on partial results")
		require.Equal(t, liftPlacementMaxVisits, f.spatial.Index.LastQueryStats().Visits)
	})
	t.Run("cell budget", func(t *testing.T) {
		f := newLiftPlacementFixture(0)
		collider := components.Collider{HalfWidth: 1, HalfHeight: 1}
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				spawnLiftPlacementCollider(f.w, types.EntityID(y*16+x+100), float64(16+32*x), float64(16+32*y), collider)
			}
		}
		f.lifted.OriginalCollider.HalfWidth, f.lifted.OriginalCollider.HalfHeight = 520, 520
		require.True(t, f.allowed(512, 512))
		require.Equal(t, liftPlacementMaxCells, f.spatial.Index.LastQueryStats().Cells)
		spawnLiftPlacementCollider(f.w, 1000, 528, 16, collider)
		require.False(t, f.allowed(512, 512), "all colliders are inactive, but the query still must complete")
		require.LessOrEqual(t, f.spatial.Index.LastQueryStats().Cells, liftPlacementMaxCells)
	})
}

func TestLiftPlacementDoesNotAllocate(t *testing.T) {
	f := newLiftPlacementFixture(0)
	spawnLiftPlacementCollider(f.w, 100, 100, 100, components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: 1, Mask: 1})
	for _, tc := range []struct {
		name    string
		x       float64
		allowed bool
	}{
		{"overlap", 100, false},
		{"edge contact", 107, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Zero(t, testing.AllocsPerRun(1000, func() {
				if f.allowed(tc.x, 100) != tc.allowed {
					panic("unexpected placement decision")
				}
			}))
		})
	}
}

func BenchmarkLiftPlacementCollision(b *testing.B) {
	f := newLiftPlacementFixture(0)
	spawnLiftPlacementCollider(f.w, 100, 100, 100, components.Collider{HalfWidth: 6, HalfHeight: 6, Layer: 1, Mask: 1})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !f.allowed(107, 100) {
			b.Fatal("edge contact must remain allowed")
		}
	}
}
