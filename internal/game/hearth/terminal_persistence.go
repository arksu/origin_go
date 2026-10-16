package hearth

import (
	"context"
	"fmt"
	"math"
	"time"

	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// SaveTerminal persists the current conversion and removes only its matching
// association. The caller holds the owning shard lock and any source chunk pin;
// object must be the current converted campfire, not a stale saved row. A nil
// object removes the association without recreating an unavailable source.
func (s *ConstructionSaverDB) SaveTerminal(ctx context.Context, character types.EntityID, key ObjectKey, object *repository.Object) error {
	if s == nil || s.withTx == nil || ctx == nil || !validID(character) ||
		!validHome(Home{Object: key, Layer: SurfaceLayer}) {
		return fmt.Errorf("save terminal hearth: invalid dependencies or identity")
	}
	if object != nil && (object.ID != int64(key.ID) || object.Region != key.Region || object.DeletedAt.Valid ||
		!validHome(Home{Object: key, X: object.X, Y: object.Y, Layer: object.Layer}) ||
		object.TypeID <= 0 || object.TypeID > math.MaxInt32 ||
		object.Hp.Valid && (math.IsNaN(object.Hp.Float64) || math.IsInf(object.Hp.Float64, 0))) {
		return fmt.Errorf("save terminal hearth: invalid converted object")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.withTx(ctx, func(q *repository.Queries) error {
		if object != nil {
			if err := q.UpsertObject(ctx, constructionObjectParams(object)); err != nil {
				return fmt.Errorf("save terminal hearth conversion: %w", err)
			}
		}
		rows, err := q.RemoveCharacterHearth(ctx, repository.RemoveCharacterHearthParams{
			CharacterID: int64(character), ObjectRegion: key.Region, ObjectID: int64(key.ID),
		})
		if err != nil {
			return fmt.Errorf("remove terminal hearth owner: %w", err)
		}
		if rows < 0 || rows > 1 {
			return fmt.Errorf("remove terminal hearth owner: expected zero or one association, removed %d", rows)
		}
		return nil
	})
}
