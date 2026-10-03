package game

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/persistence"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newCombatSavePostgres(t *testing.T) *persistence.Postgres {
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
	schema := fmt.Sprintf("combat_save_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, cleanupErr := admin.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+quotedSchema+" CASCADE")
		require.NoError(t, cleanupErr)
		require.NoError(t, admin.Close(cleanupCtx))
	})
	schemaSQL, err := os.ReadFile(filepath.Join("..", "..", "migrations", "schema.sql"))
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
		FROM generate_series(1, 1) id;
	`)
	require.NoError(t, err)
	return db
}

func TestCombatPostgresSpentStaminaAndQualitySurviveReload(t *testing.T) {
	db := newCombatSavePostgres(t)
	fixture := newCombatFixture(t, 120)
	equipment := fixture.world.GetHandleByEntityID(10)
	require.True(t, fixture.world.Alive(equipment))
	ecs.WithComponent(fixture.world, equipment, func(container *components.InventoryContainer) {
		container.Version = 1
		container.Items[0].Quantity = 1
	})
	ecs.AddComponent(fixture.world, fixture.actor, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryEquipment, OwnerID: 1, Handle: equipment}}})
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	// Freeze regeneration and use the real transactional save path immediately
	// after payment, before any timer can obscure the saved amount.
	saver := systems.NewCharacterSaver(db, 0, inventory.NewInventorySaver(zap.NewNop()), zap.NewNop())
	t.Cleanup(saver.Stop)
	require.NoError(t, saver.SaveSync(fixture.world, 1, fixture.actor))
	var stamina, energy float64
	require.NoError(t, db.Pool().QueryRow(context.Background(), "SELECT stamina, energy FROM character WHERE id = 1").Scan(&stamina, &energy))
	require.Equal(t, float64(60), stamina)
	profile, _ := ecs.GetComponent[components.CharacterProfile](fixture.world, fixture.actor)
	loadedStats := buildInitialEntityStats(stamina, energy, profile.Attributes)
	require.Equal(t, float64(60), loadedStats.Stamina)
	rows, err := db.Queries().GetInventoriesByOwner(context.Background(), 1)
	require.NoError(t, err)
	loader := inventory.NewInventoryLoader(zap.NewNop())
	snapshots, warnings := loader.ParseInventoriesFromDB(rows)
	require.Empty(t, warnings)
	freshWorld := ecs.NewWorldForTesting()
	result, err := loader.LoadPlayerInventories(freshWorld, 1, snapshots)
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Len(t, result.ContainerHandles, 1)
	reloaded, ok := ecs.GetComponent[components.InventoryContainer](freshWorld, result.ContainerHandles[0])
	require.True(t, ok)
	require.Len(t, reloaded.Items, 1)
	require.Equal(t, uint32(10), reloaded.Items[0].Quality)
	require.Equal(t, uint32(1002), reloaded.Items[0].TypeID)
	actor := freshWorld.Spawn(1, nil)
	require.False(t, ecs.HasComponent[components.CombatState](freshWorld, actor), "runtime combat timers and activity are not persistent")
}
