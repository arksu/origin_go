package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	constt "origin/internal/const"
	"origin/internal/game/inventory"
	"origin/internal/persistence/repository"
	"origin/internal/types"

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
