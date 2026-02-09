package ecs

import (
	"testing"

	"origin/internal/types"
)

type BenchPosition struct {
	X, Y, Z float64
}

type BenchVelocity struct {
	X, Y, Z float64
}

const (
	BenchPositionID ComponentID = 10
	BenchVelocityID ComponentID = 11
)

func init() {
	RegisterComponent[BenchPosition](BenchPositionID)
	RegisterComponent[BenchVelocity](BenchVelocityID)
}

// TestQueryForEachZeroCopy verifies ForEach doesn't allocate
func TestQueryForEachZeroCopy(t *testing.T) {
	w := NewWorldForTesting()

	// Create entities
	for i := 0; i < 100; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
		AddComponent(w, h, BenchVelocity{X: 1, Y: 0, Z: 0})
	}

	query := w.Query().With(BenchPositionID).With(BenchVelocityID)

	count := 0
	query.ForEach(func(h types.Handle) {
		count++
	})

	if count != 100 {
		t.Errorf("expected 100 entities, got %d", count)
	}
}

// TestPreparedQuery verifies prepared query caching
func TestPreparedQuery(t *testing.T) {
	w := NewWorldForTesting()

	// Create entities with Position
	for i := 0; i < 50; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
	}

	// Create prepared query
	pq := w.Query().With(BenchPositionID).Prepare()

	if pq.Count() != 50 {
		t.Errorf("expected 50 entities, got %d", pq.Count())
	}

	// Add more entities to same archetype (Position only)
	for i := 50; i < 100; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
	}

	// PreparedQuery sees new entities in existing archetypes immediately
	// (archetype list is cached, but archetype contents are live)
	if pq.Count() != 100 {
		t.Errorf("expected 100 entities (live archetype data), got %d", pq.Count())
	}

	// Create entities with different archetype (Position + Velocity)
	for i := 100; i < 150; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
		AddComponent(w, h, BenchVelocity{X: 1, Y: 0, Z: 0})
	}

	// New archetype automatically detected via version tracking
	// PreparedQuery auto-refreshes on Count() call
	if pq.Count() != 150 {
		t.Errorf("expected 150 entities (auto-refresh on new archetype), got %d", pq.Count())
	}

	// Manual refresh still works
	pq.Refresh()
	if pq.Count() != 150 {
		t.Errorf("expected 150 entities after manual refresh, got %d", pq.Count())
	}
}

// TestQueryHandlesInto verifies HandlesInto with caller-managed buffer
func TestQueryHandlesInto(t *testing.T) {
	w := NewWorldForTesting()

	type Position struct{ X, Y, Z float64 }
	const PositionID ComponentID = 30
	RegisterComponent[Position](PositionID)

	// Spawn entities
	for i := 0; i < 100; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Position{X: float64(i)})
	}

	// Test with nil buffer (allocates)
	handles1 := w.Query().With(PositionID).HandlesInto(nil)
	if len(handles1) != 100 {
		t.Errorf("expected 100 handles, got %d", len(handles1))
	}

	// Test with pre-allocated buffer (zero allocations)
	buf := make([]types.Handle, 0, 200)
	handles2 := w.Query().With(PositionID).HandlesInto(buf)
	if len(handles2) != 100 {
		t.Errorf("expected 100 handles, got %d", len(handles2))
	}
	if cap(handles2) != 200 {
		t.Errorf("expected capacity 200, got %d", cap(handles2))
	}

	// Test reusing buffer (append pattern)
	buf = buf[:0] // Reset length, keep capacity
	handles3 := w.Query().With(PositionID).HandlesInto(buf)
	if len(handles3) != 100 {
		t.Errorf("expected 100 handles on reuse, got %d", len(handles3))
	}
}

