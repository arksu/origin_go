package world

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type spotTestStore struct {
	rows      []repository.Spot
	loadErr   error
	loadCalls int
	writes    []SpotStateWrite
	saveErr   error
}

func (s *spotTestStore) LoadSpots(context.Context, int) ([]repository.Spot, error) {
	s.loadCalls++
	return s.rows, s.loadErr
}

func (s *spotTestStore) SaveSpotState(_ context.Context, write SpotStateWrite) error {
	s.writes = append(s.writes, write)
	return s.saveErr
}

type spotTestQueue struct {
	accept bool
	jobs   []PersistenceJob
}

func (q *spotTestQueue) submit(job PersistenceJob) bool {
	if q.accept {
		q.jobs = append(q.jobs, job)
	}
	return q.accept
}

func spotTestConfig() *config.Config {
	return &config.Config{Game: config.GameConfig{
		Region: 1, WorldMinXChunks: -2, WorldMinYChunks: -2,
		WorldWidthChunks: 4, WorldHeightChunks: 4,
	}}
}

func spotTestRow(id int64, kind string, x, y, radius int) repository.Spot {
	return repository.Spot{ID: id, Region: 1, SpotType: kind, CenterX: x, CenterY: y,
		Radius: radius, PeakQuality: 30, Revision: 1, State: json.RawMessage(`{}`)}
}

func newSpotTestManager(t *testing.T, rows ...repository.Spot) (*SpotManager, *spotTestStore, *spotTestQueue) {
	t.Helper()
	store, queue := &spotTestStore{rows: rows}, &spotTestQueue{accept: true}
	m, err := NewSpotManager(t.Context(), spotTestConfig(), store, queue.submit, nil)
	require.NoError(t, err)
	return m, store, queue
}

func TestSpotManagerAllTypesAndSharedChunkLookup(t *testing.T) {
	var rows []repository.Spot
	for i, kind := range []string{"www", "clay", "soil", "sand", "water"} {
		rows = append(rows, spotTestRow(int64(i+1), kind, constt.ChunkWorldSize, 6, 24))
	}
	m, store, _ := newSpotTestManager(t, rows...)
	for _, x := range []int{127, 128} {
		require.Equal(t, []int64{1, 2, 3, 4, 5}, m.FindAtTile(0, x, 0, "", nil))
		require.Equal(t, []int64{1}, m.FindAtTile(0, x, 0, "www", nil))
	}
	require.Empty(t, m.FindAtTile(1, 128, 0, "", nil))
	require.Empty(t, m.FindAtTile(0, 128, 0, "unknown", nil))
	require.Equal(t, []int64{1, 2, 3, 4, 5}, m.IntersectingChunk(0, types.ChunkCoord{X: 0, Y: 0}, nil))
	require.Equal(t, []int64{1, 2, 3, 4, 5}, m.IntersectingChunk(0, types.ChunkCoord{X: 1, Y: 0}, nil))
	require.Empty(t, m.IntersectingChunk(1, types.ChunkCoord{}, nil))
	require.Equal(t, 1, store.loadCalls, "chunk queries must not reload spots")
	info, found := m.Get(1)
	require.True(t, found)
	info.Radius = 0
	ids := m.IntersectingChunk(0, types.ChunkCoord{}, nil)
	ids[0] = 99
	require.Equal(t, []int64{1}, m.FindAtTile(0, 128, 0, "www", nil), "callers must not mutate the index or geometry")
	_, found = m.Get(999)
	require.False(t, found)
}

func TestSpotManagerTileCentersCircleEdgesAndNegativeCoordinates(t *testing.T) {
	m, _, _ := newSpotTestManager(t,
		spotTestRow(1, "www", 6, 6, 12),
		spotTestRow(2, "clay", 12, 6, 5),
		spotTestRow(3, "www", -6, -6, 12),
		spotTestRow(4, "www", -1542, 6, 12),
	)
	require.Equal(t, []int64{1}, m.FindAtTile(0, 1, 0, "", nil), "point on circle belongs to it")
	require.Empty(t, m.FindAtTile(0, 2, 0, "", nil))
	require.Empty(t, m.FindAtTile(0, 0, 0, "clay", nil), "tile square overlap must not substitute for center membership")
	require.Equal(t, []int64{3}, m.FindAtTile(0, -1, -1, "www", nil))
	require.Equal(t, []int64{4}, m.FindAtTile(0, -129, 0, "www", nil))
	require.Empty(t, m.FindAtTile(0, math.MaxInt, math.MinInt, "", nil))
	require.Empty(t, m.FindAtTile(0, 0, 256, "", nil))
}

