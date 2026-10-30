package inventory

import (
	"errors"

	"origin/internal/ecs/systems"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
)

var (
	ErrInvalidCropGrant             = errors.New("invalid prepared crop grant")
	ErrCropHarvestSourceConflict    = errors.New("crop harvest source is no longer available")
	ErrCropHarvestInventoryConflict = errors.New("crop harvest recipient inventory has a newer version")
)

const MaxCropHarvestUnits = 32

// CropHarvestReceipt is retained on the deleted source. The first allocated
// item identifies this immutable operation even when every output goes to ground.
type CropHarvestReceipt struct {
	FirstItemID types.EntityID `json:"first_item_id"`
	LastItemID  types.EntityID `json:"last_item_id"`
	RecipientID types.EntityID `json:"recipient_id"`
}

type CropHarvestPersistenceRecord struct {
	Source            *repository.Object
	Receipt           CropHarvestReceipt
	DeletedData       pqtype.NullRawMessage
	PlayerInventories []systems.InventorySnapshot
	Drops             []DroppedItemPersistenceRecord
}
