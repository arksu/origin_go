package world

import (
	"errors"
	"testing"
	"time"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func newActivationPreparerChunk(cm *ChunkManager, objects ...*repository.Object) *core.Chunk {
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStatePreloaded)
	chunk.SetRawObjects(objects)
	cm.chunks[chunk.Coord] = chunk
	interest := newChunkInterest()
	interest.activeEntities[1] = struct{}{}
	cm.chunkInterests[chunk.Coord] = interest
	return chunk
}

func activationPreparerObject(id int64) *repository.Object {
	return &repository.Object{ID: id, TypeID: 9401, Region: 1, X: int(id % 100), Y: 10}
}

func TestActivationPreparerRunsBeforeBuildAndReadsUpdatedRawObjects(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm, _ := newCapacityChunkManager(t, 2)
	original := activationPreparerObject(9501)
	original.TypeID = 9499
	chunk := newActivationPreparerChunk(cm, original)
	prepared := false
	builds := 0
	cm.world.AddComponentObserver(components.EntityInfoComponentID, func(types.Handle) {
		require.True(t, prepared, "preparation must finish before the first BuildForChunk")
		builds++
	})
	cm.SetActivationPreparer(func(current *core.Chunk) error {
		require.Same(t, chunk, current)
		require.Zero(t, cm.world.EntityCount())
		require.Empty(t, current.GetHandles())
		require.Empty(t, cm.ActiveChunkCoords())
		gate := cm.persistenceFor(current.Coord)
		if gate.ioMu.TryLock() {
			gate.ioMu.Unlock()
			t.Fatal("activation preparation must hold the existing persistence gate")
		}
		// Appending reallocates this one-row cache. Replacing the original only
		// afterward makes a raw slice captured before preparation stale.
		current.UpsertRawObject(activationPreparerObject(9502))
		replacement := *original
		replacement.TypeID = 9401
		replacement.X = 30
		current.UpsertRawObject(&replacement)
		prepared = true
		return nil
	})

	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, 1, builds)
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
	require.Equal(t, 1, chunk.Spatial().StaticCount())
	require.Equal(t, []*repository.Object{activationPreparerObject(9502)}, chunk.GetRawObjects(),
		"appended objects remain outside the current activation suffix")
	handle := cm.world.GetHandleByEntityID(9501)
	info, ok := ecs.GetComponent[components.EntityInfo](cm.world, handle)
	require.True(t, ok)
	require.EqualValues(t, 9401, info.TypeID)
	transform, ok := ecs.GetComponent[components.Transform](cm.world, handle)
	require.True(t, ok)
	require.Equal(t, float64(30), transform.X)
	require.Equal(t, 9499, original.TypeID, "replacement must not mutate the old snapshot")
}

func TestActivationPreparerFailureDoesNotBuildOrExposeObjects(t *testing.T) {
	installCapacityObjectDefinitions(t)
	for _, state := range []types.ChunkState{types.ChunkStatePreloaded, types.ChunkStateInactive} {
		t.Run(state.String(), func(t *testing.T) {
			cm, _ := newCapacityChunkManager(t, 1)
			raw := activationPreparerObject(9501)
			chunk := newActivationPreparerChunk(cm, raw)
			chunk.SetState(state)
			failure := errors.New("ownership preparation failed")
			cm.SetActivationPreparer(func(*core.Chunk) error { return failure })

			require.ErrorIs(t, cm.activateChunkInternal(chunk.Coord, chunk), failure)
			require.Equal(t, state, chunk.GetState())
			require.Zero(t, cm.world.EntityCount())
			require.Empty(t, chunk.GetHandles())
			require.Empty(t, cm.ActiveChunkCoords())
			require.Equal(t, []*repository.Object{raw}, chunk.GetRawObjects())
			gate := cm.persistenceFor(chunk.Coord)
			require.True(t, gate.ioMu.TryLock(), "failure must release the existing gate")
			gate.ioMu.Unlock()
		})
	}
}

func TestActivationPreparerRunsOnlyAfterPersistenceAndStateGuards(t *testing.T) {
	tests := []struct {
		name  string
		state types.ChunkState
		busy  bool
		pins  int32
		err   error
	}{
		{name: "busy", state: types.ChunkStatePreloaded, busy: true, err: ErrChunkPersistenceBusy},
		{name: "pinned_preloaded", state: types.ChunkStatePreloaded, pins: 1, err: ErrChunkPersistenceBusy},
		{name: "pinned_inactive", state: types.ChunkStateInactive, pins: 1, err: ErrChunkPersistenceBusy},
		{name: "unloaded", state: types.ChunkStateUnloaded, err: ErrChunkNotLoaded},
		{name: "loading", state: types.ChunkStateLoading, err: ErrChunkNotLoaded},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cm, _ := newCapacityChunkManager(t, 1)
			chunk := newActivationPreparerChunk(cm, activationPreparerObject(9501))
			chunk.SetState(test.state)
			gate := cm.persistenceFor(chunk.Coord)
			gate.pins.Store(test.pins)
			if test.busy {
				gate.ioMu.Lock()
				defer gate.ioMu.Unlock()
			}
			calls := 0
			cm.SetActivationPreparer(func(*core.Chunk) error { calls++; return nil })

			require.ErrorIs(t, cm.activateChunkInternal(chunk.Coord, chunk), test.err)
			require.Zero(t, calls)
			require.Equal(t, test.state, chunk.GetState())
			require.Zero(t, cm.world.EntityCount())
			require.Empty(t, chunk.GetHandles())
		})
	}
}

