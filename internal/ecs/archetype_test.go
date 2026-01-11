package ecs

import (
	"testing"

	"origin/internal/types"
)

// TestArchetypeRemoveEntityAt verifies O(1) removal with location tracking
func TestArchetypeRemoveEntityAt(t *testing.T) {
	arch := NewArchetype(0)

	// Add entities
	h1 := types.MakeHandle(1, 0)
	h2 := types.MakeHandle(2, 0)
	h3 := types.MakeHandle(3, 0)

	idx1 := arch.AddEntity(h1)
	idx2 := arch.AddEntity(h2)
	idx3 := arch.AddEntity(h3)

	if idx1 != 0 || idx2 != 1 || idx3 != 2 {
		t.Errorf("expected indices 0,1,2, got %d,%d,%d", idx1, idx2, idx3)
	}

	if arch.Len() != 3 {
		t.Errorf("expected 3 entities, got %d", arch.Len())
	}

	// Remove middle entity (idx1)
	swapped := arch.RemoveEntityAt(idx1)
	if swapped != h3 {
		t.Errorf("expected h3 to be swapped, got %v", swapped)
	}

	if arch.Len() != 2 {
		t.Errorf("expected 2 entities after removal, got %d", arch.Len())
	}

	// Remove last entity
	swapped = arch.RemoveEntityAt(1)
	if swapped != types.InvalidHandle {
		t.Errorf("expected InvalidHandle when removing last, got %v", swapped)
	}

	if arch.Len() != 1 {
		t.Errorf("expected 1 entity after removal, got %d", arch.Len())
	}
}

// TestWorldLocationTracking verifies entity location tracking
func TestWorldLocationTracking(t *testing.T) {
	w := NewWorld()

	// Spawn entities
	h1 := w.Spawn(types.EntityID(1))
	h2 := w.Spawn(types.EntityID(2))
	h3 := w.Spawn(types.EntityID(3))

	// All should be in same archetype (ExternalID only)
	loc1, ok1 := w.locations[h1]
	loc2, ok2 := w.locations[h2]
	loc3, ok3 := w.locations[h3]
	_ = h2 // Used for location tracking

	if !ok1 || !ok2 || !ok3 {
		t.Error("locations not tracked")
	}

	if loc1.archetype != loc2.archetype || loc2.archetype != loc3.archetype {
		t.Error("entities should be in same archetype")
	}

	// Despawn h2 (middle entity)
	w.Despawn(h2)

	// h2 location should be deleted
	if _, ok := w.locations[h2]; ok {
		t.Error("h2 location should be deleted")
	}

	// h3 should have been swapped to h2's old position
	newLoc3 := w.locations[h3]
	if newLoc3.index != loc2.index {
		t.Errorf("h3 should be at h2's old index %d, got %d", loc2.index, newLoc3.index)
	}
}

// TestComponentChangeArchetypeTransition verifies location tracking during archetype changes
func TestComponentChangeArchetypeTransition(t *testing.T) {
	w := NewWorld()

	type TestComp struct{ Value int }
	const TestCompID ComponentID = 20
	RegisterComponent[TestComp](TestCompID)

	// Spawn entities with ExternalID
	h1 := w.Spawn(types.EntityID(1))
	_ = w.Spawn(types.EntityID(2))
	h3 := w.Spawn(types.EntityID(3))

	loc1Before := w.locations[h1]

	// Add component to h1 - should move to new archetype
	AddComponent(w, h1, TestComp{Value: 42})

	loc1After := w.locations[h1]

	// h1 should be in different archetype now
	if loc1Before.archetype == loc1After.archetype {
		t.Error("h1 should have moved to new archetype")
	}

	// h1 should be at index 0 in new archetype
	if loc1After.index != 0 {
		t.Errorf("h1 should be at index 0 in new archetype, got %d", loc1After.index)
	}

	// h3 should have been swapped to h1's old position in original archetype
	loc3 := w.locations[h3]
	if loc3.archetype != loc1Before.archetype {
		t.Error("h3 should still be in original archetype")
	}
	if loc3.index != loc1Before.index {
		t.Errorf("h3 should be at h1's old index %d, got %d", loc1Before.index, loc3.index)
	}
}

// BenchmarkArchetypeRemoval_Old simulates O(n) linear search removal
func BenchmarkArchetypeRemoval_Old(b *testing.B) {
	sizes := []int{100, 1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(string(rune(size)), func(b *testing.B) {
			arch := NewArchetype(0)
			handles := make([]types.Handle, size)

			// Populate archetype
			for i := 0; i < size; i++ {
				handles[i] = types.MakeHandle(uint32(i), 0)
				arch.AddEntity(handles[i])
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Remove from middle (worst case for linear search)
				h := handles[size/2]
				arch.RemoveEntity(h)
				// Re-add to maintain size
				arch.AddEntity(h)
			}
		})
	}
}

// BenchmarkArchetypeRemoval_New uses O(1) index-based removal
func BenchmarkArchetypeRemoval_New(b *testing.B) {
	sizes := []int{100, 1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(string(rune(size)), func(b *testing.B) {
			arch := NewArchetype(0)
			handles := make([]types.Handle, size)
			indices := make([]int, size)

			// Populate archetype
			for i := 0; i < size; i++ {
				handles[i] = types.MakeHandle(uint32(i), 0)
				indices[i] = arch.AddEntity(handles[i])
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Remove from middle using O(1) index
				idx := indices[size/2]
				swapped := arch.RemoveEntityAt(idx)

				// Update swapped entity's index
				if swapped != types.InvalidHandle {
					for j := range handles {
						if handles[j] == swapped {
							indices[j] = idx
							break
						}
					}
				}

				// Re-add to maintain size
				indices[size/2] = arch.AddEntity(handles[size/2])
			}
		})
	}
}

// BenchmarkWorldDespawn measures real-world despawn performance
func BenchmarkWorldDespawn(b *testing.B) {
	sizes := []int{1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(string(rune(size)), func(b *testing.B) {
			w := NewWorld()
			handles := make([]types.Handle, size)

			// Spawn entities
			for i := 0; i < size; i++ {
				handles[i] = w.Spawn(types.EntityID(i))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Despawn and respawn middle entity
				h := handles[size/2]
				w.Despawn(h)
				handles[size/2] = w.Spawn(types.EntityID(size / 2))
			}
		})
	}
}

// BenchmarkComponentChange measures archetype transition performance
func BenchmarkComponentChange(b *testing.B) {
	type BenchComp struct{ Value int }
	const BenchCompID ComponentID = 21
	RegisterComponent[BenchComp](BenchCompID)

	sizes := []int{1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(string(rune(size)), func(b *testing.B) {
			w := NewWorld()
			handles := make([]types.Handle, size)

			// Spawn entities
			for i := 0; i < size; i++ {
				handles[i] = w.Spawn(types.EntityID(i))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				h := handles[size/2]
				// Add component (archetype transition)
				AddComponent(w, h, BenchComp{Value: 42})
				// Remove component (archetype transition back)
				RemoveComponent[BenchComp](w, h)
			}
		})
	}
}
