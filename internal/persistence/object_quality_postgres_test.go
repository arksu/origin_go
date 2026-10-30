package persistence_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestObjectQualityPostgresSchemaAndMigration(t *testing.T) {
	for _, useMigration := range []bool{false, true} {
		name := "schema"
		if useMigration {
			name = "migration"
		}
		t.Run(name, func(t *testing.T) {
			db := testutil.NewPostgres(t, "ORIGIN_OBJECT_QUALITY_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
			ctx := context.Background()
			if useMigration {
				// Recreate the legacy column on the parent and its region partitions.
				_, err := db.Pool().Exec(ctx, `ALTER TABLE object
					DROP CONSTRAINT object_quality_check,
					ALTER COLUMN quality TYPE SMALLINT USING quality::smallint,
					ADD CONSTRAINT object_quality_check CHECK (quality >= 0)`)
				require.NoError(t, err)
				for region, quality := range map[int]int64{1: 0, 2: 32767} {
					require.NoError(t, db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
						ID: int64(region), TypeID: 1, Region: region, Quality: quality,
					}))
				}
				migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "20261030_object_quality_uint32.sql"))
				require.NoError(t, err)
				for range 2 {
					_, err = db.Pool().Exec(ctx, strings.ReplaceAll(string(migration), "origin.object", "object"))
					require.NoError(t, err)
				}
				for region, quality := range map[int]int64{1: 0, 2: 32767} {
					loaded, err := db.Queries().GetObjectByID(ctx, int64(region))
					require.NoError(t, err)
					require.Equal(t, quality, loaded.Quality)
				}
			}
			for _, region := range []int{1, 2} {
				for index, quality := range []int64{0, 32767, 32768, math.MaxInt32 + 1, math.MaxUint32} {
					params := repository.UpsertObjectParams{ID: int64(region*100 + index), TypeID: 1, Region: region, Quality: quality}
					require.NoError(t, db.Queries().UpsertObject(ctx, params))
					loaded, err := db.Queries().GetObjectByID(ctx, params.ID)
					require.NoError(t, err)
					require.Equal(t, quality, loaded.Quality)
				}
				for _, quality := range []int64{-1, math.MaxUint32 + 1, math.MaxInt64} {
					err := db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{ID: int64(region*100 + 99), TypeID: 1, Region: region, Quality: quality})
					var pgError *pgconn.PgError
					require.ErrorAs(t, err, &pgError)
					require.Equal(t, "23514", pgError.Code)
				}
			}
		})
	}
}
