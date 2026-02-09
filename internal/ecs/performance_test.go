package ecs

import (
	"testing"

	"origin/internal/types"
)

// BenchmarkHandleValidation compares validation performance
func BenchmarkHandleValidation(b *testing.B) {
	allocator := NewHandleAllocator(1000000)

	// Allocate handles
	handles := make([]types.Handle, 10000)
	for i := 0; i < 10000; i++ {
		handles[i] = allocator.Alloc()
	}

	b.Run("IsValid-ArrayLookup", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = allocator.IsValid(handles[i%10000])
		}
	})
}

// BenchmarkWorldAlive measures Alive() performance on hot path
func BenchmarkWorldAlive(b *testing.B) {
	w := NewWorldForTesting()

	// Spawn entities
	handles := make([]types.Handle, 100000)
	for i := 0; i < 100000; i++ {
		handles[i] = w.Spawn(types.EntityID(i), nil)
	}

	b.Run("Alive-SingleLookup", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			h := handles[i%100000]
			_ = w.Alive(h)
		}
	})

	// Test with stale handles
	staleHandles := make([]types.Handle, 1000)
	for i := 0; i < 1000; i++ {
		h := w.Spawn(types.EntityID(100000+i), nil)
		staleHandles[i] = h
		w.Despawn(h)
	}

	b.Run("Alive-StaleHandle", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = w.Alive(staleHandles[i%1000])
		}
	})
}

// BenchmarkMemoryOverhead measures memory usage
func BenchmarkMemoryOverhead(b *testing.B) {
	b.Run("HandleAllocator-1M", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = NewHandleAllocator(1000000)
		}
	})

	b.Run("World-1M", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = NewWorldForTesting()
		}
	})
}

// BenchmarkHotPath simulates typical hot path operations
func BenchmarkHotPath(b *testing.B) {
	type Transform struct{ X, Y, Z float64 }
	const TransformID ComponentID = 30
	RegisterComponent[Transform](TransformID)

	w := NewWorldForTesting()

	// Spawn 10k entities
	handles := make([]types.Handle, 10000)
	for i := 0; i < 10000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Transform{X: float64(i), Y: 0, Z: 0})
		handles[i] = h
	}

	pq := w.Query().With(TransformID).Prepare()

	b.Run("QueryIteration-WithAliveCheck", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			count := 0
			pq.ForEach(func(h types.Handle) {
				if w.Alive(h) { // Hot path: Alive() check
					count++
				}
			})
		}
	})

	b.Run("QueryIteration-WithComponentAccess", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pq.ForEach(func(h types.Handle) {
				if w.Alive(h) {
					MutateComponent[Transform](w, h, func(t *Transform) bool {
						t.X += 1.0
						return true
					})
				}
			})
		}
	})
}

// TestHandleValidationCorrectness verifies generation-based validation
func TestHandleValidationCorrectness(t *testing.T) {
	allocator := NewHandleAllocator(100)

	// Allocate handle
	h1 := allocator.Alloc()
	if !allocator.IsValid(h1) {
		t.Error("newly allocated handle should be valid")
	}

	// Free handle
	allocator.Free(h1)
	if allocator.IsValid(h1) {
		t.Error("freed handle should be invalid")
	}

	// Allocate again - same index, new generation
	h2 := allocator.Alloc()
	if h1.Index() != h2.Index() {
		t.Error("should reuse same index")
	}
	if h1.Generation() == h2.Generation() {
		t.Error("should have different generation")
	}

	// h1 should still be invalid
	if allocator.IsValid(h1) {
		t.Error("old handle should remain invalid")
	}

	// h2 should be valid
	if !allocator.IsValid(h2) {
		t.Error("new handle should be valid")
	}
}

// TestWorldAliveCorrectness verifies World.Alive() behavior
func TestWorldAliveCorrectness(t *testing.T) {
	w := NewWorldForTesting()

	// Spawn entity
	h1 := w.Spawn(types.EntityID(1), nil)
	if !w.Alive(h1) {
		t.Error("spawned entity should be alive")
	}

	// Despawn
	w.Despawn(h1)
	if w.Alive(h1) {
		t.Error("despawned entity should not be alive")
	}

	// Spawn new entity (reuses index)
	h2 := w.Spawn(types.EntityID(2), nil)
	if !w.Alive(h2) {
		t.Error("new entity should be alive")
	}

	// h1 should still be dead (stale handle)
	if w.Alive(h1) {
		t.Error("stale handle should not be alive")
	}
}
