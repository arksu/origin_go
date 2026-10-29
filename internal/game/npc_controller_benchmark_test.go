package game

import (
	"fmt"
	"testing"
	"time"
	"unsafe"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

// This includes the real shard lock, chunk manager, every production system,
// collision resolution, and sound/event preparation. There are no player
// connections; it is a simulation benchmark, not a network capacity claim.
func BenchmarkNPCProductionShardTick(b *testing.B) {
	for _, population := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("population_%d", population), func(b *testing.B) {
			shard, def, registry := newNPCProductionShard(b, population+16)
			chunk := shard.chunkManager.GetChunkFast(types.ChunkCoord{})
			// Model an observer keeping this chunk active. The animals themselves
			// never register AOI or change their normal unload policy.
			shard.chunkManager.RegisterEntity(types.EntityID(population+1), 768, 768, false)
			ts := ecs.TimeState{Tick: 1, TickRate: 10, TickPeriod: 100 * time.Millisecond, Delta: .1, Now: time.Unix(100, 0), UnixMs: 100000}
			*ecs.GetResource[ecs.TimeState](shard.world) = ts
			for i := 0; i < population; i++ {
				x, y := 150+float64(i%100)*12, 150+float64(i/100)*12
				h := gameworld.SpawnEntityFromDef(shard.world, def, gameworld.DefSpawnParams{
					EntityID: types.EntityID(i + 1), X: x, Y: y, Region: 1,
					InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: registry,
				})
				require.NotEqual(b, types.InvalidHandle, h)
				ecs.AddComponent(shard.world, h, components.ChunkRef{})
				chunk.Spatial().AddDynamic(h, int(x), int(y))
			}
			step := func() {
				shard.Update(ts)
				ts.Tick++
				ts.Now = ts.Now.Add(ts.TickPeriod)
				ts.UnixMs = ts.Now.UnixMilli()
			}
			// Include complete rest/walk cycles before measuring; a freshly
			// spawned population would only benchmark idle movement skips.
			for range 250 {
				step()
			}
			require.Equal(b, types.ChunkStateActive, chunk.GetState())
			require.Equal(b, population, chunk.Spatial().DynamicCount(), "warmup must not unload the measured population")
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				step()
			}
			b.StopTimer()
			require.Equal(b, population, chunk.Spatial().DynamicCount(), "the complete measured population must remain active")
			b.ReportMetric(float64(population), "NPCs/op")
			b.ReportMetric(float64(unsafe.Sizeof(components.NPCBrain{})), "brain_bytes/NPC")
		})
	}
}

// Exercise all real shard systems with 1,000 active combat pairs and another
// 9,000 perceiving/wandering animals. Separate chunks keep this a representative
// population rather than a collider pile that fails every bounded query.
func newNPCMixedDefensePopulation(t testing.TB) (*Shard, []types.Handle, ecs.TimeState) {
	t.Helper()
	const population, defenders = 10000, 1000
	shard, def, registry := newNPCProductionShardSize(t, population+defenders+16, 2)
	ts := ecs.TimeState{Tick: 1, TickRate: 10, TickPeriod: 100 * time.Millisecond, Delta: .1, Now: time.Unix(100, 0), UnixMs: 100000}
	*ecs.GetResource[ecs.TimeState](shard.world) = ts
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			shard.chunkManager.RegisterEntity(types.EntityID(20000+y*2+x), x*constt.ChunkWorldSize+constt.ChunkWorldSize/2, y*constt.ChunkWorldSize+constt.ChunkWorldSize/2, false)
		}
	}
	controller, ok := behaviors.GetNPCController(shard.world)
	require.True(t, ok)
	foxes := make([]types.Handle, defenders)
	for i := 0; i < population; i++ {
		var x, y float64
		if i < defenders {
			x, y = 100+float64(i%32)*40, 100+float64(i/32)*40
		} else {
			local, chunkIndex := (i-defenders)%3000, 1+(i-defenders)/3000
			x = float64((chunkIndex%2)*constt.ChunkWorldSize) + 150 + float64(local%100)*12
			y = float64((chunkIndex/2)*constt.ChunkWorldSize) + 150 + float64(local/100)*12
		}
		fox := spawnProductionFox(t, shard, def, registry, types.EntityID(i+1), x, y)
		if i < defenders {
			foxes[i] = fox
			playerID := types.EntityID(population + i + 1)
			player := spawnProductionCombatPlayer(t, shard, playerID, x+10, y, 50000)
			require.True(t, controller.NotifyDamage(contracts.EntityRef{Handle: fox, EntityID: types.EntityID(i + 1)}, contracts.EntityRef{Handle: player, EntityID: playerID}))
		}
	}
	return shard, foxes, ts
}

func BenchmarkNPCProductionMixedDefense(b *testing.B) {
	const population, defenders = 10000, 1000
	shard, foxes, ts := newNPCMixedDefensePopulation(b)
	step := func() {
		shard.Update(ts)
		ts.Tick++
		ts.Now = ts.Now.Add(ts.TickPeriod)
		ts.UnixMs = ts.Now.UnixMilli()
	}
	for range 250 {
		step()
	}
	assertPopulation := func() {
		count := 0
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				chunk := shard.chunkManager.GetChunkFast(types.ChunkCoord{X: x, Y: y})
				require.Equal(b, types.ChunkStateActive, chunk.GetState())
				count += chunk.Spatial().DynamicCount()
			}
		}
		require.Equal(b, population+defenders, count)
		for _, fox := range foxes {
			brain, exists := ecs.GetComponent[components.NPCBrain](shard.world, fox)
			require.True(b, exists)
			require.Equal(b, components.NPCDefend, brain.Activity, "warmup must retain the full defending population")
		}
	}
	assertPopulation()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		step()
	}
	b.StopTimer()
	assertPopulation()
	b.ReportMetric(population, "NPCs/op")
	b.ReportMetric(defenders, "defenders/op")
	b.ReportMetric(float64(unsafe.Sizeof(components.NPCBrain{})), "brain_bytes/NPC")
}

func TestNPCProductionDecisionLatencyWithTenThousandAnimals(t *testing.T) {
	shard, foxes, ts := newNPCMixedDefensePopulation(t)
	quiet := shard.world.GetHandleByEntityID(1001)
	position, _ := ecs.GetComponent[components.Transform](shard.world, quiet)
	spawnProductionCombatPlayer(t, shard, 11001, position.X+30, position.Y, 50)
	maxReactionTick := uint64(0)
	seen := make([]bool, len(foxes))
	for range 12 {
		shard.Update(ts)
		for i, fox := range foxes {
			brain, _ := ecs.GetComponent[components.NPCBrain](shard.world, fox)
			if !seen[i] && brain.Cautious.DamageRevision == brain.DamageRevision {
				seen[i] = true
				maxReactionTick = ts.Tick
			}
		}
		ts.Tick++
		ts.Now = ts.Now.Add(ts.TickPeriod)
		ts.UnixMs = ts.Now.UnixMilli()
	}
	for _, reacted := range seen {
		require.True(t, reacted)
	}
	require.LessOrEqual(t, maxReactionTick, uint64(2), "a burst of 1,000 aggression signals must be processed within two ticks")
	brain, _ := ecs.GetComponent[components.NPCBrain](shard.world, quiet)
	require.Equal(t, components.NPCWatch, brain.Activity, "idle perception must notice the new nearby player within its staggered one-second round")
	t.Logf("1,000 aggression reactions completed by tick %d; quiet fox observed player within 12 ticks", maxReactionTick)
}
