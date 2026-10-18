package world

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	_const "origin/internal/const"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
)

func wildWeedRefillObject(id, spotID, runtime int64, x int) repository.Object {
	return repository.Object{
		ID: id, TypeID: 41, Region: 1, X: x, Y: 6, Quality: 17,
		Hp: sql.NullFloat64{Float64: 100, Valid: true},
		Data: pqtype.NullRawMessage{Valid: true, RawMessage: json.RawMessage(fmt.Sprintf(
			`{"v":1,"behaviors":{"wild_weed":{"spot_id":%d,"stage":0,"next_growth_runtime_seconds":%d}}}`,
			spotID, runtime+8*60*60))},
	}
}

func insertWildWeedRefillChunk(t *testing.T, db *persistence.Postgres) {
	t.Helper()
	_, err := db.Queries().UpsertChunk(t.Context(), repository.UpsertChunkParams{
		Region: 1, TilesData: []byte("terrain snapshot"), LastTick: 73,
		EntityCount: sql.NullInt32{Int32: 7, Valid: true}, Version: 91,
	})
	require.NoError(t, err)
}

func TestWildWeedRefillPostgresAtomicStemInsertionAndLostCommitRetry(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	spotID := insertWildWeedPersistenceFixture(t, db,
		`{"unknown":{"n":9007199254740993},"www":{"note":"keep","objects":{"777":{"x":54,"y":6}},"last_check_runtime_seconds":100}}`, 777)
	insertWildWeedRefillChunk(t, db)
	const first int64 = 9007199254740993
	write := WildWeedRefillWrite{SpotID: spotID, Region: 1, RuntimeSeconds: 1900, CheckRuntimeSeconds: 1900,
		Target: 3, Objects: []repository.Object{wildWeedRefillObject(first, spotID, 1900, 6), wildWeedRefillObject(first+1, spotID, 1901, 30)},
		MaxAllocatedID: types.EntityID(first + 1)}
	commit, err := store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.EqualValues(t, 3, commit.Snapshot.Revision)
	require.EqualValues(t, 1900, commit.Snapshot.LastRuntimeSeconds)
	require.JSONEq(t, fmt.Sprintf(`{"unknown":{"n":9007199254740993},"www":{"note":"keep","objects":{"777":{"x":54,"y":6},"%d":{"x":6,"y":6},"%d":{"x":30,"y":6}}}}`, first, first+1), string(commit.Snapshot.State))
	for _, object := range write.Objects {
		loaded, err := db.Queries().GetObjectByID(t.Context(), object.ID)
		require.NoError(t, err)
		require.Equal(t, object.TypeID, loaded.TypeID)
		require.Equal(t, object.Quality, loaded.Quality)
		require.Equal(t, object.Hp, loaded.Hp)
		require.JSONEq(t, string(object.Data.RawMessage), string(loaded.Data.RawMessage))
	}
	chunk, err := db.Queries().GetChunk(t.Context(), repository.GetChunkParams{Region: 1})
	require.NoError(t, err)
	require.EqualValues(t, 9, chunk.EntityCount.Int32)
	require.Equal(t, []byte("terrain snapshot"), chunk.TilesData)
	require.EqualValues(t, 73, chunk.LastTick)
	require.EqualValues(t, 91, chunk.Version)
	require.Equal(t, first+1, db.GetGlobalVarLong(t.Context(), _const.LAST_USED_ID))
	// The cursor was removed on filling the spot; immutable IDs still identify
	// an ambiguously acknowledged commit without reinserting or recounting.
	retry, err := store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.Equal(t, commit.Snapshot.Revision, retry.Snapshot.Revision)
	require.JSONEq(t, string(commit.Snapshot.State), string(retry.Snapshot.State))
	chunk, err = db.Queries().GetChunk(t.Context(), repository.GetChunkParams{Region: 1})
	require.NoError(t, err)
	require.EqualValues(t, 9, chunk.EntityCount.Int32)
	// A later removal must survive an older insertion retry as well.
	persister := NewDroppedItemPersisterDB(db, nil)
	deleted, err := persister.DeleteWildWeedObject(t.Context(), 1, WildWeedObjectDeletion{SpotID: spotID, EntityID: 777, RuntimeSeconds: 2000})
	require.NoError(t, err)
	retry, err = store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.Equal(t, deleted.Snapshot.Revision, retry.Snapshot.Revision)
	require.JSONEq(t, string(deleted.Snapshot.State), string(retry.Snapshot.State))
}

