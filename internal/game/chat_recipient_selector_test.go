package game

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type chatSelectorFixture struct {
	shard     *Shard
	world     *ecs.World
	connected map[types.EntityID]bool
}

func newChatSelectorFixture(tb testing.TB, capacity int, cellSize float64) *chatSelectorFixture {
	tb.Helper()
	w := ecs.NewWorldWithCapacity(uint32(capacity), nil, 0)
	return &chatSelectorFixture{
		shard:     &Shard{world: w, soundEvents: &SoundEventService{listeners: newSoundListenerIndex(cellSize)}},
		world:     w,
		connected: make(map[types.EntityID]bool),
	}
}

func (fixture *chatSelectorFixture) add(tb testing.TB, id types.EntityID, x, y float64, connected bool) types.Handle {
	tb.Helper()
	handle := fixture.world.Spawn(id, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.Transform{X: x, Y: y})
	})
	require.NotEqual(tb, types.InvalidHandle, handle)
	ecs.GetResource[ecs.CharacterEntities](fixture.world).Map[id] = ecs.CharacterEntity{Handle: handle}
	if connected {
		require.True(tb, fixture.shard.soundEvents.Attach(fixture.world, handle, uint64(id), 1))
		fixture.connected[id] = true
	}
	return handle
}

// This deliberately follows the former full-character scan, then applies the
// connected-client delivery filter. It does not consult the spatial grid.
func (fixture *chatSelectorFixture) fullScan(x, y, radiusSq float64, dst []types.EntityID) []types.EntityID {
	for id := range ecs.GetResource[ecs.CharacterEntities](fixture.world).Map {
		handle := fixture.world.GetHandleByEntityID(id)
		if handle == types.InvalidHandle || !fixture.world.Alive(handle) {
			continue
		}
		position, ok := ecs.GetComponent[components.Transform](fixture.world, handle)
		if !ok {
			continue
		}
		dx, dy := position.X-x, position.Y-y
		if dx*dx+dy*dy <= radiusSq && fixture.connected[id] {
			dst = append(dst, id)
		}
	}
	return dst
}

func TestLocalChatRecipientsMatchFullScan(t *testing.T) {
	fixture := newChatSelectorFixture(t, 1024, 256)
	random := rand.New(rand.NewSource(42))
	for id := types.EntityID(1); id <= 500; id++ {
		fixture.add(t, id, random.Float64()*10000-5000, random.Float64()*10000-5000, id%7 != 0)
	}
	fixture.add(t, 501, -257.25, -.5, true)
	fixture.add(t, 502, -257.25+600, -.5+800, true)
	fixture.add(t, 503, -257.25+1000, -.5, true)
	fixture.add(t, 504, -257.25+math.Nextafter(1000, math.Inf(1)), -.5, true)
	fixture.add(t, 505, -257.25, -.5, false)
	ghost := fixture.add(t, 506, -257.25, -.5, true)
	delete(ecs.GetResource[ecs.CharacterEntities](fixture.world).Map, 506)
	require.True(t, fixture.world.Alive(ghost))
	for _, test := range []struct {
		name         string
		x, y, radius float64
	}{
		{"negative_coordinates", -257.25, -.5, 1000},
		{"negative_radius", -257.25, -.5, -1000},
		{"zero_radius", -257.25, -.5, 0},
		{"cell_boundary", 256, -256, 512},
		{"positive_coordinates", 128, 128, 1000},
	} {
		t.Run(test.name, func(t *testing.T) {
			prefix := []types.EntityID{99999}
			radiusSq := test.radius * test.radius
			got := fixture.shard.AppendLocalChatRecipients(fixture.world, test.x, test.y, test.radius, radiusSq, prefix)
			require.Equal(t, types.EntityID(99999), got[0], "append must preserve caller's prefix")
			require.ElementsMatch(t, fixture.fullScan(test.x, test.y, radiusSq, nil), got[1:])
		})
	}
}

func TestLocalChatRecipientsInclusiveCircle(t *testing.T) {
	fixture := newChatSelectorFixture(t, 16, 256)
	fixture.add(t, 1, 0, 0, true)
	fixture.add(t, 2, 1000, 0, true)
	fixture.add(t, 3, -600, 800, true)
	fixture.add(t, 4, math.Nextafter(1000, math.Inf(1)), 0, true)
	fixture.add(t, 5, 900, 900, true)
	require.ElementsMatch(t, []types.EntityID{1, 2, 3}, fixture.shard.AppendLocalChatRecipients(fixture.world, 0, 0, 1000, 1000000, nil))
	require.ElementsMatch(t, []types.EntityID{1}, fixture.shard.AppendLocalChatRecipients(fixture.world, 0, 0, 0, 0, nil))
}

