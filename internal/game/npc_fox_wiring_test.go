package game

import (
	"context"
	"math"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	"origin/internal/sounddefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNPCFoxWandersWithProductionShardWiring(t *testing.T) {
	def, registry := loadNPCFoxCatalog(t)
	logger := zap.NewNop()
	previousActions, previousSounds := actiondefs.Global(), sounddefs.Global()
	t.Cleanup(func() {
		actiondefs.SetGlobalForTesting(previousActions)
		sounddefs.SetGlobalForTesting(previousSounds)
	})
	actions, err := actiondefs.LoadFromDirectory("../../data/actions", logger)
	require.NoError(t, err)
	actiondefs.SetGlobalForTesting(actions)
	sounds, err := sounddefs.LoadFromDirectory("../../data/sounds", logger)
	require.NoError(t, err)
	sounddefs.SetGlobalForTesting(sounds)
	combat, err := NewCombatDefinitions(itemdefs.Global(), actions)
	require.NoError(t, err)

	cfg := &config.Config{Game: config.GameConfig{
		MaxEntities: 64, Region: 1, TickRate: 10,
		ChunkLRUCapacity: 4, ChunkLRUTTL: 60, LoadWorkers: 1,
		WorldWidthChunks: 1, WorldHeightChunks: 1,
		BehaviorTickGlobalBudget: 200, BehaviorTickCatchupLimit: 10,
		Audio: config.DefaultAudioConfig(),
	}}
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	shard := NewShard(0, cfg, nil, &EntityIDManager{logger: logger, rangeEnd: 1000},
		gameworld.NewObjectFactory(nil), nil, bus, false, logger, combat, &AttackEventSequence{})
	t.Cleanup(func() {
		shard.Stop()
		require.NoError(t, bus.Shutdown(context.Background()))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, shard.chunkManager.WaitPreloaded(ctx, types.ChunkCoord{}))
	chunk := shard.chunkManager.GetChunk(types.ChunkCoord{})
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for i := range tiles {
		tiles[i] = types.TileGrass
	}
	require.NoError(t, chunk.RestoreTiles(tiles, 0, 0))
	chunk.SetState(types.ChunkStateActive)

	ts := ecs.TimeState{Tick: 1, TickRate: 10, TickPeriod: 100 * time.Millisecond, Delta: .1, Now: time.Unix(100, 0), UnixMs: 100000}
	*ecs.GetResource[ecs.TimeState](shard.world) = ts
	fox := gameworld.SpawnEntityFromDef(shard.world, def, gameworld.DefSpawnParams{
		EntityID: 100, X: 200, Y: 200, Region: 1,
		InitReason: contracts.ObjectBehaviorInitReasonSpawn, BehaviorRegistry: registry,
	})
	require.NotEqual(t, types.InvalidHandle, fox)
	ecs.AddComponent(shard.world, fox, components.ChunkRef{})
	chunk.Spatial().AddDynamic(fox, 200, 200)
	brain, ok := ecs.GetComponent[components.NPCBrain](shard.world, fox)
	require.True(t, ok)
	require.Equal(t, components.NPCIdle, brain.Activity)
	require.Greater(t, brain.DeadlineTick, ts.Tick)

	// Exercise the systems and dependencies installed by NewShard: hand-built
	// behavior systems can accidentally supply dependencies missing at startup.
	for ts.Tick <= brain.DeadlineTick {
		shard.Update(ts)
		ts.Tick++
		ts.Now = ts.Now.Add(ts.TickPeriod)
		ts.UnixMs = ts.Now.UnixMilli()
	}
	movement, ok := ecs.GetComponent[components.Movement](shard.world, fox)
	require.True(t, ok)
	require.Equal(t, constt.StateMoving, movement.State, "fox must begin wandering when its initial rest ends")
	shard.Update(ts)
	position, ok := ecs.GetComponent[components.Transform](shard.world, fox)
	require.True(t, ok)
	require.Greater(t, math.Hypot(position.X-200, position.Y-200), 0.0, "production movement systems must advance the fox")
}
