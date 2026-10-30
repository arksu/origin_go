package world

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/world/wildweed"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type refillTestStore struct {
	spotTestStore
	refills   []WildWeedRefillWrite
	refillErr error
}

func (s *refillTestStore) RefillWildWeeds(_ context.Context, write WildWeedRefillWrite) (SpotStateCommit, error) {
	s.refills = append(s.refills, write)
	if s.refillErr != nil {
		return SpotStateCommit{}, s.refillErr
	}
	for index := range s.rows {
		row := &s.rows[index]
		if row.ID != write.SpotID {
			continue
		}
		var state map[string]json.RawMessage
		var www map[string]json.RawMessage
		var objects map[string]json.RawMessage
		if err := json.Unmarshal(row.State, &state); err != nil {
			return SpotStateCommit{}, err
		}
		if err := json.Unmarshal(state["www"], &www); err != nil {
			return SpotStateCommit{}, err
		}
		if err := json.Unmarshal(www["objects"], &objects); err != nil {
			return SpotStateCommit{}, err
		}
		for _, object := range write.Objects {
			position, _ := json.Marshal(wildweed.Position{X: object.X, Y: object.Y})
			objects[strconv.FormatInt(object.ID, 10)] = position
		}
		www["objects"], _ = json.Marshal(objects)
		if len(objects) >= write.Target {
			delete(www, "last_check_runtime_seconds")
		} else {
			www["last_check_runtime_seconds"], _ = json.Marshal(write.CheckRuntimeSeconds)
		}
		state["www"], _ = json.Marshal(www)
		row.State, _ = json.Marshal(state)
		row.Revision++
		row.LastRuntimeSeconds = max(row.LastRuntimeSeconds, write.RuntimeSeconds)
		return SpotStateCommit{SpotID: row.ID, Snapshot: SpotStateSnapshot{
			State: append(json.RawMessage(nil), row.State...), Revision: row.Revision, LastRuntimeSeconds: row.LastRuntimeSeconds,
		}}, nil
	}
	return SpotStateCommit{}, ErrSpotNotFound
}

type refillTestIDs struct {
	next  types.EntityID
	calls int
}

func (ids *refillTestIDs) ReserveIDs(count uint64) (types.EntityID, types.EntityID, error) {
	ids.calls++
	first := ids.next + 1
	ids.next += types.EntityID(count)
	return first, ids.next, nil
}

func refillTestRow(id int64, count int, marker *int64) repository.Spot {
	row := spotTestRow(id, "www", 774, 774, 64*constt.CoordPerTile)
	objects := make(map[string]wildweed.Position)
	for index := range count {
		objects[strconv.Itoa(index+1)] = wildweed.Position{X: 100 + 24*index, Y: 100}
	}
	www := map[string]any{"objects": objects}
	if marker != nil {
		www["last_check_runtime_seconds"] = *marker
	}
	row.State, _ = json.Marshal(map[string]any{"www": www, "unknown": json.RawMessage(`9007199254740993`)})
	return row
}

func newRefillTestManager(t *testing.T, rows ...repository.Spot) (*SpotManager, *ChunkManager, *refillTestStore, *spotTestQueue, *refillTestIDs) {
	t.Helper()
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 41, Key: "wild_windsown_weed", HP: 100, IsStatic: true},
	}))
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	cm, _ := newCapacityChunkManager(t, 64)
	store := &refillTestStore{spotTestStore: spotTestStore{rows: rows}}
	queue, ids := &spotTestQueue{accept: true}, &refillTestIDs{next: 1000}
	cfg := spotTestConfig()
	cfg.Game.WorldMinXChunks, cfg.Game.WorldMinYChunks = 0, 0
	m, err := NewSpotManager(t.Context(), cfg, store, queue.submit, nil)
	require.NoError(t, err)
	require.NoError(t, m.EnableWildWeedRefill(cm, ids))
	m.www.random = rand.New(rand.NewSource(17))
	refillTestChunk(cm, types.ChunkCoord{}, types.ChunkStateActive, true)
	ecs.SetResource(cm.world, ecs.TimeState{RuntimeSecondsTotal: 100, Tick: 1000})
	return m, cm, store, queue, ids
}

