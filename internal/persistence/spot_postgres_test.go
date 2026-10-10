package persistence_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_const "origin/internal/const"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func newSpotPostgres(t *testing.T, useMigration bool) *persistence.Postgres {
	t.Helper()
	db := testutil.NewPostgres(t, "ORIGIN_SPOTS_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	if useMigration {
		ctx := context.Background()
		_, err := db.Pool().Exec(ctx, "DROP TABLE spot")
		require.NoError(t, err)
		migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "20261010_spots.sql"))
		require.NoError(t, err)
		// The fixture has its own search_path and must never touch origin.
		_, err = db.Pool().Exec(ctx, strings.ReplaceAll(string(migration), "origin.spot", "spot"))
		require.NoError(t, err)
	}
	return db
}

func spotInsertFixture(region int, types ...string) repository.InsertSpotsParams {
	params := repository.InsertSpotsParams{Region: region, LastRuntimeSeconds: 1234}
	for _, spotType := range types {
		params.SpotTypes = append(params.SpotTypes, spotType)
		params.DistrictXs = append(params.DistrictXs, 0)
		params.DistrictYs = append(params.DistrictYs, 0)
		params.CenterXs = append(params.CenterXs, -7*_const.CoordPerTile+_const.CoordPerTile/2)
		params.CenterYs = append(params.CenterYs, 9*_const.CoordPerTile+_const.CoordPerTile/2)
		params.Radii = append(params.Radii, 64*_const.CoordPerTile)
		params.PeakQualities = append(params.PeakQualities, 37)
	}
	return params
}

func requireSpotPostgresError(t *testing.T, err error, code string) {
	t.Helper()
	var pgError *pgconn.PgError
	require.ErrorAs(t, err, &pgError)
	require.Equal(t, code, pgError.Code)
}

func TestSpotPostgresSchemaAndMigration(t *testing.T) {
	for _, ddl := range []struct {
		name         string
		useMigration bool
	}{{"schema", false}, {"migration", true}} {
		t.Run(ddl.name, func(t *testing.T) {
			db := newSpotPostgres(t, ddl.useMigration)
			ctx := context.Background()
			params := spotInsertFixture(1, "www", "clay", "soil", "sand", "water")
			require.NoError(t, db.Queries().InsertSpots(ctx, params))
			var count int
			require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot").Scan(&count))
			require.Equal(t, len(params.SpotTypes), count)

			var firstID, lastRuntime, revision int64
			var centerX, centerY, radius, quality int
			var state string
			require.NoError(t, db.Pool().QueryRow(ctx, `
				SELECT id, center_x, center_y, radius, peak_quality,
					state::text, last_runtime_seconds, revision
				FROM spot WHERE region = 1 AND spot_type = 'www'
			`).Scan(&firstID, &centerX, &centerY, &radius, &quality, &state, &lastRuntime, &revision))
			require.Equal(t, params.CenterXs[0], centerX, "negative absolute world coordinates are valid")
			require.Equal(t, params.CenterYs[0], centerY)
			require.Equal(t, params.Radii[0], radius)
			require.EqualValues(t, params.PeakQualities[0], quality)
			require.JSONEq(t, `{}`, state)
			require.Equal(t, params.LastRuntimeSeconds, lastRuntime)
			require.EqualValues(t, 1, revision)

			requireSpotPostgresError(t, db.Queries().InsertSpots(ctx, spotInsertFixture(1, "www")), "23505")
			for _, invalid := range []string{
				"layer = 1", "spot_type = 'stone'", "district_x = -1", "district_y = -1",
				"radius = 0", "peak_quality = 9", "state = '[]'::jsonb", "state = 'null'::jsonb",
				"last_runtime_seconds = -1", "revision = 0",
			} {
				t.Run(invalid, func(t *testing.T) {
					_, err := db.Pool().Exec(ctx, "UPDATE spot SET "+invalid+" WHERE spot_type = 'www'")
					requireSpotPostgresError(t, err, "23514")
				})
			}
			_, err := db.Pool().Exec(ctx, "UPDATE spot SET state = NULL WHERE spot_type = 'www'")
			requireSpotPostgresError(t, err, "23502")

			// Deleting a region neither touches other regions nor reuses IDs.
			require.NoError(t, db.Queries().InsertSpots(ctx, spotInsertFixture(2, "www")))
			require.NoError(t, db.Queries().DeleteSpotsByRegion(ctx, 1))
			require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot WHERE region = 2").Scan(&count))
			require.Equal(t, 1, count)
			require.NoError(t, db.Queries().InsertSpots(ctx, spotInsertFixture(1, "www")))
			var replacementID int64
			require.NoError(t, db.Pool().QueryRow(ctx, "SELECT id FROM spot WHERE region = 1").Scan(&replacementID))
			require.Greater(t, replacementID, firstID)
		})
	}
}

func TestSpotPostgresBatchesRollbackTogether(t *testing.T) {
	db := newSpotPostgres(t, false)
	ctx := context.Background()
	err := db.WithTx(ctx, func(queries *repository.Queries) error {
		if err := queries.InsertSpots(ctx, spotInsertFixture(1, "www", "clay")); err != nil {
			return err
		}
		return queries.InsertSpots(ctx, spotInsertFixture(1, "soil", "invalid"))
	})
	requireSpotPostgresError(t, err, "23514")
	var count int
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT count(*) FROM spot").Scan(&count))
	require.Zero(t, count, "an error in a later batch must also roll back prior batches")
}
