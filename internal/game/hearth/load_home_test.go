package hearth

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"math"
	"testing"
	"time"

	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type homeReaderFunc func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error)

func (f homeReaderFunc) GetCharacterHearth(ctx context.Context, params repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
	return f(ctx, params)
}

type savedHomeReader struct {
	homeReaderFunc
	saved func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error)
}

func (r savedHomeReader) GetSavedCharacterHearth(ctx context.Context, character int64) (repository.GetSavedCharacterHearthRow, error) {
	return r.saved(ctx, character)
}

type homeRepairSaver struct {
	ConstructionSaver
	repair func(context.Context, types.EntityID, ObjectKey, int) error
}

func (s homeRepairSaver) RepairInvalidHome(ctx context.Context, character types.EntityID, key ObjectKey, hearthType int) error {
	return s.repair(ctx, character, key, hearthType)
}

type homeFatalHook func(*zapcore.CheckedEntry, []zapcore.Field)

func (h homeFatalHook) OnWrite(entry *zapcore.CheckedEntry, fields []zapcore.Field) {
	h(entry, fields)
}

func TestLoadHomeHydratesBothIndexesAndCachesResult(t *testing.T) {
	const character types.EntityID = 72
	const hearthType = 109
	want := Home{Object: ObjectKey{Region: 5, ID: 391}, X: -125, Y: 640, Layer: SurfaceLayer, OwnerName: "Offline owner"}
	reads := 0
	var readContext context.Context
	reader := homeReaderFunc(func(ctx context.Context, params repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
		reads++
		readContext = ctx
		require.Equal(t, repository.GetCharacterHearthParams{CharacterID: int64(character), HearthTypeID: hearthType, SurfaceLayer: SurfaceLayer}, params)
		deadline, present := ctx.Deadline()
		require.True(t, present)
		require.InDelta(t, 5, time.Until(deadline).Seconds(), 0.5)
		require.NoError(t, ctx.Err())
		return repository.GetCharacterHearthRow{CharacterID: int64(character), ObjectRegion: want.Object.Region, ObjectID: int64(want.Object.ID), X: want.X, Y: want.Y, Layer: want.Layer, OwnerName: want.OwnerName,
			OwnerDisconnectAt: sql.NullTime{Time: time.UnixMilli(123000), Valid: true}}, nil
	})
	runtime := NewRuntime(nil, nil, reader, nil, nil, nil, nil, nil)
	require.NoError(t, runtime.LoadHome(character, hearthType))
	loaded := runtime.Index.Snapshot(character)
	require.Equal(t, CurrentHome, loaded.State)
	require.Equal(t, want, loaded.Home)
	require.Equal(t, int64(123000), loaded.LastDisconnectUnixMs)
	owner, owned := runtime.Index.Owner(want.Object)
	require.True(t, owned)
	require.Equal(t, character, owner)
	require.ErrorIs(t, readContext.Err(), context.Canceled)

	require.NoError(t, runtime.LoadHome(character, hearthType))
	require.Equal(t, 1, reads)
	require.Equal(t, loaded, runtime.Index.Snapshot(character))
}

func TestLoadHomeAbsentRowCachesNoHome(t *testing.T) {
	reads, savedReads := 0, 0
	reader := savedHomeReader{
		homeReaderFunc: func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
			reads++
			return repository.GetCharacterHearthRow{}, sql.ErrNoRows
		},
		saved: func(_ context.Context, character int64) (repository.GetSavedCharacterHearthRow, error) {
			savedReads++
			require.Equal(t, int64(1), character)
			return repository.GetSavedCharacterHearthRow{}, sql.ErrNoRows
		},
	}
	runtime := NewRuntime(nil, nil, reader, nil, nil, nil, nil, nil)
	require.NoError(t, runtime.LoadHome(1, 109))
	loaded := runtime.Index.Snapshot(1)
	require.Equal(t, NoHome, loaded.State)
	require.Equal(t, Home{}, loaded.Home)
	require.Empty(t, runtime.Index.owners)
	require.NoError(t, runtime.LoadHome(1, 109))
	require.Equal(t, 1, reads)
	require.Equal(t, 1, savedReads)
	require.Equal(t, loaded, runtime.Index.Snapshot(1))
}