func refillTestChunk(cm *ChunkManager, coord types.ChunkCoord, state types.ChunkState, active bool) *core.Chunk {
	chunk := core.NewChunk(coord, cm.region, cm.layer, constt.ChunkSize)
	for index := range chunk.Tiles {
		chunk.Tiles[index] = 35
	}
	chunk.SetState(state)
	cm.chunks[coord] = chunk
	interest := newChunkInterest()
	if active {
		interest.activeEntities[99] = struct{}{}
	}
	cm.chunkInterests[coord] = interest
	return chunk
}

func refillTestOperation(m *SpotManager, count int, area wildWeedTileArea) *spotStateJob {
	spot := m.byID[1]
	job := &spotStateJob{store: m.store, spot: spot, chunks: m.www.chunks, write: SpotStateWrite{ID: spot.info.ID},
		refill: &wildWeedRefillOperation{
			write:     WildWeedRefillWrite{SpotID: spot.info.ID, Region: m.region, RuntimeSeconds: 100, CheckRuntimeSeconds: 100, Target: 5},
			remaining: count, random: rand.New(rand.NewSource(7)), areas: []wildWeedTileArea{area}, tiles: area.width * area.height,
		}}
	if count > 0 && !m.pinWildWeedSourceAreas(job.refill) {
		panic("refill fixture source area is not ready")
	}
	spot.pending = job
	m.pending = append(m.pending, job)
	return job
}

func TestWildWeedRefillTrialsArePerSpot(t *testing.T) {
	const trials = 120000
	var previous int
	for _, deficit := range []int{1, 5, 100} {
		random := rand.New(rand.NewSource(33))
		births := 0
		for range trials {
			births += wildWeedSuccesses(random, 1, deficit)
		}
		require.InDelta(t, float64(trials)*wildWeedTrialChance, births, 120)
		if previous != 0 {
			require.Equal(t, previous, births, "one trial must not multiply by deficit")
		}
		previous = births
	}
	random := rand.New(rand.NewSource(18))
	const samples = 30000
	var births int
	for range samples {
		births += wildWeedSuccesses(random, 120, 50)
	}
	require.InDelta(t, 1.0, float64(births)/samples, 0.025, "120 quarter-hours average one birth")
	require.Equal(t, 5, wildWeedSuccesses(random, math.MaxInt64, 5), "large backlog is bounded by deficit")
	require.Zero(t, wildWeedSuccesses(random, 1000, 0))
}

func TestWildWeedRefillAdmissionMarkerAndRuntime(t *testing.T) {
	t.Run("underfilled initializes on active visit", func(t *testing.T) {
		m, _, _, _, _ := newRefillTestManager(t, refillTestRow(1, 0, nil))
		m.admitWildWeedChecks()
		require.Len(t, m.pending, 1)
		op := m.pending[0].refill
		require.Equal(t, int64(100), op.write.CheckRuntimeSeconds)
		require.Zero(t, op.remaining, "an absent marker must not manufacture historical trials")
	})
	t.Run("full includes unloaded plants and stems", func(t *testing.T) {
		m, _, _, _, _ := newRefillTestManager(t, refillTestRow(1, 5, nil))
		m.admitWildWeedChecks()
		require.Empty(t, m.pending, "membership, not ECS presence or growth stage, defines fullness")
	})
	t.Run("full marker is removed once", func(t *testing.T) {
		marker := int64(0)
		m, _, _, _, _ := newRefillTestManager(t, refillTestRow(1, 5, &marker))
		m.admitWildWeedChecks()
		require.Len(t, m.pending, 1)
		require.Zero(t, m.pending[0].refill.remaining)
	})
	t.Run("legacy world untouched", func(t *testing.T) {
		m, _, _, _, _ := newRefillTestManager(t, spotTestRow(1, "www", 774, 774, 768))
		m.admitWildWeedChecks()
		require.Empty(t, m.pending)
	})
	t.Run("negative elapsed and partial interval do nothing", func(t *testing.T) {
		marker := int64(200)
		m, cm, _, _, _ := newRefillTestManager(t, refillTestRow(1, 0, &marker))
		m.admitWildWeedChecks()
		require.Empty(t, m.pending)
		ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal = marker + wildWeedCheckSeconds - 1
		m.admitWildWeedChecks()
		require.Empty(t, m.pending)
	})
	t.Run("only whole runtime periods are consumed", func(t *testing.T) {
		marker := int64(100)
		m, cm, _, _, _ := newRefillTestManager(t, refillTestRow(1, 0, &marker))
		ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal = marker + 3*wildWeedCheckSeconds + 899
		m.admitWildWeedChecks()
		require.Equal(t, marker+3*wildWeedCheckSeconds, m.pending[0].refill.write.CheckRuntimeSeconds)
	})
	t.Run("unloaded backlog remains until active", func(t *testing.T) {
		marker := int64(0)
		m, cm, _, _, _ := newRefillTestManager(t, refillTestRow(1, 0, &marker))
		ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal = 1000000 * wildWeedCheckSeconds
		cm.GetChunkFast(types.ChunkCoord{}).SetState(types.ChunkStateInactive)
		m.admitWildWeedChecks()
		require.Empty(t, m.pending)
		cm.GetChunkFast(types.ChunkCoord{}).SetState(types.ChunkStateActive)
		m.admitWildWeedChecks()
		require.Equal(t, 5, m.pending[0].refill.remaining)
	})
}

