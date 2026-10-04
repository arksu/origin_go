package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"origin/internal/actiondefs"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func spawnSectorCollider(world *ecs.World, id types.EntityID, centerX, centerY, halfWidth, halfHeight float64) types.Handle {
	return world.Spawn(id, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: centerX, Y: centerY})
		ecs.AddComponent(world, handle, components.Collider{HalfWidth: halfWidth, HalfHeight: halfHeight})
	})
}

func TestSectorResolverContacts(t *testing.T) {
	for _, scenario := range []struct {
		name                                                string
		centerX, centerY, halfWidth, halfHeight, angle, aim float64
		contact                                             bool
		distance                                            float64
	}{
		{"center outside range", 23, 0, 5, 5, math.Pi / 2, 0, true, 18},
		{"center outside angle", 10, 15, 5, 5, math.Pi / 2, 0, true, math.Sqrt(200)},
		{"angle boundary", 5, 7, 1, 1, math.Pi / 2, 0, true, math.Sqrt(72)},
		{"outside", 10, 30, 5, 5, math.Pi / 2, 0, false, 0},
		{"nearest point outside wedge", 5, 12, 3, 5, math.Pi / 2, 0, true, math.Sqrt(98)},
		{"wide sector", -5, 8, 1, 1, 3 * math.Pi / 2, 0, true, math.Sqrt(65)},
		{"circle", -10, 0, 1, 1, 2 * math.Pi, 0, true, 9},
		{"rotated", 0, 10, 1, 1, math.Pi / 2, math.Pi / 2, true, 9},
		{"origin inside", 0, 0, 2, 2, math.Pi / 2, 0, true, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			core.AttachColliderSpatial(world)
			attacker := spawnSectorCollider(world, 1, 0, 0, 5, 5)
			spawnSectorCollider(world, 2, scenario.centerX, scenario.centerY, scenario.halfWidth, scenario.halfHeight)
			resolver, err := NewSectorResolver(world)
			require.NoError(t, err)
			hits, err := resolver.ResolveInto(attacker, scenario.aim, actiondefs.Sector{Range: 18, Angle: scenario.angle}, nil)
			require.NoError(t, err)
			if !scenario.contact {
				require.Empty(t, hits)
				return
			}
			require.Len(t, hits, 1)
			require.Equal(t, types.EntityID(2), hits[0].EntityID)
			require.InDelta(t, scenario.distance, hits[0].Distance, 1e-8)
		})
	}
}

func TestSectorResolverOrderingAndLifecycle(t *testing.T) {
	world := ecs.NewWorldForTesting()
	core.AttachColliderSpatial(world)
	attacker := spawnSectorCollider(world, 1, 1535, 0, 5, 5)
	far := spawnSectorCollider(world, 2, 1555, 0, 5, 5)
	spawnSectorCollider(world, 4, 1545, 0, 5, 5)
	spawnSectorCollider(world, 3, 1545, 0, 5, 5)
	resolver, err := NewSectorResolver(world)
	require.NoError(t, err)
	sector := actiondefs.Sector{Range: 18, Angle: math.Pi / 2}
	hits, err := resolver.ResolveInto(attacker, 0, sector, make([]SectorHit, 0, 8))
	require.NoError(t, err)
	require.Len(t, hits, 3)
	require.Equal(t, []types.EntityID{3, 4, 2}, []types.EntityID{hits[0].EntityID, hits[1].EntityID, hits[2].EntityID})
	ecs.WithComponent(world, far, func(transform *components.Transform) { transform.X = 1600 })
	hits, err = resolver.ResolveInto(attacker, 0, sector, hits)
	require.NoError(t, err)
	require.Len(t, hits, 2)
	for _, angle := range []float64{math.NaN(), math.Inf(1), -1, 2 * math.Pi} {
		_, err = resolver.ResolveInto(attacker, angle, sector, hits)
		require.Error(t, err)
	}
	require.Zero(t, testing.AllocsPerRun(100, func() {
		var resolveErr error
		hits, resolveErr = resolver.ResolveInto(attacker, 0, sector, hits)
		if resolveErr != nil {
			panic(resolveErr)
		}
	}))
}

func BenchmarkSectorResolver(b *testing.B) {
	for _, population := range []int{1000, 100000} {
		b.Run(fmt.Sprint(population), func(b *testing.B) {
			world := ecs.NewWorldWithCapacity(uint32(population+1), nil, 0)
			spatial := core.AttachColliderSpatial(world)
			attacker := spawnSectorCollider(world, 1, 0, 0, 5, 5)
			for entity := 2; entity <= population; entity++ {
				centerX, centerY := float64(entity%1000)*32+10000, float64(entity/1000)*32+10000
				if entity <= 40 {
					centerX, centerY = float64(entity%8)*12, float64(entity/8)*12
				}
				spawnSectorCollider(world, types.EntityID(entity), centerX, centerY, 5, 5)
			}
			resolver, err := NewSectorResolver(world)
			if err != nil {
				b.Fatal(err)
			}
			sector := actiondefs.Sector{Range: 18, Angle: math.Pi / 2}
			hits := make([]SectorHit, 0, 64)
			if hits, err = resolver.ResolveInto(attacker, 0, sector, hits); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				hits, err = resolver.ResolveInto(attacker, 0, sector, hits)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(spatial.Index.LastQueryStats().Cells), "cells/op")
			b.ReportMetric(float64(spatial.Index.LastQueryStats().Candidates), "candidates/op")
		})
	}
}