func TestLoadHomeKnownRuntimeStateNeedsNoReader(t *testing.T) {
	for _, state := range []string{"no_home", "current_home", "pending_no_home", "pending_current_home", "terminal"} {
		t.Run(state, func(t *testing.T) {
			runtime := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil)
			var home *Home
			if state == "current_home" || state == "pending_current_home" {
				value := testHome(100)
				home = &value
			}
			hydrate(t, runtime.Index, 1, home)
			if state == "pending_no_home" || state == "pending_current_home" {
				require.NoError(t, runtime.Index.MarkTerminalPending(1))
			} else if state == "terminal" {
				_, err := runtime.Index.ApplyTerminal(1)
				require.NoError(t, err)
			}
			known := runtime.Index.Snapshot(1)
			require.NoError(t, runtime.LoadHome(1, 109))
			require.Equal(t, known, runtime.Index.Snapshot(1))
		})
	}
}

func TestLoadHomeMissingDependenciesDoNotRead(t *testing.T) {
	reads := 0
	reader := homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
		reads++
		return repository.GetCharacterHearthRow{CharacterID: 1, ObjectID: 100}, nil
	})
	runtime := NewRuntime(nil, nil, reader, nil, nil, nil, nil, nil)
	index := runtime.Index
	runtime.Index = nil
	require.Error(t, runtime.LoadHome(1, 109))
	require.Zero(t, reads)
	runtime.Index = index
	runtime.reader = nil
	require.Error(t, runtime.LoadHome(1, 109))
	require.Empty(t, index.characters, "missing reader must not change metadata")
	runtime.reader = reader
	require.NoError(t, runtime.LoadHome(1, 109))
	require.Equal(t, 1, reads)
	require.Equal(t, CurrentHome, index.Snapshot(1).State)
}

func TestLoadHomeHydratesPendingTerminalMetadata(t *testing.T) {
	for _, alreadyPending := range []bool{false, true} {
		t.Run(map[bool]string{false: "during_read", true: "before_read"}[alreadyPending], func(t *testing.T) {
			runtime := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil)
			runtime.reader = homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
				require.NoError(t, runtime.Index.MarkTerminalPending(1))
				return repository.GetCharacterHearthRow{CharacterID: 1, ObjectRegion: 1, ObjectID: 100, X: 50, Y: 60}, nil
			})
			if alreadyPending {
				require.NoError(t, runtime.Index.MarkTerminalPending(1))
			}
			require.NoError(t, runtime.LoadHome(1, 109))
			pending := runtime.Index.Snapshot(1)
			require.Equal(t, CurrentHome, pending.State)
			require.True(t, pending.PendingTerminal)
			require.False(t, pending.CanAcquire())
			require.False(t, pending.CanReturn())
			object := ObjectKey{Region: 1, ID: 100}
			_, owned := runtime.Index.Owner(object)
			require.False(t, owned)
			owner, snapshot, owned := runtime.Index.SnapshotByObject(object)
			require.True(t, owned)
			require.Equal(t, types.EntityID(1), owner)
			require.Equal(t, pending, snapshot)
		})
	}
}

func TestLoadHomeInvalidCharacterDoesNotRead(t *testing.T) {
	reads := 0
	runtime := NewRuntime(nil, nil, homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
		reads++
		return repository.GetCharacterHearthRow{}, sql.ErrNoRows
	}), nil, nil, nil, nil, nil)
	for _, character := range []types.EntityID{0, math.MaxUint64} {
		require.ErrorIs(t, runtime.LoadHome(character, 109), ErrInvalidCharacter)
	}
	require.Zero(t, reads)
	require.Empty(t, runtime.Index.characters)
}

