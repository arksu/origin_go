package game

import (
	"fmt"
	"math"
	"testing"

	"origin/internal/combat"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

var (
	creatureDamageBenchmarkResult CreatureDamageResult
	creatureDamageBenchmarkError  error
)

// Each operation includes a constant-time health reset, allowing every sample to
// exercise a real health commit rather than eventually measuring a dead target.
// Fresh notification samples also drain into caller-owned fixed-capacity buffers.
func BenchmarkCreatureDamageService(b *testing.B) {
	for _, population := range []int{1000, 100000} {
		b.Run(fmt.Sprintf("world_%d", population), func(b *testing.B) {
			world := ecs.NewWorldWithCapacity(uint32(population+2), nil, 0)
			fixture := creatureDamageFixtureInWorld(b, world)
			for i := 0; i < population; i++ {
				world.Spawn(types.EntityID(i+2), nil)
			}
			health := ecs.GetOrCreateStorage[components.EntityHealth](world)
			movement := ecs.GetOrCreateStorage[components.Movement](world)
			full := make([]components.InvItem, 0, len(combatEquipmentSlots))
			for i, slot := range combatEquipmentSlots {
				full = append(full, combatTestItem(types.EntityID(i+100), 9, 10, slot))
			}
			for _, equipment := range []struct {
				name  string
				items []components.InvItem
			}{
				{"empty", nil},
				{"partial", []components.InvItem{combatTestItem(100, 7, 10, netproto.EquipSlot_EQUIP_SLOT_CHEST)}},
				{"full", full},
			} {
				b.Run(equipment.name, func(b *testing.B) {
					fixture.equip(equipment.items...)
					for _, pending := range []bool{false, true} {
						name := "fresh"
						if pending {
							name = "pending"
							fixture.stats.MarkPlayerDirty(1, 1000, 0)
						}
						b.Run(name, func(b *testing.B) {
							statsBuffer := make([]types.EntityID, 0, 1)
							visualBuffer := make([]types.Handle, 0, 1)
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								health.Set(fixture.owner, components.EntityHealth{SHP: 10, HHP: 50})
								creatureDamageBenchmarkResult, creatureDamageBenchmarkError = fixture.service.Apply(fixture.owner, 1)
								if creatureDamageBenchmarkError != nil {
									b.Fatal(creatureDamageBenchmarkError)
								}
								if !pending {
									statsBuffer = fixture.stats.PopDuePlayerStatsPush(math.MaxInt64, statsBuffer[:0])
									visualBuffer = fixture.visual.Drain(0, visualBuffer[:0])
								}
							}
							b.ReportMetric(float64(len(equipment.items)), "entries/op")
						})
					}
				})
			}
			fixture.equip()
			for _, scenario := range []struct {
				name   string
				health components.EntityHealth
				draw   float64
				err    error
				moving bool
			}{
				{name: "KO_fresh", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: 10, moving: true},
				{name: "active_KO_fresh", health: components.EntityHealth{SHP: 5, HHP: 50, KOUntilUnixMs: 61000, IsLying: true}, draw: 10},
				{name: "invalid_draw", health: components.EntityHealth{SHP: 10, HHP: 50}, draw: math.NaN(), err: combat.ErrInvalidInput},
				{name: "invalid_health", health: components.EntityHealth{SHP: math.NaN(), HHP: 50}, draw: 1, err: ErrInvalidCreatureHealth},
				{name: "dead_unavailable", health: components.EntityHealth{}, draw: 1, err: ErrCreatureTargetDead},
			} {
				b.Run(scenario.name, func(b *testing.B) {
					statsBuffer := make([]types.EntityID, 0, 1)
					visualBuffer := make([]types.Handle, 0, 1)
					statsBuffer = fixture.stats.PopDuePlayerStatsPush(math.MaxInt64, statsBuffer)
					visualBuffer = fixture.visual.Drain(0, visualBuffer)
					if scenario.moving {
						ecs.AddComponent(world, fixture.owner, components.Movement{})
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						health.Set(fixture.owner, scenario.health)
						if scenario.moving {
							movement.Set(fixture.owner, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, VelocityX: 3})
						}
						creatureDamageBenchmarkResult, creatureDamageBenchmarkError = fixture.service.Apply(fixture.owner, scenario.draw)
						if creatureDamageBenchmarkError != scenario.err {
							b.Fatal(creatureDamageBenchmarkError)
						}
						statsBuffer = fixture.stats.PopDuePlayerStatsPush(math.MaxInt64, statsBuffer[:0])
						visualBuffer = fixture.visual.Drain(0, visualBuffer[:0])
					}
				})
			}
		})
	}
}
