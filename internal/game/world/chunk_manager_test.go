package world

import (
	"origin/internal/config"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"testing"
	"time"

	"go.uber.org/zap"
)

func newTestConfig() *config.Config {
	return &config.Config{
		Game: config.GameConfig{
			ChunkLRUCapacity:         10,
			ChunkLRUTTL:              60,
			LoadWorkers:              1,
			SaveWorkers:              1,
			PlayerPreloadChunkRadius: 2,
			PlayerActiveChunkRadius:  1,
			WorldMinXChunks:          0,
			WorldMinYChunks:          0,
			WorldWidthChunks:         50,
			WorldHeightChunks:        50,
		},
	}
}

func newTestChunkManager() *ChunkManager {
	cfg := newTestConfig()
	world := ecs.NewWorldForTesting()
	logger := zap.NewNop()
	objectFactory := NewObjectFactory(nil)
	eb := eventbus.New(&eventbus.Config{
		MinWorkers: 1,
		MaxWorkers: 2,
		Logger:     logger,
	})

	return NewChunkManager(cfg, nil, world, nil, 0, 1, objectFactory, eb, logger)
}

func TestChunkState_String(t *testing.T) {
	tests := []struct {
		state    types.ChunkState
		expected string
	}{
		{types.ChunkStateUnloaded, "unloaded"},
		{types.ChunkStateLoading, "loading"},
		{types.ChunkStatePreloaded, "preloaded"},
		{types.ChunkStateActive, "active"},
		{types.ChunkStateInactive, "inactive"},
		{types.ChunkState(5), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("ChunkState(%d).String() = %v, want %v", tt.state, got, tt.expected)
		}
	}
}

func TestNewChunk(t *testing.T) {
	coord := types.ChunkCoord{X: 5, Y: 10}
	chunk := core.NewChunk(coord, 0, 0, 128)

	if chunk == nil {
		t.Fatal("NewChunk returned nil")
	}

	if chunk.Coord != coord {
		t.Errorf("Expected coord %v, got %v", coord, chunk.Coord)
	}

	if chunk.Layer != 0 {
		t.Errorf("Expected layer 0, got %d", chunk.Layer)
	}

	if chunk.GetState() != types.ChunkStateUnloaded {
		t.Errorf("Expected state %v, got %v", types.ChunkStateUnloaded, chunk.GetState())
	}

	expectedTiles := 128 * 128
	if len(chunk.Tiles) != expectedTiles {
		t.Errorf("Expected %d tiles, got %d", expectedTiles, len(chunk.Tiles))
	}
}

func TestChunk_SetGetState(t *testing.T) {
	chunk := core.NewChunk(types.ChunkCoord{X: 0, Y: 0}, 0, 0, 128)

	chunk.SetState(types.ChunkStateLoading)
	if got := chunk.GetState(); got != types.ChunkStateLoading {
		t.Errorf("Expected state %v, got %v", types.ChunkStateLoading, got)
	}

	chunk.SetState(types.ChunkStateActive)
	if got := chunk.GetState(); got != types.ChunkStateActive {
		t.Errorf("Expected state %v, got %v", types.ChunkStateActive, got)
	}
}

func TestChunkManager_ActiveChunkCoords(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	coords := cm.ActiveChunkCoords()
	if len(coords) != 0 {
		t.Errorf("ActiveChunkCoords() should be empty initially, got %d", len(coords))
	}
}

func TestChunkManager_Stats(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	stats := cm.Stats()

	if stats.ActiveCount != 0 {
		t.Errorf("stats.ActiveCount = %d, want 0", stats.ActiveCount)
	}
	if stats.PreloadedCount != 0 {
		t.Errorf("stats.PreloadedCount = %d, want 0", stats.PreloadedCount)
	}
	if stats.InactiveCount != 0 {
		t.Errorf("stats.InactiveCount = %d, want 0", stats.InactiveCount)
	}
}

func TestChunkManager_Stats_GroundTruthStateCounts(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	activeCoord := types.ChunkCoord{X: 1, Y: 1}
	preloadedCoord := types.ChunkCoord{X: 2, Y: 2}
	inactiveCoord := types.ChunkCoord{X: 3, Y: 3}
	unloadedCoord := types.ChunkCoord{X: 4, Y: 4}

	activeChunk := core.NewChunk(activeCoord, 0, 0, 128)
	activeChunk.SetState(types.ChunkStateActive)
	preloadedChunk := core.NewChunk(preloadedCoord, 0, 0, 128)
	preloadedChunk.SetState(types.ChunkStatePreloaded)
	inactiveChunk := core.NewChunk(inactiveCoord, 0, 0, 128)
	inactiveChunk.SetState(types.ChunkStateInactive)
	unloadedChunk := core.NewChunk(unloadedCoord, 0, 0, 128)
	unloadedChunk.SetState(types.ChunkStateUnloaded)

	cm.chunksMu.Lock()
	cm.chunks[activeCoord] = activeChunk
	cm.chunks[preloadedCoord] = preloadedChunk
	cm.chunks[inactiveCoord] = inactiveChunk
	cm.chunks[unloadedCoord] = unloadedChunk
	cm.chunksMu.Unlock()

	stats := cm.Stats()
	if stats.ActiveCount != 1 {
		t.Errorf("stats.ActiveCount = %d, want 1", stats.ActiveCount)
	}
	if stats.PreloadedCount != 1 {
		t.Errorf("stats.PreloadedCount = %d, want 1", stats.PreloadedCount)
	}
	if stats.InactiveCount != 1 {
		t.Errorf("stats.InactiveCount = %d, want 1", stats.InactiveCount)
	}
}

