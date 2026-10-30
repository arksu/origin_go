package world

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestEmbeddedObjectQualityFullUint32RoundTrip(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	factory := cm.ObjectFactory()
	for index, quality := range []uint32{0, 32767, 32768, math.MaxInt32 + 1, math.MaxUint32} {
		t.Run(strconv.FormatUint(uint64(quality), 10), func(t *testing.T) {
			h := spawnObjectHealthLifecycleFixture(t, cm.world, chunk, types.EntityID(2020+index), objectHealthLifecycleTypeID, 100)
			ecs.WithComponent(cm.world, h, func(info *components.EntityInfo) { info.Quality = quality })
			snapshot, err := factory.CaptureWorldObjectSnapshot(cm.world, h)
			require.NoError(t, err)
			require.Equal(t, int64(quality), snapshot.Quality)
			data, err := SerializeSnapshotToJSON(snapshot)
			require.NoError(t, err)
			decoded, err := DeserializeSnapshotFromJSON(data)
			require.NoError(t, err)
			require.Equal(t, int64(quality), decoded.Quality)
			chunk.Spatial().RemoveStatic(h, 10, 10)
			require.True(t, cm.world.Despawn(h))
			restored, err := factory.SpawnWorldObjectFromSnapshot(cm.world, decoded, SnapshotSpawnOptions{
				X: 10, Y: 10, ChunkManager: cm,
			})
			require.NoError(t, err)
			info, ok := ecs.GetComponent[components.EntityInfo](cm.world, restored)
			require.True(t, ok)
			require.Equal(t, quality, info.Quality)
		})
	}
}

func TestEmbeddedObjectQualityRejectsInvalidBeforeSpawn(t *testing.T) {
	factory := NewObjectFactory(nil)
	w := ecs.NewWorldForTesting()
	hp := 100.0
	for _, quality := range []int64{-1, math.MaxUint32 + 1, math.MaxInt64} {
		snapshot := EmbeddedObjectSnapshotV1{
			Version: 1, EntityID: 2030, TypeID: objectHealthLifecycleTypeID,
			Quality: quality, HP: &hp,
		}
		_, err := SerializeSnapshotToJSON(snapshot)
		require.ErrorContains(t, err, "invalid object quality")
		_, err = DeserializeSnapshotFromJSON([]byte(fmt.Sprintf(
			`{"version":1,"entity_id":2030,"type_id":9811,"quality":%d,"hp":100}`, quality)))
		require.ErrorContains(t, err, "invalid object quality")
		h, err := factory.SpawnWorldObjectFromSnapshot(w, snapshot, SnapshotSpawnOptions{})
		require.ErrorContains(t, err, "invalid object quality")
		require.Equal(t, types.InvalidHandle, h)
		require.Zero(t, w.EntityCount())
	}
	for _, malformed := range []string{"1.5", "9223372036854775808", `"10"`} {
		_, err := DeserializeSnapshotFromJSON([]byte(fmt.Sprintf(
			`{"version":1,"entity_id":2030,"type_id":9811,"quality":%s,"hp":100}`, malformed)))
		require.Error(t, err)
	}
}

func TestEmbeddedObjectQualityReadsLegacySnapshots(t *testing.T) {
	for _, quality := range []int64{0, 10, 32767} {
		snapshot, err := DeserializeSnapshotFromJSON([]byte(fmt.Sprintf(
			`{"version":1,"entity_id":2030,"type_id":9811,"quality":%d,"hp":100}`, quality)))
		require.NoError(t, err)
		require.Equal(t, quality, snapshot.Quality)
		require.Equal(t, embeddedObjectSnapshotVersion, snapshot.Version)
	}
}