func TestSpotManagerIndexRejectsBBoxCornerAndClipsWorld(t *testing.T) {
	store := &spotTestStore{rows: []repository.Spot{
		spotTestRow(1, "www", 1520, 1520, 20),
		spotTestRow(2, "clay", 6, 6, 100),
	}}
	cfg, queue := spotTestConfig(), &spotTestQueue{accept: true}
	cfg.Game.WorldMinXChunks, cfg.Game.WorldMinYChunks = 0, 0
	cfg.Game.WorldWidthChunks, cfg.Game.WorldHeightChunks = 2, 2
	m, err := NewSpotManager(t.Context(), cfg, store, queue.submit, nil)
	require.NoError(t, err)
	require.Empty(t, m.IntersectingChunk(0, types.ChunkCoord{X: 1, Y: 1}, nil), "bbox corner outside circle is not a candidate")
	require.Empty(t, m.IntersectingChunk(0, types.ChunkCoord{X: -1, Y: 0}, nil))
	require.Empty(t, m.FindAtTile(0, -1, 0, "clay", nil), "circle outside world must not make out-of-world tiles queryable")
	require.True(t, spotContainsPoint(SpotInfo{Radius: math.MaxInt32}, math.MaxInt32, 0))
	require.False(t, spotContainsPoint(SpotInfo{Radius: math.MaxInt32}, math.MaxInt32, math.MaxInt32))
}

func TestSpotManagerFindAtTileDoesNotAllocate(t *testing.T) {
	m, _, _ := newSpotTestManager(t, spotTestRow(1, "www", 6, 6, 12), spotTestRow(2, "www", 6, 6, 12))
	buffer := make([]int64, 0, 2)
	require.Zero(t, testing.AllocsPerRun(100, func() {
		buffer = m.FindAtTile(0, 0, 0, "", buffer)
	}))
	require.Equal(t, []int64{1, 2}, buffer)
}

func TestSpotManagerLoadFailureAndInvalidRows(t *testing.T) {
	queue := &spotTestQueue{accept: true}
	loadErr := errors.New("database unavailable")
	m, err := NewSpotManager(t.Context(), spotTestConfig(), &spotTestStore{loadErr: loadErr}, queue.submit, nil)
	require.ErrorIs(t, err, loadErr)
	require.Nil(t, m)
	m, _, _ = newSpotTestManager(t)
	require.Empty(t, m.FindAtTile(0, 0, 0, "", nil))
	for name, change := range map[string]func(*repository.Spot){
		"wrong region": func(s *repository.Spot) { s.Region = 2 },
		"wrong layer":  func(s *repository.Spot) { s.Layer = 1 },
		"radius":       func(s *repository.Spot) { s.Radius = 0 },
		"revision":     func(s *repository.Spot) { s.Revision = 0 },
		"runtime":      func(s *repository.Spot) { s.LastRuntimeSeconds = -1 },
		"state":        func(s *repository.Spot) { s.State = json.RawMessage(`null`) },
	} {
		t.Run(name, func(t *testing.T) {
			row := spotTestRow(1, "www", 6, 6, 12)
			change(&row)
			m, err := NewSpotManager(t.Context(), spotTestConfig(), &spotTestStore{rows: []repository.Spot{row}}, queue.submit, nil)
			require.Error(t, err)
			require.Nil(t, m)
		})
	}
	row := spotTestRow(1, "www", 6, 6, 12)
	m, err = NewSpotManager(t.Context(), spotTestConfig(), &spotTestStore{rows: []repository.Spot{row, row}}, queue.submit, nil)
	require.Error(t, err)
	require.Nil(t, m)
}