func TestWildWeedRefillPostgresEmptyChecksAndMarkerCleanup(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	spotID := insertWildWeedPersistenceFixture(t, db, `{"www":{"objects":{},"note":"keep"},"unknown":9007199254740993}`)
	write := WildWeedRefillWrite{SpotID: spotID, Region: 1, RuntimeSeconds: 1000, CheckRuntimeSeconds: 1000, Target: 2}
	first, err := store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.JSONEq(t, `{"www":{"objects":{},"note":"keep","last_check_runtime_seconds":1000},"unknown":9007199254740993}`, string(first.Snapshot.State))
	retry, err := store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.Equal(t, first.Snapshot.Revision, retry.Snapshot.Revision)
	write.RuntimeSeconds, write.CheckRuntimeSeconds = 1900, 1900
	next, err := store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.Equal(t, first.Snapshot.Revision+1, next.Snapshot.Revision)
	// An older empty check cannot move the marker backwards.
	write.RuntimeSeconds, write.CheckRuntimeSeconds = 1000, 1000
	retry, err = store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.Equal(t, next.Snapshot.Revision, retry.Snapshot.Revision)
	write.Target = 0
	cleaned, err := store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.Equal(t, next.Snapshot.Revision+1, cleaned.Snapshot.Revision)
	require.EqualValues(t, 1900, cleaned.Snapshot.LastRuntimeSeconds)
	require.JSONEq(t, `{"www":{"objects":{},"note":"keep"},"unknown":9007199254740993}`, string(cleaned.Snapshot.State))
	retry, err = store.RefillWildWeeds(t.Context(), write)
	require.NoError(t, err)
	require.Equal(t, cleaned.Snapshot.Revision, retry.Snapshot.Revision)
}

func TestWildWeedRefillPostgresRollbackAndIdentityCollision(t *testing.T) {
	for _, collision := range []bool{false, true} {
		t.Run(fmt.Sprint(collision), func(t *testing.T) {
			db, store := newSpotStorePostgres(t)
			spotID := insertWildWeedPersistenceFixture(t, db, `{"www":{"objects":{},"last_check_runtime_seconds":100}}`)
			if collision {
				insertWildWeedRefillChunk(t, db)
				require.NoError(t, db.Queries().UpsertObject(t.Context(), objectHealthUpsertParams(10002, sql.NullFloat64{Float64: 20, Valid: true})))
			}
			write := WildWeedRefillWrite{SpotID: spotID, Region: 1, RuntimeSeconds: 1000, CheckRuntimeSeconds: 1000, Target: 3,
				Objects: []repository.Object{wildWeedRefillObject(10001, spotID, 1000, 6), wildWeedRefillObject(10002, spotID, 1000, 30)}, MaxAllocatedID: 10002}
			_, err := store.RefillWildWeeds(t.Context(), write)
			require.Error(t, err)
			_, err = db.Queries().GetObjectByID(t.Context(), 10001)
			require.ErrorIs(t, err, sql.ErrNoRows, "a later error rolls back earlier inserts")
			spots, err := store.LoadSpots(t.Context(), 1)
			require.NoError(t, err)
			require.EqualValues(t, 2, spots[0].Revision)
			require.JSONEq(t, `{"www":{"objects":{},"last_check_runtime_seconds":100}}`, string(spots[0].State))
			require.Zero(t, db.GetGlobalVarLong(t.Context(), _const.LAST_USED_ID))
			if collision {
				loaded, err := db.Queries().GetObjectByID(t.Context(), 10002)
				require.NoError(t, err)
				require.Equal(t, 20.0, loaded.Hp.Float64, "identity collision must never overwrite the existing row")
				chunk, err := db.Queries().GetChunk(t.Context(), repository.GetChunkParams{Region: 1})
				require.NoError(t, err)
				require.EqualValues(t, 7, chunk.EntityCount.Int32)
			}
		})
	}
}

func TestWildWeedRefillPostgresConcurrentDestructionPreservesDelta(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	spotID := insertWildWeedPersistenceFixture(t, db, `{"www":{"objects":{"10001":{"x":6,"y":6}},"last_check_runtime_seconds":100}}`, 10001)
	insertWildWeedRefillChunk(t, db)
	write := WildWeedRefillWrite{SpotID: spotID, Region: 1, RuntimeSeconds: 1000, CheckRuntimeSeconds: 1000, Target: 2,
		Objects: []repository.Object{wildWeedRefillObject(10002, spotID, 1000, 30)}, MaxAllocatedID: 10002}
	start := make(chan struct{})
	errors := make(chan error, 2)
	go func() {
		<-start
		_, err := store.RefillWildWeeds(t.Context(), write)
		errors <- err
	}()
	go func() {
		<-start
		_, err := NewDroppedItemPersisterDB(db, nil).DeleteWildWeedObject(t.Context(), 1,
			WildWeedObjectDeletion{SpotID: spotID, EntityID: 10001, RuntimeSeconds: 1500})
		errors <- err
	}()
	close(start)
	require.NoError(t, <-errors)
	require.NoError(t, <-errors)
	spots, err := store.LoadSpots(t.Context(), 1)
	require.NoError(t, err)
	require.EqualValues(t, 4, spots[0].Revision)
	require.EqualValues(t, 1500, spots[0].LastRuntimeSeconds)
	var state struct {
		WWW struct {
			Objects map[string]wildWeedRefillPosition `json:"objects"`
			Last    int64                             `json:"last_check_runtime_seconds"`
		} `json:"www"`
	}
	require.NoError(t, json.Unmarshal(spots[0].State, &state))
	require.Equal(t, map[string]wildWeedRefillPosition{"10002": {X: 30, Y: 6}}, state.WWW.Objects)
	require.Contains(t, []int64{1000, 1500}, state.WWW.Last)
	_, err = db.Queries().GetObjectByID(t.Context(), 10001)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.Queries().GetObjectByID(t.Context(), 10002)
	require.NoError(t, err)
}

