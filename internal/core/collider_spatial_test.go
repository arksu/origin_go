package core

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestColliderSpatialLifecycle(t *testing.T) {
	index := NewColliderSpatialIndex()
	for _, handle := range []types.Handle{1, 2, 3} {
		require.NoError(t, index.Upsert(handle, ColliderBounds{-10, -10, 10, 10}))
		require.LessOrEqual(t, int(index.entries[handle].count), 4)
	}
	index.Remove(2)
	index.Remove(1)
	require.NoError(t, index.Upsert(3, ColliderBounds{100, 100, 110, 110}))
	hits, err := index.QueryInto(ColliderBounds{-11, -11, 11, 11}, nil)
	require.NoError(t, err)
	require.Empty(t, hits)
	require.NoError(t, index.Upsert(3, ColliderBounds{-2000, -2000, 2000, 2000}))
	require.LessOrEqual(t, int(index.entries[3].count), 4)
	hits, err = index.QueryInto(ColliderBounds{-11, -11, 11, 11}, hits[:0])
	require.NoError(t, err)
	require.Equal(t, []types.Handle{3}, hits)
	require.Error(t, index.Upsert(3, ColliderBounds{MinX: math.NaN()}))
	index.stamp = math.MaxUint64
	hits, err = index.QueryInto(ColliderBounds{-1e38, -1e38, 1e38, 1e38}, hits[:0])
	require.NoError(t, err)
	require.Equal(t, []types.Handle{3}, hits)
	index.Remove(3)
	require.Zero(t, index.occupied)
	require.Zero(t, index.entryCount)
}

func TestWorldColliderSpatialSynchronization(t *testing.T) {
	world := ecs.NewWorldForTesting()
	spatial := AttachColliderSpatial(world)
	actor := world.Spawn(1, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(world, handle, components.Collider{HalfWidth: 5, HalfHeight: 5})
	})
	query := func() []types.Handle {
		hits, err := spatial.Index.QueryInto(ColliderBounds{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10}, nil)
		require.NoError(t, err)
		return hits
	}
	require.Empty(t, query())
	ecs.WithComponent(world, actor, func(collider *components.Collider) { collider.HalfWidth = 100; collider.HalfHeight = 100 })
	require.Equal(t, []types.Handle{actor}, query())
	ecs.RemoveComponent[components.Collider](world, actor)
	require.Empty(t, query())
	ecs.AddComponent(world, actor, components.Collider{HalfWidth: 5, HalfHeight: 5})
	// The movement pipeline writes its cached storage before notifying observers.
	ecs.GetOrCreateStorage[components.Transform](world).Set(actor, components.Transform{X: 5, Y: 5})
	spatial.OnPositionCommitted(actor, 5, 5)
	require.Equal(t, []types.Handle{actor}, query())
	require.True(t, world.Despawn(actor))
	require.Empty(t, query())
	replacement := world.Spawn(2, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Collider{HalfWidth: 5, HalfHeight: 5})
		ecs.AddComponent(world, handle, components.Transform{X: 5, Y: 5})
	})
	require.NotEqual(t, actor, replacement)
	require.Equal(t, []types.Handle{replacement}, query())
}

func TestColliderSpatialMatchesBoundsAfterUpdates(t *testing.T) {
	index := NewColliderSpatialIndex()
	random := rand.New(rand.NewPCG(4, 7))
	model := make(map[types.Handle]ColliderBounds)
	var boundedBuffer [32]types.Handle
	for iteration := 0; iteration < 500; iteration++ {
		handle := types.Handle(random.IntN(30) + 1)
		if random.IntN(4) == 0 {
			index.Remove(handle)
			delete(model, handle)
		} else {
			centerX, centerY := float64(random.IntN(400)-200), float64(random.IntN(400)-200)
			halfWidth, halfHeight := float64(random.IntN(200)+1), float64(random.IntN(200)+1)
			bounds := ColliderBounds{centerX - halfWidth, centerY - halfHeight, centerX + halfWidth, centerY + halfHeight}
			require.NoError(t, index.Upsert(handle, bounds))
			require.LessOrEqual(t, int(index.entry(handle).count), 4)
			model[handle] = bounds
		}
		query := ColliderBounds{MinX: -16, MinY: -16, MaxX: 16, MaxY: 16}
		expected := []types.Handle{}
		for candidate, bounds := range model {
			if bounds.MinX <= query.MaxX && bounds.MaxX >= query.MinX && bounds.MinY <= query.MaxY && bounds.MaxY >= query.MinY {
				expected = append(expected, candidate)
			}
		}
		actual, err := index.QueryInto(query, nil)
		require.NoError(t, err)
		require.ElementsMatch(t, expected, actual)
		bounded, err := index.QueryBoundedInto(query, boundedBuffer[:0], ColliderQueryBudget{MaxCells: 1024, MaxVisits: 4096})
		require.NoError(t, err)
		require.ElementsMatch(t, expected, bounded)
		assertColliderDenseCells(t, index)
	}
}

