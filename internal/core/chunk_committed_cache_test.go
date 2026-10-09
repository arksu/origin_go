package core

import (
	"testing"

	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestCommittedObjectCacheDoesNotCreateDirtyIntent(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.InsertCommittedObject(&repository.Object{ID: 5}, []repository.Inventory{{OwnerID: 5, Data: []byte(`{"items":[]}`)}})
	require.False(t, chunk.rawDataDirty)
	require.Empty(t, chunk.GetRawDirtyObjectIDs())
	require.Len(t, chunk.GetRawObjects(), 1)
	chunk.MarkDeletedObjectID(5)
	chunk.RemoveCommittedObject(5)
	require.Empty(t, chunk.GetRawObjects())
	require.Empty(t, chunk.GetRawInventoriesByOwner())
	require.Empty(t, chunk.GetDeletedObjectIDs())
}

func TestLoadedObjectCachePreservesNewerCommittedReplacement(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	revision := chunk.beginCacheLoad()
	chunk.RemoveCommittedObject(5)
	chunk.InsertCommittedObject(&repository.Object{ID: 6, X: 20}, []repository.Inventory{{OwnerID: 6, Version: 2}})
	chunk.installLoadedObjects(revision, []*repository.Object{{ID: 5}, {ID: 6, X: 10}, {ID: 7}}, map[types.EntityID][]repository.Inventory{
		5: {{OwnerID: 5}}, 6: {{OwnerID: 6, Version: 1}}, 7: {{OwnerID: 7}},
	})
	objects := chunk.GetRawObjects()
	require.Len(t, objects, 2)
	require.Equal(t, int64(6), objects[0].ID)
	require.Equal(t, 20, objects[0].X)
	require.Equal(t, int64(7), objects[1].ID)
	rows := chunk.GetRawInventoriesByOwner()
	require.NotContains(t, rows, types.EntityID(5))
	require.Equal(t, 2, rows[6][0].Version)
}

func TestLoadedObjectCachePreservesInPlaceTransformAndClearsInventories(t *testing.T) {
	chunk := NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetRawObjects([]*repository.Object{{ID: 5, TypeID: 15}})
	chunk.SetRawInventoriesForOwner(5, []repository.Inventory{{OwnerID: 5, Version: 1}})
	revision := chunk.beginCacheLoad()
	chunk.InsertCommittedObject(&repository.Object{ID: 5, TypeID: 17}, nil)
	chunk.installLoadedObjects(revision, []*repository.Object{{ID: 5, TypeID: 15}}, map[types.EntityID][]repository.Inventory{
		5: {{OwnerID: 5, Version: 2}},
	})
	objects := chunk.GetRawObjects()
	require.Len(t, objects, 1)
	require.Equal(t, int64(5), objects[0].ID)
	require.Equal(t, 17, objects[0].TypeID)
	require.Empty(t, chunk.GetRawInventoriesByOwner()[5])
	require.Empty(t, chunk.GetRawDirtyObjectIDs())
}