func TestWildWeedRefillPostgresLateFailureRollsBackEntireWrite(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	spotID := insertWildWeedPersistenceFixture(t, db, `{"www":{"objects":{},"last_check_runtime_seconds":100}}`)
	insertWildWeedRefillChunk(t, db)
	_, err := db.Pool().Exec(t.Context(), "UPDATE spot SET revision = $1 WHERE id = $2", int64(math.MaxInt64), spotID)
	require.NoError(t, err)
	write := WildWeedRefillWrite{SpotID: spotID, Region: 1, RuntimeSeconds: 1000, CheckRuntimeSeconds: 1000, Target: 2,
		Objects: []repository.Object{wildWeedRefillObject(10001, spotID, 1000, 6)}, MaxAllocatedID: 10001}
	_, err = store.RefillWildWeeds(t.Context(), write)
	require.Error(t, err, "exhausted revision fails after rows and chunk counters were changed inside the transaction")
	_, err = db.Queries().GetObjectByID(t.Context(), 10001)
	require.ErrorIs(t, err, sql.ErrNoRows)
	chunk, err := db.Queries().GetChunk(t.Context(), repository.GetChunkParams{Region: 1})
	require.NoError(t, err)
	require.EqualValues(t, 7, chunk.EntityCount.Int32)
	spots, err := store.LoadSpots(t.Context(), 1)
	require.NoError(t, err)
	require.EqualValues(t, math.MaxInt64, spots[0].Revision)
	require.JSONEq(t, `{"www":{"objects":{},"last_check_runtime_seconds":100}}`, string(spots[0].State))
	require.Zero(t, db.GetGlobalVarLong(t.Context(), _const.LAST_USED_ID))
}

func TestWildWeedRefillRejectsInvalidWrite(t *testing.T) {
	valid := WildWeedRefillWrite{SpotID: 1, Region: 1, RuntimeSeconds: 1000, CheckRuntimeSeconds: 1000, Target: 2,
		Objects: []repository.Object{wildWeedRefillObject(10001, 1, 1000, 6)}, MaxAllocatedID: 10001}
	for _, test := range []struct {
		name   string
		change func(*WildWeedRefillWrite)
	}{
		{"missing spot", func(w *WildWeedRefillWrite) { w.SpotID = 0 }},
		{"invalid region", func(w *WildWeedRefillWrite) { w.Region = 0 }},
		{"negative runtime", func(w *WildWeedRefillWrite) { w.RuntimeSeconds = -1 }},
		{"future check", func(w *WildWeedRefillWrite) { w.CheckRuntimeSeconds++ }},
		{"negative target", func(w *WildWeedRefillWrite) { w.Target = -1 }},
		{"excess objects", func(w *WildWeedRefillWrite) { w.Target = 0 }},
		{"lost checkpoint", func(w *WildWeedRefillWrite) { w.MaxAllocatedID-- }},
		{"wrong region", func(w *WildWeedRefillWrite) { w.Objects[0].Region++ }},
		{"wrong layer", func(w *WildWeedRefillWrite) { w.Objects[0].Layer++ }},
		{"wrong chunk", func(w *WildWeedRefillWrite) { w.Objects[0].ChunkX++ }},
		{"missing HP", func(w *WildWeedRefillWrite) { w.Objects[0].Hp.Valid = false }},
		{"invalid HP", func(w *WildWeedRefillWrite) { w.Objects[0].Hp.Float64 = math.NaN() }},
		{"negative quality", func(w *WildWeedRefillWrite) { w.Objects[0].Quality = -1 }},
		{"mature plant", func(w *WildWeedRefillWrite) {
			w.Objects[0].Data.RawMessage = json.RawMessage(`{"v":1,"behaviors":{"wild_weed":{"spot_id":1,"stage":3}}}`)
		}},
		{"expired stem", func(w *WildWeedRefillWrite) {
			w.Objects[0].Data.RawMessage = json.RawMessage(`{"v":1,"behaviors":{"wild_weed":{"spot_id":1,"stage":0,"next_growth_runtime_seconds":1000}}}`)
		}},
		{"duplicate ID", func(w *WildWeedRefillWrite) { w.Objects = append(w.Objects, w.Objects[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			write := valid
			write.Objects = append([]repository.Object(nil), valid.Objects...)
			test.change(&write)
			_, err := validateWildWeedRefill(write)
			require.Error(t, err)
		})
	}
	_, err := validateWildWeedRefill(valid)
	require.NoError(t, err)
}
