package hearth

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/persistence/repository"

	"github.com/stretchr/testify/require"
)

type constructionExecDB struct {
	exec func(context.Context, string, ...any) (sql.Result, error)
}

func (db constructionExecDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.exec(ctx, query, args...)
}

func (constructionExecDB) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	panic("unexpected PrepareContext")
}

func (constructionExecDB) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("unexpected QueryContext")
}

func (constructionExecDB) QueryRowContext(context.Context, string, ...any) *sql.Row {
	panic("unexpected QueryRowContext")
}

func TestConstructionSaverReturnsTransactionErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failExec int
		begin    bool
		commit   bool
	}{
		{name: "begin", begin: true},
		{name: "new_object", failExec: 1},
		{name: "previous_object", failExec: 2},
		{name: "owner", failExec: 3},
		{name: "id_watermark", failExec: 4},
		{name: "commit", commit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := errors.New(tc.name)
			executed := 0
			transactions := 0
			db := constructionExecDB{exec: func(context.Context, string, ...any) (sql.Result, error) {
				executed++
				if executed == tc.failExec {
					return nil, failure
				}
				return driver.RowsAffected(1), nil
			}}
			saver := &ConstructionSaverDB{withTx: func(_ context.Context, fn func(*repository.Queries) error) error {
				transactions++
				if tc.begin {
					return failure
				}
				if err := fn(repository.New(db)); err != nil {
					return err
				}
				if tc.commit {
					return failure
				}
				return nil
			}}
			err := saver.SaveConstruction(context.Background(), 1, &repository.Object{ID: 10}, &repository.Object{ID: 9})
			require.ErrorIs(t, err, failure)
			require.Equal(t, 1, transactions)
			if tc.begin {
				require.Zero(t, executed)
			} else if tc.commit {
				require.Equal(t, 4, executed)
			} else {
				require.Equal(t, tc.failExec, executed, "a failed write stops the transaction")
			}
		})
	}
}

func TestConstructionSaverRequiresOneAssociation(t *testing.T) {
	for _, rows := range []int64{0, 2} {
		calls := 0
		db := constructionExecDB{exec: func(_ context.Context, query string, _ ...any) (sql.Result, error) {
			calls++
			if strings.Contains(query, "-- name: SetCharacterHearth ") {
				return driver.RowsAffected(rows), nil
			}
			return driver.RowsAffected(1), nil
		}}
		saver := &ConstructionSaverDB{withTx: func(_ context.Context, fn func(*repository.Queries) error) error {
			return fn(repository.New(db))
		}}
		err := saver.SaveConstruction(context.Background(), 1, &repository.Object{ID: 10}, nil)
		require.ErrorContains(t, err, "expected one association")
		require.Equal(t, 2, calls, "failed association does not continue to the ID watermark")
	}
}

func TestConstructionSaverIncludesPreviousObjectInIDWatermark(t *testing.T) {
	var watermark []any
	db := constructionExecDB{exec: func(_ context.Context, query string, args ...any) (sql.Result, error) {
		if strings.Contains(query, "-- name: UpsertGlobalVarLongMax ") {
			watermark = args
		}
		return driver.RowsAffected(1), nil
	}}
	saver := &ConstructionSaverDB{withTx: func(_ context.Context, fn func(*repository.Queries) error) error {
		return fn(repository.New(db))
	}}
	// An older build site can be finished after another site with a larger ID.
	require.NoError(t, saver.SaveConstruction(context.Background(), 1, &repository.Object{ID: 10}, &repository.Object{ID: 20}))
	require.Equal(t, []any{constt.LAST_USED_ID, sql.NullInt64{Int64: 20, Valid: true}}, watermark)
}

func TestConstructionSaverBoundsDatabaseContext(t *testing.T) {
	var databaseContext context.Context
	saver := &ConstructionSaverDB{withTx: func(ctx context.Context, _ func(*repository.Queries) error) error {
		databaseContext = ctx
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, 5*time.Second, time.Until(deadline), float64(time.Second))
		return nil
	}}
	require.NoError(t, saver.SaveConstruction(context.Background(), 1, &repository.Object{ID: 10}, nil))
	require.ErrorIs(t, databaseContext.Err(), context.Canceled, "the writer releases its timeout on return")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	saver.withTx = func(databaseCtx context.Context, _ func(*repository.Queries) error) error {
		<-databaseCtx.Done()
		return databaseCtx.Err()
	}
	require.ErrorIs(t, saver.SaveConstruction(ctx, 1, &repository.Object{ID: 10}, nil), context.DeadlineExceeded)
}

func TestConstructionSaverRejectsMissingDependenciesAndIdentity(t *testing.T) {
	require.Nil(t, NewConstructionSaverDB(nil))
	var missingSaver *ConstructionSaverDB
	require.Error(t, missingSaver.SaveConstruction(context.Background(), 1, &repository.Object{ID: 10}, nil))
	saver := &ConstructionSaverDB{withTx: func(context.Context, func(*repository.Queries) error) error {
		t.Fatal("invalid inputs must not begin a transaction")
		return nil
	}}
	require.Error(t, saver.SaveConstruction(context.Background(), 0, &repository.Object{ID: 10}, nil))
	require.Error(t, saver.SaveConstruction(context.Background(), 1, nil, nil))
	require.Error(t, saver.SaveConstruction(context.Background(), 1, &repository.Object{}, nil))
	require.Error(t, saver.SaveConstruction(context.Background(), 1, &repository.Object{ID: 10}, &repository.Object{}))
}
