package systems

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence"
	"origin/internal/types"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Set ORIGIN_CHARACTER_SAVE_TEST_DSN to a disposable PostgreSQL database. Each
// test owns a separate schema and removes only that schema during cleanup.
func newCharacterSavePostgres(t *testing.T) *persistence.Postgres {
	t.Helper()
	dsn := os.Getenv("ORIGIN_CHARACTER_SAVE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ORIGIN_CHARACTER_SAVE_TEST_DSN to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	connectionConfig, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	admin, err := pgx.ConnectConfig(ctx, connectionConfig)
	require.NoError(t, err)
	schema := fmt.Sprintf("character_save_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, cleanupErr := admin.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
		require.NoError(t, cleanupErr)
		require.NoError(t, admin.Close(cleanupCtx))
	})
	schemaSQL, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "schema.sql"))
	require.NoError(t, err)
	fixtureSQL := strings.Replace(string(schemaSQL), "create schema if not exists origin;", "create schema "+quotedSchema+";", 1)
	fixtureSQL = strings.Replace(fixtureSQL, "set search_path to origin;", "set search_path to "+quotedSchema+";", 1)
	_, err = admin.Exec(ctx, fixtureSQL)
	require.NoError(t, err)
	db, err := persistence.NewPostgres(ctx, &config.DatabaseConfig{
		Host: connectionConfig.Host, Port: int(connectionConfig.Port),
		User: connectionConfig.User, Password: connectionConfig.Password,
		Database: connectionConfig.Database, Schema: schema, MaxConns: 4,
	}, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(db.Close)
	_, err = db.Pool().Exec(ctx, `
		INSERT INTO account (id, login, password_hash) VALUES (1, 'save-fixture', 'unused');
		INSERT INTO character (id, account_id, name, region, x, y, layer, heading,
		    stamina, energy, shp, hhp, attributes, exp, skills, discovery)
		SELECT id, 1, 'SaveFixture' || id, 1, 0, 0, 0, 0,
		    100, 100, 100, 100, '{}', '{}', '[]', '[]'
		FROM generate_series(11, 12) id;
	`)
	require.NoError(t, err)
	return db
}

func characterSaveIntegrationSnapshot(id int64, position, version int) CharacterSnapshot {
	return CharacterSnapshot{
		CharacterID: id, X: position, Y: position + 1, Heading: 90,
		Stamina: 75, Energy: 80, SHP: 90, HHP: 100,
		Attributes: `{}`, Exp: `{}`, Skills: `[]`, Discovery: `[]`,
		Inventories: []InventorySnapshot{{
			CharacterID: id, Kind: 0, InventoryKey: 0, Version: version,
			Data: json.RawMessage(fmt.Sprintf(`{"position":%d}`, position)),
		}},
	}
}

func requireCharacterSavePostgresState(t *testing.T, db *persistence.Postgres, id int64, position, version int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var x, y, inventoryPosition, inventoryVersion int
	err := db.Pool().QueryRow(ctx, `
		SELECT c.x, c.y, (i.data->>'position')::int, i.version
		FROM character c JOIN inventory i ON i.owner_id = c.id
		WHERE c.id = $1 AND i.kind = 0 AND i.inventory_key = 0
	`, id).Scan(&x, &y, &inventoryPosition, &inventoryVersion)
	require.NoError(t, err)
	require.Equal(t, position, x)
	require.Equal(t, position+1, y)
	require.Equal(t, position, inventoryPosition)
	require.Equal(t, version, inventoryVersion)
}

func TestCharacterSaverPostgresCoalescesDuplicateAndDistinctSnapshots(t *testing.T) {
	db := newCharacterSavePostgres(t)
	saver := NewCharacterSaver(db, 0, nil, zap.NewNop())
	t.Cleanup(saver.Stop)
	older := characterSaveIntegrationSnapshot(11, 10, 1)
	older.Inventories = append(older.Inventories, InventorySnapshot{
		CharacterID: 11, Kind: 1, Data: json.RawMessage(`{}`), Version: 1,
	})
	require.True(t, saver.enqueueSnapshot(older))
	require.True(t, saver.enqueueSnapshot(characterSaveIntegrationSnapshot(12, 40, 1)))
	latest := characterSaveIntegrationSnapshot(11, 20, 3)
	// Repeated root keys can also occur inside a snapshot. Keep the highest
	// version and the last value on ties before issuing PostgreSQL's UPSERT.
	latest.Inventories = []InventorySnapshot{
		characterSaveIntegrationSnapshot(11, 99, 1).Inventories[0],
		characterSaveIntegrationSnapshot(11, 98, 3).Inventories[0],
		latest.Inventories[0],
	}
	require.True(t, saver.enqueueSnapshot(latest))
	queue := saver.queueForCharacter(11)
	require.NoError(t, saver.flushPending(context.Background(), queue, 0))
	require.Empty(t, queue.pending)
	requireCharacterSavePostgresState(t, db, 11, 20, 3)
	requireCharacterSavePostgresState(t, db, 12, 40, 1)
	var inventoryCount int
	require.NoError(t, db.Pool().QueryRow(context.Background(), `SELECT count(*) FROM inventory`).Scan(&inventoryCount))
	require.Equal(t, 2, inventoryCount, "the replaced complete snapshot must not retain an older root inventory")
}

