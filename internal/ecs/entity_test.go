package ecs

import (
	"testing"
)

// TestGenerationalHandles verifies that stale handles are detected
func TestGenerationalHandles(t *testing.T) {
	allocator := NewHandleAllocator(100)

	// Allocate a handle
	h1 := allocator.Alloc()
	if h1 == InvalidHandle {
		t.Fatal("failed to allocate handle")
	}

	// Verify it's valid
	if !allocator.IsValid(h1) {
		t.Error("newly allocated handle should be valid")
	}

	// Free the handle
	allocator.Free(h1)

	// The old handle should now be invalid (stale)
	if allocator.IsValid(h1) {
		t.Error("freed handle should be invalid (stale)")
	}

	// Allocate again - should reuse the same index but with new generation
	h2 := allocator.Alloc()
	if h2 == InvalidHandle {
		t.Fatal("failed to allocate second handle")
	}

	// h2 should use same index as h1 but different generation
	if h1.Index() != h2.Index() {
		t.Error("expected same index for recycled handle")
	}
	if h1.Generation() == h2.Generation() {
		t.Error("expected different generation for recycled handle")
	}

	// h1 should still be invalid (stale)
	if allocator.IsValid(h1) {
		t.Error("old handle should remain invalid after index reuse")
	}

	// h2 should be valid
	if !allocator.IsValid(h2) {
		t.Error("new handle should be valid")
	}
}

// TestStaleHandleInComponentStorage demonstrates prevention of stale handle bugs
func TestStaleHandleInComponentStorage(t *testing.T) {
	type TestComponent struct {
		Value int
	}

	storage := NewComponentStorage[TestComponent](10)
	allocator := NewHandleAllocator(100)

	// Create entity 1
	h1 := allocator.Alloc()
	storage.Set(h1, TestComponent{Value: 42})

	// Verify we can get it
	if comp, ok := storage.Get(h1); !ok || comp.Value != 42 {
		t.Error("should be able to get component with valid handle")
	}

	// Simulate entity death - free handle and remove component
	allocator.Free(h1)
	storage.Remove(h1)

	// Create entity 2 - reuses same index but new generation
	h2 := allocator.Alloc()
	storage.Set(h2, TestComponent{Value: 99})

	// Critical test: h1 (stale handle) should NOT access h2's data
	if comp, ok := storage.Get(h1); ok {
		t.Errorf("stale handle should not access new entity data, got value=%d", comp.Value)
	}

	// h2 should work correctly
	if comp, ok := storage.Get(h2); !ok || comp.Value != 99 {
		t.Error("new handle should access correct data")
	}
}

// TestStaleHandleInWorld demonstrates prevention at World level
func TestStaleHandleInWorld(t *testing.T) {
	w := NewWorld()

	// Create entity 1
	h1 := w.Spawn(EntityID(1))
	if !w.Alive(h1) {
		t.Error("newly spawned entity should be alive")
	}

	// Despawn entity 1
	w.Despawn(h1)
	if w.Alive(h1) {
		t.Error("despawned entity should not be alive")
	}

	// Create entity 2 - reuses same index
	h2 := w.Spawn(EntityID(2))
	if !w.Alive(h2) {
		t.Error("newly spawned entity should be alive")
	}

	// Critical test: h1 (stale) should still be dead
	if w.Alive(h1) {
		t.Error("stale handle should not be alive after index reuse")
	}

	// Verify h1 and h2 use same index but different generation
	if h1.Index() != h2.Index() {
		t.Error("expected same index for recycled handle")
	}
	if h1.Generation() == h2.Generation() {
		t.Error("expected different generation for recycled handle")
	}
}

// TestHandlePackingUnpacking verifies bit packing correctness
func TestHandlePackingUnpacking(t *testing.T) {
	tests := []struct {
		index      uint32
		generation uint32
	}{
		{0, 0},
		{1, 0},
		{0, 1},
		{1, 1},
		{0xFFFFFFFF, 0},
		{0, 0xFFFFFFFF},
		{0xFFFFFFFF, 0xFFFFFFFF},
		{12345, 67890},
	}

	for _, tt := range tests {
		h := MakeHandle(tt.index, tt.generation)
		if h.Index() != tt.index {
			t.Errorf("index mismatch: expected %d, got %d", tt.index, h.Index())
		}
		if h.Generation() != tt.generation {
			t.Errorf("generation mismatch: expected %d, got %d", tt.generation, h.Generation())
		}
	}
}
