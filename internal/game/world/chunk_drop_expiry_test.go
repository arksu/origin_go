package world

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type dropExpiryTestDeleter struct {
	calls  int
	region int
	id     types.EntityID
	err    error
}

func (d *dropExpiryTestDeleter) DeleteObject(region int, id types.EntityID) error {
	d.calls++
	d.region, d.id = region, id
	return d.err
}

// Persistence jobs are consumed explicitly so a test can distinguish activation,
// worker I/O and application of results on the owning shard.
func newDropExpiryTestManager(t *testing.T) *ChunkManager {
	t.Helper()
	cfg := newTestConfig()
	cfg.Game.LoadWorkers, cfg.Game.SaveWorkers = 0, 0
	cm := NewChunkManager(cfg, nil, ecs.NewWorldForTesting(), nil, 0, 1, NewObjectFactory(nil), nil, nil, zap.NewNop())
	t.Cleanup(cm.Stop)
	ecs.SetResource(cm.world, ecs.TimeState{Now: time.Unix(1000, 0), RuntimeSecondsTotal: 110})
	return cm
}

func expiredDropRaw(t *testing.T, id types.EntityID) *repository.Object {
	t.Helper()
	data, err := json.Marshal(DroppedItemData{HasInventory: true, ContainedItemID: uint64(id), DropTime: 100, TimeBasis: constt.DroppedItemTimeBasisRuntimeSecondsV1})
	require.NoError(t, err)
	return &repository.Object{ID: int64(id), TypeID: constt.DroppedItemTypeID, Region: 1,
		Data: pqtype.NullRawMessage{RawMessage: data, Valid: true}}
}

func expiryChunk(cm *ChunkManager, raw ...*repository.Object) *core.Chunk {
	chunk := core.NewChunk(types.ChunkCoord{X: 2, Y: 2}, 1, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStatePreloaded)
	chunk.SetRawObjects(raw)
	cm.chunks[chunk.Coord] = chunk
	return chunk
}

func runExpiryTestJob(t *testing.T, cm *ChunkManager) {
	t.Helper()
	require.NotEmpty(t, cm.saveQueue)
	job := (<-cm.saveQueue).job
	require.NotNil(t, job)
	job.Complete(job.Run())
}

func TestExpiredDropActivationDefersDatabaseAndCacheRemoval(t *testing.T) {
	cm := newDropExpiryTestManager(t)
	deleter := &dropExpiryTestDeleter{}
	cm.objectFactory.SetObjectDeleter(deleter)
	raw := expiredDropRaw(t, 900)
	chunk := expiryChunk(cm, raw)
	chunk.SetRawInventoriesByOwner(map[types.EntityID][]repository.Inventory{
		900: {{OwnerID: 900, Kind: int16(constt.InventoryDroppedItem)}},
	})

	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Zero(t, deleter.calls, "activation must not execute SQL")
	require.Equal(t, []*repository.Object{raw}, chunk.GetRawObjects())
	require.Empty(t, chunk.GetHandles())
	require.EqualValues(t, 1, cm.persistenceFor(chunk.Coord).pins.Load())
	require.Equal(t, committedDroppedActivationBudget-1, cm.remainingDroppedActivations)

	runExpiryTestJob(t, cm)
	require.Equal(t, 1, deleter.calls)
	require.Equal(t, types.EntityID(900), deleter.id)
	require.Equal(t, 1, deleter.region)
	require.Len(t, chunk.GetRawObjects(), 1, "workers must not mutate caches")
	cm.updateExpiredDrops()
	require.Empty(t, chunk.GetRawObjects())
	require.Empty(t, chunk.GetRawInventoriesByOwner())
	require.Zero(t, cm.persistenceFor(chunk.Coord).pins.Load())
	require.Empty(t, cm.dropExpiry.byID)
	require.Equal(t, 1, deleter.calls, "applying completion must not execute SQL")
}

func TestExpiredDropShutdownPumpUsesExplicitClock(t *testing.T) {
	cm := newDropExpiryTestManager(t)
	deleter := &dropExpiryTestDeleter{err: errors.New("unavailable")}
	cm.objectFactory.SetObjectDeleter(deleter)
	chunk := expiryChunk(cm, expiredDropRaw(t, 906))
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, 1, cm.PendingExpiredDrops())
	state := ecs.GetResource[ecs.TimeState](cm.world)
	runExpiryTestJob(t, cm)
	cm.UpdateExpiredDrops(state.Now)
	cm.UpdateExpiredDrops(state.Now.Add(time.Second))
	deleter.err = nil
	runExpiryTestJob(t, cm)
	cm.UpdateExpiredDrops(state.Now.Add(time.Second))
	require.Zero(t, cm.PendingExpiredDrops())
	require.Empty(t, chunk.GetRawObjects())
	require.Zero(t, cm.persistenceFor(chunk.Coord).pins.Load())
	// The shutdown pump advances retry scheduling without changing ECS time.
	require.Equal(t, time.Unix(1000, 0), state.Now)
}

