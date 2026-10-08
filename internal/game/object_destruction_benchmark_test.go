package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// Admission includes receiver validation and reservation, with a minimal
// quarantine callback. Background capture, SQL and materialization are excluded.
func BenchmarkObjectDestructionAdmission(b *testing.B) {
	for _, population := range []struct {
		name  string
		count int
	}{{"1000", 1000}, {"100000", 100000}} {
		b.Run(population.name, func(b *testing.B) {
			f := newDestructionServiceFixtureWithCapacity(b, uint32(population.count+8))
			for i := 0; i < population.count; i++ {
				f.w.Spawn(types.EntityID(i+2), nil)
			}
			state := ecs.GetResource[ecs.ObjectDestructionState](f.w)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				f.damage.health.WithPtr(f.target, func(hp *components.ObjectInternalState) { hp.HP = 100 })
				_, err := f.damage.Apply(f.target, 100)
				if err != nil {
					b.Fatal(err)
				}
				state.Pending[f.target] = false
				f.service.release(&f.service.operations[0])
			}
		})
	}
}