func TestWildWeedRefillRequiresActualActiveAOIAndRestoredRaw(t *testing.T) {
	m, cm, _, _, _ := newRefillTestManager(t, refillTestRow(1, 0, nil))
	coord := types.ChunkCoord{}
	chunk := cm.GetChunkFast(coord)
	require.True(t, m.wildWeedChunkReady(coord))
	chunk.SetRawObjects([]*repository.Object{{ID: 7}})
	require.False(t, m.wildWeedChunkReady(coord))
	m.admitWildWeedChecks()
	require.Empty(t, m.pending)
	chunk.SetRawObjects(nil)
	delete(cm.chunkInterests[coord].activeEntities, 99)
	require.NoError(t, cm.PinPersistence(coord))
	require.False(t, m.wildWeedChunkReady(coord), "a persistence pin is not a player AOI")
	m.admitWildWeedChecks()
	require.Empty(t, m.pending)
	cm.UnpinPersistence(coord)
	cm.chunkInterests[coord].activeEntities[99] = struct{}{}
	chunk.SetState(types.ChunkStatePreloaded)
	require.False(t, m.wildWeedChunkReady(coord))
	chunk.SetState(types.ChunkStateActive)
	require.True(t, m.wildWeedChunkReady(coord))
}

func TestWildWeedRefillBoundingCornerPreservesCheckUntilCircleActivates(t *testing.T) {
	for _, hasMarker := range []bool{false, true} {
		t.Run(strconv.FormatBool(hasMarker), func(t *testing.T) {
			var marker *int64
			if hasMarker {
				lastCheck := int64(100)
				marker = &lastCheck
			}
			row := refillTestRow(1, 0, marker)
			row.CenterX, row.CenterY = 800, 800
			m, cm, store, queue, ids := newRefillTestManager(t, row, spotTestRow(2, "clay", 2000, 2000, 100))
			coord, corner := types.ChunkCoord{}, types.ChunkCoord{X: 1, Y: 1}
			cm.GetChunkFast(coord).SetState(types.ChunkStateInactive)
			delete(cm.chunkInterests[coord].activeEntities, 99)
			refillTestChunk(cm, corner, types.ChunkStateActive, true)
			require.Equal(t, []int64{2}, m.IntersectingChunk(0, corner, nil), "the corner intersects another spot, not this WWW circle")
			const runtime int64 = 100 + 1000000*wildWeedCheckSeconds + 450
			ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal = runtime
			before, found := m.GetState(1)
			require.True(t, found)
			areas, tiles := m.activeWildWeedAreas(m.byID[1].info)
			require.Empty(t, areas)
			require.Zero(t, tiles)
			now := time.Now()
			m.Update(now)
			m.Update(now.Add(time.Second))
			require.Zero(t, m.PendingCount())
			require.Empty(t, queue.jobs)
			require.Empty(t, store.refills)
			require.Zero(t, ids.calls)
			require.Zero(t, cm.persistenceFor(corner).pins.Load())
			after, _ := m.GetState(1)
			require.Equal(t, before, after, "corner-only activity must neither initialize nor advance the cursor")
			require.Equal(t, rand.New(rand.NewSource(17)).Int63(), m.www.random.Int63(), "corner-only activity must not consume the draw")
			m.www.random = rand.New(rand.NewSource(17))

			cm.GetChunkFast(coord).SetState(types.ChunkStateActive)
			cm.chunkInterests[coord].activeEntities[99] = struct{}{}
			areas, tiles = m.activeWildWeedAreas(m.byID[1].info)
			require.Len(t, areas, 1, "only the chunk intersecting the circle may supply candidates")
			require.Positive(t, tiles)
			m.Update(now.Add(2 * time.Second))
			require.Equal(t, 1, m.PendingCount())
			op := m.pending[0].refill
			if hasMarker {
				require.Equal(t, 5, op.remaining, "all skipped periods remain available for catch-up")
				require.Equal(t, runtime-450, op.write.CheckRuntimeSeconds)
			} else {
				require.Zero(t, op.remaining, "an uninitialized spot starts at its first actual active visit")
				require.Equal(t, runtime, op.write.CheckRuntimeSeconds)
			}
		})
	}
}

