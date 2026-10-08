package world

import (
	"testing"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPinnedInactiveEvictionIsReconciledAfterFinalUnpin(t *testing.T) {
	cfg := newTestConfig()
	cfg.Game.LoadWorkers, cfg.Game.SaveWorkers = 0, 0
	cfg.Game.ChunkLRUCapacity = 1
	cm := NewChunkManager(cfg, nil, ecs.NewWorldForTesting(), nil, 0, 1, NewObjectFactory(nil), nil, nil, zap.NewNop())
	t.Cleanup(cm.Stop)
	addInactive := func(x int) *core.Chunk {
		chunk := core.NewChunk(types.ChunkCoord{X: x}, 1, 0, 128)
		chunk.SetState(types.ChunkStateInactive)
		cm.chunks[chunk.Coord] = chunk
		cm.lruCache.Add(chunk.Coord, chunk)
		return chunk
	}
	pinned := addInactive(0)
	require.NoError(t, cm.PinPersistence(pinned.Coord))
	addInactive(1) // Forced capacity eviction removes the pinned LRU entry.
	require.False(t, cm.lruCache.Contains(pinned.Coord))
	require.Empty(t, cm.saveQueue, "pinned eviction must not write or remove its chunk")
	cm.UnpinPersistence(pinned.Coord)
	cm.recalculateChunkStates()
	require.True(t, cm.lruCache.Contains(pinned.Coord), "release must restore normal eviction eligibility")
	<-cm.saveQueue // The other inactive chunk was evicted by reinsertion.
	addInactive(2)
	req := <-cm.saveQueue
	require.Equal(t, pinned.Coord, req.coord)
	require.Same(t, pinned, req.chunk)
	cm.safeSaveAndRemove(req.coord, req.chunk)
	require.Nil(t, cm.GetChunkFast(pinned.Coord), "normal clean persistence eviction removes the released chunk")
}
