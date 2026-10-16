package hearth

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
)

func TestTerminalSaverUsesOneBoundedTransaction(t *testing.T) {
	object := repository.Object{
		ID: 10, Region: 2, TypeID: 16, X: 250, Y: -125, Layer: SurfaceLayer, ChunkX: 2, ChunkY: -2,
		Heading: sql.NullInt16{Int16: 90, Valid: true}, Quality: 17,
		Hp: sql.NullFloat64{Float64: 100, Valid: true}, OwnerID: sql.NullInt64{Int64: 20, Valid: true},
		Data: pqtype.NullRawMessage{RawMessage: []byte(`{"state":"unlit"}`), Valid: true}, CreateTick: 50, LastTick: 123,
	}
	original := object
	key := ObjectKey{Region: object.Region, ID: types.EntityID(object.ID)}
	writes, transactions := 0, 0
	var saveContext context.Context
	db := constructionExecDB{exec: func(ctx context.Context, query string, args ...any) (sql.Result, error) {
		require.Same(t, saveContext, ctx)
		writes++
		switch writes {
		case 1:
			require.Contains(t, query, "-- name: UpsertObject ")
			require.Equal(t, []any{
				object.ID, object.TypeID, object.Region, object.X, object.Y, object.Layer, object.ChunkX, object.ChunkY,
				object.Heading, object.Quality, object.Hp, object.OwnerID, object.Data, object.CreateTick, object.LastTick,
			}, args)
		case 2:
			require.Contains(t, query, "-- name: RemoveCharacterHearth ")
			require.Equal(t, []any{int64(1), object.Region, object.ID}, args)
		default:
			t.Fatal("terminal cleanup must not write an ID watermark or perform extra queries")
		}
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
	require.NoError(t, saver.SaveTerminal(context.Background(), 1, key, &object))
	require.Equal(t, 1, transactions)
	require.Equal(t, 2, writes)
	require.Equal(t, original, object, "saving must not mutate its caller's object")
	require.ErrorIs(t, saveContext.Err(), context.Canceled, "the writer releases its timeout on return")
}

func TestTerminalSaverAcceptsAbsentAssociationAndCleanupOnly(t *testing.T) {
	for _, withObject := range []bool{false, true} {
		for _, rows := range []int64{0, 1} {
			name := "cleanup_only"
			if withObject {
				name = "conversion"
			}
			if rows == 0 {
				name += "_already_removed"
			}
			t.Run(name, func(t *testing.T) {
				writes, removals := 0, 0
				db := constructionExecDB{exec: func(_ context.Context, query string, args ...any) (sql.Result, error) {
					writes++
					if strings.Contains(query, "-- name: RemoveCharacterHearth ") {
						removals++
						require.Equal(t, []any{int64(1), 2, int64(10)}, args)
						return driver.RowsAffected(rows), nil
					}
					require.True(t, withObject, "cleanup without a current source must never upsert")
					require.Contains(t, query, "-- name: UpsertObject ")
					return driver.RowsAffected(1), nil
				}}
				saver := &ConstructionSaverDB{withTx: func(_ context.Context, fn func(*repository.Queries) error) error {
					return fn(repository.New(db))
				}}
				var object *repository.Object
				if withObject {
					object = &repository.Object{ID: 10, Region: 2, TypeID: 16}
				}
				require.NoError(t, saver.SaveTerminal(context.Background(), 1, ObjectKey{Region: 2, ID: 10}, object))
				require.Equal(t, 1, removals)
				if withObject {
					require.Equal(t, 2, writes)
				} else {
					require.Equal(t, 1, writes)
				}
			})
		}
	}
}

func TestTerminalSaverPropagatesTransactionFailures(t *testing.T) {
	for _, scenario := range []string{"begin", "upsert", "remove", "affected_rows", "commit"} {
		t.Run(scenario, func(t *testing.T) {
			failure := errors.New(scenario)
			writes, transactions, commits, rollbacks := 0, 0, 0, 0
			db := constructionExecDB{exec: func(context.Context, string, ...any) (sql.Result, error) {
				writes++
				if scenario == "upsert" && writes == 1 || scenario == "remove" && writes == 2 {
					return nil, failure
				}
				if scenario == "affected_rows" && writes == 2 {
					return terminalRowsError{err: failure}, nil
				}
				return driver.RowsAffected(1), nil
			}}
			saver := &ConstructionSaverDB{withTx: func(_ context.Context, fn func(*repository.Queries) error) error {
				transactions++
				if scenario == "begin" {
					return failure
				}
				defer func() { rollbacks++ }()
				if err := fn(repository.New(db)); err != nil {
					return err
				}
				commits++
				if scenario == "commit" {
					return failure
				}
				return nil
			}}
			err := saver.SaveTerminal(context.Background(), 1, ObjectKey{Region: 2, ID: 10}, &repository.Object{ID: 10, Region: 2, TypeID: 16})
			require.ErrorIs(t, err, failure)
			require.Equal(t, 1, transactions, "database failures must not be retried")
			if scenario == "begin" {
				require.Zero(t, writes)
				require.Zero(t, rollbacks)
			} else {
				require.Equal(t, 1, rollbacks)
				if scenario == "upsert" {
					require.Equal(t, 1, writes)
				} else {
					require.Equal(t, 2, writes)
				}
			}
			if scenario == "commit" {
				require.Equal(t, 1, commits)
			} else {
				require.Zero(t, commits, "a failed write must abort before commit")
			}
		})
	}
}

type terminalRowsError struct{ err error }

func (r terminalRowsError) LastInsertId() (int64, error) { panic("unexpected LastInsertId") }
func (r terminalRowsError) RowsAffected() (int64, error) { return 0, r.err }

func TestTerminalSaverRejectsInvalidAffectedRows(t *testing.T) {
	for _, rows := range []int64{-1, 2} {
		committed := false
		db := constructionExecDB{exec: func(context.Context, string, ...any) (sql.Result, error) {
			return driver.RowsAffected(rows), nil
		}}
		saver := &ConstructionSaverDB{withTx: func(_ context.Context, fn func(*repository.Queries) error) error {
			if err := fn(repository.New(db)); err != nil {
				return err
			}
			committed = true
			return nil
		}}
		err := saver.SaveTerminal(context.Background(), 1, ObjectKey{Region: 2, ID: 10}, nil)
		require.ErrorContains(t, err, "expected zero or one association")
		require.False(t, committed)
	}
}

func TestTerminalSaverHonorsEarlierContextDeadline(t *testing.T) {
	saver := &ConstructionSaverDB{withTx: func(ctx context.Context, _ func(*repository.Queries) error) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, saver.SaveTerminal(ctx, 1, ObjectKey{Region: 2, ID: 10}, nil), context.DeadlineExceeded)
}

func TestTerminalSaverRejectsInvalidInputBeforeTransaction(t *testing.T) {
	saver := &ConstructionSaverDB{withTx: func(context.Context, func(*repository.Queries) error) error {
		t.Fatal("invalid inputs must not begin a transaction")
		return nil
	}}
	key := ObjectKey{Region: 2, ID: 10}
	var missing *ConstructionSaverDB
	require.Error(t, missing.SaveTerminal(context.Background(), 1, key, nil))
	require.Error(t, (&ConstructionSaverDB{}).SaveTerminal(context.Background(), 1, key, nil))
	require.Error(t, saver.SaveTerminal(nil, 1, key, nil))
	for _, character := range []types.EntityID{0, math.MaxInt64 + 1} {
		require.Error(t, saver.SaveTerminal(context.Background(), character, key, nil))
	}
	for _, invalid := range []ObjectKey{
		{Region: 2}, {Region: 2, ID: math.MaxInt64 + 1}, {Region: math.MaxInt32 + 1, ID: 10}, {Region: math.MinInt32 - 1, ID: 10},
	} {
		require.Error(t, saver.SaveTerminal(context.Background(), 1, invalid, nil))
	}
	for _, tc := range []struct {
		name   string
		mutate func(*repository.Object)
	}{
		{"zero_id", func(o *repository.Object) { o.ID = 0 }},
		{"wrong_id", func(o *repository.Object) { o.ID++ }},
		{"wrong_region", func(o *repository.Object) { o.Region++ }},
		{"wrong_layer", func(o *repository.Object) { o.Layer = 1 }},
		{"deleted", func(o *repository.Object) { o.DeletedAt.Valid = true }},
		{"missing_type", func(o *repository.Object) { o.TypeID = 0 }},
		{"invalid_type", func(o *repository.Object) { o.TypeID = math.MaxInt32 + 1 }},
		{"invalid_x", func(o *repository.Object) { o.X = math.MaxInt32 + 1 }},
		{"invalid_y", func(o *repository.Object) { o.Y = math.MinInt32 - 1 }},
		{"nan_hp", func(o *repository.Object) { o.Hp = sql.NullFloat64{Float64: math.NaN(), Valid: true} }},
		{"infinite_hp", func(o *repository.Object) { o.Hp = sql.NullFloat64{Float64: math.Inf(1), Valid: true} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			object := repository.Object{ID: 10, Region: 2, TypeID: 16}
			tc.mutate(&object)
			require.ErrorContains(t, saver.SaveTerminal(context.Background(), 1, key, &object), "invalid converted object")
		})
	}
}