func assertColliderDenseCells(t *testing.T, index *ColliderSpatialIndex) {
	t.Helper()
	for _, grid := range index.levels {
		if grid == nil {
			continue
		}
		require.Len(t, grid.occupied, len(grid.cells))
		for position, cell := range grid.occupied {
			bucket, exists := grid.cells[cell]
			require.True(t, exists, "occupied cells must be dense and contain only live keys")
			require.Equal(t, position, bucket.position)
			require.NotEmpty(t, bucket.handles)
		}
	}
}

func TestColliderSpatialBoundedFallbackAfterHistoricalChurn(t *testing.T) {
	index := NewColliderSpatialIndex()
	for handle := types.Handle(1); handle <= 100001; handle++ {
		x := float64(handle) * 32
		require.NoError(t, index.Upsert(handle, ColliderBounds{x, 1, x, 1}))
	}
	for handle := types.Handle(2); handle <= 100001; handle++ {
		index.Remove(handle)
	}
	assertColliderDenseCells(t, index)
	require.Len(t, index.levels[0].occupied, 1)
	query := ColliderBounds{-1e100, -1e100, 1e100, 1e100}
	budget := ColliderQueryBudget{MaxCells: 1, MaxVisits: 1}
	var buffer [1]types.Handle
	hits, err := index.QueryBoundedInto(query, buffer[:0], budget)
	require.NoError(t, err)
	require.Equal(t, []types.Handle{1}, hits)
	require.Equal(t, ColliderQueryStats{Cells: 1, Candidates: 1, Visits: 1}, index.LastQueryStats())
	require.Zero(t, testing.AllocsPerRun(100, func() {
		_, _ = index.QueryBoundedInto(query, buffer[:0], budget)
	}))

	// Reuse the grown level and delete its first key, exercising swap removal.
	require.NoError(t, index.Upsert(100002, ColliderBounds{10000000, 1, 10000000, 1}))
	index.Remove(1)
	assertColliderDenseCells(t, index)
	hits, err = index.QueryBoundedInto(query, buffer[:0], budget)
	require.NoError(t, err)
	require.Equal(t, []types.Handle{100002}, hits)
	index.Remove(100002)
	require.Nil(t, index.levels[0])
	require.Zero(t, index.occupied)
}

func TestColliderSpatialBoundedQueryBudgets(t *testing.T) {
	index := NewColliderSpatialIndex()
	bounds := ColliderBounds{-1, -1, 1, 1}
	require.NoError(t, index.Upsert(1, bounds))
	buffer := make([]types.Handle, 0, 4)
	hits, err := index.QueryBoundedInto(bounds, buffer, ColliderQueryBudget{MaxCells: 4, MaxVisits: 4})
	require.NoError(t, err)
	require.Equal(t, []types.Handle{1}, hits)
	require.Equal(t, ColliderQueryStats{Cells: 4, Candidates: 1, Visits: 4}, index.LastQueryStats())

	// Duplicate memberships consume work even though only one unique candidate exists.
	hits, err = index.QueryBoundedInto(bounds, buffer, ColliderQueryBudget{MaxCells: 4, MaxVisits: 3})
	require.ErrorIs(t, err, ErrColliderQueryBudget)
	require.Empty(t, hits)
	require.Equal(t, 3, index.LastQueryStats().Visits)
	hits, err = index.QueryBoundedInto(bounds, buffer, ColliderQueryBudget{MaxCells: 3, MaxVisits: 4})
	require.ErrorIs(t, err, ErrColliderQueryBudget)
	require.Empty(t, hits)
	require.LessOrEqual(t, index.LastQueryStats().Cells, 3)

	// A much larger range falls back to scanning occupied cells, which is bounded too.
	wide := ColliderBounds{-1e100, -1e100, 1e100, 1e100}
	hits, err = index.QueryBoundedInto(wide, buffer, ColliderQueryBudget{MaxCells: 4, MaxVisits: 4})
	require.NoError(t, err)
	require.Equal(t, []types.Handle{1}, hits)
	require.Equal(t, ColliderQueryStats{Cells: 4, Candidates: 1, Visits: 4}, index.LastQueryStats())
	hits, err = index.QueryBoundedInto(wide, buffer, ColliderQueryBudget{MaxCells: 3, MaxVisits: 4})
	require.ErrorIs(t, err, ErrColliderQueryBudget)
	require.Empty(t, hits)
	hits, err = index.QueryBoundedInto(wide, buffer, ColliderQueryBudget{MaxCells: 4, MaxVisits: 3})
	require.ErrorIs(t, err, ErrColliderQueryBudget)
	require.Empty(t, hits)
	require.Equal(t, 3, index.LastQueryStats().Visits)
}

