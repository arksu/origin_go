package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"

	constt "origin/internal/const"
	"origin/internal/game/inventory"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func destructionRecord(id types.EntityID) inventory.DroppedItemPersistenceRecord {
	return inventory.DroppedItemPersistenceRecord{
		EntityID: id, TypeID: constt.DroppedItemTypeID, Region: 1, X: 5, Y: 5,
		ObjectData:    []byte(fmt.Sprintf(`{"has_inventory":true,"contained_item_id":%d,"drop_time":0,"time_basis":"runtime_seconds_v1"}`, id)),
		InventoryData: []byte(fmt.Sprintf(`{"kind":3,"key":0,"v":1,"items":[{"item_id":%d,"type_id":900,"quality":10,"quantity":1}]}`, id)),
	}
}

func TestObjectDestructionPostgresStreamingReplacement(t *testing.T) {
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	const source types.EntityID = 9970
	require.NoError(t, db.Queries().UpsertObject(ctx, objectHealthUpsertParams(int64(source), sql.NullFloat64{Float64: 0, Valid: true})))
	_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: int64(source), Data: []byte(`{"items":[]}`), Version: 1})
	require.NoError(t, err)
	// A re-dropped item may have an older, soft-deleted ground root at a higher
	// version. Replacement must install the current nested content regardless.
	_, err = db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: 10000, Kind: int16(constt.InventoryDroppedItem), Data: []byte(`{"old":true}`), Version: 8})
	require.NoError(t, err)
	require.NoError(t, db.Queries().DeleteInventoriesByOwner(ctx, 10000))
	records := make([]inventory.DroppedItemPersistenceRecord, 201)
	for i := range records {
		records[i] = destructionRecord(types.EntityID(10000 + i))
	}
	records[0].InventoryData = []byte(`{"kind":3,"key":0,"v":1,"items":[{"item_id":10000,"type_id":900,"quality":10,"quantity":1,"nested_inventory":{"kind":0,"key":0,"v":7,"width":2,"height":2,"items":[{"item_id":20000,"type_id":901,"quality":21,"quantity":3}]}}]}`)
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	for attempt := 0; attempt < 2; attempt++ {
		position, calls := 0, 0
		err := persister.ReplaceObjectWithDroppedItems(ctx, 1, source, 10200, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
			calls++
			end := min(position+cap(dst), len(records))
			dst = append(dst, records[position:end]...)
			position = end
			return dst, nil
		})
		require.NoError(t, err)
		require.Equal(t, 4, calls)
	}
	_, err = db.Queries().GetObjectByID(ctx, int64(source))
	require.ErrorIs(t, err, sql.ErrNoRows)
	sourceRows, err := db.Queries().GetInventoriesByOwner(ctx, int64(source))
	require.NoError(t, err)
	require.Empty(t, sourceRows)
	rows, err := db.Queries().GetInventoriesByOwner(ctx, 10000)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.JSONEq(t, string(records[0].InventoryData), string(rows[0].Data))
	require.Equal(t, 1, rows[0].Version)
	objects, err := db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1})
	require.NoError(t, err)
	require.Len(t, objects, 201)
	mark, err := db.Queries().GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.NoError(t, err)
	require.Equal(t, int64(10200), mark.ValueLong.Int64)
	require.NoError(t, db.Queries().UpsertGlobalVarLongMax(ctx, repository.UpsertGlobalVarLongMaxParams{Name: constt.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: 100, Valid: true}}))
	mark, err = db.Queries().GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.NoError(t, err)
	require.Equal(t, int64(10200), mark.ValueLong.Int64)
}

func TestObjectDestructionPostgresRollsBackEveryBatchAndIDs(t *testing.T) {
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	const source types.EntityID = 9971
	require.NoError(t, db.Queries().UpsertObject(ctx, objectHealthUpsertParams(int64(source), sql.NullFloat64{Float64: 0, Valid: true})))
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	want := errors.New("capture failed after first batch")
	calls := 0
	err := persister.ReplaceObjectWithDroppedItems(ctx, 1, source, 30099, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
		calls++
		if calls > 1 {
			return nil, want
		}
		for i := 0; i < cap(dst); i++ {
			dst = append(dst, destructionRecord(types.EntityID(30000+i)))
		}
		return dst, nil
	})
	require.ErrorIs(t, err, want)
	_, err = db.Queries().GetObjectByID(ctx, int64(source))
	require.NoError(t, err)
	_, err = db.Queries().GetObjectByID(ctx, 30000)
	require.ErrorIs(t, err, sql.ErrNoRows)
	rows, err := db.Queries().GetInventoriesByOwner(ctx, 30000)
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = db.Queries().GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	// Object batch succeeds but its inventory batch fails JSON parsing; both
	// writes still share the replacement transaction and must roll back.
	calls = 0
	err = persister.ReplaceObjectWithDroppedItems(ctx, 1, source, 31000, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
		calls++
		if calls > 1 {
			return dst, nil
		}
		record := destructionRecord(31000)
		record.InventoryData = []byte(`{`)
		return append(dst, record), nil
	})
	require.Error(t, err)
	_, err = db.Queries().GetObjectByID(ctx, 31000)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = db.Queries().GetObjectByID(ctx, int64(source))
	require.NoError(t, err)
}

