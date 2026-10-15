package hearth

import (
	"context"
	"database/sql"
	"errors"
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

const preparationCapacity = 64
const preparationApplyBudget = 32

type ConstructionChunks interface {
	GetChunkFast(types.ChunkCoord) *core.Chunk
	PinPersistence(types.ChunkCoord) error
	UnpinPersistence(types.ChunkCoord)
	SubmitPersistenceJob(world.PersistenceJob) bool
	WaitPreloaded(context.Context, types.ChunkCoord) error
}

type HomeReader interface {
	GetCharacterHearth(context.Context, repository.GetCharacterHearthParams) (repository.GetCharacterHearthRow, error)
}

// Runtime belongs to the surface shard. Ownership lookup is synchronous;
// only old-chunk preloads run outside its lock.
type Runtime struct {
	Index     *Index
	w         *ecs.World
	chunks    ConstructionChunks
	reader    HomeReader
	registry  contracts.BehaviorRegistry
	bus       *eventbus.EventBus
	logger    *zap.Logger
	completed chan *preparation
	inFlight  int
	preloads  map[types.ChunkCoord]bool
}

func NewRuntime(w *ecs.World, chunks ConstructionChunks, reader HomeReader, registry contracts.BehaviorRegistry, bus *eventbus.EventBus, logger *zap.Logger) Runtime {
	if logger == nil {
		logger = zap.NewNop()
	}
	return Runtime{Index: NewIndex(), w: w, chunks: chunks, reader: reader, registry: registry, bus: bus, logger: logger,
		completed: make(chan *preparation, preparationCapacity), preloads: make(map[types.ChunkCoord]bool)}
}

func constructionRefused(reason string) contracts.ConstructionResult {
	return contracts.ConstructionResult{BehaviorResult: contracts.BehaviorResult{
		UserVisible: true, ReasonCode: reason, Severity: contracts.BehaviorAlertSeverityWarning,
	}}
}

func (r *Runtime) CompleteConstruction(ctx *contracts.ConstructionContext, complete func() bool) contracts.ConstructionResult {
	if ctx == nil || ctx.World != r.w || r.w.Layer != SurfaceLayer || complete == nil ||
		!r.w.Alive(ctx.PlayerHandle) || r.w.GetHandleByEntityID(ctx.PlayerID) != ctx.PlayerHandle ||
		playerstate.ItemsLocked(r.w, ctx.PlayerHandle) {
		return constructionRefused("Cannot complete this hearth here.")
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
	current := r.Index.Snapshot(ctx.PlayerID)
	if current.State == Unknown && !current.PendingTerminal && !current.Terminal {
		if r.reader == nil {
			return constructionRefused("Hearth data is unavailable.")
		}
		r.loadHome(ctx.PlayerID, hearthDef.DefID)
		current = r.Index.Snapshot(ctx.PlayerID)
	}
	if current.PendingTerminal || current.Terminal {
		return constructionRefused("This character cannot acquire a hearth.")
	}
	if current.State == Unknown {
		return contracts.ConstructionResult{Retry: true}
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
			if err := r.chunks.PinPersistence(coord); err != nil {
				if errors.Is(err, world.ErrChunkPersistenceBusy) {
					return contracts.ConstructionResult{Retry: true}
				}
				return constructionRefused("The previous hearth is unavailable.")
			}
			defer r.chunks.UnpinPersistence(coord)
			oldChunk = r.chunks.GetChunkFast(coord)
			if oldChunk == nil || (oldChunk.GetState() != types.ChunkStateActive && oldChunk.GetState() != types.ChunkStatePreloaded && oldChunk.GetState() != types.ChunkStateInactive) {
				r.prepareChunk(coord)
				return contracts.ConstructionResult{Retry: true}
			}
			raw := oldChunk.GetRawObjectByID(old.Object.ID)
			if raw == nil || raw.DeletedAt.Valid || raw.TypeID != hearthDef.DefID || raw.Region != old.Object.Region || raw.Layer != SurfaceLayer {
				return constructionRefused("The previous hearth is unavailable.")
			}
			copy := *raw
			copy.TypeID = campfireDef.DefID
			copy.Hp = sql.NullFloat64{Float64: float64(campfireDef.HP), Valid: true}
			// A fresh campfire restores its ordinary unlit station and burner state.
			copy.Data.Valid, copy.Data.RawMessage = false, nil
			oldRaw = &copy
		}
	}
	if !complete() {
		return contracts.ConstructionResult{}
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
	if _, err := r.Index.ReplaceHome(ctx.PlayerID, current.Version, next); err != nil {
		panic("prepared hearth assignment failed: " + err.Error())
	}
	return contracts.ConstructionResult{BehaviorResult: contracts.BehaviorResult{OK: true}}
}

func (r *Runtime) canTransform(id types.EntityID, handle types.Handle, typeID uint32) bool {
	if !r.w.Alive(handle) || r.w.GetHandleByEntityID(id) != handle || ecs.ObjectDestructionPending(r.w, handle) || ecs.InventoryHandleReserved(r.w, handle) {
		return false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](r.w, handle)
	return ok && info.TypeID == typeID && ecs.HasComponent[components.ObjectInternalState](r.w, handle)
}

type preparation struct {
	runtime *Runtime
	coord   types.ChunkCoord
}

func (r *Runtime) loadHome(character types.EntityID, hearthType int) {
	version, started := r.Index.BeginLookup(character)
	if !started {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row, err := r.reader.GetCharacterHearth(ctx, repository.GetCharacterHearthParams{
		CharacterID: int64(character), HearthTypeID: hearthType, SurfaceLayer: SurfaceLayer,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		r.logger.Fatal("hearth lookup failed", zap.Error(err))
		return
	}
	var home *Home
	if err == nil {
		home = &Home{Object: ObjectKey{Region: row.ObjectRegion, ID: types.EntityID(row.ObjectID)}, X: row.X, Y: row.Y, Layer: row.Layer}
	}
	if err := r.Index.CompleteLookup(character, version, home); err != nil && !errors.Is(err, ErrStaleLookup) {
		r.logger.Fatal("invalid hearth lookup result", zap.Error(err))
	}
}

func (r *Runtime) prepareChunk(coord types.ChunkCoord) {
	if r.inFlight == preparationCapacity || r.preloads[coord] {
		return
	}
	if r.chunks.SubmitPersistenceJob(&preparation{runtime: r, coord: coord}) {
		r.preloads[coord] = true
		r.inFlight++
	}
}

func (j *preparation) Run() error {
	err := j.runtime.chunks.WaitPreloaded(context.Background(), j.coord)
	if errors.Is(err, world.ErrChunkNotLoaded) {
		return nil // shutdown or rejected load admission; a later attempt can retry
	}
	return err
}

func (j *preparation) Complete(err error) {
	if err != nil {
		j.runtime.logger.Fatal("hearth preparation failed", zap.Error(err))
		return
	}
	// Admission counts jobs until application, so this bounded channel always
	// has room. Workers neither touch ECS nor wait for a player to retry.
	j.runtime.completed <- j
}

func (r *Runtime) ApplyReadResults() {
	for n := 0; n < preparationApplyBudget; n++ {
		select {
		case job := <-r.completed:
			r.inFlight--
			delete(r.preloads, job.coord)
		default:
			return
		}
	}
}
