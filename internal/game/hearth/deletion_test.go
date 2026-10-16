package hearth_test

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/hearth"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type deletionReader struct {
	*constructionReader
	read func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error)
}

func (r deletionReader) GetSavedCharacterHearth(ctx context.Context, character int64) (repository.GetSavedCharacterHearthRow, error) {
	return r.read(ctx, character)
}

func savedHearthRow(character int64, object *repository.Object) repository.GetSavedCharacterHearthRow {
	return repository.GetSavedCharacterHearthRow{
		CharacterID: character, ObjectRegion: object.Region, ObjectID: object.ID,
		TypeID: sql.NullInt32{Int32: int32(object.TypeID), Valid: true},
		X:      sql.NullInt32{Int32: int32(object.X), Valid: true}, Y: sql.NullInt32{Int32: int32(object.Y), Valid: true},
		Layer: sql.NullInt32{Int32: int32(object.Layer), Valid: true}, DeletedAt: object.DeletedAt,
	}
}

func deletionFixtureReader(f *constructionFixture, saver *terminalSaver, read func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error)) {
	index := f.runtime.Index
	f.runtime = hearth.NewRuntime(f.w, f.chunks, deletionReader{f.reader, read}, f.factory, saver, f.registry, nil,
		zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic)))
	f.runtime.Index = index
}

func TestDeletionKnownStatesNeverReadSavedOwnership(t *testing.T) {
	for _, state := range []string{"no_home", "terminal", "current_home"} {
		t.Run(state, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			if state == "current_home" {
				chunk, _ := f.oldRaw(t, types.ChunkStateInactive)
				f.chunks.chunks[chunk.Coord] = chunk
			} else {
				require.NoError(t, f.runtime.Index.LoadIfUnknown(1, nil))
				if state == "terminal" {
					_, err := f.runtime.Index.ApplyTerminal(1)
					require.NoError(t, err)
				}
			}
			deletionFixtureReader(f, saver, func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
				t.Fatal("known runtime state must not query even a hidden invalid saved link")
				return repository.GetSavedCharacterHearthRow{}, errors.New("unexpected read")
			})
			for range 2 {
				require.NoError(t, f.runtime.CompleteDeletion(1))
			}
			require.Equal(t, hearth.Snapshot{State: hearth.NoHome, Terminal: true}, f.runtime.Index.Snapshot(1))
			require.Zero(t, f.reader.reads)
			if state == "current_home" {
				require.Len(t, saver.saves, 1)
				require.Equal(t, hearth.ObjectKey{Region: 1, ID: 2000}, saver.saves[0].previous)
				require.Equal(t, 16, saver.saves[0].converted.TypeID)
			} else {
				require.Empty(t, saver.saves)
				require.Empty(t, f.chunks.waited)
			}
		})
	}
	// Known absence has no dependency on world, definitions, reader or saver.
	runtime := hearth.NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, runtime.Index.LoadIfUnknown(1, nil))
	require.NoError(t, runtime.CompleteDeletion(1))
	require.NoError(t, runtime.CompleteDeletion(1))
}

func TestDeletionUnknownAbsentAssociationHasIndependentBoundedRead(t *testing.T) {
	var runtime hearth.Runtime
	var readContext context.Context
	reads := 0
	reader := deletionReader{read: func(ctx context.Context, character int64) (repository.GetSavedCharacterHearthRow, error) {
		reads++
		readContext = ctx
		require.Equal(t, int64(1), character)
		require.NoError(t, ctx.Err())
		deadline, exists := ctx.Deadline()
		require.True(t, exists)
		require.InDelta(t, 5, time.Until(deadline).Seconds(), .5)
		require.Equal(t, hearth.Snapshot{State: hearth.Unknown, PendingTerminal: true}, runtime.Index.Snapshot(1))
		return repository.GetSavedCharacterHearthRow{}, sql.ErrNoRows
	}}
	runtime = hearth.NewRuntime(nil, nil, reader, nil, nil, nil, nil, nil)
	require.NoError(t, runtime.CompleteDeletion(1))
	require.ErrorIs(t, readContext.Err(), context.Canceled)
	require.Equal(t, hearth.Snapshot{State: hearth.NoHome, Terminal: true}, runtime.Index.Snapshot(1))
	require.NoError(t, runtime.CompleteDeletion(1))
	require.Equal(t, 1, reads)
}

func TestDeletionUnknownHomeReusesLiveRawAndUnloadedCompletion(t *testing.T) {
	for _, source := range []string{"live", "raw", "unloaded"} {
		t.Run(source, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
			handle := types.InvalidHandle
			if source == "live" {
				chunk, handle = terminalLiveHome(t, f)
			} else if source == "unloaded" {
				f.chunks.wait = func(coord types.ChunkCoord) error { f.chunks.chunks[coord] = chunk; return nil }
			} else {
				f.chunks.chunks[chunk.Coord] = chunk
			}
			f.runtime.Index = hearth.NewIndex()
			reads := 0
			deletionFixtureReader(f, saver, func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
				reads++
				require.True(t, f.runtime.Index.Snapshot(1).PendingTerminal)
				return savedHearthRow(1, raw), nil
			})
			saver.onSave = func() {
				require.True(t, f.runtime.Index.Snapshot(1).Terminal)
				require.Equal(t, 1, f.chunks.pins[chunk.Coord])
			}
			require.NoError(t, f.runtime.CompleteDeletion(1))
			require.Len(t, saver.saves, 1)
			require.Equal(t, hearth.ObjectKey{Region: 1, ID: 2000}, saver.saves[0].previous)
			require.Equal(t, 16, saver.saves[0].converted.TypeID)
			require.Equal(t, raw.X, saver.saves[0].converted.X)
			require.Equal(t, raw.Y, saver.saves[0].converted.Y)
			require.Zero(t, f.chunks.pins[chunk.Coord])
			if source == "live" {
				require.Equal(t, handle, f.w.GetHandleByEntityID(2000))
				info, _ := ecs.GetComponent[components.EntityInfo](f.w, handle)
				require.Equal(t, uint32(16), info.TypeID)
			} else {
				require.Empty(t, chunk.GetHandles())
				require.Same(t, saver.saves[0].converted, chunk.GetRawObjectByID(2000))
			}
			require.NoError(t, f.runtime.CompleteDeletion(1))
			require.Equal(t, 1, reads)
			require.Zero(t, f.reader.reads)
			require.Len(t, saver.saves, 1)
		})
	}
}