func TestColliderSpatialBoundedQueryExactLimits(t *testing.T) {
	index := NewColliderSpatialIndex()
	for handle := types.Handle(1); handle <= 1024; handle++ {
		x := float64(handle) * 16
		require.NoError(t, index.Upsert(handle, ColliderBounds{x, 1, x, 1}))
	}
	wide := ColliderBounds{-1e100, -1e100, 1e100, 1e100}
	buffer := make([]types.Handle, 0, 4096)
	hits, err := index.QueryBoundedInto(wide, buffer, ColliderQueryBudget{MaxCells: 1024, MaxVisits: 4096})
	require.NoError(t, err)
	require.Len(t, hits, 1024)
	require.Equal(t, 1024, index.LastQueryStats().Cells)
	require.NoError(t, index.Upsert(1025, ColliderBounds{20000, 1, 20000, 1}))
	hits, err = index.QueryBoundedInto(wide, buffer, ColliderQueryBudget{MaxCells: 1024, MaxVisits: 4096})
	require.ErrorIs(t, err, ErrColliderQueryBudget)
	require.Empty(t, hits)
	require.LessOrEqual(t, index.LastQueryStats().Cells, 1024)

	dense := NewColliderSpatialIndex()
	for handle := types.Handle(1); handle <= 4096; handle++ {
		require.NoError(t, dense.Upsert(handle, ColliderBounds{1, 1, 1, 1}))
	}
	hits, err = dense.QueryBoundedInto(wide, buffer, ColliderQueryBudget{MaxCells: 1024, MaxVisits: 4096})
	require.NoError(t, err)
	require.Len(t, hits, 4096)
	require.Equal(t, 4096, dense.LastQueryStats().Visits)
	require.NoError(t, dense.Upsert(4097, ColliderBounds{1, 1, 1, 1}))
	hits, err = dense.QueryBoundedInto(wide, buffer, ColliderQueryBudget{MaxCells: 1024, MaxVisits: 4096})
	require.ErrorIs(t, err, ErrColliderQueryBudget)
	require.Empty(t, hits)
	require.Equal(t, 4096, dense.LastQueryStats().Visits)
}

func TestColliderSpatialBoundedQueryCapacityAndErrors(t *testing.T) {
	index := NewColliderSpatialIndex()
	bounds := ColliderBounds{1, 1, 1, 1}
	require.NoError(t, index.Upsert(1, bounds))
	budget := ColliderQueryBudget{MaxCells: 1024, MaxVisits: 4096}
	prefix := make([]types.Handle, 1, 2)
	prefix[0] = 42
	hits, err := index.QueryBoundedInto(bounds, prefix, budget)
	require.NoError(t, err)
	require.Equal(t, []types.Handle{42, 1}, hits)
	hits, err = index.QueryBoundedInto(bounds, prefix[:1:1], budget)
	require.ErrorIs(t, err, ErrColliderQueryCapacity)
	require.Equal(t, []types.Handle{42}, hits)

	for _, invalid := range []ColliderBounds{{MinX: math.NaN()}, {MaxY: math.Inf(1)}, {MinX: 1, MaxX: 0}} {
		hits, err = index.QueryBoundedInto(invalid, prefix, budget)
		require.ErrorIs(t, err, ErrInvalidColliderQuery)
		require.Equal(t, []types.Handle{42}, hits)
	}
	_, err = index.QueryBoundedInto(bounds, prefix, ColliderQueryBudget{})
	require.ErrorIs(t, err, ErrInvalidColliderQuery)
	index.stamp = math.MaxUint64
	hits, err = index.QueryBoundedInto(bounds, prefix, budget)
	require.ErrorIs(t, err, ErrColliderQueryStamp)
	require.Equal(t, []types.Handle{42}, hits)
	require.Zero(t, index.LastQueryStats().Cells)

	for _, scenario := range []struct {
		name         string
		bounds       ColliderBounds
		budget       ColliderQueryBudget
		capacity     int
		exhaustStamp bool
	}{
		{name: "success", bounds: bounds, budget: budget, capacity: 4},
		{name: "invalid", bounds: ColliderBounds{MinX: math.NaN()}, budget: budget, capacity: 4},
		{name: "capacity", bounds: bounds, budget: budget},
		{name: "visits", bounds: ColliderBounds{-100, -100, 100, 100}, budget: ColliderQueryBudget{MaxCells: 1024, MaxVisits: 1}, capacity: 4},
		{name: "cells", bounds: ColliderBounds{-100, -100, 100, 100}, budget: ColliderQueryBudget{MaxCells: 1, MaxVisits: 4096}, capacity: 4},
		{name: "stamp", bounds: bounds, budget: budget, capacity: 4, exhaustStamp: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			spatial := NewColliderSpatialIndex()
			require.NoError(t, spatial.Upsert(1, ColliderBounds{-1, -1, 1, 1}))
			buffer := make([]types.Handle, 0, scenario.capacity)
			if scenario.exhaustStamp {
				spatial.stamp = math.MaxUint64
			}
			require.Zero(t, testing.AllocsPerRun(100, func() {
				_, _ = spatial.QueryBoundedInto(scenario.bounds, buffer, scenario.budget)
			}))
		})
	}
}

