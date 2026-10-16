package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLiftCarryTransferCapturePreservesRawCacheOnSnapshotFailure(t *testing.T) {
	g, source, _, player, _, object := newLiftHealthTransferFixture(t, nil, .6)
	participant := NewLiftCarryTransferParticipant(zap.NewNop())
	participant.patchChunkCachesAfterTransfer(g, source, &liftCarryTransferState{ObjectEntityID: liftHealthObjectID}, object)
	chunk := source.chunkManager.GetChunkFast(types.ChunkCoord{})
	rawObject := chunk.GetRawObjectByID(liftHealthObjectID)
	rawInventories := chunk.GetRawInventoriesByOwner()[liftHealthObjectID]
	require.NotNil(t, rawObject)
	require.NotEmpty(t, rawInventories)
	refs := ecs.GetResource[ecs.InventoryRefIndex](source.world)
	root, found := refs.Lookup(constt.InventoryGrid, liftHealthObjectID, 0)
	require.True(t, found)
	meta, _ := ecs.GetComponent[components.LiftedObjectState](source.world, object)

	ecs.WithComponent(source.world, object, func(state *components.ObjectInternalState) { state.HasHP = false })
	req := PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}
	state, err := participant.CaptureSource(g, source, req, player)
	require.ErrorIs(t, err, gameworld.ErrObjectHealthMissing)
	require.Nil(t, state)
	require.True(t, source.world.Alive(object))
	require.True(t, source.world.Alive(root))
	require.Equal(t, rawObject, chunk.GetRawObjectByID(liftHealthObjectID))
	require.Equal(t, rawInventories, chunk.GetRawInventoriesByOwner()[liftHealthObjectID])

	ecs.WithComponent(source.world, object, func(state *components.ObjectInternalState) { state.HasHP = true })
	state, err = participant.CaptureSource(g, source, req, player)
	require.NoError(t, err)
	t.Cleanup(state.(*liftCarryTransferState).releaseTransferResources)
	require.False(t, source.world.Alive(root))
	_, found = refs.Lookup(constt.InventoryGrid, liftHealthObjectID, 0)
	require.False(t, found)
	require.Nil(t, chunk.GetRawObjectByID(liftHealthObjectID))
	require.Empty(t, chunk.GetRawInventoriesByOwner()[liftHealthObjectID])
	require.NoError(t, participant.RestoreSourceRollback(g, source, req, player, state))
	requireLiftHealthTransferState(t, source, player, .6, meta)
}
