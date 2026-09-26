package core

import (
	"context"
	"database/sql"
	"fmt"

	"origin/internal/persistence/repository"
)

type chunkTileWriter interface {
	UpsertChunk(context.Context, repository.UpsertChunkParams) (int64, error)
}

func (c *Chunk) saveTiles(ctx context.Context, writer chunkTileWriter, entityCount int) error {
	c.mu.RLock()
	if !c.tilesDirty {
		c.mu.RUnlock()
		return nil
	}
	// The version identifies these exact bytes, including when another edit overlaps I/O.
	snapshot := repository.UpsertChunkParams{
		Region: c.Region, X: c.Coord.X, Y: c.Coord.Y, Layer: c.Layer,
		TilesData: append([]byte(nil), c.Tiles...), Version: int(c.Version), LastTick: int64(c.LastTick),
		EntityCount: sql.NullInt32{Int32: int32(entityCount), Valid: true},
	}
	c.mu.RUnlock()
	affected, err := writer.UpsertChunk(ctx, snapshot)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("chunk %v save version %d superseded: %d rows affected", c.Coord, snapshot.Version, affected)
	}
	c.mu.Lock()
	if c.Version == uint32(snapshot.Version) {
		c.tilesDirty = false
	}
	c.mu.Unlock()
	return nil
}
