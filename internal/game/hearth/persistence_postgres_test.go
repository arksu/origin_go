package hearth_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

const testHearthTypeID = 36

func newHearthPostgres(t *testing.T, useMigration bool) *persistence.Postgres {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join(root, "migrations", "schema.sql"))
	if useMigration {
		ctx := context.Background()
		_, err := db.Pool().Exec(ctx, "DROP TABLE character_hearth")
		require.NoError(t, err)
		migration, err := os.ReadFile(filepath.Join(root, "migrations", "20261012_character_hearth.sql"))
		require.NoError(t, err)
		// Only use the test schema selected by the fixture's search_path.
		_, err = db.Pool().Exec(ctx, strings.ReplaceAll(string(migration), "origin.", ""))
		require.NoError(t, err)
	}
	return db
}

func insertHearthCharacter(t *testing.T, db *persistence.Postgres, id int64) {
	t.Helper()
	ctx := context.Background()
	var accountID int64
	require.NoError(t, db.Pool().QueryRow(ctx, `
		INSERT INTO account (login, password_hash) VALUES ($1, 'test') RETURNING id
	`, fmt.Sprintf("hearth_%d", id)).Scan(&accountID))
	_, err := db.Pool().Exec(ctx, `
		INSERT INTO character (id, account_id, name, region, x, y, layer, heading,
			stamina, energy, shp, hhp, attributes, exp, skills, discovery)
		VALUES ($1, $2, 'test', 1, 0, 0, 0, 0, 100, 100, 100, 100, '{}', '{}', '[]', '[]')
	`, id, accountID)
	require.NoError(t, err)
}

func insertHearthObject(t *testing.T, db *persistence.Postgres, region int, id int64) {
	t.Helper()
	_, err := db.Pool().Exec(context.Background(), `
		INSERT INTO object (region, id, type_id, x, y, layer, chunk_x, chunk_y, create_tick, last_tick)
		VALUES ($1, $2, $3, $4, -25, 0, 0, 0, 0, 0)
	`, region, id, testHearthTypeID, region*100)
	require.NoError(t, err)
}

func requireHearthPostgresError(t *testing.T, err error, code string) {
	t.Helper()
	var pgError *pgconn.PgError
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, code, pgError.Code)
}

func hearthSetParams(characterID int64, region int, objectID int64) repository.SetCharacterHearthParams {
	return repository.SetCharacterHearthParams{
		CharacterID: characterID, ObjectRegion: region, ObjectID: objectID,
		HearthTypeID: testHearthTypeID, SurfaceLayer: 0,
	}
}

func hearthGetParams(characterID int64) repository.GetCharacterHearthParams {
	return repository.GetCharacterHearthParams{
		CharacterID: characterID, HearthTypeID: testHearthTypeID, SurfaceLayer: 0,
	}
}

func hearthRepairParams(characterID int64, region int, objectID int64) repository.DeleteInvalidCharacterHearthParams {
	return repository.DeleteInvalidCharacterHearthParams{
		CharacterID: characterID, ObjectRegion: region, ObjectID: objectID,
		HearthTypeID: testHearthTypeID, SurfaceLayer: 0,
	}
}