func TestWildWeedRefillBusySourceDefersBeforeRandomDraw(t *testing.T) {
	marker := int64(0)
	m, cm, _, _, _ := newRefillTestManager(t, refillTestRow(1, 0, &marker))
	ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal = 1000000 * wildWeedCheckSeconds
	gate := cm.persistenceFor(types.ChunkCoord{})
	gate.ioMu.Lock()
	m.admitWildWeedChecks()
	gate.ioMu.Unlock()
	require.Empty(t, m.pending)
	require.Zero(t, gate.pins.Load())
	require.Equal(t, rand.New(rand.NewSource(17)).Int63(), m.www.random.Int63(), "unaccepted work must not consume the draw")
	m.www.random = rand.New(rand.NewSource(17))
	m.admitWildWeedChecks()
	require.Len(t, m.pending, 1)
	require.Equal(t, 5, m.pending[0].refill.remaining)
	require.Equal(t, int32(1), gate.pins.Load(), "accepted source stays fully restored until publication")
}

func TestWildWeedRefillCreatesStemAtPreparationRuntime(t *testing.T) {
	m, cm, _, _, ids := newRefillTestManager(t, refillTestRow(1, 0, nil))
	job := refillTestOperation(m, 1, wildWeedTileArea{64, 64, 1, 1})
	ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal = 200000
	budget := newWildWeedPrepareBudget()
	require.NoError(t, m.prepareWildWeeds(job, &budget))
	require.True(t, job.refill.prepared)
	require.Len(t, job.refill.write.Objects, 1)
	object := job.refill.write.Objects[0]
	require.Equal(t, int64(1001), object.ID)
	require.Equal(t, 1, ids.calls)
	require.Equal(t, 41, object.TypeID)
	require.Equal(t, int64(30), object.Quality)
	require.True(t, object.Hp.Valid)
	require.Equal(t, 100.0, object.Hp.Float64)
	require.Equal(t, 774, object.X)
	require.Equal(t, 774, object.Y)
	var envelope components.ObjectStateEnvelope
	require.NoError(t, json.Unmarshal(object.Data.RawMessage, &envelope))
	var state components.WildWeedBehaviorState
	require.NoError(t, json.Unmarshal(envelope.Behaviors["wild_weed"], &state))
	require.Equal(t, int64(1), state.SpotID)
	require.Zero(t, state.Stage)
	require.Equal(t, int64(200000)+contracts.WildWeedGrowthSeconds, state.NextGrowthRuntimeSeconds,
		"catch-up trials never backdate the new stem's age")
}