func TestLocalChatRecipientsRoundedDistanceBoundaries(t *testing.T) {
	for _, test := range []struct {
		name             string
		x, listenerX     float64
		radius, cellSize float64
	}{
		{"rounded_subtraction", 744, math.Nextafter(-256, math.Inf(-1)), 1000, 256},
		{"zero_square_underflow", 0, -1e-170, 0, 256},
		{"tiny_square_underflow", 0, -1e-170, 1e-180, 256},
		{"division_underflow", 0, -1e-170, 0, math.MaxFloat64},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newChatSelectorFixture(t, 8, test.cellSize)
			fixture.add(t, 1, test.listenerX, 0, true)
			radiusSq := test.radius * test.radius
			want := fixture.fullScan(test.x, 0, radiusSq, nil)
			require.Equal(t, []types.EntityID{1}, want, "former floating-point predicate accepts this boundary")
			require.Equal(t, want, fixture.shard.AppendLocalChatRecipients(fixture.world, test.x, 0, test.radius, radiusSq, nil))
		})
	}
}

func TestLocalChatRecipientsRejectWrongWorldAndEmptyIndex(t *testing.T) {
	fixture := newChatSelectorFixture(t, 8, 256)
	fixture.add(t, 1, 0, 0, true)
	other := ecs.NewWorldWithCapacity(8, nil, 1)
	for _, test := range []struct {
		name  string
		shard *Shard
		world *ecs.World
	}{
		{"other_world", fixture.shard, other},
		{"nil_world", fixture.shard, nil},
		{"nil_shard", nil, fixture.world},
		{"no_sound_service", &Shard{world: fixture.world}, fixture.world},
	} {
		t.Run(test.name, func(t *testing.T) {
			dst, stats := test.shard.appendLocalChatRecipients(test.world, 0, 0, 1000, 1000000, []types.EntityID{99})
			require.Equal(t, []types.EntityID{99}, dst)
			require.Equal(t, localChatRecipientStats{}, stats)
		})
	}
	fixture.shard.soundEvents.Detach(fixture.world.GetHandleByEntityID(1), 1)
	dst, stats := fixture.shard.appendLocalChatRecipients(fixture.world, 0, 0, 1000, 1000000, nil)
	require.Empty(t, dst)
	require.Equal(t, localChatRecipientStats{}, stats)
}

func TestLocalChatRecipientsRejectStaleIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*chatSelectorFixture, types.Handle)
	}{
		{"missing_listener", func(f *chatSelectorFixture, handle types.Handle) {
			delete(f.shard.soundEvents.listeners.members, handle)
		}},
		{"despawned_handle", func(f *chatSelectorFixture, handle types.Handle) {
			f.world.Despawn(handle)
			f.world.Spawn(20, nil)
		}},
		{"missing_transform", func(f *chatSelectorFixture, handle types.Handle) {
			ecs.RemoveComponent[components.Transform](f.world, handle)
		}},
		{"missing_external_id", func(f *chatSelectorFixture, handle types.Handle) {
			ecs.RemoveComponent[ecs.ExternalID](f.world, handle)
		}},
		{"wrong_external_id", func(f *chatSelectorFixture, handle types.Handle) {
			ecs.WithComponent(f.world, handle, func(id *ecs.ExternalID) { id.ID = 99 })
		}},
		{"not_a_character", func(f *chatSelectorFixture, _ types.Handle) {
			delete(ecs.GetResource[ecs.CharacterEntities](f.world).Map, 2)
		}},
		{"wrong_character_handle", func(f *chatSelectorFixture, _ types.Handle) {
			characters := ecs.GetResource[ecs.CharacterEntities](f.world)
			characters.Map[2] = ecs.CharacterEntity{Handle: characters.Map[1].Handle}
		}},
		{"wrong_listener_id", func(f *chatSelectorFixture, handle types.Handle) {
			f.shard.soundEvents.listeners.members[handle].entityID = 99
			ecs.GetResource[ecs.CharacterEntities](f.world).Map[99] = ecs.CharacterEntity{Handle: handle}
		}},
		{"nan_position", func(f *chatSelectorFixture, handle types.Handle) {
			ecs.WithComponent(f.world, handle, func(position *components.Transform) { position.X = math.NaN() })
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newChatSelectorFixture(t, 8, 256)
			fixture.add(t, 1, 0, 0, true)
			handle := fixture.add(t, 2, 1, 1, true)
			fixture.add(t, 3, 2, 2, true)
			test.mutate(fixture, handle)
			require.ElementsMatch(t, []types.EntityID{1, 3}, fixture.shard.AppendLocalChatRecipients(fixture.world, 0, 0, 1000, 1000000, nil))
		})
	}
}