func BenchmarkColliderSpatialLocality(b *testing.B) {
	for _, population := range []int{1000, 100000} {
		b.Run(strconv.Itoa(population), func(b *testing.B) {
			index := NewColliderSpatialIndex()
			for entity := 1; entity <= population; entity++ {
				centerX, centerY := float64(entity%1000)*32+10000, float64(entity/1000)*32+10000
				if entity <= 40 {
					centerX, centerY = float64(entity%8)*12, float64(entity/8)*12
				}
				if err := index.Upsert(types.Handle(entity), ColliderBounds{centerX - 5, centerY - 5, centerX + 5, centerY + 5}); err != nil {
					b.Fatal(err)
				}
			}
			query := ColliderBounds{-18, -18, 18, 18}
			buffer := make([]types.Handle, 0, 64)
			var err error
			buffer, err = index.QueryInto(query, buffer[:0])
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				var err error
				buffer, err = index.QueryInto(query, buffer[:0])
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(index.LastQueryStats().Cells), "cells/op")
			b.ReportMetric(float64(index.LastQueryStats().Candidates), "candidates/op")
		})
	}
}

func BenchmarkColliderSpatialMemberships(b *testing.B) {
	for _, halfSize := range []float64{5, 2000} {
		b.Run(strconv.FormatFloat(halfSize, 'f', 0, 64), func(b *testing.B) {
			index := NewColliderSpatialIndex()
			bounds := ColliderBounds{-halfSize, -halfSize, halfSize, halfSize}
			if err := index.Upsert(1, bounds); err != nil {
				b.Fatal(err)
			}
			memberships := index.entry(1).count
			index.Remove(1)
			if index.occupied != 0 || index.entryCount != 0 {
				b.Fatal("removal left occupied cells")
			}
			if err := index.Upsert(1, bounds); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if err := index.Upsert(1, bounds); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(memberships), "memberships/entity")
		})
	}
}

// Leave one occupied cell in a level whose map has seen substantial churn. The
// query takes the fallback path, so setup cost and removed colliders cannot
// legitimately change its bounded work or steady-state allocations.
func BenchmarkColliderSpatialBoundedFallbackChurn(b *testing.B) {
	for _, removed := range []int{0, 100000} {
		b.Run(strconv.Itoa(removed), func(b *testing.B) {
			index := NewColliderSpatialIndex()
			if err := index.Upsert(1, ColliderBounds{1, 1, 1, 1}); err != nil {
				b.Fatal(err)
			}
			for entity := 2; entity <= removed+1; entity++ {
				x := float64(entity) * 32
				if err := index.Upsert(types.Handle(entity), ColliderBounds{x, 1, x, 1}); err != nil {
					b.Fatal(err)
				}
			}
			for entity := 2; entity <= removed+1; entity++ {
				index.Remove(types.Handle(entity))
			}
			query := ColliderBounds{-1e100, -1e100, 1e100, 1e100}
			budget := ColliderQueryBudget{MaxCells: 1, MaxVisits: 1}
			buffer := make([]types.Handle, 0, 1)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				var err error
				buffer, err = index.QueryBoundedInto(query, buffer[:0], budget)
				if err != nil || len(buffer) != 1 || buffer[0] != 1 {
					b.Fatalf("unexpected bounded fallback result: %v, %v", buffer, err)
				}
			}
			b.ReportMetric(float64(index.LastQueryStats().Cells), "cells/op")
			b.ReportMetric(float64(index.LastQueryStats().Visits), "visits/op")
		})
	}
}

func BenchmarkColliderSpatialCellTransitions(b *testing.B) {
	index := NewColliderSpatialIndex()
	for entity := 1; entity <= 1000; entity++ {
		x := float64(entity)*32 + 10000
		if err := index.Upsert(types.Handle(entity), ColliderBounds{x - 5, 10000 - 5, x + 5, 10000 + 5}); err != nil {
			b.Fatal(err)
		}
	}
	bounds := [2]ColliderBounds{{-4, -4, 6, 6}, {28, -4, 38, 6}}
	for _, current := range bounds {
		if err := index.Upsert(1, current); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if err := index.Upsert(1, bounds[iteration&1]); err != nil {
			b.Fatal(err)
		}
	}
}