func TestDeletionUnknownInvalidSavedSourceOnlyRemovesExactAssociation(t *testing.T) {
	for _, source := range []string{"missing", "deleted", "changed_type", "wrong_layer"} {
		t.Run(source, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			row := savedHearthRow(1, &repository.Object{Region: 2, ID: 2000, TypeID: f.result.DefID, X: 5000, Y: 200})
			switch source {
			case "missing":
				row.TypeID, row.X, row.Y, row.Layer = sql.NullInt32{}, sql.NullInt32{}, sql.NullInt32{}, sql.NullInt32{}
			case "deleted":
				row.DeletedAt = sql.NullTime{Time: time.Now(), Valid: true}
			case "changed_type":
				row.TypeID.Int32 = 16
			case "wrong_layer":
				row.Layer.Int32 = 1
			}
			deletionFixtureReader(f, saver, func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) { return row, nil })
			saver.onSave = func() { require.True(t, f.runtime.Index.Snapshot(1).Terminal) }
			require.NoError(t, f.runtime.CompleteDeletion(1))
			require.Equal(t, []terminalSave{{character: 1, previous: hearth.ObjectKey{Region: 2, ID: 2000}}}, saver.saves)
			require.Empty(t, f.chunks.pins)
			require.Empty(t, f.chunks.waited)
		})
	}
}

func TestDeletionReadAndMalformedResultsAreFatalWithoutSaving(t *testing.T) {
	for _, failure := range []string{"read", "timeout", "character", "zero_object", "negative_object", "region", "partial_source", "missing_with_timestamp", "conflicting_owner"} {
		t.Run(failure, func(t *testing.T) {
			f, saver := newTerminalFixture(t)
			row := savedHearthRow(1, &repository.Object{ID: 2000, Region: 1, TypeID: f.result.DefID})
			var readErr error
			switch failure {
			case "read":
				readErr = errors.New("saved ownership read failed")
			case "timeout":
				readErr = context.DeadlineExceeded
			case "character":
				row.CharacterID++
			case "zero_object":
				row.ObjectID = 0
			case "negative_object":
				row.ObjectID = -1
			case "region":
				row.ObjectRegion = math.MaxInt32 + 1
			case "partial_source":
				row.X.Valid = false
			case "missing_with_timestamp":
				row.TypeID, row.X, row.Y, row.Layer = sql.NullInt32{}, sql.NullInt32{}, sql.NullInt32{}, sql.NullInt32{}
				row.DeletedAt = sql.NullTime{Time: time.Now(), Valid: true}
			case "conflicting_owner":
				require.NoError(t, f.runtime.Index.LoadIfUnknown(2, &hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 2000}}))
			}
			deletionFixtureReader(f, saver, func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) { return row, readErr })
			require.Panics(t, func() { _ = f.runtime.CompleteDeletion(1) })
			require.Equal(t, hearth.Snapshot{State: hearth.Unknown, PendingTerminal: true}, f.runtime.Index.Snapshot(1))
			require.Empty(t, saver.saves)
			require.Empty(t, f.chunks.waited)
		})
	}
}

func TestDeletionInvalidSourceSaveFailureIsFatalAfterTerminal(t *testing.T) {
	f, saver := newTerminalFixture(t)
	deletionFixtureReader(f, saver, func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
		return repository.GetSavedCharacterHearthRow{CharacterID: 1, ObjectRegion: 1, ObjectID: 2000}, nil
	})
	saver.err = errors.New("save failed")
	require.Panics(t, func() { _ = f.runtime.CompleteDeletion(1) })
	require.True(t, f.runtime.Index.Snapshot(1).Terminal)
	require.Equal(t, []terminalSave{{character: 1, previous: hearth.ObjectKey{Region: 1, ID: 2000}}}, saver.saves)
}

func TestDeletionWaitsForSaveAndRetainsChunkPin(t *testing.T) {
	f, saver := newTerminalFixture(t)
	chunk, raw := f.oldRaw(t, types.ChunkStateInactive)
	f.chunks.chunks[chunk.Coord] = chunk
	f.runtime.Index = hearth.NewIndex()
	deletionFixtureReader(f, saver, func(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
		return savedHearthRow(1, raw), nil
	})
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	saver.onSave = func() { close(entered); <-release }
	go func() { done <- f.runtime.CompleteDeletion(1) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("deletion did not reach synchronous save")
	}
	require.True(t, f.runtime.Index.Snapshot(1).Terminal)
	require.Equal(t, 1, f.chunks.pins[chunk.Coord])
	select {
	case err := <-done:
		t.Fatalf("deletion returned before save: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-done)
	require.Zero(t, f.chunks.pins[chunk.Coord])
}
