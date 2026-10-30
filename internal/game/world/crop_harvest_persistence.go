package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
)

// PersistCropHarvest commits the complete output and retains an idempotency
// receipt on the deleted crop. A replay checks that receipt before any player
// inventory writes, including after a lost COMMIT acknowledgement.
func (p *DroppedItemPersisterDB) PersistCropHarvest(ctx context.Context, record inventory.CropHarvestPersistenceRecord) error {
	if p == nil || p.db == nil || ctx == nil {
		return fmt.Errorf("persist crop harvest: database or context unavailable")
	}
	if err := validateCropHarvestRecord(record); err != nil {
		return err
	}
	return p.db.WithTx(ctx, func(q *repository.Queries) error {
		source := record.Source
		// This existing insert-only query also supports sources that have not
		// reached a chunk save. UpsertObject would resurrect a deleted source.
		if err := q.InsertSkullClaimSourceIfMissing(ctx, repository.InsertSkullClaimSourceIfMissingParams{
			ID: source.ID, TypeID: source.TypeID, Region: source.Region, X: source.X, Y: source.Y, Layer: source.Layer,
			ChunkX: source.ChunkX, ChunkY: source.ChunkY, Heading: source.Heading, Quality: source.Quality, Hp: source.Hp,
			OwnerID: source.OwnerID, Data: source.Data, CreateTick: source.CreateTick, LastTick: source.LastTick,
		}); err != nil {
			return fmt.Errorf("ensure crop harvest source: %w", err)
		}
		current, err := q.GetSkullClaimSourceForUpdate(ctx, repository.GetSkullClaimSourceForUpdateParams{Region: source.Region, ID: source.ID})
		if errors.Is(err, sql.ErrNoRows) {
			return inventory.ErrCropHarvestSourceConflict
		}
		if err != nil {
			return fmt.Errorf("lock crop harvest source: %w", err)
		}
		if receipt, exists := cropHarvestReceipt(current.Data.RawMessage); exists {
			if current.DeletedAt.Valid && receipt == record.Receipt {
				return nil
			}
			return inventory.ErrCropHarvestSourceConflict
		}
		if current.DeletedAt.Valid || current.TypeID != source.TypeID || current.Layer != source.Layer || current.ChunkX != source.ChunkX || current.ChunkY != source.ChunkY {
			return inventory.ErrCropHarvestSourceConflict
		}
		if err := persistPlayerInventorySnapshots(ctx, q, record.PlayerInventories); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return inventory.ErrCropHarvestInventoryConflict
			}
			return err
		}
		for _, drop := range record.Drops {
			if err := persistDroppedItemRecord(ctx, q, drop); err != nil {
				return err
			}
		}
		if err := q.UpsertGlobalVarLongMax(ctx, repository.UpsertGlobalVarLongMaxParams{Name: constt.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: int64(record.Receipt.LastItemID), Valid: true}}); err != nil {
			return fmt.Errorf("persist harvest IDs: %w", err)
		}
		if _, err := q.CommitCropHarvestSource(ctx, repository.CommitCropHarvestSourceParams{Region: source.Region, ID: source.ID, Data: record.DeletedData}); err != nil {
			return fmt.Errorf("delete harvested crop: %w", err)
		}
		return nil
	})
}

func validateCropHarvestRecord(record inventory.CropHarvestPersistenceRecord) error {
	if validateCommittedSource(record.Source) != nil || record.Receipt.RecipientID == 0 || record.Receipt.RecipientID > math.MaxInt64 ||
		record.Receipt.FirstItemID == 0 || record.Receipt.LastItemID < record.Receipt.FirstItemID || record.Receipt.LastItemID > math.MaxInt64 ||
		record.Receipt.LastItemID-record.Receipt.FirstItemID >= inventory.MaxCropHarvestUnits {
		return inventory.ErrCropHarvestSourceConflict
	}
	definition, found := objectdefs.Global().GetByID(record.Source.TypeID)
	if !found || definition.CropConfig == nil {
		return inventory.ErrCropHarvestSourceConflict
	}
	receipt, found := cropHarvestReceipt(record.DeletedData.RawMessage)
	if !record.DeletedData.Valid || !found || receipt != record.Receipt {
		return inventory.ErrCropHarvestSourceConflict
	}
	for _, snapshot := range record.PlayerInventories {
		if snapshot.CharacterID != int64(record.Receipt.RecipientID) || snapshot.Kind < int16(constt.InventoryGrid) || snapshot.Kind > int16(constt.InventoryEquipment) || snapshot.InventoryKey < 0 || snapshot.Version < 0 || !json.Valid(snapshot.Data) {
			return inventory.ErrCropHarvestInventoryConflict
		}
	}
	if len(record.Drops) > inventory.MaxCropHarvestUnits {
		return inventory.ErrCropHarvestSourceConflict
	}
	for _, drop := range record.Drops {
		if err := validateDroppedItemPersistenceRecord(drop); err != nil {
			return inventory.ErrCropHarvestSourceConflict
		}
	}
	return nil
}

func cropHarvestReceipt(data []byte) (inventory.CropHarvestReceipt, bool) {
	var envelope components.ObjectStateEnvelope
	if len(data) == 0 || json.Unmarshal(data, &envelope) != nil || envelope.Version != 1 {
		return inventory.CropHarvestReceipt{}, false
	}
	var receipt inventory.CropHarvestReceipt
	if json.Unmarshal(envelope.Behaviors["crop_harvest_receipt"], &receipt) != nil || receipt.FirstItemID == 0 || receipt.RecipientID == 0 {
		return inventory.CropHarvestReceipt{}, false
	}
	return receipt, true
}
