package hearth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/playerstate"
	"origin/internal/types"

	"go.uber.org/zap"
)

type ConstructionChunks interface {
	GetChunkFast(types.ChunkCoord) *core.Chunk
	PinPersistenceBlocking(types.ChunkCoord) error
	UnpinPersistence(types.ChunkCoord)
	WaitPreloaded(context.Context, types.ChunkCoord) error
}

type HomeReader interface {
	GetCharacterHearth(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error)
	GetCharacterHearthOwners(context.Context, repository.GetCharacterHearthOwnersParams) ([]repository.GetCharacterHearthOwnersRow, error)
	GetSavedCharacterHearth(context.Context, int64) (repository.GetSavedCharacterHearthRow, error)
}

// Runtime belongs to the surface shard. Home reads, ownership changes and saves
// run synchronously while the caller holds that shard's lock.
type Runtime struct {
	Index    *Index
	w        *ecs.World
	chunks   ConstructionChunks
	reader   HomeReader
	factory  *world.ObjectFactory
	saver    ConstructionSaver
	registry contracts.BehaviorRegistry
	bus      *eventbus.EventBus
	logger   *zap.Logger
}

func NewRuntime(w *ecs.World, chunks ConstructionChunks, reader HomeReader, factory *world.ObjectFactory, saver ConstructionSaver, registry contracts.BehaviorRegistry, bus *eventbus.EventBus, logger *zap.Logger) Runtime {
	if logger == nil {
		logger = zap.NewNop()
	}
	return Runtime{Index: NewIndex(), w: w, chunks: chunks, reader: reader, factory: factory, saver: saver, registry: registry, bus: bus, logger: logger}
}

func constructionRefused(reason string) contracts.BehaviorResult {
	return contracts.BehaviorResult{UserVisible: true, ReasonCode: reason, Severity: contracts.BehaviorAlertSeverityWarning}
}

