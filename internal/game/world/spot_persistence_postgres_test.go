package world

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"

	"github.com/stretchr/testify/require"
)

func newSpotStorePostgres(t *testing.T) (*persistence.Postgres, *SpotStoreDB) {
	t.Helper()
	db := testutil.NewPostgres(t, "ORIGIN_SPOTS_TEST_DSN", filepath.Join("..", "..", "..", "migrations", "schema.sql"))
	return db, NewSpotStoreDB(db)
}

func insertSpotStoreFixture(t *testing.T, db *persistence.Postgres, region int, types ...string) {
	t.Helper()
	params := repository.InsertSpotsParams{Region: region, LastRuntimeSeconds: 100}
	for _, spotType := range types {
		params.SpotTypes = append(params.SpotTypes, spotType)
		params.DistrictXs = append(params.DistrictXs, 0)
		params.DistrictYs = append(params.DistrictYs, 0)
		params.CenterXs = append(params.CenterXs, -50)
		params.CenterYs = append(params.CenterYs, 200)
		params.Radii = append(params.Radii, 64)
		params.PeakQualities = append(params.PeakQualities, 37)
	}
	require.NoError(t, db.Queries().InsertSpots(context.Background(), params))
}

func TestSpotStorePostgresLoadAllTypesOrderedByID(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	ctx := context.Background()
	empty, err := store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	require.Empty(t, empty)
	insertSpotStoreFixture(t, db, 1, "water", "www", "clay", "soil", "sand")
	insertSpotStoreFixture(t, db, 2, "www")
	rows, err := store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	require.Len(t, rows, 5)
	for i, spotType := range []string{"water", "www", "clay", "soil", "sand"} {
		require.Equal(t, spotType, rows[i].SpotType)
		require.Equal(t, 1, rows[i].Region)
		require.Equal(t, -50, rows[i].CenterX)
		require.EqualValues(t, 1, rows[i].Revision)
		require.JSONEq(t, `{}`, string(rows[i].State))
		if i > 0 {
			require.Greater(t, rows[i].ID, rows[i-1].ID)
		}
	}
}

func TestSpotStorePostgresCASAndIdempotentRetry(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	ctx := context.Background()
	insertSpotStoreFixture(t, db, 1, "www")
	rows, err := store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	write := SpotStateWrite{
		ID: rows[0].ID, Region: 1, ExpectedRevision: 1, RuntimeSeconds: 200,
		State: json.RawMessage(`{"unknown":{"n":9007199254740993},"fill":{"1,2":{"cursor":10,"done":false}},"recoveries":{"9223372036854775807":108200}}`),
	}
	require.NoError(t, store.SaveSpotState(ctx, write))
	// Simulate a caller that lost the first result and retries the exact snapshot.
	require.NoError(t, store.SaveSpotState(ctx, write))
	// PostgreSQL JSONB equality, rather than byte equality, resolves the retry.
	write.State = json.RawMessage(`{ "recoveries": {"9223372036854775807":108200.0}, "fill":{"1,2":{"done":false,"cursor":10}}, "unknown":{"n":9007199254740993} }`)
	require.NoError(t, store.SaveSpotState(ctx, write))
	rows, err = store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, rows[0].Revision, "retries must not increment the revision")
	require.EqualValues(t, 200, rows[0].LastRuntimeSeconds)
	var loaded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rows[0].State, &loaded))
	var unknown map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(loaded["unknown"], &unknown))
	require.Equal(t, "9007199254740993", string(unknown["n"]), "JSON integer must not pass through float64")
	require.JSONEq(t, `{"9223372036854775807":108200.0}`, string(loaded["recoveries"]))

	for _, test := range []struct {
		name   string
		change func(*SpotStateWrite)
	}{
		{"different state", func(w *SpotStateWrite) { w.State = json.RawMessage(`{"other":true}`) }},
		{"different runtime", func(w *SpotStateWrite) { w.RuntimeSeconds++ }},
		{"future revision", func(w *SpotStateWrite) { w.ExpectedRevision = 3 }},
		{"different region", func(w *SpotStateWrite) { w.Region = 2 }},
		{"missing spot", func(w *SpotStateWrite) { w.ID++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempt := write
			test.change(&attempt)
			require.ErrorIs(t, store.SaveSpotState(ctx, attempt), ErrSpotRevisionConflict)
		})
	}
	write.ExpectedRevision = 2
	write.RuntimeSeconds = 300
	require.NoError(t, store.SaveSpotState(ctx, write))
	write.ExpectedRevision = 1
	require.ErrorIs(t, store.SaveSpotState(ctx, write), ErrSpotRevisionConflict, "an older writer cannot overwrite a newer revision")
	rows, err = store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 3, rows[0].Revision)
	require.EqualValues(t, 300, rows[0].LastRuntimeSeconds)
}

