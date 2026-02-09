package ecs

import (
	"testing"

	"origin/internal/types"
)

// TestPreparedQueryAutoRefresh verifies automatic refresh when new archetypes are created
func TestPreparedQueryAutoRefresh(t *testing.T) {
	w := NewWorldForTesting()

	type Transform struct{ X, Y, Z float64 }
	type Velocity struct{ X, Y, Z float64 }
	type InCombat struct{}
	type Swimming struct{}

	const TransformID ComponentID = 40
	const VelocityID ComponentID = 41
	const InCombatID ComponentID = 42
	const SwimmingID ComponentID = 43

	RegisterComponent[Transform](TransformID)
	RegisterComponent[Velocity](VelocityID)
	RegisterComponent[InCombat](InCombatID)
	RegisterComponent[Swimming](SwimmingID)

	// Create prepared query for Transform+Velocity
	pq := w.Query().With(TransformID).With(VelocityID).Prepare()

	// Spawn entities with Transform+Velocity
	for i := 0; i < 10; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Transform{X: float64(i)})
		AddComponent(w, h, Velocity{X: 1.0})
	}

	// Initial count
	count1 := pq.Count()
	if count1 != 10 {
		t.Errorf("expected 10 entities, got %d", count1)
	}

	// Add InCombat to some entities - creates new archetype (Transform+Velocity+InCombat)
	for i := 0; i < 5; i++ {
		h := types.MakeHandle(uint32(i+1), 0)
		AddComponent(w, h, InCombat{})
	}

	// PreparedQuery should auto-refresh and see entities in new archetype
	count2 := pq.Count()
	if count2 != 10 {
		t.Errorf("expected 10 entities after adding InCombat (auto-refresh), got %d", count2)
	}

	// Add Swimming to some entities - creates another new archetype
	for i := 0; i < 3; i++ {
		h := types.MakeHandle(uint32(i+1), 0)
		AddComponent(w, h, Swimming{})
	}

	// Should still see all entities
	count3 := pq.Count()
	if count3 != 10 {
		t.Errorf("expected 10 entities after adding Swimming (auto-refresh), got %d", count3)
	}

	// Verify ForEach also auto-refreshes
	seen := 0
	pq.ForEach(func(h types.Handle) {
		seen++
	})
	if seen != 10 {
		t.Errorf("expected to iterate over 10 entities, got %d", seen)
	}
}

// TestPreparedQueryNoRefreshWhenNoNewArchetypes verifies no unnecessary refreshes
func TestPreparedQueryNoRefreshWhenNoNewArchetypes(t *testing.T) {
	w := NewWorldForTesting()

	type Position struct{ X, Y float64 }
	const PositionID ComponentID = 50
	RegisterComponent[Position](PositionID)

	// Spawn entities - creates archetype
	for i := 0; i < 100; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Position{X: float64(i), Y: 0})
	}

	// Create prepared query
	pq := w.Query().With(PositionID).Prepare()
	initialVersion := pq.seenVersion

	// Multiple iterations without creating new archetypes
	for tick := 0; tick < 10; tick++ {
		count := 0
		pq.ForEach(func(h types.Handle) {
			count++
		})
		if count != 100 {
			t.Errorf("tick %d: expected 100 entities, got %d", tick, count)
		}
	}

	// Version should not have changed
	if pq.seenVersion != initialVersion {
		t.Errorf("expected version to stay at %d, got %d", initialVersion, pq.seenVersion)
	}
}

