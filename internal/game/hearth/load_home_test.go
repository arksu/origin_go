package hearth

import (
	"context"
	"database/sql"
	"errors"
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
	require.NotZero(t, loaded.Version)
	require.False(t, loaded.Dirty)
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
	require.False(t, loaded.Dirty)
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

func TestLoadHomeMissingDependenciesDoNotStartLookup(t *testing.T) {
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
	require.Empty(t, index.characters, "missing reader must not leave an admitted lookup")
	runtime.reader = reader
	require.NoError(t, runtime.LoadHome(1, 109))
	require.Equal(t, 1, reads)
	require.Equal(t, NoHome, index.Snapshot(1).State)
}

func TestLoadHomeDoesNotDuplicatePendingLookup(t *testing.T) {
	reads := 0
	runtime := NewRuntime(nil, nil, homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
		reads++
		return repository.GetCharacterHearthRow{}, sql.ErrNoRows
	}), nil, nil, nil, nil, nil)
	version, started := runtime.Index.BeginLookup(1)
	require.True(t, started)
	require.NoError(t, runtime.LoadHome(1, 109))
	require.Zero(t, reads)
	require.Equal(t, Unknown, runtime.Index.Snapshot(1).State)
	require.NoError(t, runtime.Index.CompleteLookup(1, version, nil))
}

func TestLoadHomeStaleResultPreservesRuntimeOwnership(t *testing.T) {
	for _, scenario := range []string{"revoked", "replaced", "terminal"} {
		t.Run(scenario, func(t *testing.T) {
			runtime := NewRuntime(nil, nil, nil, nil, nil, nil, nil, zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic)))
			var want Snapshot
			runtime.reader = homeReaderFunc(func(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
				var err error
				if scenario == "terminal" {
					want, err = runtime.Index.ApplyTerminal(1)
				} else {
					want, err = runtime.Index.RevokeHome(1)
					if err == nil && scenario == "replaced" {
						want, err = runtime.Index.ReplaceHome(1, want.Version, testHome(200))
					}
				}
				require.NoError(t, err)
				return repository.GetCharacterHearthRow{CharacterID: 1, ObjectRegion: 1, ObjectID: 100, X: 50, Y: 60, Layer: SurfaceLayer}, nil
			})
			require.NoError(t, runtime.LoadHome(1, 109))
			require.Equal(t, want, runtime.Index.Snapshot(1))
			_, owned := runtime.Index.Owner(ObjectKey{Region: 1, ID: 100})
			require.False(t, owned)
			require.False(t, runtime.Index.characters[1].loading, "stale result must release its accepted lookup")
		})
	}
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
			if scenario.err == nil {
				require.False(t, runtime.Index.characters[1].loading, "invalid completion must consume the lookup")
			}
		})
	}
}