func TestCharacterSaverPostgresInventoryFailureRollsBackAndRetries(t *testing.T) {
	db := newCharacterSavePostgres(t)
	saver := NewCharacterSaver(db, 0, nil, zap.NewNop())
	t.Cleanup(saver.Stop)
	queue := saver.queueForCharacter(11)
	require.True(t, saver.enqueueSnapshot(characterSaveIntegrationSnapshot(11, 10, 1)))
	require.True(t, saver.enqueueSnapshot(characterSaveIntegrationSnapshot(12, 30, 1)))
	require.NoError(t, saver.flushPending(context.Background(), queue, 0))
	_, err := db.Pool().Exec(context.Background(), `ALTER TABLE inventory ADD CONSTRAINT fixture_inventory_failure CHECK (version < 2)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, cleanupErr := db.Pool().Exec(ctx, `ALTER TABLE inventory DROP CONSTRAINT IF EXISTS fixture_inventory_failure`)
		require.NoError(t, cleanupErr)
	})
	require.True(t, saver.enqueueSnapshot(characterSaveIntegrationSnapshot(11, 20, 2)))
	require.True(t, saver.enqueueSnapshot(characterSaveIntegrationSnapshot(12, 40, 2)))
	err = saver.flushPending(context.Background(), queue, 0)
	var postgresError *pgconn.PgError
	require.ErrorAs(t, err, &postgresError)
	require.Equal(t, "23514", postgresError.Code)
	require.Len(t, queue.pending, 2, "failed transaction must retain every pending character")
	requireCharacterSavePostgresState(t, db, 11, 10, 1)
	requireCharacterSavePostgresState(t, db, 12, 30, 1)
	_, err = db.Pool().Exec(context.Background(), `ALTER TABLE inventory DROP CONSTRAINT fixture_inventory_failure`)
	require.NoError(t, err)
	require.NoError(t, saver.flushPending(context.Background(), queue, 0))
	require.Empty(t, queue.pending)
	requireCharacterSavePostgresState(t, db, 11, 20, 2)
	requireCharacterSavePostgresState(t, db, 12, 40, 2)
}

type characterSaveIntegrationInventorySerializer struct{}

func (characterSaveIntegrationInventorySerializer) SerializeInventories(world interface{}, id types.EntityID, handle types.Handle) []InventorySnapshot {
	transform, _ := ecs.GetComponent[components.Transform](world.(*ecs.World), handle)
	return characterSaveIntegrationSnapshot(int64(id), int(transform.X), int(transform.X)).Inventories
}

func TestCharacterSaverPostgresPeriodicThenDetachedFinalSnapshot(t *testing.T) {
	db := newCharacterSavePostgres(t)
	saver := NewCharacterSaver(db, 0, characterSaveIntegrationInventorySerializer{}, zap.NewNop())
	world := ecs.NewWorldForTesting()
	handle := world.Spawn(11, nil)
	ecs.AddComponent(world, handle, components.Transform{X: 10, Y: 11})
	ecs.AddComponent(world, handle, components.EntityStats{Stamina: 75, Energy: 80})
	saver.Save(world, 11, handle)
	ecs.AddComponent(world, handle, components.Transform{X: 20, Y: 21})
	saver.SaveDetached(world, 11, handle)
	world.Despawn(handle)
	saver.Stop()
	requireCharacterSavePostgresState(t, db, 11, 20, 20)
}

func TestCharacterSaverPostgresCooldownRoundTripAndMigration(t *testing.T) {
	db := newCharacterSavePostgres(t)
	ctx := context.Background()
	// Exercise the additive migration on this test's isolated, disposable schema.
	_, err := db.Pool().Exec(ctx, `ALTER TABLE character DROP COLUMN action_cooldowns`)
	require.NoError(t, err)
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "20261003_action_cooldowns.sql"))
	require.NoError(t, err)
	statement := strings.ReplaceAll(string(migration), "origin.character", "character")
	for range 2 {
		_, err = db.Pool().Exec(ctx, statement)
		require.NoError(t, err)
	}
	character, err := db.Queries().GetCharacter(ctx, 11)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(character.ActionCooldowns))
	saver := NewCharacterSaver(db, 0, nil, zap.NewNop())
	t.Cleanup(saver.Stop)
	snapshot := characterSaveIntegrationSnapshot(11, 20, 1)
	snapshot.ActionCooldowns = `{"plow_tile":{"startedAtMs":10000,"expiresAtMs":12000}}`
	require.True(t, saver.enqueueSnapshot(snapshot))
	require.NoError(t, saver.flushPending(ctx, saver.queueForCharacter(11), 0))
	character, err = db.Queries().GetCharacter(ctx, 11)
	require.NoError(t, err)
	restored, err := components.UnmarshalActionCooldowns(character.ActionCooldowns)
	require.NoError(t, err)
	require.Equal(t, int64(12000), restored.ByAction["plow_tile"].ExpiresAtMs)
	snapshot.ActionCooldowns, err = restored.MarshalActive(12000)
	require.NoError(t, err)
	require.True(t, saver.enqueueSnapshot(snapshot))
	require.NoError(t, saver.flushPending(ctx, saver.queueForCharacter(11), 0))
	character, err = db.Queries().GetCharacter(ctx, 11)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(character.ActionCooldowns))
}