func TestLoadHomeFatalFailuresReturnErrors(t *testing.T) {
	databaseErr := errors.New("database connection lost")
	for _, scenario := range []struct {
		name string
		row  repository.GetCharacterHearthRow
		err  error
		want error
	}{
		{name: "database", err: databaseErr, want: databaseErr},
		{name: "timeout", err: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "invalid_object", row: repository.GetCharacterHearthRow{CharacterID: 1}, want: ErrInvalidHome},
		{name: "invalid_layer", row: repository.GetCharacterHearthRow{CharacterID: 1, ObjectID: 100, Layer: -1}, want: ErrInvalidHome},
		{name: "conflicting_owner", row: repository.GetCharacterHearthRow{CharacterID: 1, ObjectRegion: 1, ObjectID: 100}, want: ErrObjectOwned},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fatals := 0
			logger := zap.NewNop().WithOptions(zap.WithFatalHook(homeFatalHook(func(entry *zapcore.CheckedEntry, _ []zapcore.Field) {
				require.Equal(t, zapcore.FatalLevel, entry.Level)
				fatals++
			})))
			runtime := NewRuntime(nil, nil, homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
				return scenario.row, scenario.err
			}), nil, nil, nil, nil, logger)
			if scenario.name == "conflicting_owner" {
				home := testHome(100)
				hydrate(t, runtime.Index, 2, &home)
			}
			require.ErrorIs(t, runtime.LoadHome(1, 109), scenario.want)
			require.Equal(t, 1, fatals, "lookup errors must reach the fatal logger hook")
			require.Equal(t, Unknown, runtime.Index.Snapshot(1).State)
		})
	}
}

func TestLoadHomeWaitsForRepairBeforeCachingAbsence(t *testing.T) {
	const character types.EntityID = 72
	const hearthType = 109
	key := ObjectKey{Region: 5, ID: 391}
	reads, savedReads, writes, transactions := 0, 0, 0, 0
	var readContext, savedContext, writeContext context.Context
	var readStartErr, savedStartErr error
	var savedCharacter int64
	var query string
	var args []any
	reader := savedHomeReader{
		homeReaderFunc: func(ctx context.Context, _ repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
			reads++
			readContext = ctx
			readStartErr = ctx.Err()
			return repository.GetCharacterHearthRow{}, sql.ErrNoRows
		},
		saved: func(ctx context.Context, id int64) (repository.GetSavedCharacterHearthRow, error) {
			savedReads++
			savedContext, savedCharacter = ctx, id
			savedStartErr = ctx.Err()
			return repository.GetSavedCharacterHearthRow{CharacterID: id, ObjectRegion: key.Region, ObjectID: int64(key.ID)}, nil
		},
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	db := constructionExecDB{exec: func(_ context.Context, statement string, params ...any) (sql.Result, error) {
		writes++
		query, args = statement, params
		return driver.RowsAffected(1), nil
	}}
	saver := &ConstructionSaverDB{withTx: func(ctx context.Context, fn func(*repository.Queries) error) error {
		transactions++
		writeContext = ctx
		if err := fn(repository.New(db)); err != nil {
			return err
		}
		close(entered)
		<-release
		return nil
	}}
	runtime := NewRuntime(nil, nil, reader, nil, saver, nil, nil, nil)
	result := make(chan error, 1)
	go func() { result <- runtime.LoadHome(character, hearthType) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("repair did not reach commit")
	}
	require.Equal(t, int64(character), savedCharacter)
	require.Equal(t, 1, reads)
	require.Equal(t, 1, savedReads)
	require.Equal(t, 1, writes)
	require.Equal(t, 1, transactions)
	require.NoError(t, readStartErr)
	require.NoError(t, savedStartErr, "the fallback read must not inherit the completed first read context")
	require.Contains(t, query, "-- name: DeleteInvalidCharacterHearth ")
	require.Equal(t, []any{int64(character), key.Region, int64(key.ID), hearthType, SurfaceLayer}, args)
	for _, ctx := range []context.Context{readContext, savedContext, writeContext} {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, 5, time.Until(deadline).Seconds(), .5)
	}
	require.NotSame(t, readContext, savedContext)
	require.NotSame(t, savedContext, writeContext)
	require.ErrorIs(t, readContext.Err(), context.Canceled)
	require.ErrorIs(t, savedContext.Err(), context.Canceled)
	require.NoError(t, writeContext.Err(), "repair must not inherit either completed read context")
	require.Equal(t, Unknown, runtime.Index.Snapshot(character).State)
	require.Empty(t, runtime.Index.owners)
	require.NoError(t, runtime.Index.MarkTerminalPending(character))
	select {
	case err := <-result:
		t.Fatalf("LoadHome returned before repair commit: %v", err)
	default:
	}
	release <- struct{}{}
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("LoadHome did not finish after repair commit")
	}
	loaded := runtime.Index.Snapshot(character)
	require.Equal(t, NoHome, loaded.State)
	require.True(t, loaded.PendingTerminal)
	require.False(t, loaded.CanAcquire())
	require.False(t, loaded.CanReturn())
	require.ErrorIs(t, writeContext.Err(), context.Canceled)
	require.NoError(t, runtime.LoadHome(character, hearthType))
	require.Equal(t, 1, reads)
	require.Equal(t, 1, savedReads)
	require.Equal(t, 1, writes)
}

