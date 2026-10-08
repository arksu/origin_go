package game

import (
	"fmt"
	"math"
	"testing"
	"unsafe"

	"go.uber.org/zap"
	"origin/internal/config"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

// Preparation is isolated from costs, lifecycle callbacks and transport. Each
// iteration cancels its own plan so the benchmark never consumes target health.
func BenchmarkMeleeExecution(b *testing.B) {
	for _, population := range []int{1000, 100000} {
		b.Run(fmt.Sprintf("world_%d", population), func(b *testing.B) {
			for _, scenario := range []struct {
				name, action              string
				targets                   int
				fullArmor, fatal, invalid bool
			}{
				{name: "miss", action: "axe_sweep"},
				{name: "nearest", action: "axe_strike", targets: 10},
				{name: "sweep_1", action: "axe_sweep", targets: 1},
				{name: "sweep_10", action: "axe_sweep", targets: 10},
				{name: "sweep_512", action: "axe_sweep", targets: 512},
				{name: "full_armor", action: "axe_sweep", targets: 10, fullArmor: true},
				{name: "invalid_health", action: "axe_sweep", targets: 10, invalid: true},
				{name: "admission_cancel", action: "axe_sweep", targets: 10, fatal: true},
			} {
				b.Run(scenario.name, func(b *testing.B) {
					f := meleeFixtureInWorld(b, ecs.NewWorldWithCapacity(uint32(population+MeleeMaxHits*2+3), nil, 0))
					f.execution.sender = nil
					for i := 0; i < scenario.targets; i++ {
						id := types.EntityID(100 + i)
						if scenario.fatal {
							f.object(b, id, 60, 50, 1)
							continue
						}
						target := f.creature(b, id, 60, 50, scenario.fullArmor)
						if scenario.fullArmor {
							container := f.world.GetHandleByEntityID(id + 10000)
							items := make([]components.InvItem, len(combatEquipmentSlots))
							for slot, equipSlot := range combatEquipmentSlots {
								items[slot] = combatTestItem(id*100+types.EntityID(slot), 9, 10, netproto.EquipSlot(equipSlot))
							}
							ecs.WithComponent(f.world, container, func(c *components.InventoryContainer) { c.Items = items })
						}
						if scenario.invalid && i == scenario.targets-1 {
							ecs.WithComponent(f.world, target, func(h *components.EntityHealth) { h.HHP = math.NaN() })
						}
					}
					for i := 0; i < population; i++ {
						x, y := 10000+float64(i%1000)*32, 10000+float64(i/1000)*32
						spawnSectorCollider(f.world, types.EntityID(1000000+i), x, y, 1, 1)
					}
					definition, _ := f.definitions.Get(scenario.action)
					prepared, err := f.catalog.PrepareMeleeAction(definition)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						err = f.execution.prepare(f.owner, 1, definition, prepared, 0)
						f.execution.abort()
						if (err != nil) != scenario.invalid {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					spatial := ecs.GetResource[*core.WorldColliderSpatial](f.world)
					stats := (*spatial).Index.LastQueryStats()
					b.ReportMetric(float64(stats.Cells), "cells/op")
					b.ReportMetric(float64(stats.Visits), "visits/op")
					b.ReportMetric(float64(stats.Candidates), "candidates/op")
					// Includes fixed execution structs, contact backing storage and
					// bounded spatial candidates, excluding shared definitions/ECS.
					bytes := unsafe.Sizeof(*f.execution) + uintptr(cap(f.execution.contacts))*unsafe.Sizeof(SectorHit{}) +
						unsafe.Sizeof(f.execution.sectors.boundedCandidates)
					b.ReportMetric(float64(bytes), "scratch-B/shard")
				})
			}
		})
	}
}

type meleeBenchmarkSender struct{}

func (meleeBenchmarkSender) SendActionStateChanged(types.EntityID, *netproto.S2C_ActionStateChanged) {
}
func (meleeBenchmarkSender) SendActionList(types.EntityID, *netproto.S2C_ActionList) {}
func (meleeBenchmarkSender) SendMiniAlert(types.EntityID, *netproto.S2C_MiniAlert)   {}
func (meleeBenchmarkSender) SendCyclicActionFinished(types.EntityID, *netproto.S2C_CyclicActionFinished) {
}

// This separately includes ActionService's cost, cooldown and completion state
// construction. No socket or unbounded recorder is part of this measurement.
func BenchmarkMeleeActionCompletion(b *testing.B) {
	f := meleeFixtureInWorld(b, ecs.NewWorldWithCapacity(16, nil, 0))
	f.actions.sender = meleeBenchmarkSender{}
	f.execution.sender = nil
	f.creature(b, 3, 60, 50, false)
	definition, _ := f.definitions.Get("axe_sweep")
	handler := f.actions.handlers[definition.ID].(*MeleeActionHandler)
	active := components.ActiveGameAction{ActionID: definition.ID, Generation: 1, Phase: components.GameActionExecuting, AimAngle: 0, DirectAttempt: true}
	health := ecs.GetOrCreateStorage[components.EntityHealth](f.world)
	stats := ecs.GetOrCreateStorage[components.EntityStats](f.world)
	cooldowns := ecs.GetOrCreateStorage[components.ActionCooldowns](f.world)
	f.actions.chargeActionCosts(f.world, f.owner, definition)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		health.Set(f.world.GetHandleByEntityID(3), components.EntityHealth{SHP: 25, HHP: 25})
		stats.Set(f.owner, components.EntityStats{Stamina: 1000, Energy: 900})
		ecs.AddComponent(f.world, f.owner, active)
		cooldown, _ := cooldowns.Get(f.owner)
		clear(cooldown.ByAction)
		f.actions.executePreparedCompletion(f.world, 1, f.owner, definition, active, ActionTarget{AimAngle: 0}, handler)
	}
}

// Measure the existing shard quarantine independently from reservation and hit
// planning. This case has no interested players or active destination chunk;
// UI/carry and visibility fanout correctness is covered by PostgreSQL tests.
func BenchmarkMeleeObjectQuarantine(b *testing.B) {
	for _, population := range []int{1000, 100000} {
		b.Run(fmt.Sprint(population), func(b *testing.B) {
			f := meleeFixtureInWorld(b, ecs.NewWorldWithCapacity(uint32(population+4), nil, 0))
			target := f.object(b, 3, 60, 50, 1)
			for i := 0; i < population; i++ {
				spawnSectorCollider(f.world, types.EntityID(1000000+i), 10000+float64(i%1000)*32, 10000+float64(i/1000)*32, 1, 1)
			}
			cfg := &config.Config{Game: config.GameConfig{ChunkLRUCapacity: 4, WorldWidthChunks: 1, WorldHeightChunks: 1}}
			shard := &Shard{world: f.world, logger: zap.NewNop()}
			shard.chunkManager = gameworld.NewChunkManager(cfg, nil, f.world, shard, 0, 1, nil, nil, nil, shard.logger)
			b.Cleanup(shard.chunkManager.Stop)
			f.objects.state.Pending[target] = true
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ecs.AddComponent(f.world, target, components.Collider{HalfWidth: 1, HalfHeight: 1})
				shard.quarantineDestroyedObject(target)
			}
		})
	}
}
