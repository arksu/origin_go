package hearth

import (
	"context"
	"fmt"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

// NotifyTerminal may run on another shard. It only marks copied ownership
// metadata; the surface owner performs the conversion and persistence later.
// Login must already have established known metadata for every living character.
func (r *Runtime) NotifyTerminal(character types.EntityID) error {
	if err := r.notifyTerminal(character); err != nil {
		r.logger.Fatal("hearth terminal notification failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Runtime) notifyTerminal(character types.EntityID) error {
	if r.Index == nil {
		return fmt.Errorf("hearth index is unavailable")
	}
	if err := r.Index.MarkTerminalPending(character); err != nil {
		return err
	}
	if r.Index.Snapshot(character).State == Unknown {
		return ErrUnknownHome
	}
	return nil
}

// CompleteTerminal runs synchronously while the caller holds the surface shard
// lock. It resolves the current home here, never from an earlier death snapshot.
func (r *Runtime) CompleteTerminal(character types.EntityID) error {
	if err := r.completeTerminal(character); err != nil {
		r.logger.Fatal("hearth terminal completion failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Runtime) completeTerminal(character types.EntityID) error {
	if !validID(character) {
		return ErrInvalidCharacter
	}
	if r.Index == nil {
		return fmt.Errorf("hearth index is unavailable")
	}
	current := r.Index.Snapshot(character)
	if current.Terminal {
		return nil
	}
	if current.State == Unknown {
		return ErrUnknownHome
	}
	if current.State == NoHome {
		_, err := r.Index.ApplyTerminal(character)
		return err
	}
	if current.State != CurrentHome || !validHome(current.Home) {
		return ErrInvalidHome
	}
	if r.w == nil || r.w.Layer != SurfaceLayer || r.chunks == nil || r.saver == nil {
		return fmt.Errorf("invalid hearth terminal dependencies")
	}
	defs := objectdefs.Global()
	if defs == nil {
		return fmt.Errorf("hearth definitions are unavailable")
	}
	hearthDef, hearthOK := defs.GetByKey("hearth_fire")
	campfireDef, campfireOK := defs.GetByKey("campfire")
	if !hearthOK || hearthDef == nil || !campfireOK || campfireDef == nil || campfireDef.HP <= 0 ||
		campfireDef.BurnerConfig == nil || campfireDef.Station == nil || campfireDef.Station.InitialState != "unlit" {
		return fmt.Errorf("hearth conversion definitions are unavailable")
	}

	home := current.Home
	coord := types.WorldToChunkCoord(home.X, home.Y, constt.ChunkSize, constt.CoordPerTile)
	if err := r.chunks.PinPersistenceBlocking(coord); err != nil {
		return fmt.Errorf("pin terminal hearth chunk: %w", err)
	}
	defer r.chunks.UnpinPersistence(coord)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := r.chunks.WaitPreloaded(ctx, coord)
	cancel()
	if err != nil {
		return fmt.Errorf("load terminal hearth chunk: %w", err)
	}
	chunk := r.chunks.GetChunkFast(coord)
	if chunk == nil || chunk.Coord != coord || chunk.Region != home.Object.Region || chunk.Layer != SurfaceLayer ||
		(chunk.GetState() != types.ChunkStateActive && chunk.GetState() != types.ChunkStatePreloaded && chunk.GetState() != types.ChunkStateInactive) {
		return fmt.Errorf("invalid terminal hearth chunk")
	}

	handle := types.InvalidHandle
	var converted *repository.Object
	if _, deleted := chunk.GetDeletedObjectIDs()[home.Object.ID]; !deleted {
		candidate := r.w.GetHandleByEntityID(home.Object.ID)
		if candidate != types.InvalidHandle {
			if r.canTransform(home.Object.ID, candidate, uint32(hearthDef.DefID)) {
				info, _ := ecs.GetComponent[components.EntityInfo](r.w, candidate)
				if info.Region == home.Object.Region && info.Layer == SurfaceLayer {
					handle = candidate
				}
			}
		} else if raw := chunk.GetRawObjectByID(home.Object.ID); raw != nil && !raw.DeletedAt.Valid &&
			raw.ID == int64(home.Object.ID) && raw.Region == home.Object.Region && raw.Layer == SurfaceLayer && raw.TypeID == hearthDef.DefID {
			converted = rawCampfire(raw, campfireDef, int64(ecs.GetResource[ecs.TimeState](r.w).Tick))
		}
	}
	// Only notification may race this owner-locked preparation. It may change
	// PendingTerminal, but it cannot replace the home being prepared.
	latest := r.Index.Snapshot(character)
	if latest.Terminal || latest.State != CurrentHome || latest.Home != home {
		return fmt.Errorf("terminal hearth changed during owning preparation")
	}
	if handle != types.InvalidHandle {
		if r.factory == nil {
			return fmt.Errorf("hearth object factory is unavailable")
		}
		r.breakLinksBeforeConversion(home.Object.ID)
		if !world.TransformObjectToDefInPlace(r.w, home.Object.ID, handle, campfireDef, world.TransformObjectInPlaceOptions{
			ClearFlags: true, BehaviorRegistry: r.registry, Chunks: r.chunks, EventBus: r.bus, Logger: r.logger,
		}) {
			return fmt.Errorf("prepared terminal hearth conversion failed")
		}
	} else if converted != nil {
		chunk.UpsertRawObject(converted)
	}
	if _, err := r.Index.ApplyTerminal(character); err != nil {
		return fmt.Errorf("revoke terminal hearth: %w", err)
	}
	if handle != types.InvalidHandle {
		converted, err = r.factory.Serialize(r.w, handle)
		if err != nil {
			return fmt.Errorf("serialize terminal hearth conversion: %w", err)
		}
		if converted == nil {
			return fmt.Errorf("terminal hearth conversion is not persistable")
		}
		converted.LastTick = int64(ecs.GetResource[ecs.TimeState](r.w).Tick)
	}
	// A missing, changed or quarantined source only removes its association. In
	// particular, never recreate an object from the old ownership metadata.
	if err := r.saver.SaveTerminal(context.Background(), character, home.Object, converted); err != nil {
		return fmt.Errorf("save terminal hearth: %w", err)
	}
	return nil
}
