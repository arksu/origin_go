package game

import (
	"context"
	"fmt"
	"time"

	"origin/internal/ecs"
	"origin/internal/game/hearth"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

// DeleteCharacter serializes account-scoped deletion and hearth cleanup with
// surface construction and shutdown. Once SQL deletes the character, cleanup
// completes synchronously even if the request is canceled.
func (g *Game) DeleteCharacter(ctx context.Context, params repository.DeleteCharacterParams) error {
	if g.shardManager == nil {
		return fmt.Errorf("character deletion shard manager is unavailable")
	}
	surface := g.shardManager.GetShard(hearth.SurfaceLayer)
	if surface == nil {
		return fmt.Errorf("character deletion surface shard is unavailable")
	}
	surface.mu.Lock()
	defer surface.mu.Unlock()
	if surface.state != ShardStateRunning {
		return fmt.Errorf("character deletion surface shard is stopping")
	}
	if surface.world == nil {
		return fmt.Errorf("character deletion surface world is unavailable")
	}
	runtime, ok := ecs.TryGetResource[hearth.Runtime](surface.world)
	if !ok || runtime == nil || runtime.Index == nil {
		return fmt.Errorf("character deletion hearth runtime is unavailable")
	}
	if g.db == nil {
		return fmt.Errorf("character deletion database is unavailable")
	}
	queries := g.db.Queries()
	if queries == nil {
		return fmt.Errorf("character deletion queries are unavailable")
	}

	deleteCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	affected, err := queries.DeleteCharacter(deleteCtx, params)
	if err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}
	if affected != 1 || params.ID <= 0 {
		err := fmt.Errorf("invalid character deletion result: id=%d affected=%d", params.ID, affected)
		g.logger.Fatal("invalid character deletion result", zap.Error(err))
		return err
	}
	return runtime.CompleteDeletion(types.EntityID(params.ID))
}
