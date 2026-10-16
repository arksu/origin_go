package hearth

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"origin/internal/persistence/repository"

	"github.com/stretchr/testify/require"
)

func TestOrphanSaverUsesOneBoundedTransaction(t *testing.T) {
	objects := []*repository.Object{{ID: 10, Region: 1, TypeID: 16}, {ID: 11, Region: 1, TypeID: 16}}
	calls, transactions := 0, 0
	var saveContext context.Context
	db := constructionExecDB{exec: func(ctx context.Context, query string, args ...any) (sql.Result, error) {
		require.Same(t, saveContext, ctx)
		require.True(t, strings.Contains(query, "-- name: UpsertObject "), "no ownership mutation or ID allocation during restoration")
		require.Equal(t, objects[calls].ID, args[0])
		calls++
		return driver.RowsAffected(1), nil
	}}
	saver := &ConstructionSaverDB{withTx: func(ctx context.Context, fn func(*repository.Queries) error) error {
		transactions++
		saveContext = ctx
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, 5, time.Until(deadline).Seconds(), .5)
		return fn(repository.New(db))
	}}
	require.NoError(t, saver.SaveOrphans(context.Background(), objects))
	require.Equal(t, 1, transactions)
	require.Equal(t, 2, calls)
	require.ErrorIs(t, saveContext.Err(), context.Canceled)
	require.NoError(t, saver.SaveOrphans(context.Background(), nil))
	require.Equal(t, 1, transactions)
}

func TestOrphanSaverPropagatesTransactionFailures(t *testing.T) {
	for _, scenario := range []string{"begin", "first_write", "second_write", "commit"} {
		t.Run(scenario, func(t *testing.T) {
			failure := errors.New(scenario)
			writes := 0
			db := constructionExecDB{exec: func(context.Context, string, ...any) (sql.Result, error) {
				writes++
				if scenario == "first_write" || scenario == "second_write" && writes == 2 {
					return nil, failure
				}
				return driver.RowsAffected(1), nil
			}}
			saver := &ConstructionSaverDB{withTx: func(_ context.Context, fn func(*repository.Queries) error) error {
				if scenario == "begin" {
					return failure
				}
				if err := fn(repository.New(db)); err != nil {
					return err
				}
				if scenario == "commit" {
					return failure
				}
				return nil
			}}
			require.ErrorIs(t, saver.SaveOrphans(context.Background(), []*repository.Object{{ID: 10}, {ID: 11}}), failure)
			switch scenario {
			case "begin":
				require.Zero(t, writes)
			case "first_write":
				require.Equal(t, 1, writes)
			default:
				require.Equal(t, 2, writes)
			}
		})
	}
}

func TestOrphanSaverRejectsInvalidInputBeforeTransaction(t *testing.T) {
	saver := &ConstructionSaverDB{withTx: func(context.Context, func(*repository.Queries) error) error {
		t.Fatal("invalid inputs must not reach the database")
		return nil
	}}
	var missing *ConstructionSaverDB
	require.Error(t, missing.SaveOrphans(context.Background(), []*repository.Object{{ID: 1}}))
	require.Error(t, saver.SaveOrphans(nil, []*repository.Object{{ID: 1}}))
	for _, objects := range [][]*repository.Object{{nil}, {{ID: 0}}, {{ID: 1}, {ID: -1}}} {
		require.Error(t, saver.SaveOrphans(context.Background(), objects))
	}
}