func TestSpotStorePostgresConcurrentWriters(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	ctx := context.Background()
	insertSpotStoreFixture(t, db, 1, "www")
	rows, err := store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, state := range []string{`{"writer":1}`, `{"writer":2}`} {
		go func(state string) {
			<-start
			results <- store.SaveSpotState(ctx, SpotStateWrite{
				ID: rows[0].ID, Region: 1, ExpectedRevision: 1,
				State: json.RawMessage(state), RuntimeSeconds: 200,
			})
		}(state)
	}
	close(start)
	var saved, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			saved++
		} else {
			require.ErrorIs(t, err, ErrSpotRevisionConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, saved)
	require.Equal(t, 1, conflicts)
	rows, err = store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 2, rows[0].Revision)
}

func TestSpotStorePostgresWriteRollsBackWithTransaction(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	ctx := context.Background()
	insertSpotStoreFixture(t, db, 1, "www")
	rows, err := store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	rollback := errors.New("fixture rollback")
	err = db.WithTx(ctx, func(q *repository.Queries) error {
		count, err := q.SaveSpotState(ctx, repository.SaveSpotStateParams{
			ID: rows[0].ID, Region: 1, ExpectedRevision: 1,
			State: json.RawMessage(`{"fill":{"0,0":{"cursor":1}}}`), LastRuntimeSeconds: 200,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	rows, err = store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows[0].Revision)
	require.EqualValues(t, 100, rows[0].LastRuntimeSeconds)
	require.JSONEq(t, `{}`, string(rows[0].State))
}

func TestSpotStorePostgresRejectsInvalidSnapshot(t *testing.T) {
	db, store := newSpotStorePostgres(t)
	ctx := context.Background()
	insertSpotStoreFixture(t, db, 1, "www")
	rows, err := store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	valid := SpotStateWrite{ID: rows[0].ID, Region: 1, ExpectedRevision: 1, State: json.RawMessage(`{}`)}
	for _, test := range []struct {
		name   string
		change func(*SpotStateWrite)
	}{
		{"zero ID", func(w *SpotStateWrite) { w.ID = 0 }},
		{"zero region", func(w *SpotStateWrite) { w.Region = 0 }},
		{"zero revision", func(w *SpotStateWrite) { w.ExpectedRevision = 0 }},
		{"revision overflow", func(w *SpotStateWrite) { w.ExpectedRevision = math.MaxInt64 }},
		{"negative runtime", func(w *SpotStateWrite) { w.RuntimeSeconds = -1 }},
		{"nil state", func(w *SpotStateWrite) { w.State = nil }},
		{"null state", func(w *SpotStateWrite) { w.State = json.RawMessage(`null`) }},
		{"array state", func(w *SpotStateWrite) { w.State = json.RawMessage(`[]`) }},
		{"invalid state", func(w *SpotStateWrite) { w.State = json.RawMessage(`{`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempt := valid
			test.change(&attempt)
			err := store.SaveSpotState(ctx, attempt)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrSpotRevisionConflict)
		})
	}
	rows, err = store.LoadSpots(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows[0].Revision)
}
