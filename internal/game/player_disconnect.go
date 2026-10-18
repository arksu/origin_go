package game

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

// The disconnect caller has released its source shard lock before doing SQL or
// acquiring the surface lock. A duplicate offline transition returns no row.
func (g *Game) recordCharacterDisconnect(character types.EntityID, at time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := g.db.Queries().SetCharacterDisconnected(ctx, repository.SetCharacterDisconnectedParams{
		ID: int64(character), DisconnectedAt: at,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		g.logger.Error("Failed to record character disconnect", zap.Uint64("character_id", uint64(character)), zap.Error(err))
		return
	}
}

// Network callbacks skip normal detach while stopping. Persist their remaining
// online rows together after network handlers and login goroutines have drained.
func (g *Game) recordShutdownDisconnects(at time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := g.db.Queries().SetOnlineCharactersDisconnected(ctx, repository.SetOnlineCharactersDisconnectedParams{
		Region: g.cfg.Game.Region, DisconnectedAt: at,
	}); err != nil {
		g.logger.Error("Failed to record shutdown disconnects", zap.Error(err))
	}
}
