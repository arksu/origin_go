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
		return deleteObjectRows(ctx, q, region, sourceID, true)
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