func TestObjectDestructionPostgresEmptyUnsavedSource(t *testing.T) {
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: 9972, Data: []byte(`{"items":[]}`), Version: 1})
	require.NoError(t, err)
	for attempt := 0; attempt < 2; attempt++ {
		require.NoError(t, NewDroppedItemPersisterDB(db, zap.NewNop()).ReplaceObjectWithDroppedItems(ctx, 1, 9972, 0, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
			return dst, nil
		}))
	}
	rows, err := db.Queries().GetInventoriesByOwner(ctx, 9972)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func transformationReplacement(id int64) *repository.Object {
	return &repository.Object{
		ID: id, TypeID: objectHealthLifecycleTypeID, Region: 1, X: 19, Y: 23, Layer: 0,
		Quality: 27, Heading: sql.NullInt16{Int16: 90, Valid: true},
		Hp:      sql.NullFloat64{Float64: 100, Valid: true},
		OwnerID: sql.NullInt64{Int64: 92, Valid: true}, CreateTick: 7, LastTick: 21601,
	}
}

func TestObjectTransformationPostgresStreamingReplacementKeepsIdentity(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	const source int64 = 9980
	require.NoError(t, db.Queries().UpsertObject(ctx, objectHealthUpsertParams(source, sql.NullFloat64{Float64: .49, Valid: true})))
	for _, kind := range []int16{0, 1, 2} {
		_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: source, Kind: kind, Data: []byte(`{"items":[]}`), Version: 1})
		require.NoError(t, err)
	}
	replacement := transformationReplacement(source)
	records := make([]inventory.DroppedItemPersistenceRecord, 201)
	for i := range records {
		records[i] = destructionRecord(types.EntityID(40000 + i))
	}
	records[0].InventoryData = []byte(`{"kind":3,"key":0,"v":1,"items":[{"item_id":40000,"type_id":900,"quality":10,"quantity":1,"nested_inventory":{"kind":0,"key":0,"v":7,"width":2,"height":2,"items":[{"item_id":50000,"type_id":901,"quality":21,"quantity":3}]}}]}`)
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	for range 2 {
		position, calls := 0, 0
		require.NoError(t, persister.TransformObjectWithDroppedItems(ctx, replacement, 40200, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
			calls++
			end := min(position+cap(dst), len(records))
			dst = append(dst, records[position:end]...)
			position = end
			if position == len(records) {
				return dst, io.EOF
			}
			return dst, nil
		}))
		require.Equal(t, 3, calls)
	}
	loaded, err := db.Queries().GetObjectByID(ctx, source)
	require.NoError(t, err)
	require.Equal(t, replacement.ID, loaded.ID)
	require.Equal(t, replacement.TypeID, loaded.TypeID)
	require.Equal(t, replacement.Region, loaded.Region)
	require.Equal(t, replacement.X, loaded.X)
	require.Equal(t, replacement.Y, loaded.Y)
	require.Equal(t, replacement.Quality, loaded.Quality)
	require.Equal(t, replacement.Heading, loaded.Heading)
	require.Equal(t, replacement.Hp, loaded.Hp)
	require.Equal(t, replacement.OwnerID, loaded.OwnerID)
	require.Equal(t, replacement.LastTick, loaded.LastTick)
	require.False(t, loaded.DeletedAt.Valid)
	require.False(t, loaded.Data.Valid)
	rows, err := db.Queries().GetInventoriesByOwner(ctx, source)
	require.NoError(t, err)
	require.Empty(t, rows)
	rows, err = db.Queries().GetInventoriesByOwner(ctx, 40000)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.JSONEq(t, string(records[0].InventoryData), string(rows[0].Data))
	objects, err := db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{Region: 1})
	require.NoError(t, err)
	require.Len(t, objects, 202)
	mark, err := db.Queries().GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.NoError(t, err)
	require.Equal(t, int64(40200), mark.ValueLong.Int64)
}

