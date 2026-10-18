package game

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	gameworld "origin/internal/game/world"
	"origin/internal/persistence/repository"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type spotLifecycleStore struct {
	rows []repository.Spot
	save func(gameworld.SpotStateWrite) error
}

func (s *spotLifecycleStore) LoadSpots(context.Context, int) ([]repository.Spot, error) {
	if s.rows != nil {
		return s.rows, nil
	}
	return []repository.Spot{{
		ID: 1, Region: 1, Layer: 0, SpotType: "www", CenterX: 100, CenterY: 100,
		Radius: 40, PeakQuality: 30, State: json.RawMessage(`{}`), Revision: 1,
	}}, nil
}

func (s *spotLifecycleStore) SaveSpotState(_ context.Context, write gameworld.SpotStateWrite) error {
	if s.save != nil {
		return s.save(write)
	}
	return nil
}

func spotLifecycleConfig() *config.Config {
	return &config.Config{Game: config.GameConfig{
		Region: 1, ChunkLRUCapacity: 16, ChunkLRUTTL: 60, SaveWorkers: 1,
		WorldWidthChunks: 2, WorldHeightChunks: 2,
	}}
}

func TestShardStopDrainsSpotSavesBeforeStoppingWorkers(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	store := &spotLifecycleStore{save: func(gameworld.SpotStateWrite) error {
		close(started)
		<-release
		return nil
	}}
	cfg := spotLifecycleConfig()
	s := &Shard{world: ecs.NewWorldWithCapacity(16, nil, 0), logger: zap.NewNop()}
	s.chunkManager = gameworld.NewChunkManager(cfg, nil, s.world, s, 0, 1, gameworld.NewObjectFactory(nil), nil, nil, s.logger)
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(s.Stop) }
	t.Cleanup(func() { unblock(); stop() })
	var err error
	s.spotManager, err = gameworld.NewSpotManager(t.Context(), cfg, store, s.chunkManager.SubmitPersistenceJob, s.logger)
	require.NoError(t, err)
	state := json.RawMessage(`{"fill":{"0,0":{"cursor":64,"done":false}}}`)
	require.NoError(t, s.spotManager.SaveState(1, 1, state, 100))
	stopped := make(chan struct{})
	go func() { defer close(stopped); stop() }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("accepted spot save did not reach the existing worker")
	}
	select {
	case <-stopped:
		t.Fatal("shard stopped before the accepted spot write completed")
	default:
	}
	// I/O waits must allow other callers to acquire the owning shard lock.
	require.Eventually(t, func() bool {
		if !s.mu.TryLock() {
			return false
		}
		s.mu.Unlock()
		return true
	}, time.Second, time.Millisecond)
	unblock()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("shard did not acknowledge the finished spot save")
	}
	require.Zero(t, s.spotManager.PendingCount())
	snapshot, found := s.spotManager.GetState(1)
	require.True(t, found)
	require.Equal(t, int64(2), snapshot.Revision)
	require.JSONEq(t, string(state), string(snapshot.State))
	require.Equal(t, int64(100), snapshot.LastRuntimeSeconds)
	require.ErrorIs(t, s.spotManager.SaveState(1, 2, state, 101), gameworld.ErrSpotManagerStopped)
	require.False(t, s.chunkManager.SubmitPersistenceJob(&unusedSpotLifecycleJob{}))
}

type unusedSpotLifecycleJob struct{}

func (*unusedSpotLifecycleJob) Run() error     { return nil }
func (*unusedSpotLifecycleJob) Complete(error) {}

func TestSpotSaveShutdownDeadlineReportsAndRetainsPendingState(t *testing.T) {
	logCore, logs := observer.New(zap.ErrorLevel)
	s := &Shard{logger: zap.New(logCore)}
	var err error
	s.spotManager, err = gameworld.NewSpotManager(t.Context(), spotLifecycleConfig(), &spotLifecycleStore{}, func(gameworld.PersistenceJob) bool {
		return true // A worker owns the job but has not finished it.
	}, s.logger)
	require.NoError(t, err)
	state := json.RawMessage(`{"recoveries":{"42":108000}}`)
	require.NoError(t, s.spotManager.SaveState(1, 1, state, 100))
	s.spotManager.StopAdmission()
	s.drainSpotSavesFor(0)
	require.Len(t, logs.FilterMessage("Shutdown spot persistence incomplete").All(), 1)
	require.Equal(t, 1, s.spotManager.PendingCount())
	snapshot, found := s.spotManager.GetState(1)
	require.True(t, found)
	require.Equal(t, int64(1), snapshot.Revision)
	require.JSONEq(t, `{}`, string(snapshot.State))
	require.ErrorIs(t, s.spotManager.SaveState(1, 1, state, 101), gameworld.ErrSpotManagerStopped)
}

