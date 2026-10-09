package world

import (
	"errors"
	"sync"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
)

type persistenceTestJob struct {
	err  error
	done chan error
}

func (j *persistenceTestJob) Run() error         { return j.err }
func (j *persistenceTestJob) Complete(err error) { j.done <- err }

func TestPersistenceJobUsesSaveWorkersAndReturnsError(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()
	want := errors.New("worker failure")
	job := &persistenceTestJob{err: want, done: make(chan error, 1)}
	require.True(t, cm.SubmitPersistenceJob(job))
	select {
	case got := <-job.done:
		require.ErrorIs(t, got, want)
	case <-time.After(time.Second):
		t.Fatal("persistence job was not executed")
	}
}

func TestPersistenceJobAdmissionNeverWaitsForQueueSpace(t *testing.T) {
	cm := &ChunkManager{saveQueue: make(chan saveRequest, 1)}
	job := &persistenceTestJob{done: make(chan error, 1)}
	require.True(t, cm.SubmitPersistenceJob(job))
	require.False(t, cm.SubmitPersistenceJob(job))
	require.False(t, cm.SubmitPersistenceJob(nil))
}

func TestPersistencePinAndWorkerGatePreserveChunk(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()
	coord := types.ChunkCoord{X: 2, Y: 2}
	chunk := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[coord] = chunk
	require.NoError(t, cm.PinPersistence(coord))
	require.ErrorIs(t, cm.deactivateChunkInternal(chunk), ErrChunkPersistenceBusy)
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
	cm.UnpinPersistence(coord)
	gate := cm.persistenceFor(coord)
	gate.ioMu.Lock()
	chunk.SetState(types.ChunkStatePreloaded)
	require.ErrorIs(t, cm.PinPersistence(coord), ErrChunkPersistenceBusy)
	require.ErrorIs(t, cm.activateChunkInternal(coord, chunk), ErrChunkPersistenceBusy)
	gate.ioMu.Unlock()
	require.NoError(t, cm.activateChunkInternal(coord, chunk))
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
}

func TestCrossChunkPersistenceCanonicalLockOrder(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()
	a, b := types.ChunkCoord{X: 1}, types.ChunkCoord{X: 2}
	var wg sync.WaitGroup
	for _, coords := range [][]types.ChunkCoord{{a, b, a}, {b, a}} {
		wg.Add(1)
		go func(coords []types.ChunkCoord) {
			defer wg.Done()
			require.NoError(t, cm.WithPersistence(coords, func() error { return nil }))
		}(coords)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cross-chunk persistence deadlocked")
	}
}

func TestCommittedDropOwnsCleanCacheData(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()
	coord := types.ChunkCoord{X: 2, Y: 2}
	chunk := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStatePreloaded)
	cm.chunks[coord] = chunk
	object := repository.Object{ID: 900, TypeID: constt.DroppedItemTypeID, Region: 1, ChunkX: 2, ChunkY: 2, Data: pqtype.NullRawMessage{RawMessage: []byte(`{"a":1}`), Valid: true}}
	root := repository.Inventory{OwnerID: 900, Kind: int16(constt.InventoryDroppedItem), Version: 1, Data: []byte(`{"items":[]}`)}
	require.NoError(t, cm.InsertCommittedDropped(&object, &root))
	object.Data.RawMessage[2] = 'b'
	root.Data[2] = 'x'
	require.Equal(t, `{"a":1}`, string(chunk.GetRawObjects()[0].Data.RawMessage))
	require.Equal(t, `{"items":[]}`, string(chunk.GetRawInventoriesByOwner()[900][0].Data))
	require.Empty(t, chunk.GetRawDirtyObjectIDs())
	cm.RemoveCommittedSource(coord, 900)
	require.Empty(t, chunk.GetRawObjects())
}

