package hearth_test

import (
	"context"
	"database/sql"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/game/hearth"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestTerminalSaverPostgresConversionAndRemovalSurviveRestart(t *testing.T) {
	db := newHearthPostgres(t, false)
	newConstructionFixture(t)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	insertHearthObject(t, db, 1, 10)
	rows, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	_, err = db.Pool().Exec(ctx, `UPDATE object SET hp = 7, quality = 17, heading = 90, create_tick = 7, last_tick = 10, data = '{"old":"hearth"}' WHERE region = 1 AND id = 10`)
	require.NoError(t, err)
	original, err := q.GetObjectByID(ctx, 10)
	require.NoError(t, err)
	converted := original
	converted.TypeID = campfire.DefID
	converted.Hp = sql.NullFloat64{Float64: float64(campfire.HP), Valid: true}
	converted.Data = pqtype.NullRawMessage{}
	converted.LastTick = 123
	key := hearth.ObjectKey{Region: original.Region, ID: types.EntityID(original.ID)}
	saver := hearth.NewConstructionSaverDB(db)
	require.NoError(t, saver.SaveTerminal(ctx, 1, key, &converted))
	stored, err := q.GetObjectByID(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, converted.TypeID, stored.TypeID)
	require.Equal(t, converted.Hp, stored.Hp)
	require.Equal(t, converted.LastTick, stored.LastTick)
	require.False(t, stored.Data.Valid)
	require.Equal(t, original.ID, stored.ID)
	require.Equal(t, original.Region, stored.Region)
	require.Equal(t, original.X, stored.X)
	require.Equal(t, original.Y, stored.Y)
	require.Equal(t, original.Layer, stored.Layer)
	require.Equal(t, original.Heading, stored.Heading)
	require.Equal(t, original.Quality, stored.Quality)
	require.Equal(t, original.CreateTick, stored.CreateTick)
	_, err = q.GetSavedCharacterHearth(ctx, 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = q.GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "terminal conversion does not allocate an object ID")
	require.NoError(t, saver.SaveTerminal(ctx, 1, key, nil), "removing an already absent association succeeds")

	restarted := hearth.NewRuntime(ecs.NewWorldForTesting(), nil, q, nil, restorationSaver(func(context.Context, []*repository.Object) error {
		t.Fatal("restart must not repeat a committed terminal conversion")
		return nil
	}), nil, nil, zap.NewNop())
	require.NoError(t, restarted.LoadHome(1, testHearthTypeID))
	require.Equal(t, hearth.NoHome, restarted.Index.Snapshot(1).State)
	_, owned := restarted.Index.Owner(key)
	require.False(t, owned)
	chunk := loadHearthRestorationPostgresChunk(t, q)
	require.NoError(t, restarted.PrepareChunkActivation(chunk))
	require.Equal(t, campfire.DefID, chunk.GetRawObjectByID(key.ID).TypeID)
}

func TestTerminalSaverPostgresRemovalFailureRollsBackConversion(t *testing.T) {
	db := newHearthPostgres(t, false)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	insertHearthObject(t, db, 1, 10)
	rows, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	original, err := q.GetObjectByID(ctx, 10)
	require.NoError(t, err)
	// Fail the second write inside this isolated test schema, after the conversion.
	_, err = db.Pool().Exec(ctx, `
		CREATE FUNCTION reject_terminal_hearth_removal() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'terminal association removal failed';
		END;
		$$;
		CREATE TRIGGER reject_terminal_hearth_removal BEFORE DELETE ON character_hearth
		FOR EACH ROW EXECUTE FUNCTION reject_terminal_hearth_removal();
	`)
	require.NoError(t, err)
	converted := original
	converted.TypeID = 16
	converted.LastTick = 123
	err = hearth.NewConstructionSaverDB(db).SaveTerminal(ctx, 1, hearth.ObjectKey{Region: 1, ID: 10}, &converted)
	requireHearthPostgresError(t, err, "P0001")
	require.ErrorContains(t, err, "remove terminal hearth owner")
	stored, err := q.GetObjectByID(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, original, stored, "association failure must roll back the preceding object upsert")
	association, err := q.GetSavedCharacterHearth(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, repository.CharacterHearth{CharacterID: 1, ObjectRegion: 1, ObjectID: 10}, association)
}

func TestTerminalSaverPostgresCleanupDoesNotReviveDeletedSource(t *testing.T) {
	db := newHearthPostgres(t, false)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	insertHearthObject(t, db, 1, 10)
	rows, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	_, err = db.Pool().Exec(ctx, "UPDATE character SET deleted_at = now() WHERE id = 1")
	require.NoError(t, err)
	_, err = db.Pool().Exec(ctx, "UPDATE object SET deleted_at = now() WHERE region = 1 AND id = 10")
	require.NoError(t, err)
	var deletedAt sql.NullTime
	require.NoError(t, db.Pool().QueryRow(ctx, "SELECT deleted_at FROM object WHERE region = 1 AND id = 10").Scan(&deletedAt))
	require.True(t, deletedAt.Valid)
	saver := hearth.NewConstructionSaverDB(db)
	for range 2 {
		require.NoError(t, saver.SaveTerminal(ctx, 1, hearth.ObjectKey{Region: 1, ID: 10}, nil))
		_, err = q.GetSavedCharacterHearth(ctx, 1)
		require.ErrorIs(t, err, sql.ErrNoRows)
		_, err = q.GetObjectByID(ctx, 10)
		require.ErrorIs(t, err, sql.ErrNoRows, "cleanup must not revive a soft-deleted source")
		var storedType int
		var storedDeletedAt sql.NullTime
		require.NoError(t, db.Pool().QueryRow(ctx, "SELECT type_id, deleted_at FROM object WHERE region = 1 AND id = 10").Scan(&storedType, &storedDeletedAt))
		require.Equal(t, testHearthTypeID, storedType)
		require.Equal(t, deletedAt, storedDeletedAt)
	}
}
