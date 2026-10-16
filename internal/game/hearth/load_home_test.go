package hearth

import (
	"context"
	"database/sql"
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

type homeFatalHook func(*zapcore.CheckedEntry, []zapcore.Field)

func (h homeFatalHook) OnWrite(entry *zapcore.CheckedEntry, fields []zapcore.Field) {
	h(entry, fields)
}

func TestLoadHomeHydratesBothIndexesAndCachesResult(t *testing.T) {
	const character types.EntityID = 72
	const hearthType = 109
	want := Home{Object: ObjectKey{Region: 5, ID: 391}, X: -125, Y: 640, Layer: SurfaceLayer}
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
		return repository.GetCharacterHearthRow{CharacterID: int64(character), ObjectRegion: want.Object.Region, ObjectID: int64(want.Object.ID), X: want.X, Y: want.Y, Layer: want.Layer}, nil
	})
	runtime := NewRuntime(nil, nil, reader, nil, nil, nil, nil, nil)
	require.NoError(t, runtime.LoadHome(character, hearthType))
	loaded := runtime.Index.Snapshot(character)
	require.Equal(t, CurrentHome, loaded.State)
	require.Equal(t, want, loaded.Home)
	owner, owned := runtime.Index.Owner(want.Object)
	require.True(t, owned)
	require.Equal(t, character, owner)
	require.ErrorIs(t, readContext.Err(), context.Canceled)

	require.NoError(t, runtime.LoadHome(character, hearthType))
	require.Equal(t, 1, reads)
	require.Equal(t, loaded, runtime.Index.Snapshot(character))
}

func TestLoadHomeAbsentRowCachesNoHome(t *testing.T) {
	reads := 0
	runtime := NewRuntime(nil, nil, homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
		reads++
		return repository.GetCharacterHearthRow{}, sql.ErrNoRows
	}), nil, nil, nil, nil, nil)
	require.NoError(t, runtime.LoadHome(1, 109))
	loaded := runtime.Index.Snapshot(1)
	require.Equal(t, NoHome, loaded.State)
	require.Equal(t, Home{}, loaded.Home)
	require.Empty(t, runtime.Index.owners)
	require.NoError(t, runtime.LoadHome(1, 109))
	require.Equal(t, 1, reads)
	require.Equal(t, loaded, runtime.Index.Snapshot(1))
}

func TestLoadHomeKnownRuntimeStateNeedsNoReader(t *testing.T) {
	for _, state := range []string{"no_home", "current_home"} {
		t.Run(state, func(t *testing.T) {
			runtime := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil)
			var home *Home
			if state == "current_home" {
				value := testHome(100)
				home = &value
			}
			known := hydrate(t, runtime.Index, 1, home)
			require.NoError(t, runtime.LoadHome(1, 109))
			require.Equal(t, known, runtime.Index.Snapshot(1))
		})
	}
}

func TestLoadHomeMissingDependenciesDoNotRead(t *testing.T) {
	reads := 0
	reader := homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
		reads++
		return repository.GetCharacterHearthRow{}, sql.ErrNoRows
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
	require.Equal(t, NoHome, index.Snapshot(1).State)
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

func (f homeReaderFunc) GetCharacterHearthOwners(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
	panic("unexpected GetCharacterHearthOwners")
}

func (homeReaderFunc) GetSavedCharacterHearth(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
	panic("unexpected saved hearth query")
}