func TestWildWeedRefillPreparationBudgetsContinueAcceptedResult(t *testing.T) {
	t.Run("four object budget preserves fifth birth", func(t *testing.T) {
		m, _, _, _, ids := newRefillTestManager(t, refillTestRow(1, 0, nil))
		job := refillTestOperation(m, 5, wildWeedTileArea{24, 24, 80, 80})
		budget := newWildWeedPrepareBudget()
		require.NoError(t, m.prepareWildWeeds(job, &budget))
		require.Len(t, job.refill.write.Objects, 4)
		require.Equal(t, 1, job.refill.remaining)
		require.False(t, job.refill.prepared)
		require.GreaterOrEqual(t, budget.candidates, 0)
		first := append([]repository.Object(nil), job.refill.write.Objects...)
		budget = newWildWeedPrepareBudget()
		require.NoError(t, m.prepareWildWeeds(job, &budget))
		require.True(t, job.refill.prepared)
		require.Len(t, job.refill.write.Objects, 5)
		require.Equal(t, first, job.refill.write.Objects[:4])
		require.Equal(t, 5, ids.calls)
	})
	t.Run("sixty four candidate budget preserves remaining tries", func(t *testing.T) {
		m, cm, _, _, ids := newRefillTestManager(t, refillTestRow(1, 0, nil))
		chunk := cm.GetChunkFast(types.ChunkCoord{})
		require.True(t, chunk.SetTile(64, 64, types.TileDeepWater))
		job := refillTestOperation(m, 2, wildWeedTileArea{64, 64, 1, 1})
		budget := newWildWeedPrepareBudget()
		require.NoError(t, m.prepareWildWeeds(job, &budget))
		require.Zero(t, budget.candidates)
		require.Equal(t, 1, job.refill.remaining)
		require.False(t, job.refill.prepared)
		budget = newWildWeedPrepareBudget()
		require.NoError(t, m.prepareWildWeeds(job, &budget))
		require.True(t, job.refill.prepared)
		require.Empty(t, job.refill.write.Objects)
		require.Zero(t, ids.calls)
	})
}

func TestWildWeedRefillNeighborGapAndUnavailableArea(t *testing.T) {
	for _, gap := range []int{-1, 19, 20} {
		t.Run(strconv.Itoa(gap), func(t *testing.T) {
			m, cm, _, _, ids := newRefillTestManager(t, refillTestRow(1, 0, nil))
			if gap >= 0 {
				neighbor := refillTestChunk(cm, types.ChunkCoord{X: 1}, types.ChunkStatePreloaded, false)
				neighbor.SetRawObjects([]*repository.Object{{ID: 99, X: 1530 + gap, Y: 774}})
			}
			job := refillTestOperation(m, 1, wildWeedTileArea{127, 64, 1, 1})
			budget := newWildWeedPrepareBudget()
			require.NoError(t, m.prepareWildWeeds(job, &budget))
			require.True(t, job.refill.prepared)
			if gap == 20 {
				require.Len(t, job.refill.write.Objects, 1, "distance exactly 20 is allowed across chunk boundary")
			} else {
				require.Empty(t, job.refill.write.Objects)
				require.Zero(t, ids.calls)
				require.Equal(t, int32(1), cm.persistenceFor(types.ChunkCoord{}).pins.Load(), "only the ready source pin remains until commit")
				if gap < 0 {
					require.Nil(t, cm.GetChunkFast(types.ChunkCoord{X: 1}), "missing neighbor must not be force loaded")
				}
			}
		})
	}
}

func TestWildWeedRefillReservationsAndOwnership(t *testing.T) {
	first, second := refillTestRow(1, 0, nil), refillTestRow(2, 0, nil)
	second.DistrictX = 1
	m, _, _, _, _ := newRefillTestManager(t, first, second)
	point := types.ChunkCoord{}
	for _, gap := range []int{19, 20} {
		m.byID[2].pending = &spotStateJob{refill: &wildWeedRefillOperation{write: WildWeedRefillWrite{
			Objects: []repository.Object{{ID: 999, X: 774 + gap, Y: 774}},
		}}}
		position := wildweed.Position{X: 774, Y: 774}
		require.Equal(t, gap == 20, m.wildWeedPositionFree(position), "pending neighboring spot reserves prepared coordinates")
		require.True(t, m.ownsWildWeedPoint(m.byID[1].info, point, position), "equal Q prefers first district")
		require.False(t, m.ownsWildWeedPoint(m.byID[2].info, point, position))
		m.byID[2].info.PeakQuality = 40
		require.False(t, m.ownsWildWeedPoint(m.byID[1].info, point, position), "higher Q wins over district order")
		m.byID[2].info.PeakQuality = 30
	}
}

func TestWildWeedRefillRoundRobinAdmissionIsBounded(t *testing.T) {
	rows := make([]repository.Spot, 40)
	for index := range rows {
		rows[index] = refillTestRow(int64(index+1), 0, nil)
	}
	m, _, _, _, _ := newRefillTestManager(t, rows...)
	m.admitWildWeedChecks()
	require.Len(t, m.pending, 32)
	m.admitWildWeedChecks()
	require.Len(t, m.pending, 40, "round robin must reach spots beyond first tick budget")
}

