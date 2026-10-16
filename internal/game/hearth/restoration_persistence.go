package hearth

import (
	"context"
	"fmt"
	"time"

	"origin/internal/persistence/repository"
)

// SaveOrphans persists current raw conversions while activation holds the owning
// shard lock and chunk I/O gate. These existing objects need no new ID watermark.
func (s *ConstructionSaverDB) SaveOrphans(ctx context.Context, objects []*repository.Object) error {
	if len(objects) == 0 {
		return nil
	}
	if s == nil || s.withTx == nil || ctx == nil {
		return fmt.Errorf("save orphan hearths: missing dependencies")
	}
	for _, object := range objects {
		if object == nil || object.ID <= 0 {
			return fmt.Errorf("save orphan hearths: invalid object")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.withTx(ctx, func(q *repository.Queries) error {
		for _, object := range objects {
			if err := q.UpsertObject(ctx, constructionObjectParams(object)); err != nil {
				return fmt.Errorf("save orphan hearth %d: %w", object.ID, err)
			}
		}
		return nil
	})
}
