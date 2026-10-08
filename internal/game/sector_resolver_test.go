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

func TestSectorResolverBoundedContactsAndIdentity(t *testing.T) {
	world := ecs.NewWorldForTesting()
	core.AttachColliderSpatial(world)
	attacker := spawnSectorCollider(world, 1, 0, 0, 5, 5)
	far := spawnSectorCollider(world, 2, 23, 0, 5, 5)
	spawnSectorCollider(world, 4, 10, 0, 1, 1)
	spawnSectorCollider(world, 3, 10, 0, 1, 1)
	duplicate := spawnSectorCollider(world, 5, 1, 0, 1, 1)
	ecs.AddComponent(world, duplicate, ecs.ExternalID{ID: 1})
	resolver, err := NewSectorResolver(world)
	require.NoError(t, err)
	sector := actiondefs.Sector{Range: 18, Angle: math.Pi / 2}
	buffer := make([]SectorHit, 0, MeleeMaxContacts)
	hits, err := resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	require.NoError(t, err)
	require.Len(t, hits, 3)
	require.Equal(t, []types.EntityID{3, 4, 2}, []types.EntityID{hits[0].EntityID, hits[1].EntityID, hits[2].EntityID})
	require.InDelta(t, 18, hits[2].Distance, 1e-8)
	ecs.WithComponent(world, far, func(transform *components.Transform) { transform.X = 100 })
	hits, err = resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	require.NoError(t, err)
	require.Len(t, hits, 2)
	_, err = resolver.ResolveBoundedInto(attacker, 0, sector, make([]SectorHit, 0, 1))
	require.ErrorIs(t, err, ErrMeleeContactCapacity)

	ecs.AddComponent(world, attacker, ecs.ExternalID{ID: 99})
	hits, err = resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	require.ErrorIs(t, err, ErrInvalidMeleeAttacker)
	require.Empty(t, hits)
	ecs.AddComponent(world, attacker, ecs.ExternalID{ID: 1})
	// Inject corruption without the collider observer, which correctly rejects
	// nonfinite positions at the authoritative mutation boundary.
	ecs.GetOrCreateStorage[components.Transform](world).Set(attacker, components.Transform{X: math.NaN()})
	_, err = resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	require.ErrorIs(t, err, ErrInvalidMeleeAttacker)
}

func TestSectorResolverBoundedRawVisitOverflow(t *testing.T) {
	// The world's membership padding puts each of these small colliders in four
	// cells. The limit is raw visits, not the number of unique target handles.
	const collidersAtLimit = MeleeMaxQueryVisits / 4
	world := ecs.NewWorldWithCapacity(collidersAtLimit+2, nil, 0)
	spatial := core.AttachColliderSpatial(world)
	attacker := spawnSectorCollider(world, 1, 1, 1, 0.1, 0.1)
	for entity := 2; entity <= collidersAtLimit; entity++ {
		spawnSectorCollider(world, types.EntityID(entity), 2, 1, 0.1, 0.1)
	}
	resolver, err := NewSectorResolver(world)
	require.NoError(t, err)
	sector := actiondefs.Sector{Range: 18, Angle: math.Pi / 2}
	buffer := make([]SectorHit, 0, MeleeMaxContacts)
	hits, err := resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	require.NoError(t, err)
	require.Len(t, hits, collidersAtLimit-1)
	require.Equal(t, MeleeMaxQueryVisits, spatial.Index.LastQueryStats().Visits)
	spawnSectorCollider(world, collidersAtLimit+1, 2, 1, 0.1, 0.1)
	hits, err = resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	require.ErrorIs(t, err, core.ErrColliderQueryBudget)
	require.Empty(t, hits)
	require.Equal(t, MeleeMaxQueryVisits, spatial.Index.LastQueryStats().Visits)
	require.Zero(t, testing.AllocsPerRun(100, func() {
		_, _ = resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	}))
}

