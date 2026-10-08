package game

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// All population setup is excluded from measurement. Rotation through registered
// players includes the registry/cache footprint of the selected population.
func logoutBenchmarkPopulation(b *testing.B, population int, pending bool) (*logoutFixture, []ecs.LogoutContext) {
	b.Helper()
	world := ecs.NewWorldWithCapacity(uint32(population+1), nil, 0)
	fixture := &logoutFixture{
		world: world, clock: ecs.GetResource[ecs.TimeState](world),
		detached: ecs.GetResource[ecs.DetachedEntities](world),
		activity: ecs.GetResource[ecs.CombatActivityState](world), other: &logoutTestPolicy{},
	}
	fixture.clock.Now, fixture.clock.UnixMs = time.Unix(100, 0), 100_000
	combat, err := NewCombatLogoutPolicy(world)
	if err != nil {
		b.Fatal(err)
	}
	fixture.service, err = NewPlayerLogoutService(world, NewDisconnectDelayLogoutPolicy(), combat, fixture.other)
	if err != nil {
		b.Fatal(err)
	}
	contexts := make([]ecs.LogoutContext, population)
	for index := range contexts {
		identity := types.EntityID(index + 1)
		handle := world.Spawn(identity, nil)
		ecs.AddComponent(world, handle, components.EntityHealth{SHP: 10, HHP: 50})
		if !fixture.activity.Prepare(handle, identity) {
			b.Fatal("combat registration rejected")
		}
		if err := fixture.service.PreparePlayer(handle); err != nil {
			b.Fatal(err)
		}
		if pending {
			fixture.detached.AddDetachedEntity(identity, handle, fixture.clock.Now, fixture.clock.Now)
		}
		detached, _ := fixture.detached.GetDetachedEntity(identity)
		contexts[index] = ecs.LogoutContext{EntityID: identity, Handle: handle, Detached: detached, Time: *fixture.clock}
	}
	fixture.handle = contexts[0].Handle
	return fixture, contexts
}

func BenchmarkPlayerLogoutPolicies(b *testing.B) {
	for _, population := range []int{1000, 30000} {
		b.Run(fmt.Sprintf("players_%d", population), func(b *testing.B) {
			for _, scenario := range []string{"allow", "combat", "KO", "unknown", "error"} {
				b.Run(scenario, func(b *testing.B) {
					fixture, contexts := logoutBenchmarkPopulation(b, population, true)
					for _, context := range contexts {
						if scenario == "combat" {
							fixture.activity.RecordPreparedEvent(context.Handle, fixture.clock.UnixMs)
						}
						if scenario == "KO" {
							ecs.WithComponent(fixture.world, context.Handle, func(health *components.EntityHealth) {
								health.KOUntilUnixMs = fixture.clock.UnixMs + 60_000
							})
						}
					}
					fixture.other.decision.Blocked = scenario == "unknown"
					if scenario == "error" {
						fixture.other.err = ErrInvalidPlayerLogoutHealth
					}
					b.ReportAllocs()
					b.ResetTimer()
					for index := 0; index < b.N; index++ {
						decision, err := fixture.service.Check(contexts[index%population])
						if scenario == "error" {
							if err != ErrInvalidPlayerLogoutHealth || !decision.Blocked {
								b.Fatalf("expected fail-closed health error, got decision=%+v error=%v", decision, err)
							}
						} else if err != nil || decision.Blocked != (scenario != "allow") {
							b.Fatalf("unexpected policy decision=%+v error=%v", decision, err)
						}
					}
				})
			}
		})
	}
}

// Validation, marking and wakeup are measured together, without health writes,
// costs, serialization, cleanup or transport.
func BenchmarkCombatActivityAndLogoutWake(b *testing.B) {
	for _, population := range []int{1000, 30000} {
		b.Run(fmt.Sprintf("players_%d", population), func(b *testing.B) {
			for _, scenario := range []string{"connected_fresh", "detached_fresh", "detached_equal_time"} {
				b.Run(scenario, func(b *testing.B) {
					fixture, contexts := logoutBenchmarkPopulation(b, population, scenario != "connected_fresh")
					b.ReportAllocs()
					b.ResetTimer()
					for index := 0; index < b.N; index++ {
						context := contexts[index%population]
						now := int64(index)
						if scenario == "detached_equal_time" {
							now = 100_000
						}
						if err := fixture.activity.ValidateEvent(context.Handle, context.EntityID, now); err != nil {
							b.Fatal(err)
						}
						fixture.activity.RecordPreparedEvent(context.Handle, now)
						fixture.service.RequestRecheck(context.Handle)
					}
				})
			}
		})
	}
}

// This isolates the two runtime registries from World, component storage and
// player inventories. Timed work is population preparation; GC is excluded and
// the retained metric includes their prepared maps, records and reserved heap.
func BenchmarkCombatLogoutRegistryMemory(b *testing.B) {
	for _, population := range []int{1000, 30000} {
		b.Run(fmt.Sprintf("players_%d", population), func(b *testing.B) {
			b.ReportAllocs()
			var retainedTotal float64
			for run := 0; run < b.N; run++ {
				b.StopTimer()
				runtime.GC()
				var before runtime.MemStats
				runtime.ReadMemStats(&before)
				b.StartTimer()
				activity := ecs.NewCombatActivityState()
				detached := &ecs.DetachedEntities{}
				for index := 0; index < population; index++ {
					identity := types.EntityID(index + 1)
					handle := types.MakeHandle(uint32(index+1), 1)
					if !activity.Prepare(handle, identity) {
						b.Fatal("combat registration rejected")
					}
					if err := detached.PreparePlayer(identity, handle); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				runtime.GC()
				var after runtime.MemStats
				runtime.ReadMemStats(&after)
				runtime.KeepAlive(activity)
				runtime.KeepAlive(detached)
				retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
				if retained < 0 {
					b.Fatal("unstable retained heap measurement")
				}
				retainedTotal += float64(retained)
				b.StartTimer()
			}
			b.ReportMetric(retainedTotal/float64(b.N*population), "retained-B/player")
		})
	}
}
