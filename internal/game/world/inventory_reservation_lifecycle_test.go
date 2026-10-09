package world

import (
	"testing"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestInventoryReservationRetainsCorpseOnChunkDeactivation(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	cm := newTestChunkManagerWithLoadWorkers(0)
	defer cm.Stop()
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, 128)
	chunk.SetState(types.ChunkStateActive)
	owner := spawnObjectHealthLifecycleFixture(t, cm.world, chunk, 41, objectHealthLifecycleTypeID, 100)
	require.True(t, ecs.ReserveInventoryOwner(cm.world, 41, owner))
	require.ErrorIs(t, cm.deactivateChunkInternal(chunk), ErrChunkPersistenceBusy)
	require.True(t, cm.world.Alive(owner))
	require.Equal(t, types.ChunkStateActive, chunk.GetState())
	require.True(t, ecs.ReleaseInventoryOwner(cm.world, 41, owner))
	require.NoError(t, cm.deactivateChunkInternal(chunk))
	require.False(t, cm.world.Alive(owner))
}

func TestInventoryReservationGuardsGenericObjectTransform(t *testing.T) {
	w := ecs.NewWorldForTesting()
	owner := w.Spawn(41, nil)
	ecs.AddComponent(w, owner, components.EntityInfo{TypeID: 17})
	ecs.AddComponent(w, owner, components.ObjectInternalState{HP: 100, HasHP: true})
	require.True(t, ecs.ReserveInventoryOwner(w, 41, owner))
	require.False(t, TransformObjectToDefInPlace(w, 41, owner, &objectdefs.ObjectDef{DefID: 18, Key: "new", HP: 100}, TransformObjectInPlaceOptions{}))
	info, _ := ecs.GetComponent[components.EntityInfo](w, owner)
	require.Equal(t, uint32(17), info.TypeID)
}
