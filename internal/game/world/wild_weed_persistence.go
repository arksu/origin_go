package world

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	"origin/internal/game/inventory"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// WildWeedObjectDeletion is captured before the live object is quarantined.
// Keeping its identity permits retries after the object row was already deleted.
type WildWeedObjectDeletion struct {
	SpotID         int64
	EntityID       types.EntityID
	RuntimeSeconds int64
}

// SpotStateCommit owns the state returned by the object's committed transaction.
// It is applied only by the owning shard, never by the persistence worker.
type SpotStateCommit struct {
	SpotID   int64
	Snapshot SpotStateSnapshot
}

func validWildWeedDeletion(region int, deletion WildWeedObjectDeletion) bool {
	return region > 0 && deletion.SpotID > 0 && deletion.EntityID > 0 &&
		deletion.EntityID <= math.MaxInt64 && deletion.RuntimeSeconds >= 0
}

// ReplaceWildWeedWithDroppedItems joins the manifest change to the existing
// streaming destruction transaction; no independent spot save can race it.
func (p *DroppedItemPersisterDB) ReplaceWildWeedWithDroppedItems(
	ctx context.Context,
	region int,
	deletion WildWeedObjectDeletion,
	maxAllocatedID types.EntityID,
	next func([]inventory.DroppedItemPersistenceRecord) ([]inventory.DroppedItemPersistenceRecord, error),
) (SpotStateCommit, error) {
	if !validWildWeedDeletion(region, deletion) {
		return SpotStateCommit{}, fmt.Errorf("delete wild weed: invalid identity or runtime")
	}
	return p.persistObjectReplacement(ctx, region, deletion.EntityID, maxAllocatedID, nil, next, &deletion)
}

// DeleteWildWeedObject is the ordinary administrative deletion with the same
// atomic manifest update as gameplay destruction.
func (p *DroppedItemPersisterDB) DeleteWildWeedObject(ctx context.Context, region int, deletion WildWeedObjectDeletion) (SpotStateCommit, error) {
	if p == nil || p.db == nil || ctx == nil || !validWildWeedDeletion(region, deletion) {
		return SpotStateCommit{}, fmt.Errorf("delete wild weed: invalid database, context or identity")
	}
	var commit SpotStateCommit
	err := p.db.WithTx(ctx, func(q *repository.Queries) error {
		if err := deleteObjectRows(ctx, q, region, deletion.EntityID, true); err != nil {
			return err
		}
		var err error
		commit, err = deleteWildWeedSpotObject(ctx, q, region, deletion)
		return err
	})
	if err != nil {
		return SpotStateCommit{}, err
	}
	return commit, nil
}

func deleteWildWeedSpotObject(ctx context.Context, q *repository.Queries, region int, deletion WildWeedObjectDeletion) (SpotStateCommit, error) {
	row, err := q.DeleteWildWeedSpotObject(ctx, repository.DeleteWildWeedSpotObjectParams{
		ID: deletion.SpotID, Region: region,
		ObjectID: strconv.FormatUint(uint64(deletion.EntityID), 10), RuntimeSeconds: deletion.RuntimeSeconds,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SpotStateCommit{}, fmt.Errorf("delete wild weed from spot %d: missing WWW spot, malformed manifest or exhausted revision", deletion.SpotID)
		}
		return SpotStateCommit{}, fmt.Errorf("delete wild weed from spot %d: %w", deletion.SpotID, err)
	}
	return SpotStateCommit{SpotID: deletion.SpotID, Snapshot: SpotStateSnapshot{
		State: row.State, Revision: row.Revision, LastRuntimeSeconds: row.LastRuntimeSeconds,
	}}, nil
}

// ApplyCommittedState publishes an owned snapshot under the owning shard lock.
// Independent committed deletes may be acknowledged in reverse order.
func (m *SpotManager) ApplyCommittedState(commit SpotStateCommit) error {
	spot, ok := m.byID[commit.SpotID]
	if !ok {
		return ErrSpotNotFound
	}
	if commit.Snapshot.Revision <= 0 || commit.Snapshot.LastRuntimeSeconds < 0 || !validSpotState(commit.Snapshot.State) {
		return ErrInvalidSpotState
	}
	if commit.Snapshot.Revision > spot.state.Revision {
		spot.state = commit.Snapshot
		spot.state.State = append(json.RawMessage(nil), commit.Snapshot.State...)
	}
	return nil
}
