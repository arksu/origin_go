package game

import (
	"testing"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// This baseline deliberately includes the existing regeneration scheduler and
// cooldown commit. Networking is measured separately from authoritative costs.
func BenchmarkMeleeActionCosts(b *testing.B) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, nil)
	ecs.AddComponent(w, player, components.EntityStats{Stamina: 1000, Energy: 900})
	ecs.GetResource[ecs.TimeState](w).UnixMs = 1000
	service := &ActionService{world: w}
	definition := &actiondefs.Definition{ID: "benchmark_melee", Cooldown: 2000,
		Execution: actiondefs.Execution{Stamina: 60}}
	service.chargeActionCosts(w, player, definition)
	stats := ecs.GetOrCreateStorage[components.EntityStats](w)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats.Set(player, components.EntityStats{Stamina: 1000, Energy: 900})
		if !service.chargeActionCosts(w, player, definition) {
			b.Fatal("cost rejected")
		}
	}
}