func TestObjectTransformationPostgresFailurePreservesSourceAndInventory(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	const source int64 = 9981
	initialHP := sql.NullFloat64{Float64: .49, Valid: true}
	require.NoError(t, db.Queries().UpsertObject(ctx, objectHealthUpsertParams(source, initialHP)))
	_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: source, Data: []byte(`{"items":[{"item_id":60000}]}`), Version: 9})
	require.NoError(t, err)
	persister := NewDroppedItemPersisterDB(db, zap.NewNop())
	want := errors.New("capture failed after first batch")
	for _, invalidReplacement := range []bool{false, true} {
		replacement := transformationReplacement(source)
		if invalidReplacement {
			replacement.Data = pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{`)}
		}
		calls := 0
		err := persister.TransformObjectWithDroppedItems(ctx, replacement, 60099, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
			calls++
			if calls > 1 {
				if invalidReplacement {
					return dst, nil
				}
				return nil, want
			}
			for i := 0; i < cap(dst); i++ {
				dst = append(dst, destructionRecord(types.EntityID(60000+i)))
			}
			return dst, nil
		})
		require.Error(t, err)
		if !invalidReplacement {
			require.ErrorIs(t, err, want)
		}
		loaded, err := db.Queries().GetObjectByID(ctx, source)
		require.NoError(t, err)
		require.Equal(t, initialHP, loaded.Hp)
		require.Equal(t, 10, loaded.X)
		rows, err := db.Queries().GetInventoriesByOwner(ctx, source)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Equal(t, 9, rows[0].Version)
		_, err = db.Queries().GetObjectByID(ctx, 60000)
		require.ErrorIs(t, err, sql.ErrNoRows)
		rows, err = db.Queries().GetInventoriesByOwner(ctx, 60000)
		require.NoError(t, err)
		require.Empty(t, rows)
		_, err = db.Queries().GetGlobalVar(ctx, constt.LAST_USED_ID)
		require.ErrorIs(t, err, sql.ErrNoRows)
	}
}

func TestObjectTransformationPostgresEmptyUnsavedSource(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	const source int64 = 9982
	_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: source, Data: []byte(`{"items":[]}`), Version: 1})
	require.NoError(t, err)
	for range 2 {
		require.NoError(t, NewDroppedItemPersisterDB(db, zap.NewNop()).TransformObjectWithDroppedItems(ctx, transformationReplacement(source), 0, func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error) {
			return dst, nil
		}))
	}
	loaded, err := db.Queries().GetObjectByID(ctx, source)
	require.NoError(t, err)
	require.Equal(t, objectHealthLifecycleTypeID, loaded.TypeID)
	rows, err := db.Queries().GetInventoriesByOwner(ctx, source)
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = db.Queries().GetGlobalVar(ctx, constt.LAST_USED_ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestCommittedSourceValidationRejectsInvalidReplacement(t *testing.T) {
	installObjectHealthLifecycleDefinitions(t)
	for _, mutate := range []func(*repository.Object){
		func(raw *repository.Object) { raw.ID = 0 },
		func(raw *repository.Object) { raw.ID = -1 },
		func(raw *repository.Object) { raw.Region = 0 },
		func(raw *repository.Object) { raw.TypeID = constt.DroppedItemTypeID },
		func(raw *repository.Object) { raw.TypeID = objectHealthContainerTypeID },
		func(raw *repository.Object) { raw.TypeID = 99999 },
		func(raw *repository.Object) { raw.Hp = sql.NullFloat64{} },
		func(raw *repository.Object) { raw.Hp.Float64 = 0 },
		func(raw *repository.Object) { raw.Hp.Float64 = -1 },
		func(raw *repository.Object) { raw.Hp.Float64 = math.NaN() },
		func(raw *repository.Object) { raw.Hp.Float64 = math.Inf(1) },
		func(raw *repository.Object) { raw.DeletedAt.Valid = true },
	} {
		raw := transformationReplacement(9983)
		mutate(raw)
		require.ErrorIs(t, validateCommittedSource(raw), ErrInvalidCommittedObject)
	}
	require.ErrorIs(t, validateCommittedSource(nil), ErrInvalidCommittedObject)
	require.NoError(t, validateCommittedSource(transformationReplacement(9983)))
}
