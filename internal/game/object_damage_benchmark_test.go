package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	"origin/internal/types"
)

// These baselines stay unchanged so HP lookup/write overhead is comparable
// before and after the damage receiver and destruction guards are installed.
func BenchmarkObjectDamageHealthBaseline(b *testing.B) {
	for _, population := range []struct {
		name  string
		count int
	}{{"1000", 1000}, {"100000", 100000}} {
		b.Run(population.name, func(b *testing.B) {
			w := ecs.NewWorldWithCapacity(uint32(population.count+8), nil, 0)
			target := w.Spawn(1, nil)
			ecs.AddComponent(w, target, components.ObjectInternalState{HP: 100, HasHP: true})
			for i := 0; i < population.count; i++ {
				w.Spawn(types.EntityID(i+2), nil)
			}
			b.Run("Read", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					health, ok := ecs.GetComponent[components.ObjectInternalState](w, target)
					if !ok || health.HP < 0 {
						b.Fatal("missing HP")
					}
				}
			})
			b.Run("Set", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if err := gameworld.SetObjectHP(w, target, float64(100+i%2)); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func BenchmarkObjectDamageApply(b *testing.B) {
	for _, population := range []struct {
		name  string
		count int
	}{{"1000", 1000}, {"100000", 100000}} {
		b.Run(population.name, func(b *testing.B) {
			w := ecs.NewWorldWithCapacity(uint32(population.count+8), nil, 0)
			f := newObjectDamageFixture(b, w)
			for i := 0; i < population.count; i++ {
				w.Spawn(types.EntityID(i+2), nil)
			}
			for _, test := range []struct {
				name string
				draw float64
			}{{"Nonfatal", 0.49}, {"Zero", 0}, {"Invalid", -1}} {
				b.Run(test.name, func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						// Reset is included identically in every Apply benchmark.
						f.healthForBenchmarkReset()
						_, err := f.service.Apply(f.target, test.draw)
						if test.draw >= 0 && err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func (f *objectDamageFixture) healthForBenchmarkReset() {
	f.service.health.WithPtr(f.target, func(hp *components.ObjectInternalState) { hp.HP = 100 })
}