func TestHearthPostgresSchemaAndMigration(t *testing.T) {
	for _, useMigration := range []bool{false, true} {
		t.Run(fmt.Sprintf("migration_%t", useMigration), func(t *testing.T) {
			db := newHearthPostgres(t, useMigration)
			ctx := context.Background()
			insertHearthCharacter(t, db, 1)
			insertHearthCharacter(t, db, 2)
			insertHearthObject(t, db, 1, 10)
			insertHearthObject(t, db, 1, 11)
			_, err := db.Pool().Exec(ctx, "INSERT INTO character_hearth VALUES (1, 1, 10)")
			require.NoError(t, err)

			for _, statement := range []string{
				"INSERT INTO character_hearth VALUES (1, 1, 11)",
				"INSERT INTO character_hearth VALUES (2, 1, 10)",
			} {
				_, err = db.Pool().Exec(ctx, statement)
				requireHearthPostgresError(t, err, "23505")
			}
			for _, statement := range []string{
				"INSERT INTO character_hearth VALUES (99, 1, 11)",
				"INSERT INTO character_hearth VALUES (2, 1, 99)",
				"INSERT INTO character_hearth VALUES (2, 2, 11)",
			} {
				_, err = db.Pool().Exec(ctx, statement)
				requireHearthPostgresError(t, err, "23503")
			}
			for _, statement := range []string{
				"DELETE FROM character WHERE id = 1",
				"DELETE FROM object WHERE region = 1 AND id = 10",
			} {
				_, err = db.Pool().Exec(ctx, statement)
				var pgError *pgconn.PgError
				require.ErrorAs(t, err, &pgError)
				// PostgreSQL versions report either FK or explicit RESTRICT violation.
				require.Contains(t, []string{"23503", "23001"}, pgError.Code)
			}
			var restrictedFKCount int
			require.NoError(t, db.Pool().QueryRow(ctx, `
				SELECT count(*) FROM pg_constraint
				WHERE conrelid = 'character_hearth'::regclass
					AND contype = 'f' AND confdeltype = 'r' AND conparentid = 0
			`).Scan(&restrictedFKCount))
			require.Equal(t, 2, restrictedFKCount)

			insertHearthObject(t, db, 2, 10)
			_, err = db.Pool().Exec(ctx, "INSERT INTO character_hearth VALUES (2, 2, 10)")
			require.NoError(t, err, "same numeric ID in another region is a different object")
			var indexCount int
			require.NoError(t, db.Pool().QueryRow(ctx, `
				SELECT count(*) FROM pg_indexes
				WHERE schemaname = current_schema() AND tablename = 'character_hearth'
			`).Scan(&indexCount))
			require.Equal(t, 2, indexCount, "PK and object UNIQUE already support the lookups")
		})
	}
}

