package game

import (
	"fmt"
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

// Reserved slots never reach I/O workers. Compare empty and nearly full queues
// to guard against restoring a linear scan when selecting an operation slot.
func BenchmarkObjectDestructionReservation(b *testing.B) {
	for _, population := range []int{1000, 100000} {
		for _, occupied := range []int{0, ObjectDestructionQueueCapacity - 1, ObjectDestructionQueueCapacity} {
			b.Run(fmt.Sprintf("world_%d/occupied_%d", population, occupied), func(b *testing.B) {
				f := newDestructionServiceFixtureWithCapacity(b, uint32(population+ObjectDestructionQueueCapacity+2))
				for i := 0; i < occupied; i++ {
					target := f.targetWithID(b, types.EntityID(i+2))
					if _, err := f.service.reserve(target); err != nil {
						b.Fatal(err)
					}
				}
				for i := 0; i < population; i++ {
					f.w.Spawn(types.EntityID(i+ObjectDestructionQueueCapacity+2), nil)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					reservation, err := f.service.reserve(f.target)
					if occupied == ObjectDestructionQueueCapacity {
						if err != ErrObjectDestructionQueueFull {
							b.Fatal(err)
						}
					} else if err != nil || !f.service.cancelReservation(reservation) {
						b.Fatal("reservation failed", err)
					}
				}
			})
		}
	}
}
