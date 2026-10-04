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
