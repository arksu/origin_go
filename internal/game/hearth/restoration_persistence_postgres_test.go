package hearth_test

import (
	"context"
	"database/sql"
	"testing"

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

func loadHearthRestorationPostgresChunk(t *testing.T, q *repository.Queries) *core.Chunk {
	t.Helper()
	rows, err := q.GetObjectsByChunk(context.Background(), repository.GetObjectsByChunkParams{Region: 1})
	require.NoError(t, err)
	raw := make([]*repository.Object, len(rows))
	for i := range rows {
		raw[i] = &rows[i]
	}
	chunk := core.NewChunk(types.ChunkCoord{}, 1, hearth.SurfaceLayer, constt.ChunkSize)
	chunk.SetState(types.ChunkStatePreloaded)
	chunk.SetRawObjects(raw)
	return chunk
}

func TestHearthRestorationPostgresNormalizesBatchAndSurvivesRestart(t *testing.T) {
	db := newHearthPostgres(t, false)
	f := newConstructionFixture(t)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	ctx := context.Background()
	q := db.Queries()
	for _, character := range []int64{1, 2} {
		insertHearthCharacter(t, db, character)
	}
	for _, object := range []int64{101, 102, 103, 104} {
		insertHearthObject(t, db, 1, object)
	}
	for _, association := range []repository.SetCharacterHearthParams{hearthSetParams(1, 1, 101), hearthSetParams(2, 1, 102)} {
		rows, err := q.SetCharacterHearth(ctx, association)
		require.NoError(t, err)
		require.EqualValues(t, 1, rows)
	}
	_, err := db.Pool().Exec(ctx, "UPDATE character SET deleted_at = now() WHERE id = 2")
	require.NoError(t, err)
	_, err = db.Pool().Exec(ctx, `UPDATE object SET hp = 7, quality = 17, heading = 90, create_tick = 7, last_tick = 10, data = '{"old":"hearth"}'`)
	require.NoError(t, err)

	chunk := loadHearthRestorationPostgresChunk(t, q)
	before := make(map[int64]repository.Object)
	for _, raw := range chunk.GetRawObjects() {
		before[raw.ID] = *raw
	}
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).Tick = 123
	reads, saves := 0, 0
	saver := hearth.NewConstructionSaverDB(db)
	logger := zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic))
	runtime := hearth.NewRuntime(w, nil, restorationReader(func(ctx context.Context, params repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
		reads++
		require.ElementsMatch(t, []int64{101, 102, 103, 104}, params.ObjectIds)
		return q.GetCharacterHearthOwners(ctx, params)
	}), nil, restorationSaver(func(ctx context.Context, converted []*repository.Object) error {
		saves++
		require.Len(t, converted, 3, "the deleted owner's home and both unowned hearths are normalized together")
		for _, raw := range converted {
			require.Equal(t, f.result.DefID, chunk.GetRawObjectByID(types.EntityID(raw.ID)).TypeID, "cache publication must follow commit")
		}
		return saver.SaveOrphans(ctx, converted)
	}), nil, nil, logger)
	require.NoError(t, runtime.PrepareChunkActivation(chunk))
	require.Equal(t, 1, reads)
	require.Equal(t, 1, saves)
	owner, owned := runtime.Index.Owner(hearth.ObjectKey{Region: 1, ID: 101})
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	require.False(t, w.Alive(w.GetHandleByEntityID(owner)), "ownership hydration must work for an offline character")

	for _, id := range []int64{101, 102, 103, 104} {
		stored, err := q.GetObjectByID(ctx, id)
		require.NoError(t, err)
		cached := chunk.GetRawObjectByID(types.EntityID(id))
		require.NotNil(t, cached)
		if id == 101 {
			require.Equal(t, f.result.DefID, stored.TypeID)
			require.Equal(t, before[id].Hp, stored.Hp)
			require.Equal(t, before[id].LastTick, stored.LastTick)
		} else {
			require.Equal(t, campfire.DefID, stored.TypeID)
			require.Equal(t, campfire.DefID, cached.TypeID)
			require.Equal(t, sql.NullFloat64{Float64: float64(campfire.HP), Valid: true}, stored.Hp)
			require.False(t, stored.Data.Valid)
			require.Equal(t, int64(123), stored.LastTick)
			_, owned := runtime.Index.Owner(hearth.ObjectKey{Region: 1, ID: types.EntityID(id)})
			require.False(t, owned)
		}
		require.Equal(t, before[id].Region, stored.Region)
		require.Equal(t, before[id].X, stored.X)
		require.Equal(t, before[id].Y, stored.Y)
		require.Equal(t, before[id].Heading, stored.Heading)
		require.Equal(t, before[id].Quality, stored.Quality)
		require.Equal(t, before[id].CreateTick, stored.CreateTick)
	}
	_, err = q.GetCharacterHearth(ctx, hearthGetParams(2))
	require.ErrorIs(t, err, sql.ErrNoRows)

	freshChunk := loadHearthRestorationPostgresChunk(t, q)
	freshWorld := ecs.NewWorldForTesting()
	restartReads := 0
	restarted := hearth.NewRuntime(freshWorld, nil, restorationReader(func(ctx context.Context, params repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error) {
		restartReads++
		require.Equal(t, []int64{101}, params.ObjectIds, "durable campfires must not return as Hearth candidates after restart")
		return q.GetCharacterHearthOwners(ctx, params)
	}), nil, restorationSaver(func(context.Context, []*repository.Object) error {
		t.Fatal("restart must not repeat committed conversions")
		return nil
	}), nil, nil, logger)
	require.NoError(t, restarted.PrepareChunkActivation(freshChunk))
	require.Equal(t, 1, restartReads)
	require.Equal(t, runtime.Index.Snapshot(1), restarted.Index.Snapshot(1))
	for _, id := range []types.EntityID{102, 103, 104} {
		require.Equal(t, campfire.DefID, freshChunk.GetRawObjectByID(id).TypeID)
	}
}

func TestHearthRestorationPostgresOrphanBatchRollsBackOnLaterFailure(t *testing.T) {
	db := newHearthPostgres(t, false)
	newConstructionFixture(t)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	ctx := context.Background()
	q := db.Queries()
	var originals []repository.Object
	var converted []*repository.Object
	for _, id := range []int64{101, 102} {
		insertHearthObject(t, db, 1, id)
		original, err := q.GetObjectByID(ctx, id)
		require.NoError(t, err)
		originals = append(originals, original)
		copy := original
		copy.TypeID = campfire.DefID
		copy.Hp = sql.NullFloat64{Float64: float64(campfire.HP), Valid: true}
		copy.Data = pqtype.NullRawMessage{}
		copy.LastTick = 123
		converted = append(converted, &copy)
	}
	converted[1].Quality = -1 // The second SQL write fails the schema's quality constraint.
	err := hearth.NewConstructionSaverDB(db).SaveOrphans(ctx, converted)
	requireHearthPostgresError(t, err, "23514")
	for _, original := range originals {
		stored, err := q.GetObjectByID(ctx, original.ID)
		require.NoError(t, err)
		require.Equal(t, original, stored, "a later failed conversion must roll back every earlier row")
	}
}