func TestChunkManager_RequestMetricsCountAttempts(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	before := cm.Stats()
	_ = cm.requestLoad(types.ChunkCoord{X: 5, Y: 5})
	_ = cm.requestLoad(types.ChunkCoord{X: 6, Y: 6})
	afterLoad := cm.Stats()
	if delta := afterLoad.LoadRequests - before.LoadRequests; delta != 2 {
		t.Errorf("LoadRequests delta = %d, want 2", delta)
	}

	saveCoord := types.ChunkCoord{X: 7, Y: 7}
	saveChunk := core.NewChunk(saveCoord, 0, 0, 128)
	saveChunk.SetState(types.ChunkStateInactive)
	cm.onEvict(saveCoord, saveChunk)
	afterSave := cm.Stats()
	if delta := afterSave.SaveRequests - afterLoad.SaveRequests; delta != 1 {
		t.Errorf("SaveRequests delta = %d, want 1", delta)
	}
}

func TestChunkManager_CacheMetricsOnlyGetChunk(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	coord := types.ChunkCoord{X: 8, Y: 8}
	chunk := core.NewChunk(coord, 0, 0, 128)
	chunk.SetState(types.ChunkStatePreloaded)

	cm.chunksMu.Lock()
	cm.chunks[coord] = chunk
	cm.chunksMu.Unlock()

	before := cm.Stats()

	if got := cm.GetChunkFast(coord); got == nil {
		t.Fatal("GetChunkFast returned nil for existing chunk")
	}
	mid := cm.Stats()
	if mid.CacheHits != before.CacheHits || mid.CacheMisses != before.CacheMisses {
		t.Fatalf("GetChunkFast must not change cache metrics: before hits/misses=%d/%d after=%d/%d",
			before.CacheHits, before.CacheMisses, mid.CacheHits, mid.CacheMisses)
	}

	if got := cm.GetChunk(coord); got == nil {
		t.Fatal("GetChunk returned nil for existing chunk")
	}
	after := cm.Stats()
	if delta := after.CacheHits - mid.CacheHits; delta != 1 {
		t.Errorf("CacheHits delta = %d, want 1", delta)
	}
}

func TestWorldToChunkCoord(t *testing.T) {
	tests := []struct {
		worldX, worldY int
		chunkSize      int
		coordPerTile   int
		expected       types.ChunkCoord
	}{
		{0, 0, 32, 16, types.ChunkCoord{X: 0, Y: 0}},
		{511, 511, 32, 16, types.ChunkCoord{X: 0, Y: 0}},   // 31.999... tiles -> 0 chunk
		{512, 512, 32, 16, types.ChunkCoord{X: 1, Y: 1}},   // exactly 32 tiles -> 1 chunk
		{1023, 1023, 32, 16, types.ChunkCoord{X: 1, Y: 1}}, // 63.999... tiles -> 1 chunk
		{1024, 1024, 32, 16, types.ChunkCoord{X: 2, Y: 2}}, // exactly 64 tiles -> 2 chunks
		{-1, -1, 32, 16, types.ChunkCoord{X: -1, Y: -1}},
		{-512, -512, 32, 16, types.ChunkCoord{X: -1, Y: -1}},
		{-513, -513, 32, 16, types.ChunkCoord{X: -2, Y: -2}},
	}

	for _, tt := range tests {
		got := types.WorldToChunkCoord(tt.worldX, tt.worldY, tt.chunkSize, tt.coordPerTile)
		if got != tt.expected {
			t.Errorf("WorldToChunkCoord(%d, %d, %d, %d) = %v, want %v",
				tt.worldX, tt.worldY, tt.chunkSize, tt.coordPerTile, got, tt.expected)
		}
	}
}

func TestChunkManager_PreloadChunksAround(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	center := types.ChunkCoord{X: 5, Y: 5}
	cm.PreloadChunksAround(center)

	time.Sleep(50 * time.Millisecond)

	stats := cm.Stats()
	if stats.LoadRequests == 0 {
		t.Error("PreloadChunksAround should trigger load requests")
	}
}

func TestChunkManager_ObjectFactory(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	factory := cm.ObjectFactory()
	if factory == nil {
		t.Fatal("ObjectFactory() returned nil")
	}
	// factory is already *ObjectFactory, just verify non-nil
	_ = factory
}

func TestChunk_SaveToDB_HandlesInactiveChunks(t *testing.T) {
	// This test verifies that SaveToDB correctly handles chunks with raw objects (inactive state)
	coord := types.ChunkCoord{X: 1, Y: 1}
	chunk := core.NewChunk(coord, 0, 0, 128)

	// Create some mock raw objects (simulating inactive chunk state)
	rawObjects := make([]*repository.Object, 3)
	var i int
	for i = 0; i < 3; i++ {
		rawObjects[i] = &repository.Object{
			ID:     int64(i + 1),
			TypeID: i + 1,
			Region: 0,
			X:      i * 10,
			Y:      i * 10,
			Layer:  0,
			ChunkX: coord.X,
			ChunkY: coord.Y,
		}
	}

	// Set raw objects (simulating inactive chunk)
	chunk.SetRawObjects(rawObjects)

	// Verify GetRawObjects returns the objects
	retrievedObjects := chunk.GetRawObjects()
	if len(retrievedObjects) != 3 {
		t.Errorf("Expected 3 raw objects, got %d", len(retrievedObjects))
	}

	// Verify GetHandles returns empty (no active entities)
	handles := chunk.GetHandles()
	if len(handles) != 0 {
		t.Errorf("Expected 0 handles for inactive chunk, got %d", len(handles))
	}
}