func TestSpotSaveShutdownReportsTerminalConflict(t *testing.T) {
	logCore, logs := observer.New(zap.ErrorLevel)
	s := &Shard{logger: zap.New(logCore)}
	store := &spotLifecycleStore{save: func(gameworld.SpotStateWrite) error {
		return gameworld.ErrSpotRevisionConflict
	}}
	var job gameworld.PersistenceJob
	var err error
	s.spotManager, err = gameworld.NewSpotManager(t.Context(), spotLifecycleConfig(), store, func(accepted gameworld.PersistenceJob) bool {
		job = accepted
		return true
	}, s.logger)
	require.NoError(t, err)
	require.NoError(t, s.spotManager.SaveState(1, 1, json.RawMessage(`{"fill":{}}`), 100))
	s.spotManager.Update(time.Now())
	require.NotNil(t, job)
	job.Complete(job.Run())
	s.spotManager.StopAdmission()
	s.drainSpotSavesFor(0)
	require.ErrorIs(t, s.spotManager.Err(), gameworld.ErrSpotRevisionConflict)
	require.Len(t, logs.FilterMessage("Shutdown spot persistence incomplete").All(), 1)
	snapshot, found := s.spotManager.GetState(1)
	require.True(t, found)
	require.Equal(t, int64(1), snapshot.Revision)
	require.JSONEq(t, `{}`, string(snapshot.State))
}

func TestSpotSaveShutdownDrainsHealthyWritesAfterAnotherSpotConflicts(t *testing.T) {
	logCore, logs := observer.New(zap.ErrorLevel)
	s := &Shard{logger: zap.New(logCore)}
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	store := &spotLifecycleStore{rows: []repository.Spot{
		{ID: 1, Region: 1, Layer: 0, SpotType: "www", CenterX: 100, CenterY: 100, Radius: 40, PeakQuality: 30, State: json.RawMessage(`{}`), Revision: 1},
		{ID: 2, Region: 1, Layer: 0, SpotType: "www", CenterX: 200, CenterY: 100, Radius: 40, PeakQuality: 30, State: json.RawMessage(`{}`), Revision: 1},
	}, save: func(write gameworld.SpotStateWrite) error {
		if write.ID == 1 {
			return gameworld.ErrSpotRevisionConflict
		}
		close(started)
		<-release
		return nil
	}}
	var jobs []gameworld.PersistenceJob
	var err error
	s.spotManager, err = gameworld.NewSpotManager(t.Context(), spotLifecycleConfig(), store, func(job gameworld.PersistenceJob) bool {
		jobs = append(jobs, job)
		return true
	}, s.logger)
	require.NoError(t, err)
	require.NoError(t, s.spotManager.SaveState(1, 1, json.RawMessage(`{"failed":true}`), 100))
	require.NoError(t, s.spotManager.SaveState(2, 1, json.RawMessage(`{"saved":true}`), 100))
	require.Len(t, jobs, 2)
	jobs[0].Complete(jobs[0].Run())
	s.spotManager.Update(time.Now())
	require.ErrorIs(t, s.spotManager.Err(), gameworld.ErrSpotRevisionConflict)
	go func() { jobs[1].Complete(jobs[1].Run()) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("healthy write did not start")
	}
	s.spotManager.StopAdmission()
	drained := make(chan struct{})
	go func() { defer close(drained); s.drainSpotSavesFor(100 * time.Millisecond) }()
	select {
	case <-drained:
		t.Fatal("a conflict stopped draining before the healthy write finished")
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("spot drain did not respect its deadline")
	}
	require.Equal(t, 1, s.spotManager.PendingCount(), "only the conflicted snapshot remains pending")
	snapshot, found := s.spotManager.GetState(2)
	require.True(t, found)
	require.Equal(t, int64(2), snapshot.Revision)
	require.JSONEq(t, `{"saved":true}`, string(snapshot.State))
	require.Len(t, logs.FilterMessage("Shutdown spot persistence incomplete").All(), 1)
}