func TestLocalChatRecipientsExtremeCellSizesAndBounds(t *testing.T) {
	for _, test := range []struct {
		name                   string
		cellSize, x, y, radius float64
		fallback               bool
	}{
		{"tiny_cells", .000001, 0, 0, 1000, true},
		{"smallest_cells", math.SmallestNonzeroFloat64, 0, 0, 1000, true},
		{"huge_cells", math.MaxFloat64, 0, 0, 1000, false},
		{"negative_int64_endpoint", 1, -0x1p63, 0, 0, true},
		{"below_positive_int64_endpoint", 1, math.Nextafter(0x1p63, 0), 0, 0, true},
		{"positive_int64_endpoint", 1, 0x1p63, 0, 0, true},
		{"huge_position", 256, math.MaxFloat64, math.MaxFloat64, 1000, true},
		{"large_radius", 256, 0, 0, 1e16, true},
		{"negative_large_radius", 256, 0, 0, -0x1p63, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newChatSelectorFixture(t, 16, test.cellSize)
			fixture.add(t, 1, test.x, test.y, true)
			fixture.add(t, 2, test.x+1000, test.y, true)
			fixture.add(t, 3, test.x+2000, test.y+2000, true)
			fixture.add(t, 4, -12345, 54321, true)
			radiusSq := test.radius * test.radius
			got, stats := fixture.shard.appendLocalChatRecipients(fixture.world, test.x, test.y, test.radius, radiusSq, nil)
			require.ElementsMatch(t, fixture.fullScan(test.x, test.y, radiusSq, nil), got)
			require.Equal(t, test.fallback, stats.occupiedFallback)
		})
	}
}

func TestLocalChatRectangleCountOverflow(t *testing.T) {
	for _, test := range []struct {
		name             string
		minimum, maximum soundCell
		count            uint64
		ok               bool
	}{
		{"negative_rectangle", soundCell{-4, -4}, soundCell{3, 3}, 64, true},
		{"single_maximum_cell", soundCell{math.MaxInt64, math.MaxInt64}, soundCell{math.MaxInt64, math.MaxInt64}, 1, true},
		{"full_signed_span", soundCell{math.MinInt64, 0}, soundCell{math.MaxInt64, 0}, 0, false},
		{"area_overflow", soundCell{math.MinInt64, -1}, soundCell{0, 1}, 0, false},
		{"reversed_bounds", soundCell{1, 0}, soundCell{0, 0}, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			count, ok := localChatRectangleCellCount(test.minimum, test.maximum)
			require.Equal(t, test.count, count)
			require.Equal(t, test.ok, ok)
		})
	}
}

func TestLocalChatRecipientsDefaultQueryRemainsLocal(t *testing.T) {
	fixture := newChatSelectorFixture(t, 1024, 256)
	for id := types.EntityID(1); id <= 8; id++ {
		fixture.add(t, id, 128+float64(id)*10, 128, true)
	}
	want, initial := fixture.shard.appendLocalChatRecipients(fixture.world, 128, 128, 1000, 1000000, nil)
	require.Equal(t, uint64(81), initial.cellVisits)
	require.Equal(t, uint64(8), initial.candidates)
	for id := types.EntityID(9); id <= 1008; id++ {
		fixture.add(t, id, 100000+float64(id)*1024, 100000, true)
	}
	got, expanded := fixture.shard.appendLocalChatRecipients(fixture.world, 128, 128, 1000, 1000000, nil)
	require.ElementsMatch(t, want, got)
	require.Equal(t, initial, expanded)
}

