package game

import (
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLiftCarryTransferBusyPinPreservesSource(t *testing.T) {
	g, source, _, player, _, object := newLiftHealthTransferFixture(t, nil, .6)
	participant := NewLiftCarryTransferParticipant(zap.NewNop())
	participant.patchChunkCachesAfterTransfer(g, source, &liftCarryTransferState{ObjectEntityID: liftHealthObjectID}, object)
	chunk := source.chunkManager.GetChunkFast(types.ChunkCoord{})
	rawObject := chunk.GetRawObjectByID(liftHealthObjectID)
	rawInventories := chunk.GetRawInventoriesByOwner()[liftHealthObjectID]
	beforeHandles := chunk.GetHandles()
	beforeCarry, _ := ecs.GetComponent[components.LiftCarryState](source.world, player)
	root, found := ecs.GetResource[ecs.InventoryRefIndex](source.world).Lookup(constt.InventoryGrid, liftHealthObjectID, 0)
	require.True(t, found)
	require.NotNil(t, rawObject)
	require.NotEmpty(t, rawInventories)

	type captureResult struct {
		state any
		err   error
	}
	done := make(chan captureResult, 1)
	var result captureResult
	blocked := false
	// Hold the actual worker I/O gate outside the owning shard lock. Capture
	// must reject immediately instead of waiting for this operation to finish.
	require.NoError(t, source.chunkManager.WithPersistence([]types.ChunkCoord{chunk.Coord}, func() error {
		go func() {
			source.mu.Lock()
			state, err := participant.CaptureSource(g, source, PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}, player)
			source.mu.Unlock()
			done <- captureResult{state: state, err: err}
		}()
		select {
		case result = <-done:
		case <-time.After(time.Second):
			blocked = true
		}
		return nil
	}))
	if blocked {
		result = <-done // The gate is released; join capture before fixture cleanup.
	}
	if state, ok := result.state.(*liftCarryTransferState); ok {
		t.Cleanup(state.releaseTransferResources)
	}
	require.False(t, blocked, "carry capture must not wait for chunk I/O")
	require.ErrorIs(t, result.err, gameworld.ErrChunkPersistenceBusy)
	require.Nil(t, result.state)
	require.True(t, source.world.Alive(object))
	require.True(t, source.world.Alive(root))
	afterCarry, _ := ecs.GetComponent[components.LiftCarryState](source.world, player)
	require.Equal(t, beforeCarry, afterCarry)
	require.Equal(t, beforeHandles, chunk.GetHandles())
	require.Equal(t, rawObject, chunk.GetRawObjectByID(liftHealthObjectID))
	require.Equal(t, rawInventories, chunk.GetRawInventoriesByOwner()[liftHealthObjectID])

	source.chunkManager.UnregisterEntity(liftHealthPlayerID)
	require.Equal(t, types.ChunkStateInactive, chunk.GetState(), "rejected capture must not retain a source pin")
}