func TestSpotManagerSavePublishesOnlyAfterAcknowledgement(t *testing.T) {
	row := spotTestRow(1, "www", constt.ChunkWorldSize, 6, 24)
	row.State = json.RawMessage(`{"unknown":9007199254740993}`)
	m, store, queue := newSpotTestManager(t, row)
	// Neither the loader nor a state reader retains mutable access to the manager.
	store.rows[0].State[0] = '['
	loaded, _ := m.GetState(1)
	require.JSONEq(t, `{"unknown":9007199254740993}`, string(loaded.State))
	loaded.State[0] = '['
	state := json.RawMessage(`{"fill":{"0,0":{"cursor":128,"done":true}},"recoveries":{"9223372036854775807":108000},"unknown":9007199254740993}`)
	want := string(state)
	a, b := m.FindAtTile(0, 127, 0, "www", nil), m.FindAtTile(0, 128, 0, "www", nil)
	require.NoError(t, m.SaveState(a[0], 1, state, 123))
	state[0] = '['
	require.ErrorIs(t, m.SaveState(b[0], 1, json.RawMessage(`{}`), 124), ErrSpotSaveBusy)
	before, _ := m.GetState(b[0])
	require.Equal(t, int64(1), before.Revision)
	require.Len(t, queue.jobs, 1)
	job := queue.jobs[0]
	job.Complete(job.Run())
	before, _ = m.GetState(b[0])
	require.Equal(t, int64(1), before.Revision, "worker completion must not mutate canonical state")
	m.Update(time.Now())
	after, _ := m.GetState(b[0])
	require.Equal(t, int64(2), after.Revision)
	require.Equal(t, int64(123), after.LastRuntimeSeconds)
	require.JSONEq(t, want, string(after.State))
	require.JSONEq(t, want, string(store.writes[0].State))
	require.Zero(t, m.PendingCount())
	require.ErrorIs(t, m.SaveState(1, 1, json.RawMessage(`{}`), 125), ErrSpotRevisionConflict)
}

func TestSpotManagerRejectedSavesHaveNoEffect(t *testing.T) {
	m, _, queue := newSpotTestManager(t, spotTestRow(1, "www", 6, 6, 12))
	queue.accept = false
	require.ErrorIs(t, m.SaveState(1, 1, json.RawMessage(`{}`), 1), ErrSpotSaveBusy)
	require.Zero(t, m.PendingCount())
	queue.accept = true
	require.ErrorIs(t, m.SaveState(999, 1, json.RawMessage(`{}`), 1), ErrSpotNotFound)
	for _, invalid := range []string{`[]`, `null`, `{"bad":`, ``} {
		require.ErrorIs(t, m.SaveState(1, 1, json.RawMessage(invalid), 1), ErrInvalidSpotState)
	}
	require.Error(t, m.SaveState(1, 1, json.RawMessage(`{}`), -1))
	require.Empty(t, queue.jobs)
	require.NoError(t, m.SaveState(1, 1, json.RawMessage(`{}`), 1))
	m.StopAdmission()
	require.ErrorIs(t, m.SaveState(1, 1, json.RawMessage(`{}`), 1), ErrSpotManagerStopped)
}

func TestSpotManagerRetriesSameSnapshotWithBackpressure(t *testing.T) {
	m, store, queue := newSpotTestManager(t, spotTestRow(1, "www", 6, 6, 12))
	store.saveErr = errors.New("ambiguous network failure")
	require.NoError(t, m.SaveState(1, 1, json.RawMessage(`{"recoveries":{"42":108000}}`), 7))
	job := queue.jobs[0]
	job.Complete(job.Run())
	now := time.Now()
	m.Update(now)
	require.Len(t, queue.jobs, 1)
	m.Update(now.Add(50 * time.Millisecond))
	require.Len(t, queue.jobs, 1)
	queue.accept = false
	m.Update(now.Add(time.Second))
	require.Len(t, queue.jobs, 1)
	require.Equal(t, 1, m.PendingCount())
	queue.accept = true
	m.Update(now.Add(2 * time.Second))
	require.Len(t, queue.jobs, 2)
	require.Same(t, job, queue.jobs[1])
	store.saveErr = nil
	// Completing on a worker must not race with the owner polling Update.
	done := make(chan struct{})
	go func() { defer close(done); job.Complete(job.Run()) }()
	for range 5 {
		m.Update(now.Add(3 * time.Second))
	}
	<-done
	m.Update(now.Add(4 * time.Second))
	require.Zero(t, m.PendingCount())
	require.Nil(t, m.Err())
	require.Len(t, store.writes, 2)
	require.Equal(t, store.writes[0], store.writes[1], "retry must retain revision, bytes and timestamp")
}