func TestExpiredDropDeleteFailureRetainsDataAndRetriesInRuntimeTime(t *testing.T) {
	cm := newDropExpiryTestManager(t)
	deleter := &dropExpiryTestDeleter{err: errors.New("database unavailable")}
	cm.objectFactory.SetObjectDeleter(deleter)
	raw := expiredDropRaw(t, 901)
	chunk := expiryChunk(cm, raw)
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	runExpiryTestJob(t, cm)
	cm.updateExpiredDrops()
	require.Equal(t, []*repository.Object{raw}, chunk.GetRawObjects())
	require.EqualValues(t, 1, cm.persistenceFor(chunk.Coord).pins.Load())
	require.Empty(t, cm.saveQueue)
	state := ecs.GetResource[ecs.TimeState](cm.world)
	state.Now = state.Now.Add(time.Second - time.Nanosecond)
	cm.updateExpiredDrops()
	require.Empty(t, cm.saveQueue)
	state.Now = state.Now.Add(time.Nanosecond)
	cm.updateExpiredDrops()
	deleter.err = nil
	runExpiryTestJob(t, cm)
	cm.updateExpiredDrops()
	require.Empty(t, chunk.GetRawObjects())
	require.Equal(t, 2, deleter.calls)
	require.Zero(t, cm.persistenceFor(chunk.Coord).pins.Load())
}

func TestExpiredDropQueueFullPreservesAdmission(t *testing.T) {
	cm := newDropExpiryTestManager(t)
	cm.saveQueue = make(chan saveRequest, 1)
	cm.saveQueue <- saveRequest{}
	deleter := &dropExpiryTestDeleter{}
	cm.objectFactory.SetObjectDeleter(deleter)
	raw := expiredDropRaw(t, 902)
	chunk := expiryChunk(cm, raw)
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Len(t, cm.dropExpiry.byID, 1)
	require.EqualValues(t, 1, cm.persistenceFor(chunk.Coord).pins.Load())
	require.Equal(t, []*repository.Object{raw}, chunk.GetRawObjects())
	<-cm.saveQueue
	cm.updateExpiredDrops()
	runExpiryTestJob(t, cm)
	cm.updateExpiredDrops()
	require.Empty(t, chunk.GetRawObjects())
	require.Zero(t, cm.persistenceFor(chunk.Coord).pins.Load())
}

func TestExpiredDropRetryBackoffStopsAtSixtySeconds(t *testing.T) {
	cm := newDropExpiryTestManager(t)
	cm.objectFactory.SetObjectDeleter(&dropExpiryTestDeleter{err: errors.New("unavailable")})
	chunk := expiryChunk(cm, expiredDropRaw(t, 905))
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	for _, delay := range []time.Duration{1, 2, 4, 8, 16, 32, 60, 60} {
		runExpiryTestJob(t, cm)
		cm.updateExpiredDrops()
		op := cm.dropExpiry.byID[905]
		require.Equal(t, delay*time.Second, op.backoff)
		state := ecs.GetResource[ecs.TimeState](cm.world)
		require.Equal(t, state.Now.Add(delay*time.Second), op.due)
		state.Now = op.due
		cm.updateExpiredDrops()
	}
	require.Len(t, chunk.GetRawObjects(), 1)
	require.EqualValues(t, 1, cm.persistenceFor(chunk.Coord).pins.Load())
}

func TestExpiredDropAdmissionAndCompletionAreBounded(t *testing.T) {
	cm := newDropExpiryTestManager(t)
	cm.objectFactory.SetObjectDeleter(&dropExpiryTestDeleter{})
	chunk := expiryChunk(cm)
	gate := cm.persistenceFor(chunk.Coord)
	gate.ioMu.Lock()
	for i := 0; i <= chunkDropExpiryCapacity; i++ {
		cm.queueExpiredDropRaw(chunk, expiredDropRaw(t, types.EntityID(1000+i)))
	}
	gate.ioMu.Unlock()
	require.Len(t, cm.dropExpiry.byID, chunkDropExpiryCapacity)
	require.Len(t, cm.saveQueue, chunkDropExpiryCapacity)
	require.EqualValues(t, chunkDropExpiryCapacity, gate.pins.Load())
	// A retained duplicate must not reserve a second operation or pin.
	gate.ioMu.Lock()
	cm.queueExpiredDropRaw(chunk, expiredDropRaw(t, 1000))
	gate.ioMu.Unlock()
	require.EqualValues(t, chunkDropExpiryCapacity, gate.pins.Load())
	for len(cm.saveQueue) > 0 {
		runExpiryTestJob(t, cm)
	}
	cm.updateExpiredDrops()
	require.Len(t, cm.dropExpiry.byID, chunkDropExpiryCapacity-chunkDropExpiryCompletionBudget)
	require.EqualValues(t, chunkDropExpiryCapacity-chunkDropExpiryCompletionBudget, gate.pins.Load())
	for len(cm.dropExpiry.byID) > 0 {
		cm.updateExpiredDrops()
	}
	require.Zero(t, gate.pins.Load())
}

func TestExpiredDropDetectionPreservesLiveOrMalformedRaw(t *testing.T) {
	cm := newDropExpiryTestManager(t)
	malformed := expiredDropRaw(t, 903)
	malformed.Data.RawMessage = []byte(`{"contained_item_id":4}`)
	_, err := cm.objectFactory.BuildForChunk(cm.world, malformed, nil)
	require.NotErrorIs(t, err, ErrDroppedItemExpired)
	state := ecs.GetResource[ecs.TimeState](cm.world)
	state.RuntimeSecondsTotal = 109
	_, err = cm.objectFactory.BuildForChunk(cm.world, expiredDropRaw(t, 904), nil)
	require.NotErrorIs(t, err, ErrDroppedItemExpired)
	require.Empty(t, cm.dropExpiry.byID)
	require.Empty(t, cm.saveQueue)
}
