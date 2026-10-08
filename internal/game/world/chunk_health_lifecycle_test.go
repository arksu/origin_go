package world

import (
	"math"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestObjectHealthChunkDeactivationFailureAndRetryPreserveInventories(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	containerObject := spawnObjectHealthLifecycleFixture(t, cm.world, chunk, 9891, objectHealthContainerTypeID, .6)
	invalidObject := spawnObjectHealthLifecycleFixture(t, cm.world, chunk, 9892, objectHealthLifecycleTypeID, .49)
	ecs.WithComponent(cm.world, invalidObject, func(health *components.ObjectInternalState) { health.HP = math.NaN() })
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](cm.world)
	root := cm.world.SpawnWithoutExternalID()
	ecs.AddComponent(cm.world, root, components.InventoryContainer{
		OwnerID: 9891, Kind: constt.InventoryGrid, Width: 2, Height: 2, Version: 1,
	})
	refIndex.Add(constt.InventoryGrid, 9891, 0, root)
	beforeHandles := chunk.GetHandles()
	now := time.Unix(1000, 0)
	require.False(t, cm.deactivateChunkWhenDue(chunk, now))
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
	require.Equal(t, beforeHandles, chunk.GetHandles())
	require.True(t, cm.world.Alive(containerObject))
	require.True(t, cm.world.Alive(invalidObject))
	require.True(t, cm.world.Alive(root))
	indexed, ok := refIndex.Lookup(constt.InventoryGrid, 9891, 0)
	require.True(t, ok)
	require.Equal(t, root, indexed)
	require.Empty(t, chunk.GetRawObjects())
	require.Equal(t, time.Second, cm.deactivationRetries[chunk.Coord].delay)
	require.False(t, cm.deactivateChunkWhenDue(chunk, now.Add(time.Second)))
	require.Equal(t, 2*time.Second, cm.deactivationRetries[chunk.Coord].delay)
	require.NoError(t, SetObjectHP(cm.world, invalidObject, .49))
	require.False(t, cm.deactivateChunkWhenDue(chunk, now.Add(2*time.Second)))
	require.True(t, cm.world.Alive(root), "repair must not bypass existing retry delay")
	require.True(t, cm.deactivateChunkWhenDue(chunk, now.Add(3*time.Second)))
	require.Equal(t, types.ChunkStatePreloaded, chunk.GetState())
	require.False(t, cm.world.Alive(root))
	require.False(t, cm.world.Alive(containerObject))
	require.False(t, cm.world.Alive(invalidObject))
	_, ok = refIndex.Lookup(constt.InventoryGrid, 9891, 0)
	require.False(t, ok)
	require.Len(t, chunk.GetRawObjects(), 2)
	require.Len(t, chunk.GetRawInventoriesByOwner()[9891], 1)
	require.NotContains(t, cm.deactivationRetries, chunk.Coord)
	for _, raw := range chunk.GetRawObjects() {
		require.True(t, raw.Hp.Valid)
		if raw.ID == 9891 {
			require.Equal(t, .6, raw.Hp.Float64)
		} else {
			require.Equal(t, .49, raw.Hp.Float64)
		}
	}
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	for id, expected := range map[types.EntityID]float64{9891: .6, 9892: .49} {
		h := cm.world.GetHandleByEntityID(id)
		require.True(t, cm.world.Alive(h))
		health, found := ecs.GetComponent[components.ObjectInternalState](cm.world, h)
		require.True(t, found)
		require.True(t, health.HasHP)
		require.Equal(t, expected, health.HP)
	}
	_, ok = refIndex.Lookup(constt.InventoryGrid, 9891, 0)
	require.True(t, ok)
}

func TestObjectHealthChunkZeroSurvivesDeactivationAndReactivation(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	h := spawnObjectHealthLifecycleFixture(t, cm.world, chunk, 9893, objectHealthLifecycleTypeID, 0)
	require.NoError(t, cm.deactivateChunkInternal(chunk))
	require.False(t, cm.world.Alive(h))
	require.Len(t, chunk.GetRawObjects(), 1)
	require.True(t, chunk.GetRawObjects()[0].Hp.Valid)
	require.Zero(t, chunk.GetRawObjects()[0].Hp.Float64)
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	restored := cm.world.GetHandleByEntityID(9893)
	require.True(t, cm.world.Alive(restored))
	health, ok := ecs.GetComponent[components.ObjectInternalState](cm.world, restored)
	require.True(t, ok)
	require.True(t, health.HasHP)
	require.Zero(t, health.HP, "zero HP must never reset to definition health")
}
