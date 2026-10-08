package core

import (
	"fmt"
	"testing"

	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func rawCacheObjects(count int) []*repository.Object {
	objects := make([]*repository.Object, count, count+32)
	for i := range objects {
		objects[i] = &repository.Object{ID: int64(i + 1)}
	}
	return objects
}

func TestRawActivationPrefixPreservesTailAndIndexedUpdates(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	objects := rawCacheObjects(10000)
	retained := []*repository.Object{objects[1], objects[4]}
	unvisited := objects[32]
	chunk.SetRawObjects(objects)
	rows := make(map[types.EntityID][]repository.Inventory, len(objects))
	for _, object := range objects {
		rows[types.EntityID(object.ID)] = []repository.Inventory{{OwnerID: object.ID}}
	}
	chunk.SetRawInventoriesByOwner(rows)
	dirty := map[types.EntityID]struct{}{1: {}, 2: {}, 33: {}}
	chunk.SetRawDirtyObjectIDs(dirty)
	chunk.RetainRawActivationPrefix(32, retained)
	after := chunk.GetRawObjects()
	require.Len(t, after, 9970)
	require.Equal(t, retained, after[:2])
	require.Same(t, unvisited, after[2])
	require.Equal(t, &objects[32], &after[2], "the unvisited suffix must retain its backing storage")
	require.Len(t, chunk.GetRawInventoriesByOwner(), 9970)
	require.NotContains(t, rows, types.EntityID(1))
	require.Contains(t, rows, types.EntityID(2))
	require.Equal(t, dirty, chunk.GetRawDirtyObjectIDs(), "active objects retain pending write intent")
	require.Equal(t, 9999, chunk.rawObjectIndex[10000], "tail indices must not be rebuilt")
	updated := &repository.Object{ID: 10000, X: 7}
	chunk.InsertCommittedObject(updated, nil)
	require.Same(t, updated, after[len(after)-1])
	chunk.InsertCommittedObject(&repository.Object{ID: 10001}, nil)
	require.Len(t, chunk.GetRawObjects(), 9971)
	require.Equal(t, int64(10001), chunk.GetRawObjects()[9970].ID)
}

func TestCommittedRawRemovalLeavesBoundedActivationHole(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	objects := rawCacheObjects(10000)
	chunk.SetRawObjects(objects)
	chunk.RemoveCommittedObject(5000)
	require.Nil(t, chunk.GetRawObjects()[4999])
	require.NotContains(t, chunk.rawObjectIndex, types.EntityID(5000))
	require.Equal(t, 9999, chunk.rawObjectIndex[10000])
	chunk.RetainRawActivationPrefix(32, nil)
	require.Equal(t, int64(33), chunk.GetRawObjects()[0].ID)
	require.Nil(t, chunk.GetRawObjects()[4967])
	require.Equal(t, 9999, chunk.rawObjectIndex[10000])
	chunk.InsertCommittedObject(&repository.Object{ID: 5000, X: 8}, nil)
	require.Equal(t, int64(5000), chunk.GetRawObjects()[len(chunk.GetRawObjects())-1].ID)
}

func TestRawActivationCursorVisitsTailBeforeRetainedFailures(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	objects := rawCacheObjects(10000)
	chunk.SetRawObjects(objects)
	start, pending := chunk.GetRawActivationObjects()
	require.Zero(t, start)
	require.Same(t, objects[0], pending[0])
	chunk.RetainRawActivationRange(start, 32, append([]*repository.Object(nil), pending[:32]...))
	start, pending = chunk.GetRawActivationObjects()
	require.Equal(t, 32, start)
	require.Equal(t, &objects[32], &pending[0], "the unvisited tail must retain its backing storage")
	chunk.RetainRawActivationRange(start, 32, nil)
	require.Equal(t, 9999, chunk.rawObjectIndex[10000], "tail indices are never rebuilt")
	start, pending = chunk.GetRawActivationObjects()
	require.Equal(t, 64, start)
	require.Same(t, objects[64], pending[0])
	updated := &repository.Object{ID: 10000, X: 7}
	chunk.InsertCommittedObject(updated, nil)
	require.Same(t, updated, pending[len(pending)-1])
	chunk.RemoveCommittedObject(100)
	require.Nil(t, pending[35])
	// After the remaining suffix was visited, the failures become eligible again.
	chunk.RetainRawActivationRange(start, len(pending), nil)
	start, pending = chunk.GetRawActivationObjects()
	require.Zero(t, start)
	require.Len(t, pending, 64)
	require.Same(t, objects[0], pending[0])
	chunk.RetainRawActivationRange(start, len(pending), nil)
	require.Empty(t, chunk.GetRawObjects())
	require.Empty(t, chunk.rawObjectIndex)
}

func TestRawActivationCursorSurvivesCommittedHeadRemovalAndAppend(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	objects := rawCacheObjects(64)
	chunk.SetRawObjects(objects)
	chunk.RetainRawActivationRange(0, 32, append([]*repository.Object(nil), objects[:32]...))
	chunk.RemoveCommittedObject(1)
	start, pending := chunk.GetRawActivationObjects()
	require.Equal(t, 31, start)
	require.Same(t, objects[32], pending[0])
	chunk.InsertCommittedObject(&repository.Object{ID: 65}, nil)
	chunk.RetainRawActivationRange(start, len(pending), nil)
	start, pending = chunk.GetRawActivationObjects()
	require.Zero(t, start)
	require.Equal(t, int64(2), pending[0].ID)
	// A committed append cannot extend the previous cycle forever. The retained
	// failures get their next attempt before that newly appended record is visited.
	chunk.RetainRawActivationRange(start, 31, append([]*repository.Object(nil), pending[:31]...))
	start, pending = chunk.GetRawActivationObjects()
	require.Equal(t, 31, start)
	chunk.RetainRawActivationRange(start, 32, nil)
	start, pending = chunk.GetRawActivationObjects()
	require.Equal(t, 63, start)
	require.Equal(t, int64(65), pending[0].ID)
}

func TestRawActivationCursorAndRetentionAllocateNothing(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetRawObjects(rawCacheObjects(10240))
	var retained [32]*repository.Object
	allocs := testing.AllocsPerRun(1000, func() {
		start, pending := chunk.GetRawActivationObjects()
		copy(retained[:], pending[:len(retained)])
		chunk.RetainRawActivationRange(start, len(retained), retained[:])
	})
	require.Zero(t, allocs)
	require.Len(t, chunk.GetRawObjects(), 10240)
}

func TestRawIndexPreservesDuplicateCacheInputSemantics(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	oldDuplicate := &repository.Object{ID: 1, X: 9}
	chunk.SetRawObjects([]*repository.Object{{ID: 1}, {ID: 2}, oldDuplicate})
	updated := &repository.Object{ID: 1, X: 7}
	chunk.InsertCommittedObject(updated, nil)
	require.Same(t, updated, chunk.GetRawObjects()[0])
	require.Same(t, oldDuplicate, chunk.GetRawObjects()[2])
	chunk.RemoveCommittedObject(1)
	require.Len(t, chunk.GetRawObjects(), 1)
	require.Equal(t, int64(2), chunk.GetRawObjects()[0].ID)
}

func TestCommittedRemovalFencesExistOnlyDuringProtectedLoad(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	for id := types.EntityID(1); id <= 10000; id++ {
		chunk.RemoveCommittedObject(id)
	}
	require.Empty(t, chunk.committedRemovedIDs, "active chunk history must not accumulate")
	revision := chunk.beginCacheLoad()
	chunk.RemoveCommittedObject(5)
	require.Contains(t, chunk.committedRemovedIDs, types.EntityID(5))
	chunk.installLoadedObjects(revision, []*repository.Object{{ID: 5}, {ID: 6}}, map[types.EntityID][]repository.Inventory{5: {{OwnerID: 5}}})
	require.Len(t, chunk.GetRawObjects(), 1)
	require.Equal(t, int64(6), chunk.GetRawObjects()[0].ID)
	require.Empty(t, chunk.committedRemovedIDs)
	require.False(t, chunk.cacheLoadInFlight)
	chunk.beginCacheLoad()
	chunk.RemoveCommittedObject(6)
	chunk.finishCacheLoad() // A DB read failure releases the same fence.
	require.Empty(t, chunk.committedRemovedIDs)
	require.False(t, chunk.cacheLoadInFlight)
}

func BenchmarkCommittedRawCacheUpdate(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
			chunk.SetRawObjects(rawCacheObjects(count))
			updated := &repository.Object{ID: int64(count)}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				chunk.InsertCommittedObject(updated, nil)
			}
		})
	}
}