func (r *Runtime) CompleteConstruction(ctx *contracts.ConstructionContext, complete func() bool) contracts.BehaviorResult {
	if ctx == nil || ctx.World != r.w || r.w.Layer != SurfaceLayer || complete == nil ||
		!r.w.Alive(ctx.PlayerHandle) || r.w.GetHandleByEntityID(ctx.PlayerID) != ctx.PlayerHandle ||
		playerstate.ItemsLocked(r.w, ctx.PlayerHandle) {
		return constructionRefused("Cannot complete this hearth here.")
	}
	if r.factory == nil || r.saver == nil {
		return constructionRefused("Hearth persistence is unavailable.")
	}
	defs := objectdefs.Global()
	if defs == nil {
		return constructionRefused("Hearth definitions are unavailable.")
	}
	hearthDef, hearthOK := defs.GetByKey("hearth_fire")
	campfireDef, campfireOK := defs.GetByKey("campfire")
	if !hearthOK || !campfireOK || hearthDef == nil || campfireDef == nil || ctx.ResultType != uint32(hearthDef.DefID) ||
		hearthDef.HP <= 0 || campfireDef.HP <= 0 || campfireDef.BurnerConfig == nil || campfireDef.Station == nil || campfireDef.Station.InitialState != "unlit" {
		return constructionRefused("Hearth definitions are unavailable.")
	}
	if !r.canTransform(ctx.TargetID, ctx.TargetHandle, constt.BuildObjectTypeID) {
		return constructionRefused("This construction site is unavailable.")
	}
	info, _ := ecs.GetComponent[components.EntityInfo](r.w, ctx.TargetHandle)
	position, positioned := ecs.GetComponent[components.Transform](r.w, ctx.TargetHandle)
	if !positioned || info.Layer != SurfaceLayer {
		return constructionRefused("Cannot complete this hearth here.")
	}
	next := Home{Object: ObjectKey{Region: info.Region, ID: ctx.TargetID}, X: int(position.X), Y: int(position.Y), Layer: SurfaceLayer}
	if !validHome(next) {
		return constructionRefused("This construction site is unavailable.")
	}
	if r.Index == nil {
		return constructionRefused("Hearth data is unavailable.")
	}
	current := r.Index.Snapshot(ctx.PlayerID)
	if current.State == Unknown && !current.PendingTerminal && !current.Terminal {
		if err := r.LoadHome(ctx.PlayerID, hearthDef.DefID); err != nil {
			return constructionRefused("Hearth data is unavailable.")
		}
		current = r.Index.Snapshot(ctx.PlayerID)
	}
	if current.PendingTerminal || current.Terminal {
		return constructionRefused("This character cannot acquire a hearth.")
	}
	if current.State == Unknown {
		return constructionRefused("Hearth data is unavailable.")
	}

	oldHandle := types.InvalidHandle
	var oldChunk *core.Chunk
	var oldRaw *repository.Object
	if current.State == CurrentHome {
		old := current.Home
		if old.Object.Region != info.Region || old.Object.ID == ctx.TargetID {
			return constructionRefused("The previous hearth is unavailable.")
		}
		oldHandle = r.w.GetHandleByEntityID(old.Object.ID)
		if oldHandle != types.InvalidHandle {
			if !r.canTransform(old.Object.ID, oldHandle, uint32(hearthDef.DefID)) {
				return constructionRefused("The previous hearth is unavailable.")
			}
		} else {
			coord := types.WorldToChunkCoord(old.X, old.Y, constt.ChunkSize, constt.CoordPerTile)
			if r.chunks == nil {
				return constructionRefused("The previous hearth is unavailable.")
			}
			if err := r.chunks.PinPersistenceBlocking(coord); err != nil {
				return constructionRefused("The previous hearth is unavailable.")
			}
			defer r.chunks.UnpinPersistence(coord)
			loadCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := r.chunks.WaitPreloaded(loadCtx, coord)
			cancel()
			if err != nil {
				if errors.Is(err, world.ErrChunkNotLoaded) {
					return constructionRefused("The previous hearth is unavailable.")
				}
				r.logger.Fatal("hearth chunk load failed", zap.Error(err))
				return contracts.BehaviorResult{}
			}
			oldChunk = r.chunks.GetChunkFast(coord)
			if oldChunk == nil || (oldChunk.GetState() != types.ChunkStateActive && oldChunk.GetState() != types.ChunkStatePreloaded && oldChunk.GetState() != types.ChunkStateInactive) {
				return constructionRefused("The previous hearth is unavailable.")
			}
			raw := oldChunk.GetRawObjectByID(old.Object.ID)
			if raw == nil || raw.DeletedAt.Valid || raw.TypeID != hearthDef.DefID || raw.Region != old.Object.Region || raw.Layer != SurfaceLayer {
				return constructionRefused("The previous hearth is unavailable.")
			}
			oldRaw = rawCampfire(raw, campfireDef, int64(ecs.GetResource[ecs.TimeState](r.w).Tick))
		}
	}
	if !r.Index.Snapshot(ctx.PlayerID).CanAcquire() {
		return constructionRefused("This character's hearth has changed.")
	}
	if !complete() {
		return contracts.BehaviorResult{}
	}
	if oldHandle != types.InvalidHandle {
		if !world.TransformObjectToDefInPlace(r.w, current.Home.Object.ID, oldHandle, campfireDef, world.TransformObjectInPlaceOptions{
			ClearFlags: true, BehaviorRegistry: r.registry, Chunks: r.chunks, EventBus: r.bus, Logger: r.logger,
		}) {
			panic("prepared hearth conversion failed")
		}
	} else if oldRaw != nil {
		oldChunk.UpsertRawObject(oldRaw)
	}
	_, err := r.Index.ReplaceHome(ctx.PlayerID, next)
	if err != nil {
		panic("prepared hearth assignment failed: " + err.Error())
	}
	if err := r.saveConstruction(ctx.PlayerID, ctx.TargetHandle, oldHandle, oldRaw); err != nil {
		r.logger.Fatal("hearth construction save failed", zap.Error(err))
		return contracts.BehaviorResult{}
	}
	return contracts.BehaviorResult{OK: true}
}

