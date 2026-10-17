package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_const "origin/internal/const"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var resetWorldTables = []string{"character_hearth", "inventory", "build_site", "skill", "chat", "spot", "object", "chunk", "global_var"}

func seedMapgenResetWorld(t *testing.T) *persistence.Postgres {
	t.Helper()
	db := testutil.NewPostgres(t, "ORIGIN_MAPGEN_RESET_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	_, err := db.Pool().Exec(context.Background(), `
		CREATE TABLE object_region_7 PARTITION OF object FOR VALUES IN (7);
		INSERT INTO account (login, password_hash, token, status)
		VALUES ('retained-account', 'retained-hash', 'retained-token', 2);
		INSERT INTO character (id, account_id, name, region, x, y, layer, heading,
			stamina, energy, shp, hhp, attributes, exp, skills, discovery, action_cooldowns,
			auth_token, token_expires_at, online_time, is_online, is_ghost, deleted_at)
		VALUES
			(1001, 1, 'Active', 1, 123, 456, -1, 90, 500, 800, 70, 80,
			 '{"strength":25}', '{"lp":123}', '["survival"]', '["branch"]',
			 '{"hearth_return":999}', 'saved-auth', '2030-01-01', 789, true, true, NULL),
			(1002, 1, 'Deleted', 2, 321, 654, 0, 180, 100, 200, 0, 0,
			 '{}', '{}', '[]', '[]', '{}', NULL, NULL, 456, false, false, '2020-01-01');
		INSERT INTO object (id, type_id, region, x, y, layer, chunk_x, chunk_y, create_tick, last_tick, deleted_at)
		VALUES (2001, 1, 1, 123, 456, 0, 99, 99, 0, 0, NULL),
		       (2002, 1, 2, 321, 654, -1, 99, 99, 0, 0, '2020-01-01'),
		       (2003, 1, 7, 1, 2, 1, 99, 99, 0, 0, NULL);
		INSERT INTO character_hearth (character_id, object_region, object_id) VALUES (1001, 1, 2001);
		INSERT INTO inventory (owner_id, kind, inventory_key, data, version, deleted_at)
		VALUES (1001, 0, 0, '{"items":[1]}', 1, NULL),
		       (2002, 0, 0, '{"items":[2]}', 1, NULL),
		       (1002, 0, 0, '{"items":[3]}', 1, '2020-01-01');
		INSERT INTO chunk (region, x, y, layer, last_tick, tiles_data)
		VALUES (1, 99, 99, 0, 10, '\x01'), (2, 99, 99, -1, 20, '\x02'), (7, 99, 99, 1, 30, '\x03');
		INSERT INTO spot (region, layer, spot_type, district_x, district_y, center_x, center_y,
			radius, peak_quality, last_runtime_seconds)
		VALUES (1, 0, 'www', 0, 0, 0, 0, 1, 20, 300),
		       (2, 0, 'soil', 0, 0, 0, 0, 1, 20, 300),
		       (7, 0, 'clay', 0, 0, 0, 0, 1, 20, 300);
		INSERT INTO skill (character_id, skill_id, experience, level)
		VALUES (1001, 1, 42, 2), (1002, 2, 99, 3);
		INSERT INTO chat (channel, sender_id, region, x, y, layer, message)
		VALUES (0, 1001, 1, 123, 456, 0, 'Previous world'), (0, 1002, 2, 321, 654, -1, 'Previous layer');
		INSERT INTO build_site (object_id, recipe_id, builder_id, build_points_total, required_items)
		VALUES (2002, 1, 1001, 100, '[{"item_id":1,"quantity":2}]');
		INSERT INTO global_var (name, value_long, value_string)
		VALUES ('last_used_id', 9000, NULL), ('server_runtime_seconds_total', 321, NULL),
		       ('retained-setting', 456, 'preserved');
	`)
	require.NoError(t, err)
	return db
}

func mapgenTableSnapshots(t *testing.T, db *persistence.Postgres, tables ...string) map[string]string {
	t.Helper()
	snapshots := make(map[string]string, len(tables))
	for _, table := range tables {
		var snapshot string
		err := db.Pool().QueryRow(context.Background(),
			"SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text), '[]'::jsonb)::text FROM "+table+" t").Scan(&snapshot)
		require.NoError(t, err)
		snapshots[table] = snapshot
	}
	return snapshots
}

func mapgenSequenceSnapshots(t *testing.T, db *persistence.Postgres) map[string]string {
	t.Helper()
	snapshots := make(map[string]string)
	for _, sequence := range []string{"account_id_seq", "inventory_id_seq", "chat_id_seq", "build_site_id_seq", "spot_id_seq"} {
		var snapshot string
		err := db.Pool().QueryRow(context.Background(),
			"SELECT jsonb_build_array(last_value, is_called)::text FROM "+sequence).Scan(&snapshot)
		require.NoError(t, err)
		snapshots[sequence] = snapshot
	}
	return snapshots
}

func TestMapgenTruncateWorldPostgres(t *testing.T) {
	db := seedMapgenResetWorld(t)
	ctx := context.Background()
	retained := mapgenTableSnapshots(t, db, "account", "character")
	sequences := mapgenSequenceSnapshots(t, db)
	for _, table := range resetWorldTables {
		require.NotEqual(t, "[]", mapgenTableSnapshots(t, db, table)[table], "%s fixture must contain rows", table)
	}

	// The hearth references object_region_1 with ON DELETE RESTRICT. All world
	// tables must be truncated together, including every object/chat partition.
	require.NoError(t, db.Queries().TruncateWorld(ctx))
	for table, snapshot := range mapgenTableSnapshots(t, db, resetWorldTables...) {
		require.Equal(t, "[]", snapshot, "%s must be empty", table)
	}
	for table, snapshot := range mapgenTableSnapshots(t, db, "object_region_1", "object_region_2", "object_region_7", "chat_default") {
		require.Equal(t, "[]", snapshot, "%s partition must be empty", table)
	}
	require.Equal(t, retained, mapgenTableSnapshots(t, db, "account", "character"))
	require.Equal(t, sequences, mapgenSequenceSnapshots(t, db),
		"world reset must preserve sequence progress")
}

func TestMapgenTruncateWorldRollbackPostgres(t *testing.T) {
	db := seedMapgenResetWorld(t)
	tables := append([]string{"account", "character"}, resetWorldTables...)
	before := mapgenTableSnapshots(t, db, tables...)
	failure := errors.New("abort world reset")
	err := db.WithTx(context.Background(), func(queries *repository.Queries) error {
		if err := queries.TruncateWorld(context.Background()); err != nil {
			return err
		}
		return failure
	})
	require.ErrorIs(t, err, failure)
	require.Equal(t, before, mapgenTableSnapshots(t, db, tables...), "a failed reset transaction must retain the entire old world")
}

func TestMapgenGenerateResetsFullWorldPostgres(t *testing.T) {
	db := seedMapgenResetWorld(t)
	ctx := context.Background()
	retained := mapgenTableSnapshots(t, db, "account", "character")
	options := spotStageOptions(t)
	config := validSpotsConfig()
	for index := range config.Spots {
		config.Spots[index].CenterTiles = []int{int(tileGrass)}
	}
	content, err := yaml.Marshal(config)
	require.NoError(t, err)
	options.SpotsConfigPath = filepath.Join(t.TempDir(), "spots.yaml")
	require.NoError(t, os.WriteFile(options.SpotsConfigPath, content, 0o600))

	generator := spotStageGenerator(db, options)
	require.NoError(t, generator.Generate(ctx))
	require.Equal(t, retained, mapgenTableSnapshots(t, db, "account", "character"), "account and character rows must remain unchanged")
	for table, snapshot := range mapgenTableSnapshots(t, db, "character_hearth", "inventory", "build_site", "skill", "chat") {
		require.Equal(t, "[]", snapshot, "%s must not retain old-world state", table)
	}
	for _, table := range []string{"chunk", "object", "spot"} {
		var oldCount, generatedCount int
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE region <> 7 OR layer <> 0").Scan(&oldCount))
		require.Zero(t, oldCount, "%s must not retain other regions or layers", table)
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE region = 7 AND layer = 0").Scan(&generatedCount))
		require.Positive(t, generatedCount, "%s must contain new world data", table)
	}
	var oldChunks, oldObjects, oldSpots int
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM chunk WHERE x = 99 OR y = 99").Scan(&oldChunks))
	require.Zero(t, oldChunks)
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM object WHERE id <= 1002").Scan(&oldObjects))
	require.Zero(t, oldObjects, "new entity IDs must exceed every preserved character ID, including deleted characters")
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot WHERE id <= 3").Scan(&oldSpots))
	require.Zero(t, oldSpots, "spot sequence must continue after reset")
	require.Greater(t, db.GetGlobalVarLong(ctx, "last_used_id"), int64(1002))
	require.Less(t, db.GetGlobalVarLong(ctx, "last_used_id"), int64(9000), "old entity checkpoint must be discarded")
	require.Zero(t, db.GetGlobalVarLong(ctx, _const.SERVER_RUNTIME_SECONDS_TOTAL))
	var oldGlobals, oldRuntimeSpots int
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM global_var WHERE name <> 'last_used_id'").Scan(&oldGlobals))
	require.Zero(t, oldGlobals, "previous world globals must be removed")
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot WHERE last_runtime_seconds <> 0").Scan(&oldRuntimeSpots))
	require.Zero(t, oldRuntimeSpots, "new spots must start in the new world's runtime")
}
