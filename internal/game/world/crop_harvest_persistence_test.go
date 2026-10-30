package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func cropHarvestRecord(t *testing.T) inventory.CropHarvestPersistenceRecord {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 42, Key: "crop_carrot", HP: 100, IsStatic: true, CropConfig: &objectdefs.CropBehaviorConfig{}}}))
	return inventory.CropHarvestPersistenceRecord{
		Source:            &repository.Object{ID: 41, TypeID: 42, Region: 1, X: 23, Y: 29, Layer: 2, ChunkX: 3, ChunkY: 4, Quality: 4000000000, Hp: sql.NullFloat64{Float64: 100, Valid: true}, Data: pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{"v":1,"behaviors":{"crop":{"stage":3}}}`)}},
		Receipt:           inventory.CropHarvestReceipt{FirstItemID: 10001, LastItemID: 10002, RecipientID: 73},
		DeletedData:       pqtype.NullRawMessage{Valid: true, RawMessage: []byte(`{"v":1,"behaviors":{"crop":{"stage":3},"crop_harvest_receipt":{"first_item_id":10001,"last_item_id":10002,"recipient_id":73}}}`)},
		PlayerInventories: []systems.InventorySnapshot{{CharacterID: 73, Kind: int16(constt.InventoryGrid), Version: 2, Data: []byte(`{"kind":0,"key":0,"v":2,"width":4,"height":4,"items":[{"item_id":10001,"type_id":201,"quality":4000000000,"quantity":1}]}`)}},
		Drops:             []inventory.DroppedItemPersistenceRecord{destructionRecord(types.EntityID(10002))},
	}
}

func TestCropHarvestPostgresAtomicCommitAndReceiptBeforeInventoryReplay(t *testing.T) {
	for _, saved := range []bool{true, false} {
		t.Run(fmt.Sprintf("source_saved_%t", saved), func(t *testing.T) {
			record := cropHarvestRecord(t)
			db := newObjectHealthPostgres(t)
			ctx := context.Background()
			p := NewDroppedItemPersisterDB(db, zap.NewNop())
			if saved {
				persistSkullTestSource(t, p, record.Source)
			}
			require.NoError(t, p.PersistCropHarvest(ctx, record))
			_, err := db.Queries().GetObjectByID(ctx, record.Source.ID)
			require.ErrorIs(t, err, sql.ErrNoRows)
			deleted, err := db.Queries().GetSkullClaimSourceForUpdate(ctx, repository.GetSkullClaimSourceForUpdateParams{Region: 1, ID: record.Source.ID})
			require.NoError(t, err)
			require.True(t, deleted.DeletedAt.Valid)
			receipt, ok := cropHarvestReceipt(deleted.Data.RawMessage)
			require.True(t, ok)
			require.Equal(t, record.Receipt, receipt)
			rows, err := db.Queries().GetInventoriesByOwner(ctx, 73)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.JSONEq(t, string(record.PlayerInventories[0].Data), string(rows[0].Data))
			_, err = db.Queries().GetObjectByID(ctx, 10002)
			require.NoError(t, err)
			require.Equal(t, int64(10002), db.GetGlobalVarLong(ctx, constt.LAST_USED_ID))
			// Test a newer payload with the SAME version too. The receipt must
			// return before UpsertInventory can overwrite this legitimate update.
			newer := []byte(`{"must_survive":"same_version"}`)
			_, err = db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: 73, Kind: int16(constt.InventoryGrid), Version: 2, Data: newer})
			require.NoError(t, err)
			require.NoError(t, p.PersistCropHarvest(ctx, record))
			rows, err = db.Queries().GetInventoriesByOwner(ctx, 73)
			require.NoError(t, err)
			require.JSONEq(t, string(newer), string(rows[0].Data))
			_, err = db.Queries().GetObjectByID(ctx, record.Source.ID)
			require.ErrorIs(t, err, sql.ErrNoRows, "replay cannot resurrect source")
		})
	}
}

func TestCropHarvestPostgresRollbackPreservesUnsavedCropAndInventories(t *testing.T) {
	for _, scenario := range []string{"inventory_conflict", "drop_write_failure"} {
		t.Run(scenario, func(t *testing.T) {
			record := cropHarvestRecord(t)
			db := newObjectHealthPostgres(t)
			ctx := context.Background()
			p := NewDroppedItemPersisterDB(db, zap.NewNop())
			if scenario == "inventory_conflict" {
				record.PlayerInventories = append(record.PlayerInventories, systems.InventorySnapshot{CharacterID: 73, Kind: int16(constt.InventoryHand), Version: 2, Data: []byte(`{"items":[]}`)})
				_, err := db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{OwnerID: 73, Kind: int16(constt.InventoryHand), Version: 9, Data: []byte(`{"must_survive":true}`)})
				require.NoError(t, err)
				require.ErrorIs(t, p.PersistCropHarvest(ctx, record), inventory.ErrCropHarvestInventoryConflict)
			} else {
				record.Drops[0].InventoryData = []byte(`invalid json`)
				require.Error(t, p.PersistCropHarvest(ctx, record))
			}
			_, err := db.Queries().GetSkullClaimSourceForUpdate(ctx, repository.GetSkullClaimSourceForUpdateParams{Region: 1, ID: 41})
			require.ErrorIs(t, err, sql.ErrNoRows)
			_, err = db.Queries().GetObjectByID(ctx, 10002)
			require.ErrorIs(t, err, sql.ErrNoRows)
			rows, err := db.Queries().GetInventoriesByOwner(ctx, 73)
			require.NoError(t, err)
			if scenario == "inventory_conflict" {
				require.Len(t, rows, 1)
				require.Equal(t, int16(constt.InventoryHand), rows[0].Kind)
				require.Equal(t, 9, rows[0].Version)
			} else {
				require.Empty(t, rows)
			}
			require.Zero(t, db.GetGlobalVarLong(ctx, constt.LAST_USED_ID))
		})
	}
}

func TestCropHarvestPostgresAllGroundAndDifferentReceiptConflict(t *testing.T) {
	record := cropHarvestRecord(t)
	db := newObjectHealthPostgres(t)
	ctx := context.Background()
	p := NewDroppedItemPersisterDB(db, zap.NewNop())
	record.PlayerInventories = nil
	record.Drops = append(record.Drops, destructionRecord(types.EntityID(10001)))
	require.NoError(t, p.PersistCropHarvest(ctx, record))
	require.NoError(t, p.PersistCropHarvest(ctx, record))
	rows, err := db.Queries().GetInventoriesByOwner(ctx, 73)
	require.NoError(t, err)
	require.Empty(t, rows)
	record.Receipt.RecipientID++
	var data map[string]any
	require.NoError(t, json.Unmarshal(record.DeletedData.RawMessage, &data))
	data["behaviors"].(map[string]any)["crop_harvest_receipt"] = record.Receipt
	record.DeletedData.RawMessage, err = json.Marshal(data)
	require.NoError(t, err)
	require.ErrorIs(t, p.PersistCropHarvest(ctx, record), inventory.ErrCropHarvestSourceConflict)
}
