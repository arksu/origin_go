package world

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	constt "origin/internal/const"

	"github.com/sqlc-dev/pqtype"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// DroppedItemPersisterDB implements inventory.DroppedItemPersister using Postgres.
// It persists/deletes both the object row and the inventory row.
type DroppedItemPersisterDB struct {
	db *persistence.Postgres
}

func NewDroppedItemPersisterDB(db *persistence.Postgres) *DroppedItemPersisterDB {
	return &DroppedItemPersisterDB{db: db}
}

func (p *DroppedItemPersisterDB) PersistDroppedObject(
	entityID types.EntityID, typeID int,
	region, x, y, layer, chunkX, chunkY int,
	objectData json.RawMessage, inventoryData json.RawMessage,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Persist object row
	if err := p.db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
		ID:     int64(entityID),
		TypeID: typeID,
		Region: region,
		X:      x,
		Y:      y,
		Layer:  layer,
		ChunkX: chunkX,
		ChunkY: chunkY,
		Data: pqtype.NullRawMessage{
			RawMessage: objectData,
			Valid:      true,
		},
		CreateTick: 0,
		LastTick:   0,
	}); err != nil {
		return fmt.Errorf("upsert object: %w", err)
	}

	// Persist inventory row (kind=DroppedItem, key=0)
	if _, err := p.db.Queries().UpsertInventory(ctx, repository.UpsertInventoryParams{
		OwnerID:      int64(entityID),
		Kind:         int16(constt.InventoryDroppedItem),
		InventoryKey: 0,
		Data:         inventoryData,
		Version:      1,
	}); err != nil {
		return fmt.Errorf("upsert inventory: %w", err)
	}

	return nil
}

func (p *DroppedItemPersisterDB) DeleteDroppedObject(region int, entityID types.EntityID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Soft-delete object row
	if err := p.db.Queries().SoftDeleteObject(ctx, repository.SoftDeleteObjectParams{
		Region: region,
		ID:     int64(entityID),
	}); err != nil {
		return fmt.Errorf("soft-delete object: %w", err)
	}

	// Delete inventory row
	if err := p.db.Queries().DeleteInventory(ctx, repository.DeleteInventoryParams{
		OwnerID:      int64(entityID),
		Kind:         int16(constt.InventoryDroppedItem),
		InventoryKey: 0,
	}); err != nil {
		return fmt.Errorf("delete inventory: %w", err)
	}

	return nil
}
