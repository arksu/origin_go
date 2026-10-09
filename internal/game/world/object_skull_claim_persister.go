package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// LoadDeadCharacter runs on a persistence worker, never on the shard tick.
// A deleted character's retained row is the authority for the skull inscription.
func (p *DroppedItemPersisterDB) LoadDeadCharacter(ctx context.Context, characterID types.EntityID) (inventory.DeadCharacterInfo, error) {
	if p == nil || p.db == nil || ctx == nil {
		return inventory.DeadCharacterInfo{}, fmt.Errorf("load dead character: database or context is unavailable")
	}
	if characterID == 0 || characterID > math.MaxInt64 {
		return inventory.DeadCharacterInfo{}, inventory.ErrSkullClaimCharacterMissing
	}
	row, err := p.db.Queries().GetDeadCharacterMetadata(ctx, int64(characterID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !row.DeletedAt.Valid) {
		return inventory.DeadCharacterInfo{}, inventory.ErrSkullClaimCharacterMissing
	}
	if err != nil {
		return inventory.DeadCharacterInfo{}, fmt.Errorf("load dead character %d: %w", characterID, err)
	}
	return inventory.DeadCharacterInfo{
		Nickname: row.Name, DeathDate: row.DeletedAt.Time.In(time.Local).Format("2006-01-02"),
	}, nil
}

// PersistSkullClaim atomically replaces the skeleton, grants the skull and
// advances the allocator. The source receipt is checked before any inventory
// write: replay after a lost commit acknowledgement must not roll back newer
// recipient inventory snapshots, even if they still have the same version.
func (p *DroppedItemPersisterDB) PersistSkullClaim(ctx context.Context, record inventory.SkullClaimPersistenceRecord) error {
	if p == nil || p.db == nil || ctx == nil {
		return fmt.Errorf("persist skull claim: database or context is unavailable")
	}
	if err := validateSkullClaimRecord(record); err != nil {
		return err
	}
	return p.db.WithTx(ctx, func(q *repository.Queries) error {
		source := record.Source
		// Chunk objects may not have reached their first save. Insert only if
		// absent, so a previous claim/deletion can never be resurrected.
		if err := q.InsertSkullClaimSourceIfMissing(ctx, repository.InsertSkullClaimSourceIfMissingParams{
			ID: source.ID, TypeID: source.TypeID, Region: source.Region,
			X: source.X, Y: source.Y, Layer: source.Layer, ChunkX: source.ChunkX, ChunkY: source.ChunkY,
			Heading: source.Heading, Quality: source.Quality, Hp: source.Hp, OwnerID: source.OwnerID,
			Data: source.Data, CreateTick: source.CreateTick, LastTick: source.LastTick,
		}); err != nil {
			return fmt.Errorf("ensure skull claim source: %w", err)
		}
		current, err := q.GetSkullClaimSourceForUpdate(ctx, repository.GetSkullClaimSourceForUpdateParams{
			Region: source.Region, ID: source.ID,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return inventory.ErrSkullClaimSourceConflict
		}
		if err != nil {
			return fmt.Errorf("lock skull claim source: %w", err)
		}
		if current.DeletedAt.Valid {
			return inventory.ErrSkullClaimSourceConflict
		}
		if current.TypeID == record.Replacement.TypeID {
			receipt, ok := skullClaimReceipt(current.Data.RawMessage)
			if ok && receipt.SkullItemID == record.SkullItemID && receipt.RecipientID == record.RecipientID {
				return nil
			}
			return inventory.ErrSkullClaimSourceConflict
		}
		if current.TypeID != source.TypeID {
			return inventory.ErrSkullClaimSourceConflict
		}
		if _, exists := skullClaimReceipt(current.Data.RawMessage); exists {
			return inventory.ErrSkullClaimSourceConflict
		}
		if err := persistPlayerInventorySnapshots(ctx, q, record.PlayerInventories); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("persist skull recipient: %w", inventory.ErrSkullClaimInventoryConflict)
			}
			return err
		}
		if err := q.UpsertGlobalVarLongMax(ctx, repository.UpsertGlobalVarLongMaxParams{
			Name: constt.LAST_USED_ID, ValueLong: sql.NullInt64{Int64: int64(record.SkullItemID), Valid: true},
		}); err != nil {
			return fmt.Errorf("persist skull ID high watermark: %w", err)
		}
		replacement := record.Replacement
		if err := q.UpsertObject(ctx, repository.UpsertObjectParams{
			ID: replacement.ID, TypeID: replacement.TypeID, Region: replacement.Region,
			X: replacement.X, Y: replacement.Y, Layer: replacement.Layer,
			ChunkX: replacement.ChunkX, ChunkY: replacement.ChunkY,
			Heading: replacement.Heading, Quality: replacement.Quality, Hp: replacement.Hp,
			OwnerID: replacement.OwnerID, Data: replacement.Data,
			CreateTick: replacement.CreateTick, LastTick: replacement.LastTick,
		}); err != nil {
			return fmt.Errorf("persist headless skeleton: %w", err)
		}
		return nil
	})
}

func validateSkullClaimRecord(record inventory.SkullClaimPersistenceRecord) error {
	if validateCommittedSource(record.Source) != nil || validateCommittedSource(record.Replacement) != nil ||
		record.Source.ID != record.Replacement.ID || record.Source.Region != record.Replacement.Region ||
		record.RecipientID == 0 || record.RecipientID > math.MaxInt64 || record.SkullItemID == 0 || record.SkullItemID > math.MaxInt64 {
		return inventory.ErrSkullClaimSourceConflict
	}
	registry := objectdefs.Global()
	source, sourceOK := registry.GetByID(record.Source.TypeID)
	replacement, replacementOK := registry.GetByID(record.Replacement.TypeID)
	if !sourceOK || !replacementOK || source.Key != "player_skeleton" || replacement.Key != "player_skeleton_without_skull" {
		return inventory.ErrSkullClaimSourceConflict
	}
	receipt, ok := skullClaimReceipt(record.Replacement.Data.RawMessage)
	if !record.Replacement.Data.Valid || !ok || receipt.SkullItemID != record.SkullItemID || receipt.RecipientID != record.RecipientID {
		return inventory.ErrSkullClaimSourceConflict
	}
	if len(record.PlayerInventories) == 0 {
		return inventory.ErrSkullClaimInventoryConflict
	}
	for _, snapshot := range record.PlayerInventories {
		if snapshot.CharacterID != int64(record.RecipientID) || snapshot.Kind < int16(constt.InventoryGrid) ||
			snapshot.Kind > int16(constt.InventoryEquipment) || snapshot.InventoryKey < 0 || snapshot.Version < 0 || !json.Valid(snapshot.Data) {
			return inventory.ErrSkullClaimInventoryConflict
		}
	}
	return nil
}

func skullClaimReceipt(data []byte) (inventory.SkullClaimReceipt, bool) {
	var envelope components.ObjectStateEnvelope
	if len(data) == 0 || json.Unmarshal(data, &envelope) != nil || envelope.Version != 1 {
		return inventory.SkullClaimReceipt{}, false
	}
	var receipt inventory.SkullClaimReceipt
	if json.Unmarshal(envelope.Behaviors["player_skeleton"], &receipt) != nil || receipt.SkullItemID == 0 || receipt.RecipientID == 0 {
		return inventory.SkullClaimReceipt{}, false
	}
	return receipt, true
}
