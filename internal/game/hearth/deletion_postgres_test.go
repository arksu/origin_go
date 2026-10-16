package hearth_test

import (
	"context"
	"database/sql"
	"testing"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/game/hearth"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestDeletionPostgresSavedResolutionAndCleanup(t *testing.T) {
	for _, source := range []string{"valid", "absent", "deleted", "changed_type", "wrong_layer", "missing", "known_no_home_hidden_invalid"} {
		t.Run(source, func(t *testing.T) {
			db := newHearthPostgres(t, false)
			f := newConstructionFixture(t)
			ctx := context.Background()
			q := db.Queries()
			insertHearthCharacter(t, db, 1)
			insertHearthObject(t, db, 1, 10)
			original, err := q.GetObjectByID(ctx, 10)
			require.NoError(t, err)
			// A second object with the same numeric ID must not supply source
			// metadata or receive conversion through this region-1 association.
			insertHearthObject(t, db, 2, 10)
			if source != "absent" {
				rows, err := q.SetCharacterHearth(ctx, hearthSetParams(1, 1, 10))
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
			}
			_, err = db.Pool().Exec(ctx, "UPDATE character SET deleted_at = now() WHERE id = 1")
			require.NoError(t, err)
			switch source {
			case "deleted":
				_, err = db.Pool().Exec(ctx, "UPDATE object SET deleted_at = now() WHERE region = 1 AND id = 10")
			case "changed_type", "known_no_home_hidden_invalid":
				_, err = db.Pool().Exec(ctx, "UPDATE object SET type_id = 16 WHERE region = 1 AND id = 10")
			case "wrong_layer":
				_, err = db.Pool().Exec(ctx, "UPDATE object SET layer = 1 WHERE region = 1 AND id = 10")
			case "missing":
				// Only this isolated test schema relaxes its FK to exercise an
				// invalid historical association without inventing a source row.
				_, err = db.Pool().Exec(ctx, "ALTER TABLE character_hearth DROP CONSTRAINT character_hearth_object_fk")
				require.NoError(t, err)
				_, err = db.Pool().Exec(ctx, "DELETE FROM object WHERE region = 1 AND id = 10")
			}
			require.NoError(t, err)
			saved, err := q.GetSavedCharacterHearth(ctx, 1)
			if source == "absent" {
				require.ErrorIs(t, err, sql.ErrNoRows)
			} else {
				require.NoError(t, err, "soft-deleted owners retain their exact saved association")
				require.Equal(t, int64(1), saved.CharacterID)
				require.Equal(t, 1, saved.ObjectRegion)
				require.Equal(t, int64(10), saved.ObjectID)
				if source == "missing" {
					require.False(t, saved.TypeID.Valid)
					require.False(t, saved.X.Valid)
					require.False(t, saved.Y.Valid)
					require.False(t, saved.Layer.Valid)
					require.False(t, saved.DeletedAt.Valid)
				} else {
					require.True(t, saved.TypeID.Valid)
					require.Equal(t, sql.NullInt32{Int32: int32(original.X), Valid: true}, saved.X)
					require.Equal(t, sql.NullInt32{Int32: int32(original.Y), Valid: true}, saved.Y)
					require.Equal(t, source == "deleted", saved.DeletedAt.Valid)
				}
			}
			coord := types.WorldToChunkCoord(original.X, original.Y, constt.ChunkSize, constt.CoordPerTile)
			chunk := core.NewChunk(coord, original.Region, hearth.SurfaceLayer, constt.ChunkSize)
			chunk.SetState(types.ChunkStateInactive)
			chunk.SetRawObjects([]*repository.Object{&original})
			f.chunks.chunks[coord] = chunk
			runtime := hearth.NewRuntime(f.w, f.chunks, q, f.factory, hearth.NewConstructionSaverDB(db), f.registry, nil,
				zap.NewNop().WithOptions(zap.WithFatalHook(zapcore.WriteThenPanic)))
			if source == "known_no_home_hidden_invalid" {
				require.NoError(t, runtime.Index.LoadIfUnknown(1, nil))
			}
			for range 2 {
				require.NoError(t, runtime.CompleteDeletion(1))
			}
			require.Equal(t, hearth.Snapshot{State: hearth.NoHome, Terminal: true}, runtime.Index.Snapshot(1))
			_, err = q.GetSavedCharacterHearth(ctx, 1)
			if source == "known_no_home_hidden_invalid" {
				require.NoError(t, err, "known NoHome performs no Hearth I/O; broader repair is separate")
			} else {
				require.ErrorIs(t, err, sql.ErrNoRows)
			}
			var storedType int
			var storedDeleted sql.NullTime
			err = db.Pool().QueryRow(ctx, "SELECT type_id, deleted_at FROM object WHERE region = 1 AND id = 10").Scan(&storedType, &storedDeleted)
			if source == "missing" {
				require.ErrorIs(t, err, sql.ErrNoRows)
			} else {
				require.NoError(t, err)
				require.Equal(t, saved.DeletedAt, storedDeleted)
				wantType := testHearthTypeID
				if source == "valid" || source == "changed_type" || source == "known_no_home_hidden_invalid" {
					wantType = 16
				}
				require.Equal(t, wantType, storedType)
			}
			require.NoError(t, db.Pool().QueryRow(ctx, "SELECT type_id FROM object WHERE region = 2 AND id = 10").Scan(&storedType))
			require.Equal(t, testHearthTypeID, storedType, "same-ID object in another region is untouched")
			if source == "valid" {
				require.Len(t, f.chunks.waited, 1)
				require.Equal(t, 16, chunk.GetRawObjectByID(10).TypeID)
			} else {
				require.Empty(t, f.chunks.waited, "invalid saved source cleanup must not load a chunk")
				require.Empty(t, chunk.GetRawDirtyObjectIDs())
			}
			require.Empty(t, chunk.GetHandles())
			require.Zero(t, f.chunks.pins[coord])
		})
	}
}
