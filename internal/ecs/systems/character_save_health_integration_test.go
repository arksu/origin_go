package systems

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func requireCharacterSavePostgresHealth(t *testing.T, db *persistence.Postgres, id int64, shp, hhp float64) {
	t.Helper()
	character, err := db.Queries().GetCharacter(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, shp, character.Shp)
	require.Equal(t, hhp, character.Hhp)
}

func applyFractionalCharacterHealthMigration(t *testing.T, db *persistence.Postgres) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var schema string
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT current_schema()").Scan(&schema))
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "20261007_fractional_character_health.sql"))
	require.NoError(t, err)
	statement := strings.ReplaceAll(string(migration), "origin.character", pgx.Identifier{schema, "character"}.Sanitize())
	_, err = db.Pool().Exec(ctx, statement)
	require.NoError(t, err)
}

func restoreLegacyCharacterHealthSchema(t *testing.T, db *persistence.Postgres) {
	t.Helper()
	_, err := db.Pool().Exec(context.Background(), `
		ALTER TABLE character
		    DROP CONSTRAINT character_shp_check,
		    DROP CONSTRAINT character_hhp_check,
		    ALTER COLUMN shp TYPE INT USING shp::int,
		    ALTER COLUMN hhp TYPE INT USING hhp::int,
		    ADD CONSTRAINT character_shp_check CHECK (shp >= 0),
		    ADD CONSTRAINT character_hhp_check CHECK (hhp >= 0);
	`)
	require.NoError(t, err)
}

func TestCharacterSaverPostgresFractionalHealthMigration(t *testing.T) {
	db := newCharacterSavePostgres(t)
	restoreLegacyCharacterHealthSchema(t, db)
	_, err := db.Pool().Exec(context.Background(), `
		UPDATE character SET shp = 0, hhp = 2147483647 WHERE id = 11;
		UPDATE character SET shp = 21, hhp = 24 WHERE id = 12;
	`)
	require.NoError(t, err)
	applyFractionalCharacterHealthMigration(t, db)
	requireCharacterSavePostgresHealth(t, db, 11, 0, math.MaxInt32)
	requireCharacterSavePostgresHealth(t, db, 12, 21, 24)
	var shpType, hhpType string
	require.NoError(t, db.Pool().QueryRow(context.Background(), `SELECT pg_typeof(shp)::text, pg_typeof(hhp)::text FROM character WHERE id = 11`).Scan(&shpType, &hhpType))
	require.Equal(t, "double precision", shpType)
	require.Equal(t, "double precision", hhpType)
	_, err = db.Pool().Exec(context.Background(), `UPDATE character SET shp = .25, hhp = .49 WHERE id = 11`)
	require.NoError(t, err)
	applyFractionalCharacterHealthMigration(t, db)
	requireCharacterSavePostgresHealth(t, db, 11, .25, .49)
	requireCharacterSavePostgresHealth(t, db, 12, 21, 24)
}

func TestCharacterSaverPostgresHealthConstraints(t *testing.T) {
	for _, schema := range []string{"fresh", "migrated"} {
		t.Run(schema, func(t *testing.T) {
			db := newCharacterSavePostgres(t)
			if schema == "migrated" {
				restoreLegacyCharacterHealthSchema(t, db)
				applyFractionalCharacterHealthMigration(t, db)
			}
			for _, column := range []string{"shp", "hhp"} {
				for _, value := range []float64{-.01, math.NaN(), math.Inf(-1), math.Inf(1)} {
					_, err := db.Pool().Exec(context.Background(), "UPDATE character SET "+column+" = $1 WHERE id = 11", value)
					var constraintError *pgconn.PgError
					require.ErrorAs(t, err, &constraintError)
					require.Equal(t, "23514", constraintError.Code)
					require.Equal(t, "character_"+column+"_check", constraintError.ConstraintName)
					requireCharacterSavePostgresHealth(t, db, 11, 100, 100)
				}
			}
		})
	}
}

func TestCharacterSaverPostgresFractionalHealthEverySavePath(t *testing.T) {
	db := newCharacterSavePostgres(t)
	for _, mode := range []string{"periodic", "disconnect", "detached_expiry", "transfer_sync", "shutdown"} {
		t.Run(mode, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			handle := world.Spawn(11, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 100, Energy: 900})
				ecs.AddComponent(w, h, components.EntityHealth{SHP: 21.4, HHP: 24.28, IsLying: true, KOUntilUnixMs: 61000})
			})
			ecs.GetResource[ecs.CharacterEntities](world).Add(11, handle, time.Time{})
			ecs.GetResource[ecs.TimeState](world).Now = time.Unix(100, 0)
			saver := NewCharacterSaver(db, 0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot { return nil }), zap.NewNop())
			t.Cleanup(saver.Stop)
			switch mode {
			case "periodic":
				NewCharacterSaveSystem(saver, time.Minute, zap.NewNop()).Update(world, 0)
			case "disconnect":
				require.NoError(t, saver.Save(world, 11, handle))
			case "detached_expiry":
				require.NoError(t, saver.SaveDetached(world, 11, handle))
			case "transfer_sync":
				require.NoError(t, saver.SaveSync(world, 11, handle))
			case "shutdown":
				require.NoError(t, saver.SaveAll(world))
			}
			saver.Stop()
			requireCharacterSavePostgresHealth(t, db, 11, 21.4, 24.28)
			health, exists := ecs.GetComponent[components.EntityHealth](world, handle)
			require.True(t, exists)
			require.Equal(t, 21.4, health.SHP)
			require.Equal(t, 24.28, health.HHP)
			require.Equal(t, int64(61000), health.KOUntilUnixMs)
		})
	}
}

func TestCharacterSaverPostgresFiniteHealthRoundTrip(t *testing.T) {
	db := newCharacterSavePostgres(t)
	saver := NewCharacterSaver(db, 0, nil, zap.NewNop())
	t.Cleanup(saver.Stop)
	for _, value := range []float64{0, math.SmallestNonzeroFloat64, .49, 81.0 / 13, float64(math.MaxInt32) + .25, math.MaxFloat64} {
		snapshot := characterSaveIntegrationSnapshot(11, 10, 1)
		snapshot.SHP, snapshot.HHP = value, value
		require.True(t, saver.enqueueSnapshot(snapshot))
		require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(11), 0))
		requireCharacterSavePostgresHealth(t, db, 11, value, value)
	}
	created, err := db.Queries().CreateCharacter(context.Background(), repository.CreateCharacterParams{
		ID: 13, AccountID: 1, Name: "FractionalFixture", Stamina: 100, Energy: 100,
		Shp: .25, Hhp: .49, Attributes: []byte(`{}`), Exp: []byte(`{}`), Skills: []byte(`[]`), Discovery: []byte(`[]`),
	})
	require.NoError(t, err)
	require.Equal(t, .25, created.Shp)
	require.Equal(t, .49, created.Hhp)
	requireCharacterSavePostgresHealth(t, db, 13, .25, .49)
}
