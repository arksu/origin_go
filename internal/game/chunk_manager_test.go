package game

import (
	"origin/internal/config"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/types"
	"testing"
	"time"

	"go.uber.org/zap"
)

func newTestConfig() *config.Config {
	return &config.Config{
		Game: config.GameConfig{
			ChunkSize:                128,
			CoordPerTile:             12,
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
	world := ecs.NewWorld()
	logger := zap.NewNop()
	objectFactory := NewObjectFactory()
	eb := eventbus.New(&eventbus.Config{
		MinWorkers: 1,
		MaxWorkers: 2,
		Logger:     logger,
	})

	return NewChunkManager(cfg, nil, world, 0, 1, objectFactory, eb, logger)
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

func TestChunkManager_GetOrCreateChunk(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	coord := types.ChunkCoord{X: 1, Y: 2}

	chunk1 := cm.GetOrCreateChunk(coord)
	if chunk1 == nil {
		t.Fatal("GetOrCreateChunk returned nil")
	}

	chunk2 := cm.GetOrCreateChunk(coord)
	if chunk1 != chunk2 {
		t.Error("GetOrCreateChunk should return same chunk for same coord")
	}
}

func TestChunkManager_GetChunk(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	coord := types.ChunkCoord{X: 3, Y: 4}

	if chunk := cm.GetChunk(coord); chunk != nil {
		t.Error("GetChunk should return nil for non-existent chunk")
	}

	cm.GetOrCreateChunk(coord)

	if chunk := cm.GetChunk(coord); chunk == nil {
		t.Error("GetChunk should return chunk after creation")
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
}