func TestActivationWithoutPreparerPreservesOrdinaryRestore(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm, _ := newCapacityChunkManager(t, 1)
	chunk := newActivationPreparerChunk(cm, activationPreparerObject(9501))

	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.True(t, cm.world.Alive(cm.world.GetHandleByEntityID(9501)))
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
	require.Len(t, chunk.GetHandles(), 1)
	require.Empty(t, chunk.GetRawObjects())
}

func TestActivationPreparerRunsOnCapacityRetryAndReactivation(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm, _ := newCapacityChunkManager(t, 2)
	blocker := cm.world.SpawnWithoutExternalID()
	first, second := activationPreparerObject(9501), activationPreparerObject(9502)
	chunk := newActivationPreparerChunk(cm, first, second)
	var preparedIDs [][]int64
	cm.SetActivationPreparer(func(current *core.Chunk) error {
		_, raw := current.GetRawActivationObjects()
		ids := make([]int64, 0, len(raw))
		for _, object := range raw {
			ids = append(ids, object.ID)
		}
		preparedIDs = append(preparedIDs, ids)
		return nil
	})

	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	firstHandle := cm.world.GetHandleByEntityID(9501)
	require.True(t, cm.world.Alive(firstHandle))
	require.Equal(t, []*repository.Object{second}, chunk.GetRawObjects())
	require.Contains(t, cm.activationRetries, chunk.Coord)
	require.True(t, cm.world.Despawn(blocker))
	// Existing pins permit retries on an already active chunk.
	gate := cm.persistenceFor(chunk.Coord)
	gate.pins.Store(1)
	retry := cm.activationRetries[chunk.Coord]
	retry.due = time.Now().Add(-time.Second)
	cm.activationRetries[chunk.Coord] = retry
	cm.Update(0)
	gate.pins.Store(0)
	require.Equal(t, [][]int64{{9501, 9502}, {9502}}, preparedIDs)
	require.Equal(t, firstHandle, cm.world.GetHandleByEntityID(9501))
	require.True(t, cm.world.Alive(cm.world.GetHandleByEntityID(9502)))
	require.Empty(t, chunk.GetRawObjects())
	require.NotContains(t, cm.activationRetries, chunk.Coord)

	require.NoError(t, cm.deactivateChunkInternal(chunk))
	require.Zero(t, cm.world.EntityCount())
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Len(t, preparedIDs, 3)
	require.ElementsMatch(t, []int64{9501, 9502}, preparedIDs[2])
	require.Equal(t, 2, cm.world.EntityCount())
	require.Len(t, chunk.GetHandles(), 2)
}

func TestActivationPreparerUsesCurrentSuffixAfterRetainedBuildFailure(t *testing.T) {
	installCapacityObjectDefinitions(t)
	cm, _ := newCapacityChunkManager(t, 2)
	blocker := cm.world.SpawnWithoutExternalID()
	invalid := activationPreparerObject(9501)
	invalid.TypeID = 9499
	chunk := newActivationPreparerChunk(cm, invalid, activationPreparerObject(9502), activationPreparerObject(9503))
	calls := 0
	cm.SetActivationPreparer(func(current *core.Chunk) error {
		calls++
		start, raw := current.GetRawActivationObjects()
		if calls == 1 {
			require.Zero(t, start)
			require.Len(t, raw, 3)
			return nil
		}
		require.Equal(t, 1, start, "retained failure must not reset the activation cursor")
		require.Len(t, raw, 1)
		require.EqualValues(t, 9503, raw[0].ID)
		replacement := *raw[0]
		replacement.X = 40
		current.UpsertRawObject(&replacement)
		return nil
	})

	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.True(t, cm.world.Alive(cm.world.GetHandleByEntityID(9502)))
	require.Len(t, chunk.GetRawObjects(), 2)
	require.True(t, cm.world.Despawn(blocker))
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, 2, calls)
	transform, ok := ecs.GetComponent[components.Transform](cm.world, cm.world.GetHandleByEntityID(9503))
	require.True(t, ok)
	require.Equal(t, float64(40), transform.X)
	require.Equal(t, []*repository.Object{invalid}, chunk.GetRawObjects())
}