func TestLocalChatRecipientsAdaptiveCellTraversal(t *testing.T) {
	fixture := newChatSelectorFixture(t, 4096, 1)
	fixture.add(t, 1, 0, 0, true)
	got, stats := fixture.shard.appendLocalChatRecipients(fixture.world, 0, 0, 25, 625, nil)
	require.Equal(t, []types.EntityID{1}, got)
	require.True(t, stats.occupiedFallback)
	require.Equal(t, uint64(1), stats.cellVisits)
	for id := types.EntityID(2); id <= 3000; id++ {
		fixture.add(t, id, 100000+float64(id)*4, 100000, true)
	}
	got, stats = fixture.shard.appendLocalChatRecipients(fixture.world, 0, 0, 25, 625, nil)
	require.Equal(t, []types.EntityID{1}, got)
	require.False(t, stats.occupiedFallback, "rectangles larger than threshold still use direct probes when cheaper than occupied cells")
	require.Equal(t, uint64(52*52), stats.cellVisits)
	require.Equal(t, uint64(1), stats.candidates)
	// The retained map allocation must not force a default-sized query to range
	// over its historical contents after most listeners have gone away.
	for id := types.EntityID(2); id <= 3000; id++ {
		fixture.shard.soundEvents.Detach(fixture.world.GetHandleByEntityID(id), uint64(id))
	}
	_, stats = fixture.shard.appendLocalChatRecipients(fixture.world, 0, 0, 10, 100, nil)
	require.False(t, stats.occupiedFallback)
	require.Equal(t, uint64(22*22), stats.cellVisits)
}

func TestLocalChatRecipientsReuseDestinationWithoutAllocations(t *testing.T) {
	for _, cellSize := range []float64{256, 1} {
		fixture := newChatSelectorFixture(t, 16, cellSize)
		for id := types.EntityID(1); id <= 8; id++ {
			fixture.add(t, id, float64(id)*10, 0, true)
		}
		dst := make([]types.EntityID, 0, 8)
		allocations := testing.AllocsPerRun(100, func() {
			dst = fixture.shard.AppendLocalChatRecipients(fixture.world, 0, 0, 1000, 1000000, dst[:0])
		})
		require.Zero(t, allocations)
		require.Len(t, dst, 8)
	}
}

func BenchmarkLocalChatRecipientSelection(b *testing.B) {
	for _, workload := range []struct {
		name           string
		nearby, remote int
		cellSize       float64
		churn          bool
	}{
		{"sparse1", 1, 0, 256, false},
		{"sparse8", 8, 0, 256, false},
		{"remote1000", 8, 1000, 256, false},
		{"remote30000", 8, 30000, 256, false},
		{"grow30000_shrink1", 1, 30000, 256, true},
		{"grow30000_shrink8", 8, 30000, 256, true},
		{"tiny_cells_fallback", 8, 1000, 1, false},
	} {
		b.Run(workload.name, func(b *testing.B) {
			fixture := newChatSelectorFixture(b, workload.nearby+workload.remote+16, workload.cellSize)
			for id := 1; id <= workload.nearby; id++ {
				fixture.add(b, types.EntityID(id), 128+float64(id)*10, 128, true)
			}
			for remote := 0; remote < workload.remote; remote++ {
				id := types.EntityID(workload.nearby + remote + 1)
				fixture.add(b, id, 100000+float64(remote)*workload.cellSize*4, 100000, true)
			}
			if workload.churn {
				for remote := 0; remote < workload.remote; remote++ {
					id := types.EntityID(workload.nearby + remote + 1)
					handle := fixture.world.GetHandleByEntityID(id)
					fixture.shard.soundEvents.Detach(handle, uint64(id))
					fixture.world.Despawn(handle)
					delete(ecs.GetResource[ecs.CharacterEntities](fixture.world).Map, id)
					delete(fixture.connected, id)
				}
			}
			want := fixture.fullScan(128, 128, 1000000, nil)
			got, stats := fixture.shard.appendLocalChatRecipients(fixture.world, 128, 128, 1000, 1000000, nil)
			require.ElementsMatch(b, want, got)
			require.Len(b, got, workload.nearby)
			b.Run("grid", func(b *testing.B) {
				dst := make([]types.EntityID, 0, len(got))
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					dst = fixture.shard.AppendLocalChatRecipients(fixture.world, 128, 128, 1000, 1000000, dst[:0])
				}
				b.StopTimer()
				require.Len(b, dst, workload.nearby)
				b.ReportMetric(float64(stats.cellVisits), "cells/op")
				b.ReportMetric(float64(stats.candidates), "candidates/op")
				b.ReportMetric(float64(len(dst)), "recipients/op")
			})
			b.Run("full_scan", func(b *testing.B) {
				var dst []types.EntityID
				b.ReportAllocs()
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					dst = fixture.fullScan(128, 128, 1000000, make([]types.EntityID, 0, 32))
				}
				b.StopTimer()
				require.Len(b, dst, workload.nearby)
			})
		})
	}
}
