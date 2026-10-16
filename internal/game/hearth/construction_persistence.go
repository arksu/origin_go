package hearth

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	constt "origin/internal/const"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

type ConstructionSaver interface {
	SaveConstruction(context.Context, types.EntityID, *repository.Object, *repository.Object) error
	SaveOrphans(context.Context, []*repository.Object) error
	SaveTerminal(context.Context, types.EntityID, ObjectKey, *repository.Object) error
	RepairInvalidHome(context.Context, types.EntityID, ObjectKey, int) error
}

type ConstructionSaverDB struct {
	withTx func(context.Context, func(*repository.Queries) error) error
}

func NewConstructionSaverDB(db *persistence.Postgres) *ConstructionSaverDB {
	if db == nil {
		return nil
	}
	return &ConstructionSaverDB{withTx: db.WithTx}
}

func (s *ConstructionSaverDB) SaveConstruction(ctx context.Context, characterID types.EntityID, newObject, previousObject *repository.Object) error {
	if s == nil || s.withTx == nil || ctx == nil || !validID(characterID) || newObject == nil || newObject.ID <= 0 ||
		(previousObject != nil && previousObject.ID <= 0) {
		return fmt.Errorf("save hearth construction: invalid dependencies or identity")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.withTx(ctx, func(q *repository.Queries) error {
		if err := q.UpsertObject(ctx, constructionObjectParams(newObject)); err != nil {
			return fmt.Errorf("save new hearth object: %w", err)
		}
		maxID := newObject.ID
		if previousObject != nil {
			if err := q.UpsertObject(ctx, constructionObjectParams(previousObject)); err != nil {
				return fmt.Errorf("save previous hearth conversion: %w", err)
			}
			maxID = max(maxID, previousObject.ID)
		}
		rows, err := q.SetCharacterHearth(ctx, repository.SetCharacterHearthParams{
			CharacterID: int64(characterID), ObjectRegion: newObject.Region, ObjectID: newObject.ID,
			HearthTypeID: newObject.TypeID, SurfaceLayer: SurfaceLayer,
		})
		if err != nil {
			return fmt.Errorf("save hearth owner: %w", err)
		}
		if rows != 1 {
			return fmt.Errorf("save hearth owner: expected one association, updated %d", rows)
		}
		if err := q.UpsertGlobalVarLongMax(ctx, repository.UpsertGlobalVarLongMaxParams{
			Name: constt.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: maxID, Valid: true},
		}); err != nil {
			return fmt.Errorf("save hearth object ID high watermark: %w", err)
		}
		return nil
	})
}

// RepairInvalidHome removes only the observed association while its owner or
// source remains invalid. It never changes or recreates the source object.
func (s *ConstructionSaverDB) RepairInvalidHome(ctx context.Context, character types.EntityID, key ObjectKey, hearthType int) error {
	if s == nil || s.withTx == nil || ctx == nil || !validID(character) ||
		!validHome(Home{Object: key, Layer: SurfaceLayer}) || hearthType <= 0 || hearthType > math.MaxInt32 {
		return fmt.Errorf("repair invalid hearth: invalid dependencies or identity")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.withTx(ctx, func(q *repository.Queries) error {
		rows, err := q.DeleteInvalidCharacterHearth(ctx, repository.DeleteInvalidCharacterHearthParams{
			CharacterID: int64(character), ObjectRegion: key.Region, ObjectID: int64(key.ID),
			HearthTypeID: hearthType, SurfaceLayer: SurfaceLayer,
		})
		if err != nil {
			return fmt.Errorf("repair invalid hearth owner: %w", err)
		}
		if rows < 0 || rows > 1 {
			return fmt.Errorf("repair invalid hearth owner: expected zero or one association, removed %d", rows)
		}
		return nil
	})
}

func constructionObjectParams(object *repository.Object) repository.UpsertObjectParams {
	return repository.UpsertObjectParams{
		ID: object.ID, TypeID: object.TypeID, Region: object.Region,
		X: object.X, Y: object.Y, Layer: object.Layer, ChunkX: object.ChunkX, ChunkY: object.ChunkY,
		Heading: object.Heading, Quality: object.Quality, Hp: object.Hp, OwnerID: object.OwnerID,
		Data: object.Data, CreateTick: object.CreateTick, LastTick: object.LastTick,
	}
}
