package hearth_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/game/hearth"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type restorationReader func(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error)

func (f restorationReader) GetCharacterHearthOwners(ctx context.Context, params repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
	return f(ctx, params)
}

func (restorationReader) GetCharacterHearth(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error) {
	panic("unexpected single-owner query")
}

type restorationSaver func(context.Context, []*repository.Object) error

func (f restorationSaver) SaveOrphans(ctx context.Context, objects []*repository.Object) error {
	return f(ctx, objects)
}

func (restorationSaver) SaveConstruction(context.Context, types.EntityID, *repository.Object, *repository.Object) error {
	panic("unexpected construction save")
}

func restorationChunk(f *constructionFixture, ids ...int64) *core.Chunk {
	chunk := core.NewChunk(types.ChunkCoord{}, 1, hearth.SurfaceLayer, constt.ChunkSize)
	chunk.SetState(types.ChunkStatePreloaded)
	var rows []*repository.Object
	for _, id := range ids {
		rows = append(rows, &repository.Object{ID: id, TypeID: f.result.DefID, Region: 1, X: int(id), Y: 30,
			Heading: sql.NullInt16{Int16: 90, Valid: true}, Quality: 17, CreateTick: 7, LastTick: 10,
			Hp:   sql.NullFloat64{Float64: 12, Valid: true},
			Data: pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{"old":"hearth"}`)},
		})
	}
	chunk.SetRawObjects(rows)
	return chunk
}

func restoredHome(raw *repository.Object) hearth.Home {
	return hearth.Home{Object: hearth.ObjectKey{Region: raw.Region, ID: types.EntityID(raw.ID)}, X: raw.X, Y: raw.Y, Layer: raw.Layer}
}

func restoredOwner(character int64, raw *repository.Object) repository.GetCharacterHearthOwnersRow {
	return repository.GetCharacterHearthOwnersRow{CharacterID: character, ObjectRegion: raw.Region, ObjectID: raw.ID, X: raw.X, Y: raw.Y, Layer: raw.Layer}
}

func restorationRuntime(f *constructionFixture, reader hearth.HomeReader, saver hearth.ConstructionSaver) hearth.Runtime {
	return hearth.NewRuntime(f.w, f.chunks, reader, f.factory, saver, f.registry, nil, zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic)))
}

func TestPrepareChunkActivationHydratesOfflineOwnersInOneBatch(t *testing.T) {
	f := newConstructionFixture(t)
	chunk := restorationChunk(f, 200, 201)
	first, second := chunk.GetRawObjectByID(200), chunk.GetRawObjectByID(201)
	reads := 0
	var readContext context.Context
	runtime := restorationRuntime(f, restorationReader(func(ctx context.Context, params repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
		reads++
		readContext = ctx
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.InDelta(t, 5, time.Until(deadline).Seconds(), .5)
		require.Equal(t, repository.GetCharacterHearthOwnersParams{HearthTypeID: f.result.DefID, SurfaceLayer: 0, ObjectRegions: []int{1, 1}, ObjectIds: []int64{200, 201}}, params)
		return []repository.GetCharacterHearthOwnersRow{restoredOwner(70, first), restoredOwner(71, second)}, nil
	}), nil)
	require.NoError(t, runtime.PrepareChunkActivation(chunk))
	require.ErrorIs(t, readContext.Err(), context.Canceled)
	for n, raw := range []*repository.Object{first, second} {
		character := types.EntityID(70 + n)
		require.Equal(t, restoredHome(raw), runtime.Index.Snapshot(character).Home)
		owner, ok := runtime.Index.Owner(restoredHome(raw).Object)
		require.True(t, ok)
		require.Equal(t, character, owner)
		require.Same(t, raw, chunk.GetRawObjectByID(types.EntityID(raw.ID)))
	}
	require.NoError(t, runtime.PrepareChunkActivation(chunk))
	require.Equal(t, 1, reads, "known runtime ownership avoids another query on activation retry")
}

func TestPrepareChunkActivationSavesAllOrphansBeforeChangingRaw(t *testing.T) {
	f := newConstructionFixture(t)
	chunk := restorationChunk(f, 200, 201)
	first, second := chunk.GetRawObjectByID(200), chunk.GetRawObjectByID(201)
	campfire, ok := objectdefs.Global().GetByKey("campfire")
	require.True(t, ok)
	reads, saves := 0, 0
	runtime := restorationRuntime(f, restorationReader(func(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
		reads++
		return nil, nil
	}), restorationSaver(func(_ context.Context, converted []*repository.Object) error {
		saves++
		require.Len(t, converted, 2)
		for n, raw := range []*repository.Object{first, second} {
			require.Same(t, raw, chunk.GetRawObjectByID(types.EntityID(raw.ID)), "raw cache changes only after commit")
			require.False(t, f.w.Alive(f.w.GetHandleByEntityID(types.EntityID(raw.ID))))
			want := *raw
			want.TypeID, want.Hp = campfire.DefID, sql.NullFloat64{Float64: float64(campfire.HP), Valid: true}
			want.Data, want.LastTick = pqtype.NullRawMessage{}, int64(ecs.GetResource[ecs.TimeState](f.w).Tick)
			require.Equal(t, want, *converted[n])
		}
		return nil
	}))
	require.NoError(t, runtime.PrepareChunkActivation(chunk))
	for _, raw := range []*repository.Object{first, second} {
		require.Equal(t, campfire.DefID, chunk.GetRawObjectByID(types.EntityID(raw.ID)).TypeID)
		require.Equal(t, f.result.DefID, raw.TypeID, "original snapshots remain immutable")
	}
	require.NoError(t, runtime.PrepareChunkActivation(chunk))
	require.Equal(t, 1, reads)
	require.Equal(t, 1, saves)
}

func TestPrepareChunkActivationRuntimeStateWins(t *testing.T) {
	for _, scenario := range []string{"current", "pending_current", "pending_unknown", "pending_during_read", "no_home", "replaced", "terminal"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConstructionFixture(t)
			chunk := restorationChunk(f, 200)
			raw := chunk.GetRawObjectByID(200)
			reads, saves := 0, 0
			var runtime hearth.Runtime
			runtime = restorationRuntime(f, restorationReader(func(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
				reads++
				if scenario == "pending_during_read" {
					require.NoError(t, runtime.Index.MarkTerminalPending(70))
				}
				return []repository.GetCharacterHearthOwnersRow{restoredOwner(70, raw)}, nil
			}), restorationSaver(func(context.Context, []*repository.Object) error { saves++; return nil }))
			home := restoredHome(raw)
			switch scenario {
			case "current", "pending_current":
				require.NoError(t, runtime.Index.LoadIfUnknown(70, &home))
			case "no_home":
				require.NoError(t, runtime.Index.LoadIfUnknown(70, nil))
			case "replaced":
				home.Object.ID = 999
				require.NoError(t, runtime.Index.LoadIfUnknown(70, &home))
			case "terminal":
				_, err := runtime.Index.ApplyTerminal(70)
				require.NoError(t, err)
			}
			if scenario == "pending_current" || scenario == "pending_unknown" {
				require.NoError(t, runtime.Index.MarkTerminalPending(70))
			}
			before := runtime.Index.Snapshot(70)
			require.NoError(t, runtime.PrepareChunkActivation(chunk))
			switch scenario {
			case "current", "pending_current":
				require.Zero(t, reads)
				require.Zero(t, saves)
				require.Equal(t, before, runtime.Index.Snapshot(70))
			case "pending_unknown", "pending_during_read":
				require.Equal(t, 1, reads)
				require.Zero(t, saves)
				require.Equal(t, home, runtime.Index.Snapshot(70).Home)
				require.True(t, runtime.Index.Snapshot(70).PendingTerminal)
				_, eligible := runtime.Index.Owner(home.Object)
				require.False(t, eligible)
			default:
				require.Equal(t, 1, saves)
				require.Equal(t, before, runtime.Index.Snapshot(70))
				require.NotEqual(t, f.result.DefID, chunk.GetRawObjectByID(200).TypeID)
			}
		})
	}
}

func TestPrepareChunkActivationSkipsNonCandidates(t *testing.T) {
	f := newConstructionFixture(t)
	chunk := restorationChunk(f, 200, 201, 1000)
	chunk.GetRawObjectByID(200).DeletedAt = sql.NullTime{Valid: true, Time: time.Now()}
	chunk.GetRawObjectByID(201).TypeID = 16
	// 1000 already exists as the construction fixture's live site.
	chunk.SetState(types.ChunkStateActive)
	runtime := restorationRuntime(f, nil, nil)
	require.NoError(t, runtime.PrepareChunkActivation(chunk))
	require.NoError(t, runtime.PrepareChunkActivation(restorationChunk(f)))
	require.Equal(t, f.result.DefID, chunk.GetRawObjectByID(1000).TypeID)
}

func TestPrepareChunkActivationWithoutHearthDefinitionNeedsNoPersistence(t *testing.T) {
	f := newConstructionFixture(t)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	chunk := restorationChunk(f, 200)
	raw := chunk.GetRawObjectByID(200)
	raw.TypeID = campfire.DefID
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{*campfire}))
	runtime := restorationRuntime(f, nil, nil)
	require.NoError(t, runtime.PrepareChunkActivation(chunk))
	require.Same(t, raw, chunk.GetRawObjectByID(200))
}

func TestPrepareChunkActivationFatalFailuresLeaveRawUnchanged(t *testing.T) {
	for _, scenario := range []string{"query", "save", "missing_reader", "missing_saver", "unrequested", "invalid_owner", "invalid_home", "duplicate_row", "duplicate_owner"} {
		t.Run(scenario, func(t *testing.T) {
			f := newConstructionFixture(t)
			chunk := restorationChunk(f, 200, 201)
			first, second := chunk.GetRawObjectByID(200), chunk.GetRawObjectByID(201)
			var reader hearth.HomeReader = restorationReader(func(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
				if scenario == "query" {
					return nil, errors.New("database failed")
				}
				row := restoredOwner(70, first)
				switch scenario {
				case "unrequested":
					row.ObjectRegion++
				case "invalid_owner":
					row.CharacterID = 0
				case "invalid_home":
					row.Layer = 1
				case "duplicate_row":
					return []repository.GetCharacterHearthOwnersRow{row, row}, nil
				case "duplicate_owner":
					return []repository.GetCharacterHearthOwnersRow{row, restoredOwner(70, second)}, nil
				default:
					return nil, nil
				}
				return []repository.GetCharacterHearthOwnersRow{row}, nil
			})
			var saver hearth.ConstructionSaver = restorationSaver(func(context.Context, []*repository.Object) error {
				require.Equal(t, "save", scenario)
				return errors.New("commit failed")
			})
			if scenario == "missing_reader" {
				reader = nil
			}
			if scenario == "missing_saver" {
				saver = nil
			}
			runtime := restorationRuntime(f, reader, saver)
			require.Panics(t, func() { _ = runtime.PrepareChunkActivation(chunk) })
			require.Same(t, first, chunk.GetRawObjectByID(200))
			require.Same(t, second, chunk.GetRawObjectByID(201))
			require.Equal(t, types.ChunkStatePreloaded, chunk.GetState())
		})
	}
}

func (restorationReader) GetSavedCharacterHearth(context.Context, int64) (repository.GetSavedCharacterHearthRow, error) {
	panic("unexpected saved hearth query")
}