// TestPreparedQueryWithDynamicCombinations simulates MMO scenario with dynamic component combinations
func TestPreparedQueryWithDynamicCombinations(t *testing.T) {
	w := NewWorldForTesting()

	type Transform struct{ X, Y, Z float64 }
	type Health struct{ HP int }
	type InCombat struct{}
	type Stealth struct{}
	type Dragging struct{ TargetID types.EntityID }

	const TransformID ComponentID = 55
	const HealthID ComponentID = 56
	const InCombatID ComponentID = 57
	const StealthID ComponentID = 58
	const DraggingID ComponentID = 59

	RegisterComponent[Transform](TransformID)
	RegisterComponent[Health](HealthID)
	RegisterComponent[InCombat](InCombatID)
	RegisterComponent[Stealth](StealthID)
	RegisterComponent[Dragging](DraggingID)

	// Query for all entities with Transform (regardless of other components)
	pq := w.Query().With(TransformID).Prepare()

	// Spawn base entities
	handles := make([]types.Handle, 20)
	for i := 0; i < 20; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Transform{X: float64(i)})
		AddComponent(w, h, Health{HP: 100})
		handles[i] = h
	}

	initialCount := pq.Count()
	if initialCount != 20 {
		t.Fatalf("expected 20 entities initially, got %d", initialCount)
	}

	// Simulate dynamic state changes creating new archetypes
	// Player 0 enters combat
	AddComponent(w, handles[0], InCombat{})
	if pq.Count() != 20 {
		t.Error("should see entity after entering combat")
	}

	// Player 1 enters stealth
	AddComponent(w, handles[1], Stealth{})
	if pq.Count() != 20 {
		t.Error("should see entity after entering stealth")
	}

	// Player 2 starts dragging
	AddComponent(w, handles[2], Dragging{TargetID: 999})
	if pq.Count() != 20 {
		t.Error("should see entity after starting dragging")
	}

	// Player 3 enters combat AND stealth (new combination)
	AddComponent(w, handles[3], InCombat{})
	AddComponent(w, handles[3], Stealth{})
	if pq.Count() != 20 {
		t.Error("should see entity with combat+stealth combination")
	}

	// Player 4 enters combat AND dragging (another new combination)
	AddComponent(w, handles[4], InCombat{})
	AddComponent(w, handles[4], Dragging{TargetID: 888})
	if pq.Count() != 20 {
		t.Error("should see entity with combat+dragging combination")
	}

	// Verify all entities are still visible via ForEach
	seen := make(map[types.Handle]bool)
	pq.ForEach(func(h types.Handle) {
		seen[h] = true
	})
	if len(seen) != 20 {
		t.Errorf("expected to see 20 unique entities, got %d", len(seen))
	}
}

// TestPreparedQueryWithTemporaryEntities simulates temporary entities/effects
func TestPreparedQueryWithTemporaryEntities(t *testing.T) {
	w := NewWorldForTesting()

	type Position struct{ X, Y, Z float64 }
	type Effect struct{ Duration float64 }
	type Projectile struct{}

	const PositionID ComponentID = 60
	const EffectID ComponentID = 61
	const ProjectileID ComponentID = 62

	RegisterComponent[Position](PositionID)
	RegisterComponent[Effect](EffectID)
	RegisterComponent[Projectile](ProjectileID)

	// Query for all positioned entities
	pq := w.Query().With(PositionID).Prepare()

	// Spawn permanent entities
	for i := 0; i < 10; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Position{X: float64(i)})
	}

	if pq.Count() != 10 {
		t.Fatalf("expected 10 permanent entities, got %d", pq.Count())
	}

	// Spawn temporary effect (Position+Effect) - new archetype
	effectH := w.SpawnWithoutExternalID()
	AddComponent(w, effectH, Position{X: 50, Y: 50})
	AddComponent(w, effectH, Effect{Duration: 2.0})

	// Should see the effect
	if pq.Count() != 11 {
		t.Errorf("expected 11 entities with effect, got %d", pq.Count())
	}

	// Spawn projectile (Position+Projectile) - another new archetype
	projH := w.SpawnWithoutExternalID()
	AddComponent(w, projH, Position{X: 100, Y: 100})
	AddComponent(w, projH, Projectile{})

	// Should see the projectile
	if pq.Count() != 12 {
		t.Errorf("expected 12 entities with projectile, got %d", pq.Count())
	}

	// Despawn temporary entities
	w.Despawn(effectH)
	w.Despawn(projH)

	// Should be back to 10
	if pq.Count() != 10 {
		t.Errorf("expected 10 entities after despawn, got %d", pq.Count())
	}
}

// BenchmarkPreparedQueryAutoRefresh measures overhead of version check
func BenchmarkPreparedQueryAutoRefresh(b *testing.B) {
	w := NewWorldForTesting()

	type Transform struct{ X, Y, Z float64 }
	const TransformID ComponentID = 45
	RegisterComponent[Transform](TransformID)

	// Spawn entities
	for i := 0; i < 1000; i++ {
		h := w.Spawn(types.EntityID(i), nil)
		AddComponent(w, h, Transform{X: float64(i)})
	}

	pq := w.Query().With(TransformID).Prepare()

	b.ResetTimer()
	b.Run("ForEach-NoRefresh", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			count := 0
			pq.ForEach(func(h types.Handle) {
				count++
			})
		}
	})

	// Create new archetype to trigger refresh
	type Velocity struct{ X, Y, Z float64 }
	const VelocityID ComponentID = 46
	RegisterComponent[Velocity](VelocityID)
	h := w.Spawn(types.EntityID(9999), nil)
	AddComponent(w, h, Transform{X: 999})
	AddComponent(w, h, Velocity{X: 1})

	b.Run("ForEach-WithRefresh", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			count := 0
			pq.ForEach(func(h types.Handle) {
				count++
			})
		}
	})
}
