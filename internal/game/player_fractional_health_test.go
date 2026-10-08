package game

import (
	"context"
	"math"
	"testing"

	"origin/internal/characterattrs"
	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInitialHealthPreservesFractionsAndClampsAfterValidation(t *testing.T) {
	for _, test := range []struct {
		name             string
		shp, hhp         float64
		con              int
		wantSHP, wantHHP float64
	}{
		{"fractions", 21.4, 24.28, 1, 21.4, 24.28},
		{"positive_small_hhp", .25, .49, 1, .25, .49},
		{"combat_fraction", 81.0 / 13, 24.28, 1, 81.0 / 13, 24.28},
		{"zero", 0, 0, 1, 0, 0},
		{"hhp_above_mhp", 21.4, 75, 4, 21.4, 50},
		{"shp_above_hhp", 45, 24.28, 4, 24.28, 24.28},
		{"above_int32", float64(math.MaxInt32) + .25, float64(math.MaxInt32) + .75, 10000000000000000, float64(math.MaxInt32) + .25, float64(math.MaxInt32) + .75},
	} {
		t.Run(test.name, func(t *testing.T) {
			attrs := characterattrs.Default()
			attrs[characterattrs.CON] = test.con
			health, err := buildInitialEntityHealth(test.shp, test.hhp, attrs, 1)
			require.NoError(t, err)
			require.Equal(t, test.wantSHP, health.SHP)
			require.Equal(t, test.wantHHP, health.HHP)
		})
	}
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, invalidSHP := range []bool{false, true} {
			shp, hhp := 21.4, value
			if invalidSHP {
				shp, hhp = value, 24.28
			}
			health, err := buildInitialEntityHealth(shp, hhp, characterattrs.Default(), 1)
			require.ErrorIs(t, err, entityhealth.ErrInvalidPools)
			require.Zero(t, health)
		}
	}
}

func TestLoginHealthValidatesChosenSourceWithoutLosingCache(t *testing.T) {
	world := ecs.NewWorldForTesting()
	shard := &Shard{world: world}
	game := &Game{cfg: &config.Config{}, shardManager: &ShardManager{shards: map[int]*Shard{0: shard}}}
	character := repository.Character{ID: 1, Shp: math.NaN(), Hhp: math.Inf(1)}
	cached := components.EntityHealth{SHP: 21.4, HHP: 24.28, IsLying: true, LyingRevision: 7}
	shard.offlineHealth.Store(types.EntityID(1), playerRuntimeState{Health: cached})
	require.Equal(t, cached, mustResolveLoginHealth(t, game, world, character, characterattrs.Default(), nil))
	runtime := components.EntityHealth{SHP: .25, HHP: .49}
	require.Equal(t, runtime, mustResolveLoginHealth(t, game, world, character, characterattrs.Default(), []components.EntityHealth{runtime}))
	invalid := components.EntityHealth{SHP: 1, HHP: -1}
	_, err := game.resolveLoginHealth(world, character, characterattrs.Default(), []components.EntityHealth{invalid})
	require.ErrorIs(t, err, entityhealth.ErrInvalidPools)
	value, exists := shard.offlineHealth.Load(types.EntityID(1))
	require.True(t, exists)
	require.Equal(t, playerRuntimeState{Health: cached}, value)
	shard.offlineHealth.Store(types.EntityID(1), playerRuntimeState{Health: invalid})
	character.Shp, character.Hhp = 21.4, 24.28
	_, err = game.resolveLoginHealth(world, character, characterattrs.Default(), nil)
	require.ErrorIs(t, err, entityhealth.ErrInvalidPools, "invalid cache must not fall through to the database")
	value, exists = shard.offlineHealth.Load(types.EntityID(1))
	require.True(t, exists)
	require.Equal(t, playerRuntimeState{Health: invalid}, value)
	shard.offlineHealth.Delete(types.EntityID(1))
	require.Equal(t, components.EntityHealth{SHP: 21.4, HHP: 24.28}, mustResolveLoginHealth(t, game, world, character, characterattrs.Default(), nil))
}

func TestInvalidLoginHealthRollsBackSpawnBeforeLoadingInventories(t *testing.T) {
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, Key: "player", Name: "Player"}}))
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	shard, entered := newPlayerSpawnTestShard(t, 16)
	character := repository.Character{ID: 10, X: 200, Y: 200, Shp: 21.4, Hhp: 24.28}
	invalid := components.EntityHealth{SHP: .49, HHP: math.NaN()}
	shard.offlineHealth.Store(types.EntityID(10), playerRuntimeState{Health: invalid})
	game := &Game{cfg: shard.cfg, logger: zap.NewNop(), shardManager: &ShardManager{shards: map[int]*Shard{0: shard}}}
	require.NoError(t, shard.PrepareEntityAOI(t.Context(), 10, 200, 200))
	setup := game.buildPlayerSetupFunc(context.Background(), character, spawnPos{X: 200, Y: 200}, characterattrs.Default(), components.CharacterExperience{}, nil, nil)
	// There is no DB in this fixture. The invalid chosen cache must reject before
	// inventory loading, roll back the handle, and leave the cache retryable.
	player, err := shard.trySpawnPlayerWithPolicy(200, 200, character, setup, SpawnCollisionPolicy{})
	require.ErrorIs(t, err, entityhealth.ErrInvalidPools)
	require.Equal(t, types.InvalidHandle, player)
	require.Equal(t, types.InvalidHandle, shard.world.GetHandleByEntityID(10))
	require.NotContains(t, ecs.GetResource[ecs.CharacterEntities](shard.world).Map, types.EntityID(10))
	value, exists := shard.offlineHealth.Load(types.EntityID(10))
	require.True(t, exists)
	require.Equal(t, math.Float64bits(invalid.HHP), math.Float64bits(value.(playerRuntimeState).Health.HHP))
	flushPlayerSpawnEvents(t, shard.eventBus)
	require.Empty(t, entered)
	require.Zero(t, shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().DynamicCount())
}