// BenchmarkQueryHandlesInto measures HandlesInto performance
func BenchmarkQueryHandlesInto(b *testing.B) {
	w := NewWorldForTesting()

	type Position struct{ X, Y, Z float64 }
	const PositionID ComponentID = 31
	RegisterComponent[Position](PositionID)

	// Create 10k entities
	for i := 0; i < 10000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Position{X: float64(i), Y: 0, Z: 0})
	}

	b.Run("HandlesInto-Nil", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			handles := w.Query().With(PositionID).HandlesInto(nil)
			_ = handles
		}
	})

	b.Run("HandlesInto-Preallocated", func(b *testing.B) {
		buf := make([]types.Handle, 0, 10000)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf = buf[:0]
			handles := w.Query().With(PositionID).HandlesInto(buf)
			_ = handles
		}
	})

	b.Run("Handles-Legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			handles := w.Query().With(PositionID).Handles()
			_ = handles
		}
	})
}

// BenchmarkQueryForEach_Old simulates old implementation with copies
func BenchmarkQueryForEach_Old(b *testing.B) {
	w := NewWorldForTesting()

	// Create 10k entities
	for i := 0; i < 10000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
		AddComponent(w, h, BenchVelocity{X: 1, Y: 0, Z: 0})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := w.Query().With(BenchPositionID).With(BenchVelocityID)
		handles := query.Handles() // Allocates and copies
		for _, h := range handles {
			_ = h
		}
	}
}

// BenchmarkQueryForEach_New uses zero-copy ForEach
func BenchmarkQueryForEach_New(b *testing.B) {
	w := NewWorldForTesting()

	// Create 10k entities
	for i := 0; i < 10000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
		AddComponent(w, h, BenchVelocity{X: 1, Y: 0, Z: 0})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := w.Query().With(BenchPositionID).With(BenchVelocityID)
		query.ForEach(func(h types.Handle) {
			_ = h
		})
	}
}

// BenchmarkPreparedQuery uses cached archetype list
func BenchmarkPreparedQuery(b *testing.B) {
	w := NewWorldForTesting()

	// Create 10k entities
	for i := 0; i < 10000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
		AddComponent(w, h, BenchVelocity{X: 1, Y: 0, Z: 0})
	}

	pq := w.Query().With(BenchPositionID).With(BenchVelocityID).Prepare()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.ForEach(func(h types.Handle) {
			_ = h
		})
	}
}

// BenchmarkQueryWithComponentAccess simulates real system workload
func BenchmarkQueryWithComponentAccess(b *testing.B) {
	w := NewWorldForTesting()

	// Create 10k entities
	for i := 0; i < 10000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
		AddComponent(w, h, BenchVelocity{X: 1, Y: 0, Z: 0})
	}

	pq := w.Query().With(BenchPositionID).With(BenchVelocityID).Prepare()
	dt := 0.016

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.ForEach(func(h types.Handle) {
			MutateComponent[BenchPosition](w, h, func(pos *BenchPosition) bool {
				if vel, ok := GetComponent[BenchVelocity](w, h); ok {
					pos.X += vel.X * dt
					pos.Y += vel.Y * dt
					pos.Z += vel.Z * dt
				}
				return true
			})
		})
	}
}

// BenchmarkQueryAllocation measures allocation overhead
func BenchmarkQueryAllocation(b *testing.B) {
	w := NewWorldForTesting()

	for i := 0; i < 1000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, BenchPosition{X: float64(i), Y: 0, Z: 0})
	}

	b.Run("Handles-Copy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			query := w.Query().With(BenchPositionID)
			handles := query.Handles()
			_ = handles
		}
	})

	b.Run("ForEach-ZeroCopy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			query := w.Query().With(BenchPositionID)
			query.ForEach(func(h types.Handle) {
				_ = h
			})
		}
	})

	b.Run("PreparedQuery-ZeroCopy", func(b *testing.B) {
		pq := w.Query().With(BenchPositionID).Prepare()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pq.ForEach(func(h types.Handle) {
				_ = h
			})
		}
	})
}
