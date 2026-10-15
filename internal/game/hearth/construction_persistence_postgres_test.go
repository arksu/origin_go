package hearth_test

import (
	"context"
	"database/sql"
	"testing"

	constt "origin/internal/const"
	"origin/internal/game/hearth"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestConstructionSaverPostgresFirstHomeAndReplacement(t *testing.T) {
	db := newHearthPostgres(t, false)
	ctx := context.Background()
	q := db.Queries()
	insertHearthCharacter(t, db, 1)
	saver := hearth.NewConstructionSaverDB(db)
	first := repository.Object{
		ID: 2000, TypeID: testHearthTypeID, Region: 1, X: 125, Y: 270, ChunkX: 1, ChunkY: 2,
		Quality: 12, Hp: sql.NullFloat64{Float64: 100, Valid: true}, CreateTick: 80, LastTick: 90,
	}
	_, err := q.GetObjectByID(ctx, first.ID)
	require.ErrorIs(t, err, sql.ErrNoRows, "construction can finish before the chunk's ordinary save")
	require.NoError(t, saver.SaveConstruction(ctx, 1, &first, nil))
	stored, err := q.GetObjectByID(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.TypeID, stored.TypeID)
	require.Equal(t, first.Quality, stored.Quality)
	require.Equal(t, first.Hp, stored.Hp)
	require.Equal(t, first.CreateTick, stored.CreateTick)
	require.Equal(t, first.LastTick, stored.LastTick)
	home, err := q.GetCharacterHearth(ctx, hearthGetParams(1))
	require.NoError(t, err)
	require.Equal(t, repository.GetCharacterHearthRow{
		CharacterID: 1, ObjectRegion: first.Region, ObjectID: first.ID, X: first.X, Y: first.Y,
	}, home)
	watermark, err := q.GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.NoError(t, err)
	require.Equal(t, sql.NullInt64{Int64: first.ID, Valid: true}, watermark.ValueLong)

	previous := first
	previous.TypeID = 16 // Converted campfire.
	previous.LastTick = 95
	second := first
	second.ID = 1000
	second.X = 430
	second.ChunkX = 4
	second.CreateTick = 95
	second.LastTick = 95
	require.NoError(t, saver.SaveConstruction(ctx, 1, &second, &previous))
	converted, err := q.GetObjectByID(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, previous.TypeID, converted.TypeID)
	require.Equal(t, previous.LastTick, converted.LastTick)
	require.Equal(t, first.ID, converted.ID)
	require.Equal(t, first.Region, converted.Region)
	require.Equal(t, first.X, converted.X)
	require.Equal(t, first.Y, converted.Y)
	require.Equal(t, first.Layer, converted.Layer)
	freshIndex := hearth.NewIndex()
	version, started := freshIndex.BeginLookup(1)
	require.True(t, started)
	home, err = q.GetCharacterHearth(ctx, hearthGetParams(1))
	require.NoError(t, err)
	require.NoError(t, freshIndex.CompleteLookup(1, version, &hearth.Home{
		Object: hearth.ObjectKey{Region: home.ObjectRegion, ID: types.EntityID(home.ObjectID)},
		X:      home.X, Y: home.Y, Layer: home.Layer,
	}))
	expectedHome := hearth.Home{
		Object: hearth.ObjectKey{Region: second.Region, ID: types.EntityID(second.ID)},
		X:      second.X, Y: second.Y, Layer: second.Layer,
	}
	snapshot := freshIndex.Snapshot(1)
	require.Equal(t, hearth.CurrentHome, snapshot.State)
	require.Equal(t, expectedHome, snapshot.Home)
	require.False(t, snapshot.Dirty)
	owner, owned := freshIndex.Owner(expectedHome.Object)
	require.True(t, owned)
	require.Equal(t, types.EntityID(1), owner)
	_, owned = freshIndex.Owner(hearth.ObjectKey{Region: first.Region, ID: types.EntityID(first.ID)})
	require.False(t, owned)
	watermark, err = q.GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, watermark.ValueLong.Int64, "replacement must not lower the saved ID watermark")
	require.Equal(t, testHearthTypeID, first.TypeID, "the writer must not mutate input records")
}

func TestConstructionSaverPostgresRollsBackWithoutLiveOwner(t *testing.T) {
	for _, deletedCharacter := range []bool{false, true} {
		name := "missing_character"
		if deletedCharacter {
			name = "deleted_character"
		}
		t.Run(name, func(t *testing.T) {
			db := newHearthPostgres(t, false)
			ctx := context.Background()
			q := db.Queries()
			insertHearthObject(t, db, 1, 10)
			if deletedCharacter {
				insertHearthCharacter(t, db, 1)
				rows, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
				_, err = db.Pool().Exec(ctx, "UPDATE character SET deleted_at = now() WHERE id = 1")
				require.NoError(t, err)
			}
			previous, err := q.GetObjectByID(ctx, 10)
			require.NoError(t, err)
			previous.TypeID = 16
			next := repository.Object{ID: 11, TypeID: testHearthTypeID, Region: 1}
			err = hearth.NewConstructionSaverDB(db).SaveConstruction(ctx, 1, &next, &previous)
			require.ErrorContains(t, err, "expected one association, updated 0")
			_, err = q.GetObjectByID(ctx, next.ID)
			require.ErrorIs(t, err, sql.ErrNoRows, "the new object insert must roll back")
			storedPrevious, err := q.GetObjectByID(ctx, previous.ID)
			require.NoError(t, err)
			require.Equal(t, testHearthTypeID, storedPrevious.TypeID, "the old conversion must roll back")
			_, err = q.GetGlobalVar(ctx, constt.LAST_USED_ID)
			require.ErrorIs(t, err, sql.ErrNoRows)
			association, err := q.GetSavedCharacterHearth(ctx, 1)
			if deletedCharacter {
				require.NoError(t, err)
				require.Equal(t, previous.ID, association.ObjectID, "the previous association survives rollback")
			} else {
				require.ErrorIs(t, err, sql.ErrNoRows)
			}
		})
	}
}
