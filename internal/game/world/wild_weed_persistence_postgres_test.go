package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"origin/internal/game/inventory"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func insertWildWeedPersistenceFixture(t *testing.T, db *persistence.Postgres, state string, ids ...types.EntityID) int64 {
	t.Helper()
	ctx := t.Context()
	insertSpotStoreFixture(t, db, 1, "www")
	spots, err := db.Queries().LoadSpotsByRegion(ctx, 1)
	require.NoError(t, err)
	id := spots[len(spots)-1].ID
	rows, err := db.Queries().SaveSpotState(ctx, repository.SaveSpotStateParams{
		ID: id, Region: 1, ExpectedRevision: 1, State: json.RawMessage(state), LastRuntimeSeconds: 100,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	for _, entityID := range ids {
		require.NoError(t, db.Queries().UpsertObject(ctx, objectHealthUpsertParams(int64(entityID), sql.NullFloat64{Float64: 100, Valid: true})))
		_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: int64(entityID), Data: []byte(`{"items":[]}`), Version: 1})
		require.NoError(t, err)
	}
	return id
}

func TestWildWeedPostgresConcurrentDeletionAndIdempotentRetry(t *testing.T) {
	db, _ := newSpotStorePostgres(t)
	ctx := context.Background()
	const first types.EntityID = 9007199254740993
	const second types.EntityID = 9007199254740994
	state := fmt.Sprintf(`{"unknown":{"n":9007199254740993},"www":{"note":"preserved","objects":{"%d":{"x":1,"y":2},"%d":{"x":3,"y":4}}}}`, first, second)
	spotID := insertWildWeedPersistenceFixture(t, db, state, first, second)
	persister := NewDroppedItemPersisterDB(db, nil)
	start := make(chan struct{})
	type result struct {
		commit SpotStateCommit
		err    error
	}
	results := make(chan result, 2)
	for i, id := range []types.EntityID{first, second} {
		go func(id types.EntityID, runtime int64) {
			<-start
			commit, err := persister.DeleteWildWeedObject(ctx, 1, WildWeedObjectDeletion{SpotID: spotID, EntityID: id, RuntimeSeconds: runtime})
			results <- result{commit, err}
		}(id, int64(300-i*100))
	}
	close(start)
	for range 2 {
		r := <-results
		require.NoError(t, r.err)
		require.Equal(t, spotID, r.commit.SpotID)
	}
	spots, err := db.Queries().LoadSpotsByRegion(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 4, spots[0].Revision)
	require.EqualValues(t, 300, spots[0].LastRuntimeSeconds)
	var manifest struct {
		WWW struct {
			LastCheck int64 `json:"last_check_runtime_seconds"`
		} `json:"www"`
	}
	require.NoError(t, json.Unmarshal(spots[0].State, &manifest))
	require.Contains(t, []int64{200, 300}, manifest.WWW.LastCheck, "the first actual deletion initializes the marker; the next deletion preserves it")
	require.JSONEq(t, fmt.Sprintf(`{"unknown":{"n":9007199254740993},"www":{"note":"preserved","objects":{},"last_check_runtime_seconds":%d}}`, manifest.WWW.LastCheck), string(spots[0].State))
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(spots[0].State, &document))
	var unknown map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(document["unknown"], &unknown))
	require.Equal(t, "9007199254740993", string(unknown["n"]))
	for _, id := range []types.EntityID{first, second} {
		_, err := db.Queries().GetObjectByID(ctx, int64(id))
		require.ErrorIs(t, err, sql.ErrNoRows)
		inventories, err := db.Queries().GetInventoriesByOwner(ctx, int64(id))
		require.NoError(t, err)
		require.Empty(t, inventories)
		// A retry must return the newest manifest, including the other deletion,
		// while preserving its revision and runtime even with a later timestamp.
		commit, err := persister.DeleteWildWeedObject(ctx, 1, WildWeedObjectDeletion{SpotID: spotID, EntityID: id, RuntimeSeconds: 999})
		require.NoError(t, err)
		require.EqualValues(t, 4, commit.Snapshot.Revision)
		require.EqualValues(t, 300, commit.Snapshot.LastRuntimeSeconds)
		require.JSONEq(t, string(spots[0].State), string(commit.Snapshot.State))
	}
}

func TestWildWeedPostgresStreamingDeletionRollsBackManifestAndDrops(t *testing.T) {
	db, _ := newSpotStorePostgres(t)
	const source types.EntityID = 9910
	spotID := insertWildWeedPersistenceFixture(t, db, `{"www":{"objects":{"9910":{"x":1,"y":2}}}}`, source)
	persister := NewDroppedItemPersisterDB(db, nil)
	want := errors.New("iterator failed")
	calls := 0
	_, err := persister.ReplaceWildWeedWithDroppedItems(t.Context(), 1, WildWeedObjectDeletion{SpotID: spotID, EntityID: source, RuntimeSeconds: 200}, 9911,
		func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
			calls++
			if calls == 1 {
				return append(dst, destructionRecord(9911)), nil
			}
			return nil, want
		})
	require.ErrorIs(t, err, want)
	_, err = db.Queries().GetObjectByID(t.Context(), int64(source))
	require.NoError(t, err)
	_, err = db.Queries().GetObjectByID(t.Context(), 9911)
	require.ErrorIs(t, err, sql.ErrNoRows)
	spots, err := db.Queries().LoadSpotsByRegion(t.Context(), 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, spots[0].Revision)
	require.JSONEq(t, `{"www":{"objects":{"9910":{"x":1,"y":2}}}}`, string(spots[0].State))

	// The same existing streaming transaction commits the successful replacement.
	calls = 0
	commit, err := persister.ReplaceWildWeedWithDroppedItems(t.Context(), 1, WildWeedObjectDeletion{SpotID: spotID, EntityID: source, RuntimeSeconds: 200}, 9911,
		func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
			calls++
			if calls == 1 {
				return append(dst, destructionRecord(9911)), nil
			}
			return dst, nil
		})
	require.NoError(t, err)
	require.EqualValues(t, 3, commit.Snapshot.Revision)
	require.JSONEq(t, `{"www":{"objects":{},"last_check_runtime_seconds":200}}`, string(commit.Snapshot.State))
	_, err = db.Queries().GetObjectByID(t.Context(), int64(source))
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.Queries().GetObjectByID(t.Context(), 9911)
	require.NoError(t, err)
}

func TestWildWeedPostgresRejectsInvalidSpotWithoutDeletingObject(t *testing.T) {
	for _, state := range []string{`{}`, `{"www":null}`, `{"www":{"objects":[]}}`, `{"www":{"objects":null}}`} {
		t.Run(state, func(t *testing.T) {
			db, _ := newSpotStorePostgres(t)
			const source types.EntityID = 9920
			spotID := insertWildWeedPersistenceFixture(t, db, state, source)
			persister := NewDroppedItemPersisterDB(db, nil)
			for _, deletion := range []WildWeedObjectDeletion{
				{SpotID: spotID, EntityID: source, RuntimeSeconds: 200},
				{SpotID: spotID + 100, EntityID: source, RuntimeSeconds: 200},
			} {
				_, err := persister.DeleteWildWeedObject(t.Context(), 1, deletion)
				require.Error(t, err)
				_, err = db.Queries().GetObjectByID(t.Context(), int64(source))
				require.NoError(t, err, "source deletion must roll back with the failed manifest update")
				inventories, err := db.Queries().GetInventoriesByOwner(t.Context(), int64(source))
				require.NoError(t, err)
				require.Len(t, inventories, 1)
			}
		})
	}
}