func TestSpotManagerConflictDoesNotOverwriteOrBlockOtherSpots(t *testing.T) {
	m, store, queue := newSpotTestManager(t, spotTestRow(1, "www", 6, 6, 12), spotTestRow(2, "clay", 6, 6, 12))
	require.NoError(t, m.SaveState(1, 1, json.RawMessage(`{"fill":{}}`), 10))
	store.saveErr = ErrSpotRevisionConflict
	queue.jobs[0].Complete(queue.jobs[0].Run())
	m.Update(time.Now())
	require.ErrorIs(t, m.Err(), ErrSpotRevisionConflict)
	require.ErrorIs(t, m.SaveState(1, 1, json.RawMessage(`{}`), 11), ErrSpotRevisionConflict)
	state, _ := m.GetState(1)
	require.Equal(t, int64(1), state.Revision)
	require.JSONEq(t, `{}`, string(state.State))
	store.saveErr = nil
	require.NoError(t, m.SaveState(2, 1, json.RawMessage(`{"fill":{}}`), 12))
	queue.jobs[1].Complete(queue.jobs[1].Run())
	m.Update(time.Now())
	state, _ = m.GetState(2)
	require.Equal(t, int64(2), state.Revision)
	require.Equal(t, 1, m.PendingCount(), "conflicted snapshot is retained without retry")
	count := len(queue.jobs)
	m.Update(time.Now().Add(time.Minute))
	require.Len(t, queue.jobs, count)
}

func TestSpotManagerUpdateIsBoundedAndEventuallyVisitsEveryPendingSave(t *testing.T) {
	var rows []repository.Spot
	for i := 1; i <= 2*spotSaveUpdateBudget+1; i++ {
		rows = append(rows, spotTestRow(int64(i), "www", 6, 6, 12))
	}
	m, _, queue := newSpotTestManager(t, rows...)
	for _, row := range rows {
		require.NoError(t, m.SaveState(row.ID, 1, json.RawMessage(`{}`), 1))
	}
	for _, job := range queue.jobs {
		job.Complete(job.Run())
	}
	m.Update(time.Now())
	require.Equal(t, len(rows)-spotSaveUpdateBudget, m.PendingCount())
	m.Update(time.Now())
	m.Update(time.Now())
	require.Zero(t, m.PendingCount())
}

func TestSpotManagerPostgresRestartPreservesStateAndGeometry(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	insertSpotStoreFixture(t, db, 1, "www")
	queue := &spotTestQueue{accept: true}
	m, err := NewSpotManager(t.Context(), spotTestConfig(), store, queue.submit, nil)
	require.NoError(t, err)
	ids := m.FindAtTile(0, -5, 16, "www", nil)
	require.Len(t, ids, 1)
	state := json.RawMessage(`{"fill":{"-1,0":{"cursor":321,"done":false}},"recoveries":{"9223372036854775807":108000},"extra":{"n":9007199254740993}}`)
	require.NoError(t, m.SaveState(ids[0], 1, state, 500))
	queue.jobs[0].Complete(queue.jobs[0].Run())
	m.Update(time.Now())
	restarted, err := NewSpotManager(t.Context(), spotTestConfig(), store, queue.submit, nil)
	require.NoError(t, err)
	require.Equal(t, ids, restarted.FindAtTile(0, -5, 16, "www", nil))
	snapshot, found := restarted.GetState(ids[0])
	require.True(t, found)
	require.Equal(t, int64(2), snapshot.Revision)
	require.Equal(t, int64(500), snapshot.LastRuntimeSeconds)
	// Compare raw fields as JSONB's whitespace changes but large numbers survive.
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(snapshot.State, &fields))
	require.Contains(t, string(fields["extra"]), "9007199254740993")
	require.Contains(t, string(fields["recoveries"]), "9223372036854775807")
	require.JSONEq(t, string(state), string(snapshot.State))
}