func TestHearthPostgresReadReplaceAndRemove(t *testing.T) {
	db := newHearthPostgres(t, false)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	insertHearthObject(t, db, 1, 10)
	insertHearthObject(t, db, 1, 11)
	_, err := q.GetCharacterHearth(ctx, hearthGetParams(1))
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = q.GetSavedCharacterHearth(ctx, 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
	for _, params := range []repository.SetCharacterHearthParams{
		hearthSetParams(99, 1, 10), hearthSetParams(1, 1, 99), hearthSetParams(1, 2, 10),
	} {
		rows, err := q.SetCharacterHearth(ctx, params)
		require.NoError(t, err)
		require.Zero(t, rows, "missing source or target does not create an association")
	}

	for _, objectID := range []int64{10, 10, 11} {
		rows, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, objectID))
		require.NoError(t, err)
		require.EqualValues(t, 1, rows)
	}
	home, err := q.GetCharacterHearth(ctx, hearthGetParams(1))
	require.NoError(t, err)
	require.EqualValues(t, 11, home.ObjectID)
	require.Equal(t, "test", home.OwnerName)
	require.Equal(t, 1, home.ObjectRegion)
	require.Equal(t, 100, home.X)
	require.Equal(t, -25, home.Y)
	require.Zero(t, home.Layer)

	rows, err := q.RemoveCharacterHearth(ctx, repository.RemoveCharacterHearthParams{
		CharacterID: 1, ObjectRegion: 1, ObjectID: 10,
	})
	require.NoError(t, err)
	require.Zero(t, rows, "a stale deletion must not remove a newer home")
	rows, err = q.RemoveCharacterHearth(ctx, repository.RemoveCharacterHearthParams{
		CharacterID: 1, ObjectRegion: 2, ObjectID: 11,
	})
	require.NoError(t, err)
	require.Zero(t, rows, "removal matches the full composite key")
	rows, err = q.RemoveCharacterHearth(ctx, repository.RemoveCharacterHearthParams{
		CharacterID: 1, ObjectRegion: 1, ObjectID: 11,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	_, err = q.GetSavedCharacterHearth(ctx, 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestHearthPostgresInvalidAssociations(t *testing.T) {
	for _, invalid := range []struct {
		name string
		sql  string
	}{
		{"deleted_character", "UPDATE character SET deleted_at = now() WHERE id = 1"},
		{"deleted_object", "UPDATE object SET deleted_at = now() WHERE region = 1 AND id = 10"},
		{"wrong_type", "UPDATE object SET type_id = 1 WHERE region = 1 AND id = 10"},
		{"underground", "UPDATE object SET layer = 1 WHERE region = 1 AND id = 10"},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			db := newHearthPostgres(t, false)
			ctx := context.Background()
			q := db.Queries()
			insertHearthCharacter(t, db, 1)
			insertHearthObject(t, db, 1, 10)
			rows, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
			require.NoError(t, err)
			require.EqualValues(t, 1, rows)
			_, err = db.Pool().Exec(ctx, invalid.sql)
			require.NoError(t, err)
			_, err = q.GetCharacterHearth(ctx, hearthGetParams(1))
			require.ErrorIs(t, err, sql.ErrNoRows)
			owners, err := q.GetCharacterHearthOwners(ctx, repository.GetCharacterHearthOwnersParams{
				ObjectRegions: []int{1}, ObjectIds: []int64{10}, HearthTypeID: testHearthTypeID, SurfaceLayer: 0,
			})
			require.NoError(t, err)
			require.Empty(t, owners)
			saved, err := q.GetSavedCharacterHearth(ctx, 1)
			require.NoError(t, err, "replacement can still inspect an invalid saved link")
			require.EqualValues(t, 10, saved.ObjectID)
			rows, err = q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
			require.NoError(t, err)
			require.Zero(t, rows, "invalid characters or targets cannot acquire a link")
			rows, err = q.DeleteInvalidCharacterHearth(ctx, hearthRepairParams(1, 1, 10))
			require.NoError(t, err)
			require.EqualValues(t, 1, rows)
			rows, err = q.DeleteInvalidCharacterHearth(ctx, hearthRepairParams(1, 1, 10))
			require.NoError(t, err)
			require.Zero(t, rows)
		})
	}
}

func TestHearthPostgresOwnerBatchUsesCompositePairs(t *testing.T) {
	db := newHearthPostgres(t, false)
	ctx := context.Background()
	q := db.Queries()
	for i, key := range []struct {
		region int
		id     int64
	}{{1, 10}, {2, 20}, {1, 20}, {2, 10}} {
		characterID := int64(i + 1)
		insertHearthCharacter(t, db, characterID)
		insertHearthObject(t, db, key.region, key.id)
		rows, err := q.SetCharacterHearth(ctx, hearthSetParams(characterID, key.region, key.id))
		require.NoError(t, err)
		require.EqualValues(t, 1, rows)
	}
	owners, err := q.GetCharacterHearthOwners(ctx, repository.GetCharacterHearthOwnersParams{
		ObjectRegions: []int{1, 2, 1}, ObjectIds: []int64{10, 20, 10},
		HearthTypeID: testHearthTypeID, SurfaceLayer: 0,
	})
	require.NoError(t, err)
	require.Len(t, owners, 2, "pairs are not a regions-by-IDs Cartesian product; duplicates do not duplicate owners")
	characterIDs := make([]int64, 0, len(owners))
	for _, owner := range owners {
		require.Equal(t, "test", owner.OwnerName)
		characterIDs = append(characterIDs, owner.CharacterID)
	}
	require.ElementsMatch(t, []int64{1, 2}, characterIDs)
	owners, err = q.GetCharacterHearthOwners(ctx, repository.GetCharacterHearthOwnersParams{
		HearthTypeID: testHearthTypeID, SurfaceLayer: 0,
	})
	require.NoError(t, err)
	require.Empty(t, owners)
}

func TestHearthPostgresRepairPreservesFreshAssociation(t *testing.T) {
	db := newHearthPostgres(t, false)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	insertHearthObject(t, db, 1, 10)
	insertHearthObject(t, db, 2, 10)
	_, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
	require.NoError(t, err)
	_, err = db.Pool().Exec(ctx, "UPDATE object SET type_id = 1 WHERE region = 1 AND id = 10")
	require.NoError(t, err)
	_, err = q.SetCharacterHearth(ctx, hearthSetParams(1, 2, 10))
	require.NoError(t, err)
	rows, err := q.DeleteInvalidCharacterHearth(ctx, hearthRepairParams(1, 1, 10))
	require.NoError(t, err)
	require.Zero(t, rows, "old invalidity cannot remove the replacement association")
	rows, err = q.DeleteInvalidCharacterHearth(ctx, hearthRepairParams(1, 2, 10))
	require.NoError(t, err)
	require.Zero(t, rows, "repair also rechecks current validity")
	home, err := q.GetCharacterHearth(ctx, hearthGetParams(1))
	require.NoError(t, err)
	require.Equal(t, 2, home.ObjectRegion)
}
