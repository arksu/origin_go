package game

import (
	"fmt"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

var (
	combatBenchmarkArmor  float64
	combatBenchmarkWeapon EquippedMelee
)

func BenchmarkCombatEquipment(b *testing.B) {
	for _, population := range []int{1000, 100000} {
		b.Run(fmt.Sprintf("world_%d", population), func(b *testing.B) {
			world := ecs.NewWorldWithCapacity(uint32(population+2), nil, 0)
			fixture := combatFixtureInWorld(b, world, combatTestDefinitions())
			for index := 0; index < population; index++ {
				world.Spawn(types.EntityID(index+2), nil)
			}
			full := make([]components.InvItem, 0, len(combatEquipmentSlots))
			for index, slot := range combatEquipmentSlots {
				full = append(full, combatTestItem(types.EntityID(index+100), 9, 10, slot))
			}
			for _, scenario := range []struct {
				name  string
				items []components.InvItem
			}{
				{"empty", nil},
				{"partial", []components.InvItem{combatTestItem(100, 6, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND)}},
				{"full", full},
				{"two_weapons", []components.InvItem{
					combatTestItem(100, 1, 10, netproto.EquipSlot_EQUIP_SLOT_RIGHT_HAND),
					combatTestItem(101, 2, 160, netproto.EquipSlot_EQUIP_SLOT_LEFT_HAND),
				}},
			} {
				b.Run(scenario.name, func(b *testing.B) {
					fixture.equip(scenario.items...)
					b.Run("armor", func(b *testing.B) {
						b.ReportAllocs()
						b.ResetTimer()
						for iteration := 0; iteration < b.N; iteration++ {
							var err error
							combatBenchmarkArmor, err = fixture.resolver.ResolveArmor(fixture.owner)
							if err != nil {
								b.Fatal(err)
							}
						}
						b.ReportMetric(float64(len(scenario.items)), "entries/op")
					})
					b.Run("weapon", func(b *testing.B) {
						var expectedError error
						if len(scenario.items) == 0 {
							expectedError = ErrMeleeWeaponUnavailable
						}
						b.ReportAllocs()
						b.ResetTimer()
						for iteration := 0; iteration < b.N; iteration++ {
							var err error
							combatBenchmarkWeapon, err = fixture.resolver.ResolveMeleeWeapon(fixture.owner, fixture.action, 2.25)
							if err != expectedError {
								b.Fatal(err)
							}
						}
						b.ReportMetric(float64(len(scenario.items)), "entries/op")
					})
				})
			}
		})
	}
}