func TestSectorResolverBoundedRejectsMalformedContacts(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		corrupt func(*ecs.World, types.Handle)
	}{
		{"missing identity", func(w *ecs.World, h types.Handle) { ecs.RemoveComponent[ecs.ExternalID](w, h) }},
		{"zero identity", func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, ecs.ExternalID{}) }},
		{"identity mapping", func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, ecs.ExternalID{ID: 99}) }},
		{"nonfinite position", func(w *ecs.World, h types.Handle) {
			ecs.GetOrCreateStorage[components.Transform](w).Set(h, components.Transform{X: math.NaN()})
		}},
		{"nonfinite collider", func(w *ecs.World, h types.Handle) {
			ecs.GetOrCreateStorage[components.Collider](w).Set(h, components.Collider{HalfWidth: math.Inf(1)})
		}},
		{"negative collider", func(w *ecs.World, h types.Handle) {
			ecs.GetOrCreateStorage[components.Collider](w).Set(h, components.Collider{HalfWidth: -1})
		}},
		{"zero collider", func(w *ecs.World, h types.Handle) {
			ecs.GetOrCreateStorage[components.Collider](w).Set(h, components.Collider{})
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			core.AttachColliderSpatial(world)
			actor := spawnSectorCollider(world, 1, 0, 0, 1, 1)
			target := spawnSectorCollider(world, 2, 10, 0, 1, 1)
			scenario.corrupt(world, target)
			resolver, err := NewSectorResolver(world)
			require.NoError(t, err)
			buffer := make([]SectorHit, 0, MeleeMaxContacts)
			sector := actiondefs.Sector{Range: 18, Angle: math.Pi / 2}
			hits, err := resolver.ResolveBoundedInto(actor, 0, sector, buffer)
			require.ErrorIs(t, err, ErrInvalidMeleeTarget)
			require.Empty(t, hits)
			require.Zero(t, testing.AllocsPerRun(100, func() {
				_, _ = resolver.ResolveBoundedInto(actor, 0, sector, buffer)
			}))
		})
	}

	world := ecs.NewWorldForTesting()
	core.AttachColliderSpatial(world)
	actor := spawnSectorCollider(world, 1, 0, 0, 1, 1)
	target := spawnSectorCollider(world, 2, 0, 12, 1, 1)
	ecs.RemoveComponent[ecs.ExternalID](world, target)
	resolver, err := NewSectorResolver(world)
	require.NoError(t, err)
	hits, err := resolver.ResolveBoundedInto(actor, 0, actiondefs.Sector{Range: 18, Angle: math.Pi / 2}, make([]SectorHit, 0, MeleeMaxContacts))
	require.NoError(t, err) // Malformed identity outside the sector is irrelevant.
	require.Empty(t, hits)
}

func TestSectorResolverBoundedAllocationsAndStaleActor(t *testing.T) {
	world := ecs.NewWorldForTesting()
	core.AttachColliderSpatial(world)
	attacker := spawnSectorCollider(world, 1, 0, 0, 5, 5)
	spawnSectorCollider(world, 2, 10, 0, 1, 1)
	resolver, err := NewSectorResolver(world)
	require.NoError(t, err)
	sector := actiondefs.Sector{Range: 18, Angle: math.Pi / 2}
	buffer := make([]SectorHit, 0, MeleeMaxContacts)
	for _, scenario := range []struct {
		name     string
		angle    float64
		capacity int
	}{
		{"success", 0, MeleeMaxContacts},
		{"invalid angle", math.NaN(), MeleeMaxContacts},
		{"contact capacity", 0, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			contacts := buffer[:0:scenario.capacity]
			require.Zero(t, testing.AllocsPerRun(100, func() {
				_, _ = resolver.ResolveBoundedInto(attacker, scenario.angle, sector, contacts)
			}))
		})
	}
	require.True(t, world.Despawn(attacker))
	spawnSectorCollider(world, 3, 0, 0, 5, 5)
	hits, err := resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	require.ErrorIs(t, err, ErrInvalidMeleeAttacker)
	require.Empty(t, hits)
	require.Zero(t, testing.AllocsPerRun(100, func() {
		_, _ = resolver.ResolveBoundedInto(attacker, 0, sector, buffer)
	}))
}

func TestSectorResolverBoundedLocality(t *testing.T) {
	var expected core.ColliderQueryStats
	for _, population := range []int{1000, 100000} {
		world := ecs.NewWorldWithCapacity(uint32(population+2), nil, 0)
		spatial := core.AttachColliderSpatial(world)
		attacker := spawnSectorCollider(world, 1, 0, 0, 5, 5)
		spawnSectorCollider(world, 2, 10, 0, 1, 1)
		for entity := 3; entity <= population; entity++ {
			spawnSectorCollider(world, types.EntityID(entity), float64(entity%1000)*32+10000, float64(entity/1000)*32+10000, 5, 5)
		}
		resolver, err := NewSectorResolver(world)
		require.NoError(t, err)
		hits, err := resolver.ResolveBoundedInto(attacker, 0, actiondefs.Sector{Range: 18, Angle: math.Pi / 2}, make([]SectorHit, 0, MeleeMaxContacts))
		require.NoError(t, err)
		require.Len(t, hits, 1)
		stats := spatial.Index.LastQueryStats()
		if population == 1000 {
			expected = stats
		} else {
			require.Equal(t, expected, stats)
		}
		require.LessOrEqual(t, stats.Cells, MeleeMaxQueryCells)
		require.LessOrEqual(t, stats.Visits, MeleeMaxQueryVisits)
	}
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

func BenchmarkSectorResolverBounded(b *testing.B) {
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
			buffer := make([]SectorHit, 0, MeleeMaxContacts)
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if _, err := resolver.ResolveBoundedInto(attacker, 0, sector, buffer); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(spatial.Index.LastQueryStats().Cells), "cells/op")
			b.ReportMetric(float64(spatial.Index.LastQueryStats().Candidates), "candidates/op")
			b.ReportMetric(float64(spatial.Index.LastQueryStats().Visits), "visits/op")
		})
	}
}
