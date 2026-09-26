package world

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/types"
)

func newStreamingTestManager(t *testing.T) *ChunkManager {
	t.Helper()
	cfg := newTestConfig()
	cfg.Game.LoadWorkers, cfg.Game.SaveWorkers = 0, 0
	cfg.Game.PlayerActiveChunkRadius, cfg.Game.PlayerPreloadChunkRadius = 0, 0
	bus := eventbus.New(nil)
	cm := NewChunkManager(cfg, nil, ecs.NewWorldForTesting(), nil, 0, 1, NewObjectFactory(nil), nil, bus, zap.NewNop())
	t.Cleanup(func() { cm.Stop(); bus.Shutdown(context.Background()) })
	return cm
}

func addStreamingTestChunk(t *testing.T, cm *ChunkManager, coord types.ChunkCoord) *core.Chunk {
	t.Helper()
	chunk := core.NewChunk(coord, 1, 0, constt.ChunkSize)
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for i := range tiles {
		tiles[i] = types.TileGrass
	}
	if err := chunk.RestoreTiles(tiles, 0, 10); err != nil {
		t.Fatal(err)
	}
	chunk.SetState(types.ChunkStatePreloaded)
	cm.chunks[coord] = chunk
	return chunk
}

func TestChunkEventsCaptureSnapshotsAndSequencesBeforeDispatch(t *testing.T) {
	cm := newStreamingTestManager(t)
	coord := types.ChunkCoord{}
	addStreamingTestChunk(t, cm, coord)
	addStreamingTestChunk(t, cm, types.ChunkCoord{X: 1})
	events := make(chan eventbus.Event, 32)
	cm.eventBus.SubscribeAsync(ecs.TopicGameplayChunk, eventbus.PriorityMedium, func(_ context.Context, e eventbus.Event) error { events <- e; return nil })
	cm.RegisterEntity(1, 0, 0, false)
	cm.RegisterEntity(2, 0, 0, false)
	cm.EnableChunkLoadEvents(1, 4)
	cm.EnableChunkLoadEvents(2, 8)
	cm.SetTile(0, 0, types.TilePlowed)
	cm.SetTile(1, 0, types.TilePlowed)
	cm.UpdateEntityPosition(1, types.ChunkCoord{X: 1})
	cm.EnableChunkLoadEvents(1, 4)
	cm.UpdateEntityPosition(1, coord)
	var firstPlayer []eventbus.Event
	var secondPlayer []*ecs.ChunkLoadEvent
	for i := 0; i < 11; i++ {
		select {
		case e := <-events:
			switch event := e.(type) {
			case *ecs.ChunkLoadEvent:
				if event.EntityID == 1 {
					firstPlayer = append(firstPlayer, event)
				} else {
					secondPlayer = append(secondPlayer, event)
				}
			case *ecs.ChunkUnloadEvent:
				firstPlayer = append(firstPlayer, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing event %d", i)
		}
	}
	sequence := func(e eventbus.Event) uint64 {
		if load, ok := e.(*ecs.ChunkLoadEvent); ok {
			return load.EventSeq
		}
		return e.(*ecs.ChunkUnloadEvent).EventSeq
	}
	sort.Slice(firstPlayer, func(i, j int) bool { return sequence(firstPlayer[i]) < sequence(firstPlayer[j]) })
	for i, e := range firstPlayer {
		if sequence(e) != uint64(i+1) {
			t.Fatalf("counter reset or skipped: %#v", firstPlayer)
		}
	}
	initial := firstPlayer[0].(*ecs.ChunkLoadEvent)
	second := firstPlayer[1].(*ecs.ChunkLoadEvent)
	third := firstPlayer[2].(*ecs.ChunkLoadEvent)
	if initial.Version != 10 || initial.Tiles[0] != types.TileGrass || second.Version != 11 || second.Tiles[1] != types.TileGrass || third.Version != 12 || third.Tiles[0] != types.TilePlowed || third.Tiles[1] != types.TilePlowed {
		t.Fatal("snapshot bytes/version did not preserve both edits")
	}
	if _, ok := firstPlayer[3].(*ecs.ChunkUnloadEvent); !ok {
		t.Fatal("unload was not ordered before reload")
	}
	if len(secondPlayer) != 3 {
		t.Fatal("second viewer missed edits")
	}
	cm.EnableChunkLoadEvents(1, 5)
	select {
	case e := <-events:
		load := e.(*ecs.ChunkLoadEvent)
		if load.Epoch != 5 || load.EventSeq != 1 {
			t.Fatalf("new stream did not reset counter: %#v", load)
		}
	case <-time.After(time.Second):
		t.Fatal("new stream missing load")
	}
}

func TestDirtyChunkEvictionRetainsAndDelaysRetry(t *testing.T) {
	cm := newStreamingTestManager(t)
	coord := types.ChunkCoord{}
	chunk := addStreamingTestChunk(t, cm, coord)
	chunk.SetState(types.ChunkStateInactive)
	chunk.SetTile(0, 0, types.TilePlowed)
	cm.safeSaveAndRemove(coord, chunk)
	if cm.GetChunkFast(coord) != chunk || !chunk.TilesDirty() {
		t.Fatal("failed save discarded dirty chunk")
	}
	cm.retryMu.Lock()
	retry, exists := cm.saveRetries[coord]
	cm.retryMu.Unlock()
	if !exists || time.Until(retry.due) <= 0 {
		t.Fatal("missing delayed retry")
	}
	cm.enqueueSaveRetries(time.Now())
	if len(cm.saveQueue) != 0 {
		t.Fatal("retry loop has no delay")
	}
	cm.enqueueSaveRetries(retry.due)
	if len(cm.saveQueue) != 1 {
		t.Fatal("due retry not enqueued")
	}
	<-cm.saveQueue
	chunk.ClearTilesDirty()
	cm.safeSaveAndRemove(coord, chunk)
	if cm.GetChunkFast(coord) != nil {
		t.Fatal("clean inactive chunk not evicted")
	}
}