func TestCommittedDropInsertionDoesNotWaitForInFlightLoad(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()
	coord := types.ChunkCoord{X: 2, Y: 2}
	chunk := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStatePreloaded)
	cm.chunks[coord] = chunk
	object := repository.Object{ID: 901, TypeID: constt.DroppedItemTypeID, Region: 1, ChunkX: 2, ChunkY: 2}
	root := repository.Inventory{OwnerID: 901, Kind: int16(constt.InventoryDroppedItem), Version: 1}
	gate := cm.persistenceFor(coord)
	gate.ioMu.Lock()
	err := cm.InsertCommittedDropped(&object, &root)
	gate.ioMu.Unlock()
	require.ErrorIs(t, err, ErrChunkPersistenceBusy)
	require.Empty(t, chunk.GetRawObjects())
	require.NoError(t, cm.InsertCommittedDropped(&object, &root))
	require.Len(t, chunk.GetRawObjects(), 1)
}

func TestCommittedSourceReplacementOwnsCleanInventoryFreeSnapshot(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	coord := types.ChunkCoord{X: 2, Y: 2}
	chunk := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStatePreloaded)
	cm.chunks[coord] = chunk
	chunk.SetRawObjects([]*repository.Object{{ID: 902, TypeID: objectHealthContainerTypeID}})
	chunk.SetRawInventoriesForOwner(902, []repository.Inventory{{OwnerID: 902, Data: []byte(`{"items":[{"item_id":1}]}`)}})
	chunk.SetRawDirtyObjectIDs(map[types.EntityID]struct{}{902: {}})
	chunk.MarkDeletedObjectID(902)
	raw := transformationReplacement(902)
	raw.ChunkX, raw.ChunkY = coord.X, coord.Y
	raw.Data = pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{"a":1}`)}
	require.NoError(t, cm.PinPersistence(coord))
	require.NoError(t, cm.ReplaceCommittedSource(raw))
	raw.Data.RawMessage[2] = 'b'
	raw.TypeID = objectHealthContainerTypeID
	objects := chunk.GetRawObjects()
	require.Len(t, objects, 1)
	require.Equal(t, int64(902), objects[0].ID)
	require.Equal(t, objectHealthLifecycleTypeID, objects[0].TypeID)
	require.Equal(t, `{"a":1}`, string(objects[0].Data.RawMessage))
	require.Empty(t, chunk.GetRawInventoriesByOwner()[902])
	require.Empty(t, chunk.GetRawDirtyObjectIDs())
	require.Empty(t, chunk.GetDeletedObjectIDs())
	require.EqualValues(t, 1, cm.persistenceFor(coord).pins.Load(), "cache application must not release the operation pin")
	cm.UnpinPersistence(coord)
}

func TestCommittedSourceReplacementInsertionAndValidation(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	coord := types.ChunkCoord{X: 2, Y: 2}
	chunk := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStatePreloaded)
	cm.chunks[coord] = chunk
	raw := transformationReplacement(903)
	raw.ChunkX, raw.ChunkY = coord.X, coord.Y
	gate := cm.persistenceFor(coord)
	gate.ioMu.Lock()
	err := cm.ReplaceCommittedSource(raw)
	gate.ioMu.Unlock()
	require.ErrorIs(t, err, ErrChunkPersistenceBusy)
	require.Empty(t, chunk.GetRawObjects())
	require.NoError(t, cm.ReplaceCommittedSource(raw))
	require.Len(t, chunk.GetRawObjects(), 1)
	for _, mutate := range []func(*repository.Object){
		func(raw *repository.Object) { raw.Region++ },
		func(raw *repository.Object) { raw.Layer++ },
		func(raw *repository.Object) { raw.Hp.Valid = false },
	} {
		invalid := *raw
		mutate(&invalid)
		require.ErrorIs(t, cm.ReplaceCommittedSource(&invalid), ErrInvalidCommittedObject)
	}
	raw.ChunkX = 500
	require.ErrorIs(t, cm.ReplaceCommittedSource(raw), ErrChunkOutsideWorld)
	require.ErrorIs(t, cm.ReplaceCommittedSource(nil), ErrInvalidCommittedObject)
}