func TestLoadHomeSavedAssociationFailuresAreFatal(t *testing.T) {
	databaseErr := errors.New("saved association read failed")
	repairErr := errors.New("repair commit failed")
	valid := repository.GetSavedCharacterHearthRow{CharacterID: 1, ObjectRegion: 2, ObjectID: 100}
	for _, scenario := range []struct {
		name         string
		row          repository.GetSavedCharacterHearthRow
		readErr      error
		repairErr    error
		missingSaver bool
		want         error
	}{
		{name: "read_error", readErr: databaseErr, want: databaseErr},
		{name: "read_timeout", readErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "wrong_character", row: repository.GetSavedCharacterHearthRow{CharacterID: 2, ObjectID: 100}},
		{name: "missing_object", row: repository.GetSavedCharacterHearthRow{CharacterID: 1}},
		{name: "negative_object", row: repository.GetSavedCharacterHearthRow{CharacterID: 1, ObjectID: -1}},
		{name: "region_overflow", row: repository.GetSavedCharacterHearthRow{CharacterID: 1, ObjectRegion: math.MaxInt32 + 1, ObjectID: 100}},
		{name: "region_underflow", row: repository.GetSavedCharacterHearthRow{CharacterID: 1, ObjectRegion: math.MinInt32 - 1, ObjectID: 100}},
		{name: "missing_saver", row: valid, missingSaver: true},
		{name: "repair_error", row: valid, repairErr: repairErr, want: repairErr},
		{name: "repair_timeout", row: valid, repairErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fatals, reads, savedReads, repairs := 0, 0, 0, 0
			logger := zap.NewNop().WithOptions(zap.WithFatalHook(homeFatalHook(func(*zapcore.CheckedEntry, []zapcore.Field) { fatals++ })))
			reader := savedHomeReader{
				homeReaderFunc: func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
					reads++
					return repository.GetCharacterHearthRow{}, sql.ErrNoRows
				},
				saved: func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
					savedReads++
					return scenario.row, scenario.readErr
				},
			}
			var saver ConstructionSaver = homeRepairSaver{repair: func(context.Context, types.EntityID, ObjectKey, int) error {
				repairs++
				return scenario.repairErr
			}}
			if scenario.missingSaver {
				saver = nil
			}
			runtime := NewRuntime(nil, nil, reader, nil, saver, nil, nil, logger)
			require.NoError(t, runtime.Index.MarkTerminalPending(1))
			err := runtime.LoadHome(1, 109)
			require.Error(t, err)
			if scenario.want != nil {
				require.ErrorIs(t, err, scenario.want)
			}
			require.Equal(t, 1, fatals)
			require.Equal(t, 1, reads)
			require.Equal(t, 1, savedReads)
			if scenario.repairErr != nil {
				require.Equal(t, 1, repairs)
			} else {
				require.Zero(t, repairs)
			}
			require.Equal(t, Snapshot{State: Unknown, PendingTerminal: true}, runtime.Index.Snapshot(1))
			require.Empty(t, runtime.Index.owners)
		})
	}
}

func (f homeReaderFunc) GetCharacterHearthOwners(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
	panic("unexpected GetCharacterHearthOwners")
}

func (homeReaderFunc) GetSavedCharacterHearth(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
	panic("unexpected saved hearth query")
}
