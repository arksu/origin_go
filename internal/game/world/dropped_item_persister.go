package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs/systems"
	"origin/internal/game/inventory"

	"github.com/sqlc-dev/pqtype"
	"go.uber.org/zap"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// DroppedItemPersisterDB implements inventory.DroppedItemPersister using Postgres.
// It persists dropped objects and exposes the common object deletion path used by
// dropped-item pickup, decay, and future persistent-object despawns.
type DroppedItemPersisterDB struct {
	db     *persistence.Postgres
	logger *zap.Logger
}

var _ ObjectDespawnPersistence = (*DroppedItemPersisterDB)(nil)
var _ inventory.BatchDroppedItemPersister = (*DroppedItemPersisterDB)(nil)
var _ inventory.AtomicDroppedItemTransferPersister = (*DroppedItemPersisterDB)(nil)
var _ inventory.AtomicDroppedObjectReplacementPersister = (*DroppedItemPersisterDB)(nil)

func NewDroppedItemPersisterDB(db *persistence.Postgres, logger *zap.Logger) *DroppedItemPersisterDB {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DroppedItemPersisterDB{db: db, logger: logger}
}

func (p *DroppedItemPersisterDB) PersistDroppedObject(
	entityID types.EntityID, typeID int,
	region, x, y, layer, chunkX, chunkY int,
	objectData json.RawMessage, inventoryData json.RawMessage,
) error {
	return p.PersistDroppedObjectBatch([]inventory.DroppedItemPersistenceRecord{{
		EntityID:      entityID,
		TypeID:        typeID,
		Region:        region,
		X:             x,
		Y:             y,
		Layer:         layer,
		ChunkX:        chunkX,
		ChunkY:        chunkY,
		ObjectData:    objectData,
		InventoryData: inventoryData,
	}})
}

// PersistDroppedObjectBatch stores several one-item ground objects in a single
// transaction. This is required when a stack is split into independent dropped
// entities: either all units become durable or none do.
func (p *DroppedItemPersisterDB) PersistDroppedObjectBatch(records []inventory.DroppedItemPersistenceRecord) error {
	return p.persistDroppedObjectBatch(records, nil)
}