func TestLiftCarryTransferRejectsNonActiveSourceChunk(t *testing.T) {
	for _, chunkState := range []types.ChunkState{types.ChunkStateUnloaded, types.ChunkStatePreloaded, types.ChunkStateInactive} {
		t.Run(chunkState.String(), func(t *testing.T) {
			g, source, _, player, _, object := newLiftHealthTransferFixture(t, nil, .6)
			participant := NewLiftCarryTransferParticipant(zap.NewNop())
			participant.patchChunkCachesAfterTransfer(g, source, &liftCarryTransferState{ObjectEntityID: liftHealthObjectID}, object)
			chunk := source.chunkManager.GetChunkFast(types.ChunkCoord{})
			rawObject := chunk.GetRawObjectByID(liftHealthObjectID)
			rawInventories := chunk.GetRawInventoriesByOwner()[liftHealthObjectID]
			beforeCarry, _ := ecs.GetComponent[components.LiftCarryState](source.world, player)
			root, found := ecs.GetResource[ecs.InventoryRefIndex](source.world).Lookup(constt.InventoryGrid, liftHealthObjectID, 0)
			require.True(t, found)
			chunk.SetState(chunkState)

			state, err := participant.CaptureSource(g, source, PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}, player)
			require.ErrorContains(t, err, "source chunk is not active")
			require.Nil(t, state)
			require.True(t, source.world.Alive(object))
			require.True(t, source.world.Alive(root))
			afterCarry, _ := ecs.GetComponent[components.LiftCarryState](source.world, player)
			require.Equal(t, beforeCarry, afterCarry)
			require.Equal(t, rawObject, chunk.GetRawObjectByID(liftHealthObjectID))
			require.Equal(t, rawInventories, chunk.GetRawInventoriesByOwner()[liftHealthObjectID])

			chunk.SetState(types.ChunkStateActive)
			source.chunkManager.UnregisterEntity(liftHealthPlayerID)
			require.Equal(t, types.ChunkStateInactive, chunk.GetState(), "rejected source state must not leak a pin")
		})
	}
}

func TestLiftCarryTransferSnapshotFailureDoesNotRetainSourcePin(t *testing.T) {
	g, source, _, player, _, object := newLiftHealthTransferFixture(t, nil, .6)
	participant := NewLiftCarryTransferParticipant(zap.NewNop())
	ecs.WithComponent(source.world, object, func(state *components.ObjectInternalState) { state.HasHP = false })
	state, err := participant.CaptureSource(g, source, PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}, player)
	require.ErrorIs(t, err, gameworld.ErrObjectHealthMissing)
	require.Nil(t, state)
	require.True(t, source.world.Alive(object))

	// Repair health so ordinary serialization can deactivate the chunk. A pin
	// leaked by failed capture would keep it active even after all AOI leaves.
	ecs.WithComponent(source.world, object, func(state *components.ObjectInternalState) { state.HasHP = true })
	source.chunkManager.UnregisterEntity(liftHealthPlayerID)
	require.Equal(t, types.ChunkStateInactive, source.chunkManager.GetChunkFast(types.ChunkCoord{}).GetState())
}

func TestLiftCarryTransferResourceReleaseResumesDeactivation(t *testing.T) {
	for _, cleanup := range []string{"state", "participant_states"} {
		t.Run(cleanup, func(t *testing.T) {
			g, source, _, player, _, _ := newLiftHealthTransferFixture(t, nil, .6)
			participant := NewLiftCarryTransferParticipant(zap.NewNop())
			stateAny, err := participant.CaptureSource(g, source, PlayerTransferRequest{PlayerID: liftHealthPlayerID, SourceLayer: 0, TargetLayer: 1}, player)
			require.NoError(t, err)
			state := stateAny.(*liftCarryTransferState)
			t.Cleanup(state.releaseTransferResources)
			chunk := source.chunkManager.GetChunkFast(state.SourceChunk)
			source.chunkManager.UnregisterEntity(liftHealthPlayerID)
			source.chunkManager.Update(0)
			require.Same(t, chunk, source.chunkManager.GetChunkFast(state.SourceChunk))
			require.Equal(t, types.ChunkStateActive, chunk.GetState(), "captured carry must retain its source without AOI")

			if cleanup == "state" {
				state.releaseTransferResources()
			} else {
				releaseTransferResources(map[string]any{
					liftCarryTransferParticipantKey: state,
					"plain":                         struct{ Value int }{Value: 1},
					"nil":                           nil,
					"empty_carry":                   &liftCarryTransferState{},
					"nil_carry":                     (*liftCarryTransferState)(nil),
				})
				releaseTransferResources(nil)
			}
			state.releaseTransferResources() // Cleanup may run after an earlier release.
			source.chunkManager.Update(0)
			require.Equal(t, types.ChunkStateInactive, chunk.GetState(), "release must schedule ordinary deactivation without another AOI event")
		})
	}
}
