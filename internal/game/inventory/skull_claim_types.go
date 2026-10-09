package inventory

import (
	"errors"

	"origin/internal/ecs/systems"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

var (
	ErrSkullClaimSourceConflict    = errors.New("skull claim source is no longer available")
	ErrSkullClaimCharacterMissing  = errors.New("dead character metadata is unavailable")
	ErrSkullClaimInventoryConflict = errors.New("skull claim recipient inventory has a newer version")
)

type DeadCharacterInfo struct {
	Nickname  string
	DeathDate string // YYYY-MM-DD in the server's local calendar.
}

type SkullClaimReceipt struct {
	SkullItemID types.EntityID `json:"skull_item_id"`
	RecipientID types.EntityID `json:"recipient_id"`
}

// SkullClaimPersistenceRecord owns the immutable pre-claim source, post-claim
// replacement, and complete post-grant recipient root snapshots for one claim.
// A repeated claim must use the same identity and payload after an uncertain commit.
type SkullClaimPersistenceRecord struct {
	Source            *repository.Object
	Replacement       *repository.Object
	RecipientID       types.EntityID
	SkullItemID       types.EntityID
	PlayerInventories []systems.InventorySnapshot
}
