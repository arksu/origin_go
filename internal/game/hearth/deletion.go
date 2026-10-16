package hearth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

// CompleteDeletion runs synchronously under the surface shard lock after an
// authorized character deletion. Only this path may resolve an Unknown owner;
// actual death still requires metadata established before entering the world.
func (r *Runtime) CompleteDeletion(character types.EntityID) error {
	if err := r.completeDeletion(character); err != nil {
		r.logger.Fatal("hearth deletion completion failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Runtime) completeDeletion(character types.EntityID) error {
	if r.Index == nil {
		return fmt.Errorf("hearth index is unavailable")
	}
	if err := r.Index.MarkTerminalPending(character); err != nil {
		return err
	}
	current := r.Index.Snapshot(character)
	if current.Terminal || current.State != Unknown {
		return r.completeTerminal(character)
	}
	if r.reader == nil {
		return fmt.Errorf("hearth reader is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	row, err := r.reader.GetSavedCharacterHearth(ctx, int64(character))
	cancel()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read deleted character hearth: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) {
		if err := r.Index.LoadIfUnknown(character, nil); err != nil {
			return err
		}
		return r.completeTerminal(character)
	}
	key := ObjectKey{Region: row.ObjectRegion, ID: types.EntityID(row.ObjectID)}
	if row.CharacterID != int64(character) || !validHome(Home{Object: key}) {
		return fmt.Errorf("invalid saved hearth association")
	}
	// LEFT JOIN produces either all source fields or none. A partial source is
	// an invalid query result, rather than evidence that the object is missing.
	present := row.TypeID.Valid
	if row.X.Valid != present || row.Y.Valid != present || row.Layer.Valid != present || (!present && row.DeletedAt.Valid) {
		return fmt.Errorf("invalid saved hearth source")
	}
	if present && !row.DeletedAt.Valid && row.Layer.Int32 == SurfaceLayer {
		defs := objectdefs.Global()
		if defs == nil {
			return fmt.Errorf("hearth definitions are unavailable")
		}
		hearthDef, found := defs.GetByKey("hearth_fire")
		if !found || hearthDef == nil {
			return fmt.Errorf("hearth definition is unavailable")
		}
		if int(row.TypeID.Int32) == hearthDef.DefID {
			home := Home{Object: key, X: int(row.X.Int32), Y: int(row.Y.Int32), Layer: SurfaceLayer}
			if err := r.Index.LoadIfUnknown(character, &home); err != nil {
				return err
			}
			return r.completeTerminal(character)
		}
	}
	// An unusable saved source needs only exact association removal. Loading its
	// chunk or upserting an object here could recreate a deleted or changed row.
	if r.saver == nil {
		return fmt.Errorf("hearth saver is unavailable")
	}
	if _, err := r.Index.ApplyTerminal(character); err != nil {
		return err
	}
	if err := r.saver.SaveTerminal(context.Background(), character, key, nil); err != nil {
		return fmt.Errorf("save deleted character hearth: %w", err)
	}
	return nil
}