func TestWildWeedRefillRetryKeepsPreparedIDsPositionsAndGrowth(t *testing.T) {
	m, cm, store, queue, ids := newRefillTestManager(t, refillTestRow(1, 0, nil))
	job := refillTestOperation(m, 1, wildWeedTileArea{64, 64, 1, 1})
	m.StopAdmission() // Accepted work must still finish during shutdown.
	queue.accept = false
	now := time.Now()
	m.Update(now)
	require.Len(t, job.refill.write.Objects, 1)
	require.Empty(t, queue.jobs, "backpressure retains an accepted operation")
	require.Equal(t, int32(1), cm.persistenceFor(types.ChunkCoord{}).pins.Load())
	want := job.refill.write.Objects[0]
	queue.accept = true
	m.Update(now.Add(time.Second))
	require.Len(t, queue.jobs, 1)
	require.Same(t, job, queue.jobs[0])
	store.refillErr = errors.New("ambiguous database connection failure")
	job.Complete(job.Run())
	m.Update(now.Add(2 * time.Second))
	require.Len(t, queue.jobs, 1, "failure is retried after existing backoff")
	ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal += 100000
	m.Update(now.Add(3 * time.Second))
	require.Len(t, queue.jobs, 2)
	require.Equal(t, want, job.refill.write.Objects[0], "retry cannot reroll or shift growth deadline")
	require.Equal(t, 1, ids.calls)
	store.refillErr = nil
	job.Complete(job.Run())
	require.Equal(t, int64(1), m.byID[1].state.Revision, "worker may not apply canonical state")
	m.Update(now.Add(4 * time.Second))
	require.Zero(t, m.PendingCount())
	require.Equal(t, int64(2), m.byID[1].state.Revision)
	require.Len(t, store.refills, 2)
	require.Equal(t, store.refills[0], store.refills[1])
	require.Len(t, cm.GetChunkFast(types.ChunkCoord{}).GetRawObjects(), 1)
	require.Zero(t, cm.persistenceFor(types.ChunkCoord{}).pins.Load())
}

func TestWildWeedRefillKnownCommitRetriesOnlyPublication(t *testing.T) {
	m, cm, store, queue, _ := newRefillTestManager(t, refillTestRow(1, 0, nil))
	job := refillTestOperation(m, 1, wildWeedTileArea{64, 64, 1, 1})
	m.StopAdmission()
	now := time.Now()
	m.Update(now)
	require.Len(t, queue.jobs, 1)
	job.Complete(job.Run())
	gate := cm.persistenceFor(types.ChunkCoord{})
	gate.ioMu.Lock()
	m.Update(now.Add(time.Second))
	gate.ioMu.Unlock()
	require.True(t, job.refill.committed)
	require.Equal(t, 1, m.PendingCount())
	require.Equal(t, int32(1), gate.pins.Load())
	require.Empty(t, cm.GetChunkFast(types.ChunkCoord{}).GetRawObjects())
	m.Update(now.Add(2 * time.Second))
	require.Zero(t, m.PendingCount())
	require.Len(t, store.refills, 1, "publication retry must not repeat a known committed transaction")
	require.Len(t, queue.jobs, 1)
	require.Len(t, cm.GetChunkFast(types.ChunkCoord{}).GetRawObjects(), 1)
	require.Zero(t, gate.pins.Load())
}

func TestWildWeedRefillEmptyCheckPersistsOnceAndWallTimeDoesNotAdvanceClock(t *testing.T) {
	m, cm, store, queue, _ := newRefillTestManager(t, refillTestRow(1, 0, nil))
	now := time.Now()
	m.Update(now)                  // Admission.
	m.Update(now.Add(time.Second)) // Existing workers receive the empty initialization.
	require.Len(t, queue.jobs, 1)
	job := queue.jobs[0]
	job.Complete(job.Run())
	m.Update(now.Add(2 * time.Second))
	require.Zero(t, m.PendingCount())
	manifest := m.byID[1].wildWeedManifest()
	require.NotNil(t, manifest.lastCheck)
	require.Equal(t, int64(100), *manifest.lastCheck)
	m.Update(now.Add(30 * 24 * time.Hour))
	require.Zero(t, m.PendingCount(), "offline wall time is not a runtime interval")
	require.Len(t, store.refills, 1)
	ecs.GetResource[ecs.TimeState](cm.world).RuntimeSecondsTotal += wildWeedCheckSeconds
	m.Update(now.Add(30*24*time.Hour + time.Second))
	require.Equal(t, 1, m.PendingCount())
	require.Equal(t, int64(100)+wildWeedCheckSeconds, m.pending[0].refill.write.CheckRuntimeSeconds)
	m.StopAdmission()
	// Closing admission must not produce a second operation after this one is retained.
	m.admitWildWeedChecks()
	require.Equal(t, 1, m.PendingCount())
}

