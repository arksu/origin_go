package ecs

import (
	"testing"
)

func TestChunkIndex_AddAndGet(t *testing.T) {
	ci := NewChunkIndex()

	h1 := Handle(1)
	h2 := Handle(2)
	h3 := Handle(3)
	key1 := uint64(100)
	key2 := uint64(200)

	// Add entities to chunks
	ci.Add(h1, key1)
	ci.Add(h2, key1)
	ci.Add(h3, key2)

	// Verify chunk keys
	if k, ok := ci.GetChunkKey(h1); !ok || k != key1 {
		t.Errorf("h1 should be in key1, got key=%d, ok=%v", k, ok)
	}
	if k, ok := ci.GetChunkKey(h2); !ok || k != key1 {
		t.Errorf("h2 should be in key1, got key=%d, ok=%v", k, ok)
	}
	if k, ok := ci.GetChunkKey(h3); !ok || k != key2 {
		t.Errorf("h3 should be in key2, got key=%d, ok=%v", k, ok)
	}

	// Verify entities in chunks
	entities1 := ci.GetEntitiesInChunk(key1)
	if len(entities1) != 2 {
		t.Errorf("key1 should have 2 entities, got %d", len(entities1))
	}

	entities2 := ci.GetEntitiesInChunk(key2)
	if len(entities2) != 1 {
		t.Errorf("key2 should have 1 entity, got %d", len(entities2))
	}
}

func TestChunkIndex_SwapBackRemoval(t *testing.T) {
	ci := NewChunkIndex()

	h1 := Handle(1)
	h2 := Handle(2)
	h3 := Handle(3)
	key := uint64(100)

	// Add 3 entities
	ci.Add(h1, key)
	ci.Add(h2, key)
	ci.Add(h3, key)

	// Remove middle entity (h2)
	ci.Remove(h2)

	// Verify h2 is removed
	if _, ok := ci.GetChunkKey(h2); ok {
		t.Error("h2 should be removed")
	}

	// Verify h1 and h3 are still present
	entities := ci.GetEntitiesInChunk(key)
	if len(entities) != 2 {
		t.Errorf("should have 2 entities after removal, got %d", len(entities))
	}

	found1, found3 := false, false
	for _, h := range entities {
		if h == h1 {
			found1 = true
		}
		if h == h3 {
			found3 = true
		}
	}
	if !found1 || !found3 {
		t.Errorf("h1 and h3 should still be present, found1=%v, found3=%v", found1, found3)
	}
}

func TestChunkIndex_UpdateChunk(t *testing.T) {
	ci := NewChunkIndex()

	h := Handle(1)
	key1 := uint64(100)
	key2 := uint64(200)

	// Add to key1
	ci.Add(h, key1)

	// Update to key2
	changed := ci.UpdateChunk(h, key2)
	if !changed {
		t.Error("UpdateChunk should return true when chunk changes")
	}

	// Verify moved
	if k, ok := ci.GetChunkKey(h); !ok || k != key2 {
		t.Errorf("entity should be in key2, got key=%d, ok=%v", k, ok)
	}

	// Verify old chunk is empty
	entities1 := ci.GetEntitiesInChunk(key1)
	if len(entities1) != 0 {
		t.Errorf("key1 should be empty, got %d entities", len(entities1))
	}

	// Update to same chunk should return false
	changed = ci.UpdateChunk(h, key2)
	if changed {
		t.Error("UpdateChunk should return false when chunk doesn't change")
	}
}

func TestChunkIndex_GetEntitiesInChunksSet(t *testing.T) {
	ci := NewChunkIndex()

	h1 := Handle(1)
	h2 := Handle(2)
	h3 := Handle(3)
	key1 := uint64(100)
	key2 := uint64(200)
	key3 := uint64(300)

	ci.Add(h1, key1)
	ci.Add(h2, key2)
	ci.Add(h3, key3)

	// Query subset of chunks
	activeChunks := map[uint64]struct{}{
		key1: {},
		key3: {},
	}

	buffer := make([]Handle, 0, 4)
	result := ci.GetEntitiesInChunksSet(activeChunks, buffer)

	if len(result) != 2 {
		t.Errorf("should get 2 entities from active chunks, got %d", len(result))
	}

	// Verify correct entities
	found1, found3 := false, false
	for _, h := range result {
		if h == h1 {
			found1 = true
		}
		if h == h3 {
			found3 = true
		}
		if h == h2 {
			t.Error("h2 should not be in result (its chunk is not active)")
		}
	}
	if !found1 || !found3 {
		t.Errorf("h1 and h3 should be in result, found1=%v, found3=%v", found1, found3)
	}
}

func TestChunkIndex_EmptyChunkCleanup(t *testing.T) {
	ci := NewChunkIndex()

	h := Handle(1)
	key := uint64(100)

	ci.Add(h, key)
	ci.Remove(h)

	// Verify chunk count is 0 after removing last entity
	if ci.ChunkCount() != 0 {
		t.Errorf("chunk count should be 0 after removing all entities, got %d", ci.ChunkCount())
	}
}
