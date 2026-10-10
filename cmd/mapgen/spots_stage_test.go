package main

import (
	"context"
	"database/sql"
	_const "origin/internal/const"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func spotStageGenerator(db *persistence.Postgres, options MapgenOptions) *MapGenerator {
	perlin := NewPerlinNoise(options.Seed)
	return &MapGenerator{
		db: db, logger: zap.NewNop(), chunkSize: _const.ChunkSize,
		coordPerTile: _const.CoordPerTile, region: 7, seed: options.Seed,
		options: options, perlin: perlin, objectDefs: objectdefs.NewRegistry(nil),
		noiseFields: NewNoiseFieldsWithTerrainScale(perlin, _const.CoordPerTile, options.TerrainScale),
	}
}

func spotStageOptions(t *testing.T) MapgenOptions {
	t.Helper()
	options := DefaultMapgenOptions()
	options.ChunksX, options.ChunksY, options.Threads, options.Seed = 1, 1, 1, 42
	options.River.Enabled, options.Biome.Enabled, options.PerlinWaterEnabled = false, false, false
	options.PNG.Export, options.PNG.OverviewOnly = false, false
	options.PNG.OutputDir = t.TempDir()
	return options
}

func TestMapgenSpotsPreflightBeforeDatabaseAccess(t *testing.T) {
	for _, failure := range []string{"missing config", "invalid config", "invalid spawn chance", "coordinate overflow"} {
		t.Run(failure, func(t *testing.T) {
			options := spotStageOptions(t)
			switch failure {
			case "missing config":
				options.SpotsConfigPath = filepath.Join(t.TempDir(), "missing.yaml")
			case "invalid config":
				options.SpotsConfigPath = filepath.Join(t.TempDir(), "invalid.yaml")
				require.NoError(t, os.WriteFile(options.SpotsConfigPath, []byte("version: 2\n"), 0o600))
			case "invalid spawn chance":
				config := validSpotsConfig()
				chance := 1.1
				config.Spots[0].SpawnChance = &chance
				content, err := yaml.Marshal(config)
				require.NoError(t, err)
				options.SpotsConfigPath = filepath.Join(t.TempDir(), "invalid.yaml")
				require.NoError(t, os.WriteFile(options.SpotsConfigPath, content, 0o600))
			case "coordinate overflow":
				// A huge dimension must fail before either terrain allocation or DB access.
				options.ChunksX = int(^uint32(0)>>1)/_const.ChunkWorldSize + 1
			}
			generator := spotStageGenerator(nil, options)
			require.Error(t, generator.Generate(context.Background()))
			require.Nil(t, generator.terrain)
		})
	}
}

func TestMapgenOverviewSkipsAllSpotPersistence(t *testing.T) {
	options := spotStageOptions(t)
	options.PNG.Export, options.PNG.OverviewOnly = true, true
	options.SpotsConfigPath = filepath.Join(t.TempDir(), "missing.yaml")
	generator := spotStageGenerator(nil, options)
	require.NoError(t, generator.Generate(context.Background()))
	_, err := os.Stat(filepath.Join(options.PNG.OutputDir, "overview.png"))
	require.NoError(t, err)
}

func TestMapgenSpotsStagePostgres(t *testing.T) {
	db := testutil.NewPostgres(t, "ORIGIN_SPOTS_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	ctx := context.Background()
	options := spotStageOptions(t)
	spotConfig := validSpotsConfig()
	for index := range spotConfig.Spots {
		spotConfig.Spots[index].CenterTiles = []int{int(tileGrass)}
	}
	content, err := yaml.Marshal(spotConfig)
	require.NoError(t, err)
	options.SpotsConfigPath = filepath.Join(t.TempDir(), "spots.yaml")
	require.NoError(t, os.WriteFile(options.SpotsConfigPath, content, 0o600))

	// Existing data must survive bad runtime preflight, then be removed by reset.
	_, err = db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{
		Region: 7, X: 99, Y: 99, TilesData: []byte{tileGrass},
	})
	require.NoError(t, err)
	_, err = db.Pool().Exec(ctx, `INSERT INTO spot
		(region, layer, spot_type, district_x, district_y, center_x, center_y, radius, peak_quality, last_runtime_seconds)
		VALUES (7, 0, 'www', 0, 0, 0, 0, 1, 20, 0)`)
	require.NoError(t, err)
	var oldID int64
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT id FROM spot").Scan(&oldID))
	for _, invalid := range []sql.NullInt64{{}, {Int64: -1, Valid: true}} {
		require.NoError(t, db.Queries().UpsertGlobalVarLong(ctx, repository.UpsertGlobalVarLongParams{
			Name: _const.SERVER_RUNTIME_SECONDS_TOTAL, ValueLong: invalid,
		}))
		generator := spotStageGenerator(db, options)
		require.Error(t, generator.Generate(ctx))
		require.Nil(t, generator.terrain)
		var count int
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM chunk WHERE x = 99").Scan(&count))
		require.Equal(t, 1, count)
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot WHERE id = $1", oldID).Scan(&count))
		require.Equal(t, 1, count)
	}
	require.NoError(t, db.SetGlobalVarLong(ctx, _const.SERVER_RUNTIME_SECONDS_TOTAL, 321))
	var previousID = oldID
	for _, workers := range []int{1, 3} {
		options.Threads = workers
		generator := spotStageGenerator(db, options)
		require.NoError(t, generator.Generate(ctx))
		var count int
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM chunk WHERE region = 7").Scan(&count))
		require.Equal(t, 1, count)
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot WHERE region = 7").Scan(&count))
		require.Equal(t, spotTypeCount, count)
		var minimumID int64
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT min(id) FROM spot").Scan(&minimumID))
		require.Greater(t, minimumID, previousID)
		previousID = minimumID
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot WHERE last_runtime_seconds = 321 AND state = '{}'::jsonb AND revision = 1").Scan(&count))
		require.Equal(t, spotTypeCount, count)
		// The stage only completes after the entity-ID checkpoint is durable.
		_, err := db.Queries().GetGlobalVar(ctx, "last_used_id")
		require.NoError(t, err)
	}
}
