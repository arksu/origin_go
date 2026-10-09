package world

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"

	constt "origin/internal/const"
	"origin/internal/game/inventory"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

const destructionPersistenceBatchSize = 100

// ReplaceObjectWithDroppedItems commits the complete replacement in one
// transaction. next fills at most 100 records and returns an empty batch or
// io.EOF when exhausted. All batches share one transaction; a failed iterator
// or write rolls back the source delete and every previously written batch.
func (p *DroppedItemPersisterDB) ReplaceObjectWithDroppedItems(
	ctx context.Context,
	region int,
	sourceID types.EntityID,
	maxAllocatedID types.EntityID,
	next func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error),
) error {
	return p.persistObjectReplacement(ctx, region, sourceID, maxAllocatedID, nil, next)
}

// TransformObjectWithDroppedItems atomically releases every source inventory
// and installs an inventory-free normal object under the same durable identity.
// replacement and iterator payloads must remain immutable until this call returns.
// Replaying the same snapshot and drops after an ambiguous commit is idempotent.
func (p *DroppedItemPersisterDB) TransformObjectWithDroppedItems(
	ctx context.Context,
	replacement *repository.Object,
	maxAllocatedID types.EntityID,
	next func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error),
) error {
	if err := validateCommittedSource(replacement); err != nil {
		return err
	}
	return p.persistObjectReplacement(ctx, replacement.Region, types.EntityID(replacement.ID), maxAllocatedID, replacement, next)
}

func validateCommittedSource(raw *repository.Object) error {
	if raw == nil || raw.ID <= 0 || raw.Region <= 0 || raw.TypeID <= 0 || raw.TypeID == constt.DroppedItemTypeID ||
		!raw.Hp.Valid || raw.Hp.Float64 <= 0 || math.IsNaN(raw.Hp.Float64) || math.IsInf(raw.Hp.Float64, 0) || raw.DeletedAt.Valid {
		return ErrInvalidCommittedObject
	}
	registry := objectdefs.Global()
	if registry == nil {
		return ErrInvalidCommittedObject
	}
	def, ok := registry.GetByID(raw.TypeID)
	if !ok || def.Key == "player" || def.HP <= 0 || (def.Components != nil && len(def.Components.Inventory) != 0) {
		return ErrInvalidCommittedObject
	}
	return nil
}

func (p *DroppedItemPersisterDB) persistObjectReplacement(
	ctx context.Context,
	region int,
	sourceID types.EntityID,
	maxAllocatedID types.EntityID,
	replacement *repository.Object,
	next func(dst []inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error),
) error {
	if p == nil || p.db == nil || ctx == nil || next == nil || region <= 0 || sourceID == 0 || sourceID > math.MaxInt64 || maxAllocatedID > math.MaxInt64 {
		return fmt.Errorf("replace destroyed object: invalid dependencies or source")
	}
	return p.db.WithTx(ctx, func(q *repository.Queries) error {
		var buffer [destructionPersistenceBatchSize]inventory.DroppedItemPersistenceRecord
		for {
			batch, err := next(buffer[:0])
			last := errors.Is(err, io.EOF)
			if err != nil && !last {
				return fmt.Errorf("serialize destruction batch: %w", err)
			}
			if len(batch) > destructionPersistenceBatchSize {
				return fmt.Errorf("destruction batch exceeds limit %d", destructionPersistenceBatchSize)
			}
			if len(batch) == 0 {
				break
			}
			if err := persistDestructionBatch(ctx, q, region, sourceID, batch); err != nil {
				return err
			}
			if last {
				break
			}
		}
		// Take the shared allocator row lock only after streaming the drops, so
		// unrelated shard transactions do not serialize their full loot writes.
		if maxAllocatedID != 0 {
			if err := q.UpsertGlobalVarLongMax(ctx, repository.UpsertGlobalVarLongMaxParams{
				Name: constt.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: int64(maxAllocatedID), Valid: true},
			}); err != nil {
				return fmt.Errorf("persist destruction ID high watermark: %w", err)
			}
		}
		if replacement == nil {
			return deleteObjectRows(ctx, q, region, sourceID, true)
		}
		if err := q.DeleteInventoriesByOwner(ctx, int64(sourceID)); err != nil {
			return fmt.Errorf("delete transformed object inventories: %w", err)
		}
		if err := q.UpsertObject(ctx, repository.UpsertObjectParams{
			ID: replacement.ID, TypeID: replacement.TypeID, Region: replacement.Region,
			X: replacement.X, Y: replacement.Y, Layer: replacement.Layer,
			ChunkX: replacement.ChunkX, ChunkY: replacement.ChunkY,
			Heading: replacement.Heading, Quality: replacement.Quality, Hp: replacement.Hp,
			OwnerID: replacement.OwnerID, Data: replacement.Data,
			CreateTick: replacement.CreateTick, LastTick: replacement.LastTick,
		}); err != nil {
			return fmt.Errorf("persist transformed object: %w", err)
		}
		return nil
	})
}

func persistDestructionBatch(ctx context.Context, q *repository.Queries, region int, sourceID types.EntityID, records []inventory.DroppedItemPersistenceRecord) error {
	var ids [destructionPersistenceBatchSize]int64
	var regions, xs, ys, layers, chunkXs, chunkYs [destructionPersistenceBatchSize]int
	var objectData, inventoryData [destructionPersistenceBatchSize]string
	for i, record := range records {
		if err := validateDroppedItemPersistenceRecord(record); err != nil {
			return err
		}
		if record.EntityID > math.MaxInt64 || record.EntityID == sourceID || record.Region != region {
			return fmt.Errorf("destroyed object drop has invalid identity or region")
		}
		ids[i], regions[i], xs[i], ys[i], layers[i], chunkXs[i], chunkYs[i] = int64(record.EntityID), record.Region, record.X, record.Y, record.Layer, record.ChunkX, record.ChunkY
		objectData[i], inventoryData[i] = string(record.ObjectData), string(record.InventoryData)
	}
	n := len(records)
	if err := q.UpsertDroppedObjects(ctx, repository.UpsertDroppedObjectsParams{
		Ids: ids[:n], TypeID: constt.DroppedItemTypeID, Regions: regions[:n], Xs: xs[:n], Ys: ys[:n],
		Layers: layers[:n], ChunkXs: chunkXs[:n], ChunkYs: chunkYs[:n], Datas: objectData[:n],
	}); err != nil {
		return fmt.Errorf("persist destroyed object drops: %w", err)
	}
	if err := q.UpsertDroppedInventories(ctx, repository.UpsertDroppedInventoriesParams{
		OwnerIds: ids[:n], Kind: int(constt.InventoryDroppedItem), Datas: inventoryData[:n],
	}); err != nil {
		return fmt.Errorf("persist destroyed object drop inventories: %w", err)
	}
	return nil
}
