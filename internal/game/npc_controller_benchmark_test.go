package game

import (
	"fmt"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
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
		})
	}
}
