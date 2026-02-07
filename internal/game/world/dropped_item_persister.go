package world

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sqlc-dev/pqtype"

	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// DroppedItemPersisterDB implements inventory.DroppedItemPersister using Postgres.
type DroppedItemPersisterDB struct {
	db *persistence.Postgres
}

func NewDroppedItemPersisterDB(db *persistence.Postgres) *DroppedItemPersisterDB {
	return &DroppedItemPersisterDB{db: db}
}

func (p *DroppedItemPersisterDB) PersistDroppedObject(
	entityID types.EntityID, typeID int,
	region, x, y, layer, chunkX, chunkY int,
	data json.RawMessage,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return p.db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
		ID:     int64(entityID),
		TypeID: typeID,
		Region: region,
		X:      x,
		Y:      y,
		Layer:  layer,
		ChunkX: chunkX,
		ChunkY: chunkY,
		Data: pqtype.NullRawMessage{
			RawMessage: data,
			Valid:      true,
		},
		CreateTick: 0,
		LastTick:   0,
	})
}

func (p *DroppedItemPersisterDB) DeleteDroppedObject(region int, entityID types.EntityID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return p.db.Queries().SoftDeleteObject(ctx, repository.SoftDeleteObjectParams{
		Region: region,
		ID:     int64(entityID),
	})
}