func (r *Runtime) saveConstruction(character types.EntityID, newHandle, oldHandle types.Handle, oldRaw *repository.Object) error {
	newObject, err := r.factory.Serialize(r.w, newHandle)
	if err != nil {
		return fmt.Errorf("serialize new hearth: %w", err)
	}
	previousObject := oldRaw
	if oldHandle != types.InvalidHandle {
		previousObject, err = r.factory.Serialize(r.w, oldHandle)
		if err != nil {
			return fmt.Errorf("serialize previous hearth: %w", err)
		}
	}
	if newObject == nil || (oldHandle != types.InvalidHandle && previousObject == nil) {
		return fmt.Errorf("completed hearth objects are not persistable")
	}
	newObject.LastTick = int64(ecs.GetResource[ecs.TimeState](r.w).Tick)
	if oldHandle != types.InvalidHandle {
		previousObject.LastTick = newObject.LastTick
	}
	return r.saver.SaveConstruction(context.Background(), character, newObject, previousObject)
}

func (r *Runtime) canTransform(id types.EntityID, handle types.Handle, typeID uint32) bool {
	if !r.w.Alive(handle) || r.w.GetHandleByEntityID(id) != handle || ecs.ObjectDestructionPending(r.w, handle) || ecs.InventoryHandleReserved(r.w, handle) {
		return false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](r.w, handle)
	return ok && info.TypeID == typeID && ecs.HasComponent[components.ObjectInternalState](r.w, handle)
}

// LoadHome restores unknown ownership metadata. The caller must hold the owning
// surface shard lock; synchronous reads and repair complete independently of logout.
func (r *Runtime) LoadHome(character types.EntityID, hearthType int) error {
	if r.Index == nil {
		return errors.New("hearth index is unavailable")
	}
	if !validID(character) {
		return ErrInvalidCharacter
	}
	if r.Index.Snapshot(character).State != Unknown {
		return nil
	}
	if r.reader == nil {
		return errors.New("hearth reader is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	row, err := r.reader.GetCharacterHearth(ctx, repository.GetCharacterHearthParams{
		CharacterID: int64(character), HearthTypeID: hearthType, SurfaceLayer: SurfaceLayer,
	})
	cancel()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		r.logger.Fatal("hearth lookup failed", zap.Error(err))
		return err
	}
	var home *Home
	if err == nil {
		home = &Home{Object: ObjectKey{Region: row.ObjectRegion, ID: types.EntityID(row.ObjectID)}, X: row.X, Y: row.Y, Layer: row.Layer}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		saved, err := r.reader.GetSavedCharacterHearth(ctx, int64(character))
		cancel()
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			r.logger.Fatal("saved hearth lookup failed", zap.Error(err))
			return err
		}
		if err == nil {
			key := ObjectKey{Region: saved.ObjectRegion, ID: types.EntityID(saved.ObjectID)}
			if saved.CharacterID != int64(character) || !validHome(Home{Object: key, Layer: SurfaceLayer}) {
				err := fmt.Errorf("invalid saved hearth association")
				r.logger.Fatal("invalid saved hearth lookup result", zap.Error(err))
				return err
			}
			if r.saver == nil {
				err := fmt.Errorf("hearth saver is unavailable")
				r.logger.Fatal("hearth repair failed", zap.Error(err))
				return err
			}
			if err := r.saver.RepairInvalidHome(context.Background(), character, key, hearthType); err != nil {
				r.logger.Fatal("hearth repair failed", zap.Error(err))
				return err
			}
		}
	}
	if err := r.Index.LoadIfUnknown(character, home); err != nil {
		r.logger.Fatal("invalid hearth lookup result", zap.Error(err))
		return err
	}
	return nil
}
