package game

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/eventbus"
)

func newTestConfig() *config.Config {
	return &config.Config{
		Game: config.GameConfig{
			ChunkSize:        128,
			CoordPerTile:     12,
			ChunkLRUCapacity: 10,
			ChunkLRUTTL:      60,
			LoadWorkers:      1,
			SaveWorkers:      1,
			PreloadRadius:    2,
			AOIRadius:        1,
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
		state    ChunkState
		expected string
	}{
		{ChunkStateUnloaded, "unloaded"},
		{ChunkStateLoading, "loading"},
		{ChunkStatePreloaded, "preloaded"},
		{ChunkStateActive, "active"},
		{ChunkState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("ChunkState(%d).String() = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestNewChunk(t *testing.T) {
	coord := ChunkCoord{X: 5, Y: 10}
	chunk := NewChunk(coord, 0, 128)

	if chunk.Coord != coord {
		t.Errorf("chunk.Coord = %v, want %v", chunk.Coord, coord)
	}
	if chunk.Layer != 0 {
		t.Errorf("chunk.Layer = %d, want 0", chunk.Layer)
	}
	if chunk.State != ChunkStateUnloaded {
		t.Errorf("chunk.State = %v, want ChunkStateUnloaded", chunk.State)
	}
	if len(chunk.Tiles) != 128*128 {
		t.Errorf("len(chunk.Tiles) = %d, want %d", len(chunk.Tiles), 128*128)
	}
}

func TestChunk_SetGetState(t *testing.T) {
	chunk := NewChunk(ChunkCoord{X: 0, Y: 0}, 0, 128)

	chunk.SetState(ChunkStateLoading)
	if got := chunk.GetState(); got != ChunkStateLoading {
		t.Errorf("GetState() = %v, want ChunkStateLoading", got)
	}

	chunk.SetState(ChunkStateActive)
	if got := chunk.GetState(); got != ChunkStateActive {
		t.Errorf("GetState() = %v, want ChunkStateActive", got)
	}
}

func TestChunkManager_GetOrCreateChunk(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	coord := ChunkCoord{X: 1, Y: 2}

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

	coord := ChunkCoord{X: 3, Y: 4}

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
		expected       ChunkCoord
	}{
		{0, 0, 128, 12, ChunkCoord{0, 0}},
		{1536, 1536, 128, 12, ChunkCoord{1, 1}},
		{3072, 0, 128, 12, ChunkCoord{2, 0}},
		{100, 200, 128, 12, ChunkCoord{0, 0}},
		{2000, 3000, 128, 12, ChunkCoord{1, 1}},
	}

	for _, tt := range tests {
		got := WorldToChunkCoord(tt.worldX, tt.worldY, tt.chunkSize, tt.coordPerTile)
		if got != tt.expected {
			t.Errorf("WorldToChunkCoord(%d, %d, %d, %d) = %v, want %v",
				tt.worldX, tt.worldY, tt.chunkSize, tt.coordPerTile, got, tt.expected)
		}
	}
}

func TestChunkManager_PreloadChunksAround(t *testing.T) {
	cm := newTestChunkManager()
	defer cm.Stop()

	center := ChunkCoord{X: 5, Y: 5}
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