// ReplaceObjectWithDroppedItem commits a burner-like replacement atomically.
// A source object can never survive a committed outcome record after a crash.
func (p *DroppedItemPersisterDB) ReplaceObjectWithDroppedItem(record inventory.DroppedItemPersistenceRecord, sourceRegion int, sourceID types.EntityID) error {
	if p == nil || p.db == nil {
		return fmt.Errorf("replace object with dropped item: database is not configured")
	}
	if err := validateDroppedItemPersistenceRecord(record); err != nil {
		return err
	}
	if sourceRegion <= 0 || sourceID == 0 {
		return fmt.Errorf("replace object with dropped item: invalid source")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.db.WithTx(ctx, func(q *repository.Queries) error {
		if err := persistDroppedItemRecord(ctx, q, record); err != nil {
			return err
		}
		// Newly built chunk objects are not written until the chunk saves, so their
		// replacement source has no DB row to soft-delete yet.
		if _, err := q.SoftDeleteObject(ctx, repository.SoftDeleteObjectParams{Region: sourceRegion, ID: int64(sourceID)}); err != nil && !isMissingReplacementSource(err) {
			return fmt.Errorf("soft-delete replacement source: %w", err)
		}
		if err := q.DeleteInventoriesByOwner(ctx, int64(sourceID)); err != nil {
			return fmt.Errorf("delete replacement source inventories: %w", err)
		}
		return nil
	})
}

func isMissingReplacementSource(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// PersistDroppedObjectBatchWithPlayerInventories commits a player drop in one
// transaction. The caller has already built the post-drop player snapshots.
func (p *DroppedItemPersisterDB) PersistDroppedObjectBatchWithPlayerInventories(
	records []inventory.DroppedItemPersistenceRecord,
	inventories []systems.InventorySnapshot,
) error {
	if len(inventories) == 0 {
		return fmt.Errorf("persist dropped object transfer: player inventories are required")
	}
	return p.persistDroppedObjectBatch(records, inventories)
}

func (p *DroppedItemPersisterDB) persistDroppedObjectBatch(
	records []inventory.DroppedItemPersistenceRecord,
	inventories []systems.InventorySnapshot,
) error {
	if p == nil || p.db == nil {
		return fmt.Errorf("persist dropped object: database is not configured")
	}
	if len(records) == 0 {
		return fmt.Errorf("persist dropped object: no records")
	}
	for _, record := range records {
		if err := validateDroppedItemPersistenceRecord(record); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return p.db.WithTx(ctx, func(q *repository.Queries) error {
		for _, record := range records {
			if err := persistDroppedItemRecord(ctx, q, record); err != nil {
				return err
			}
		}
		return persistPlayerInventorySnapshots(ctx, q, inventories)
	})
}

func validateDroppedItemPersistenceRecord(record inventory.DroppedItemPersistenceRecord) error {
	if record.EntityID == 0 {
		return fmt.Errorf("persist dropped object: entity ID is zero")
	}
	if record.TypeID != constt.DroppedItemTypeID {
		return fmt.Errorf("persist dropped object %d: invalid type ID %d", record.EntityID, record.TypeID)
	}
	if record.Region <= 0 {
		return fmt.Errorf("persist dropped object %d: invalid region %d", record.EntityID, record.Region)
	}
	if len(record.ObjectData) == 0 || len(record.InventoryData) == 0 {
		return fmt.Errorf("persist dropped object %d: missing JSON data", record.EntityID)
	}
	return nil
}

func persistDroppedItemRecord(
	ctx context.Context,
	q *repository.Queries,
	record inventory.DroppedItemPersistenceRecord,
) error {
	if err := q.UpsertObject(ctx, repository.UpsertObjectParams{
		ID:     int64(record.EntityID),
		TypeID: record.TypeID,
		Region: record.Region,
		X:      record.X,
		Y:      record.Y,
		Layer:  record.Layer,
		ChunkX: record.ChunkX,
		ChunkY: record.ChunkY,
		Data: pqtype.NullRawMessage{
			RawMessage: record.ObjectData,
			Valid:      true,
		},
		CreateTick: 0,
		LastTick:   0,
	}); err != nil {
		return fmt.Errorf("upsert object %d: %w", record.EntityID, err)
	}

	if _, err := q.UpsertInventory(ctx, repository.UpsertInventoryParams{
		OwnerID:      int64(record.EntityID),
		Kind:         int16(constt.InventoryDroppedItem),
		InventoryKey: 0,
		Data:         record.InventoryData,
		Version:      1,
	}); err != nil {
		return fmt.Errorf("upsert inventory %d: %w", record.EntityID, err)
	}
	return nil
}

func persistPlayerInventorySnapshots(
	ctx context.Context,
	q *repository.Queries,
	inventories []systems.InventorySnapshot,
) error {
	for _, snapshot := range inventories {
		if snapshot.CharacterID <= 0 || snapshot.Kind < 0 || snapshot.InventoryKey < 0 || snapshot.Version < 0 || len(snapshot.Data) == 0 {
			return fmt.Errorf("invalid player inventory snapshot owner=%d kind=%d key=%d", snapshot.CharacterID, snapshot.Kind, snapshot.InventoryKey)
		}
		if _, err := q.UpsertInventory(ctx, repository.UpsertInventoryParams{
			OwnerID:      snapshot.CharacterID,
			Kind:         snapshot.Kind,
			InventoryKey: snapshot.InventoryKey,
			Data:         snapshot.Data,
			Version:      snapshot.Version,
		}); err != nil {
			return fmt.Errorf("upsert player inventory owner=%d kind=%d key=%d: %w", snapshot.CharacterID, snapshot.Kind, snapshot.InventoryKey, err)
		}
	}
	return nil
}

// DeleteObject soft-deletes a persistent object and every root inventory row it owns.
// Object IDs and item IDs share one global sequence, so this removes a dropped item's
// object and inventory records together without leaving an orphaned owner_id.
func (p *DroppedItemPersisterDB) DeleteObject(region int, entityID types.EntityID) error {
	return p.deleteObjectWithPlayerInventories(region, entityID, nil)
}

// DeleteDroppedObjectWithPlayerInventories commits a player pickup in one
// transaction. If either side fails, neither the dropped object nor the player
// inventory changes become durable.
func (p *DroppedItemPersisterDB) DeleteDroppedObjectWithPlayerInventories(
	region int,
	entityID types.EntityID,
	inventories []systems.InventorySnapshot,
) error {
	if len(inventories) == 0 {
		return fmt.Errorf("delete dropped object transfer: player inventories are required")
	}
	return p.deleteObjectWithPlayerInventories(region, entityID, inventories)
}

func (p *DroppedItemPersisterDB) deleteObjectWithPlayerInventories(
	region int,
	entityID types.EntityID,
	inventories []systems.InventorySnapshot,
) error {
	if p == nil || p.db == nil {
		return fmt.Errorf("delete object: database is not configured")
	}
	if entityID == 0 {
		return fmt.Errorf("delete object: entity ID is zero")
	}
	if region <= 0 {
		return fmt.Errorf("delete object %d: invalid region %d", entityID, region)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p.logger.Debug("DeleteObject",
		zap.Int("region", region),
		zap.Uint64("entity_id", uint64(entityID)))

	return p.db.WithTx(ctx, func(q *repository.Queries) error {
		if err := deleteObjectRows(ctx, q, region, entityID, len(inventories) == 0); err != nil {
			return err
		}
		return persistPlayerInventorySnapshots(ctx, q, inventories)
	})
}

type objectDeletionQueries interface {
	SoftDeleteObject(context.Context, repository.SoftDeleteObjectParams) (int64, error)
	DeleteInventoriesByOwner(context.Context, int64) error
}

func deleteObjectRows(ctx context.Context, q objectDeletionQueries, region int, entityID types.EntityID, allowMissing bool) error {
	// Chunk-owned objects may not have reached their first save yet. Still delete
	// their inventory rows, but require an existing object for atomic pickups.
	if _, err := q.SoftDeleteObject(ctx, repository.SoftDeleteObjectParams{Region: region, ID: int64(entityID)}); err != nil && !(allowMissing && errors.Is(err, sql.ErrNoRows)) {
		return fmt.Errorf("soft-delete object: %w", err)
	}
	if err := q.DeleteInventoriesByOwner(ctx, int64(entityID)); err != nil {
		return fmt.Errorf("delete object inventories: %w", err)
	}
	return nil
}

// UpdateObjectData persists a metadata migration detected during chunk load.
func (p *DroppedItemPersisterDB) UpdateObjectData(region int, entityID types.EntityID, data json.RawMessage) error {
	if p == nil || p.db == nil {
		return fmt.Errorf("update object data: database is not configured")
	}
	if region <= 0 || entityID == 0 || len(data) == 0 {
		return fmt.Errorf("update object data: invalid target")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := p.db.Queries().UpdateObjectData(ctx, repository.UpdateObjectDataParams{
		Region: region,
		ID:     int64(entityID),
		Data: pqtype.NullRawMessage{
			RawMessage: data,
			Valid:      true,
		},
	}); err != nil {
		return fmt.Errorf("update object data %d: %w", entityID, err)
	}
	return nil
}

func (p *DroppedItemPersisterDB) RecordChunkObjectDespawn(chunk *core.Chunk, entityID types.EntityID) {
	if chunk == nil || entityID == 0 {
		return
	}
	chunk.MarkDeletedObjectID(entityID)
}