func TestWildWeedRefillUpdateSharesPreparationBudgetAcrossSpots(t *testing.T) {
	first, second := refillTestRow(1, 0, nil), refillTestRow(2, 0, nil)
	first.CenterX, second.CenterX, second.DistrictX = 390, 1158, 1
	m, _, _, queue, ids := newRefillTestManager(t, first, second)
	firstJob := refillTestOperation(m, 5, wildWeedTileArea{24, 54, 16, 20})
	secondSpot := m.byID[2]
	secondJob := &spotStateJob{store: m.store, spot: secondSpot, chunks: m.www.chunks, write: SpotStateWrite{ID: 2},
		refill: &wildWeedRefillOperation{
			write:     WildWeedRefillWrite{SpotID: 2, Region: m.region, RuntimeSeconds: 100, CheckRuntimeSeconds: 100, Target: 5},
			remaining: 5, random: rand.New(rand.NewSource(8)), areas: []wildWeedTileArea{{88, 54, 16, 20}}, tiles: 16 * 20,
		}}
	require.True(t, m.pinWildWeedSourceAreas(secondJob.refill))
	secondSpot.pending = secondJob
	m.pending = append(m.pending, secondJob)
	m.StopAdmission()
	now := time.Now()
	m.Update(now)
	require.Equal(t, 4, ids.calls, "four-object budget applies to the entire Update")
	require.Len(t, firstJob.refill.write.Objects, 4)
	require.Empty(t, secondJob.refill.write.Objects)
	require.Equal(t, 6, firstJob.refill.remaining+secondJob.refill.remaining)
	require.Empty(t, queue.jobs, "an unfinished sampled batch is not committed as a truncated result")
	m.Update(now.Add(time.Second))
	require.Equal(t, 8, ids.calls)
	require.Len(t, firstJob.refill.write.Objects, 5)
	require.Len(t, secondJob.refill.write.Objects, 3)
	require.Equal(t, 2, secondJob.refill.remaining)
	require.Len(t, queue.jobs, 1)
	queue.jobs[0].Complete(queue.jobs[0].Run())
	// Shutdown drain runs SpotManager.Update without chunk activation. A
	// committed raw backlog must not consume the other spot's sampled births.
	m.Update(now.Add(2 * time.Second))
	require.Equal(t, 10, ids.calls)
	require.Len(t, secondJob.refill.write.Objects, 5)
	require.Len(t, queue.jobs, 2)
	queue.jobs[1].Complete(queue.jobs[1].Run())
	for tick := 3; m.PendingCount() > 0 && tick < 10; tick++ {
		m.Update(now.Add(time.Duration(tick) * time.Second))
	}
	require.Zero(t, m.PendingCount())
	require.Zero(t, m.www.chunks.persistenceFor(types.ChunkCoord{}).pins.Load())
	require.Len(t, m.www.chunks.GetChunkFast(types.ChunkCoord{}).GetRawObjects(), 10)
}

func TestWildWeedRefillGapChecksLiveSpatialObjects(t *testing.T) {
	for _, gap := range []int{19, 20} {
		t.Run(strconv.Itoa(gap), func(t *testing.T) {
			m, cm, _, _, _ := newRefillTestManager(t, refillTestRow(1, 0, nil))
			h := cm.world.Spawn(55, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{X: float64(774 + gap), Y: 774})
			})
			cm.GetChunkFast(types.ChunkCoord{}).Spatial().AddStatic(h, 774+gap, 774)
			require.Equal(t, gap == 20, m.wildWeedPositionFree(wildweed.Position{X: 774, Y: 774}))
		})
	}
}
