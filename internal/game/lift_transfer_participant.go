package game

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	gameworld "origin/internal/game/world"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

const liftCarryTransferParticipantKey = "lift_carry"

type liftCarryTransferState struct {
	HasCarry bool

	ObjectEntityID types.EntityID
	ObjectSnapshot gameworld.EmbeddedObjectSnapshotV1
	LiftedMeta     components.LiftedObjectState
	CarryStartedAt int64

	SourceLayer int
	SourceChunk types.ChunkCoord
}

type LiftCarryTransferParticipant struct {
	logger *zap.Logger
}

func NewLiftCarryTransferParticipant(logger *zap.Logger) *LiftCarryTransferParticipant {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LiftCarryTransferParticipant{logger: logger}
}

func (p *LiftCarryTransferParticipant) Key() string { return liftCarryTransferParticipantKey }

func (p *LiftCarryTransferParticipant) CaptureSource(
	g *Game,
	sourceShard *Shard,
	req PlayerTransferRequest,
	playerHandle types.Handle,
) (any, error) {
	if g == nil || sourceShard == nil || playerHandle == types.InvalidHandle || !sourceShard.world.Alive(playerHandle) {
		return nil, nil
	}
	if sourceShard.liftService == nil {
		return nil, nil
	}
	carry, hasCarry := ecs.GetComponent[components.LiftCarryState](sourceShard.world, playerHandle)
	if !hasCarry || carry.ObjectEntityID == 0 {
		return nil, nil
	}

	objectHandle := carry.ObjectHandle
	if objectHandle == types.InvalidHandle || !sourceShard.world.Alive(objectHandle) {
		objectHandle = sourceShard.world.GetHandleByEntityID(carry.ObjectEntityID)
	}
	if objectHandle == types.InvalidHandle || !sourceShard.world.Alive(objectHandle) {
		sourceShard.liftService.clearCarryStateForPlayer(sourceShard.world, req.PlayerID, playerHandle, false)
		return nil, nil
	}

	liftedMeta, hasLifted := ecs.GetComponent[components.LiftedObjectState](sourceShard.world, objectHandle)
	if !hasLifted {
		sourceShard.liftService.clearCarryStateForPlayer(sourceShard.world, req.PlayerID, playerHandle, false)
		return nil, nil
	}
	chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](sourceShard.world, objectHandle)
	if !hasChunkRef {
		return nil, fmt.Errorf("carried object missing chunk ref")
	}
	transform, hasTransform := ecs.GetComponent[components.Transform](sourceShard.world, objectHandle)
	if !hasTransform {
		return nil, fmt.Errorf("carried object missing transform")
	}

	snapshot, err := g.objectFactory.CaptureWorldObjectSnapshot(sourceShard.world, objectHandle)
	if err != nil {
		return nil, err
	}

	state := &liftCarryTransferState{
		HasCarry:       true,
		ObjectEntityID: carry.ObjectEntityID,
		ObjectSnapshot: snapshot,
		LiftedMeta:     liftedMeta,
		CarryStartedAt: carry.StartedAtUnixMs,
		SourceLayer:    req.SourceLayer,
		SourceChunk: types.ChunkCoord{
			X: chunkRef.CurrentChunkX,
			Y: chunkRef.CurrentChunkY,
		},
	}

	invalidateVisibilityForTeleport(sourceShard.world, sourceShard.layer, objectHandle, carry.ObjectEntityID, sourceShard.EventBus())
	if chunk := sourceShard.chunkManager.GetChunkFast(state.SourceChunk); chunk != nil {
		chunk.Spatial().RemoveStatic(objectHandle, int(transform.X), int(transform.Y))
		chunk.Spatial().RemoveDynamic(objectHandle, int(transform.X), int(transform.Y))
	}
	ecs.CancelBehaviorTicksByEntityID(sourceShard.world, carry.ObjectEntityID)
	sourceShard.world.Despawn(objectHandle)

	sourceShard.liftService.clearPendingLiftTransitionState(sourceShard.world, req.PlayerID, playerHandle, false)
	sourceShard.liftService.clearCarryStateForPlayer(sourceShard.world, req.PlayerID, playerHandle, false)

	return state, nil
}

func (p *LiftCarryTransferParticipant) RestoreTarget(
	g *Game,
	targetShard *Shard,
	req PlayerTransferRequest,
	playerHandle types.Handle,
	stateAny any,
) error {
	state, _ := stateAny.(*liftCarryTransferState)
	if state == nil || !state.HasCarry {
		return nil
	}
	return p.restoreCarryToShard(g, targetShard, req, playerHandle, state, true)
}

func (p *LiftCarryTransferParticipant) RestoreSourceRollback(
	g *Game,
	sourceShard *Shard,
	req PlayerTransferRequest,
	playerHandle types.Handle,
	stateAny any,
) error {
	state, _ := stateAny.(*liftCarryTransferState)
	if state == nil || !state.HasCarry {
		return nil
	}
	return p.restoreCarryToShard(g, sourceShard, req, playerHandle, state, false)
}

func (p *LiftCarryTransferParticipant) OnTargetRestoreFailure(
	g *Game,
	targetShard *Shard,
	req PlayerTransferRequest,
	playerHandle types.Handle,
	stateAny any,
	restoreErr error,
) {
	state, _ := stateAny.(*liftCarryTransferState)
	if targetShard == nil || targetShard.liftService == nil {
		return
	}
	_ = targetShard.liftService.ForceDropCarryAtPlayerPosition(targetShard.world, req.PlayerID, playerHandle, true)
	targetShard.liftService.sendWarning(req.PlayerID, "LIFT_TRANSFER_RESTORE_FAILED")

	if g != nil && state != nil && state.HasCarry {
		// Best effort persistence for the force-dropped object if it exists.
		if objectHandle := targetShard.world.GetHandleByEntityID(state.ObjectEntityID); objectHandle != types.InvalidHandle && targetShard.world.Alive(objectHandle) {
			if err := g.objectFactory.PersistWorldObjectNow(g.db, targetShard.world, objectHandle); err != nil {
				p.logger.Error("Lift transfer restore failure: persist after force-drop failed",
					zap.Uint64("player_id", uint64(req.PlayerID)),
					zap.Uint64("object_id", uint64(state.ObjectEntityID)),
					zap.Int("target_layer", req.TargetLayer),
					zap.Int("source_layer", state.SourceLayer),
					zap.Int("source_chunk_x", state.SourceChunk.X),
					zap.Int("source_chunk_y", state.SourceChunk.Y),
					zap.Int("snapshot_type_id", state.ObjectSnapshot.TypeID),
					zap.Int("snapshot_version", state.ObjectSnapshot.Version),
					zap.Error(err),
				)
			}
		}
	}

	fields := []zap.Field{
		zap.Uint64("player_id", uint64(req.PlayerID)),
		zap.Int("target_layer", req.TargetLayer),
		zap.Error(restoreErr),
	}
	if state != nil {
		fields = append(fields,
			zap.Bool("has_carry_state", state.HasCarry),
			zap.Uint64("object_id", uint64(state.ObjectEntityID)),
			zap.Int("source_layer", state.SourceLayer),
			zap.Int("source_chunk_x", state.SourceChunk.X),
			zap.Int("source_chunk_y", state.SourceChunk.Y),
			zap.Int("snapshot_type_id", state.ObjectSnapshot.TypeID),
			zap.Int("snapshot_version", state.ObjectSnapshot.Version),
		)
	}
	p.logger.Error("Lift transfer restore failed (teleport completed; carry canceled/force-drop attempted)",
		fields...,
	)
}

func (p *LiftCarryTransferParticipant) restoreCarryToShard(
	g *Game,
	shard *Shard,
	req PlayerTransferRequest,
	playerHandle types.Handle,
	state *liftCarryTransferState,
	persistMovedObject bool,
) error {
	if g == nil || shard == nil || shard.liftService == nil || state == nil || !state.HasCarry {
		return nil
	}
	if playerHandle == types.InvalidHandle || !shard.world.Alive(playerHandle) {
		return fmt.Errorf("transfer restore player handle invalid")
	}
	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](shard.world, playerHandle)
	if !hasPlayerTransform {
		return fmt.Errorf("transfer restore player missing transform")
	}

	objectHandle, err := g.objectFactory.SpawnWorldObjectFromSnapshot(shard.world, state.ObjectSnapshot, gameworld.SnapshotSpawnOptions{
		X:                int(playerTransform.X),
		Y:                int(playerTransform.Y),
		Layer:            shard.layer,
		ChunkManager:     shard.chunkManager,
		BehaviorRegistry: behaviors.MustDefaultRegistry(),
		EventBus:         shard.EventBus(),
		Logger:           p.logger,
	})
	if err != nil {
		return err
	}

	liftedMeta := state.LiftedMeta
	liftedMeta.CarrierPlayerID = req.PlayerID
	liftedMeta.CarrierHandle = playerHandle
	ecs.AddComponent(shard.world, objectHandle, liftedMeta)
	shard.liftService.disableCarriedObjectRuntime(shard.world, objectHandle, req.PlayerID, playerHandle)
	ecs.AddComponent(shard.world, playerHandle, components.LiftCarryState{
		ObjectEntityID:  state.ObjectEntityID,
		ObjectHandle:    objectHandle,
		StartedAtUnixMs: state.CarryStartedAt,
	})

	if !gameworld.RelocateWorldObjectImmediate(
		shard.world,
		shard.chunkManager,
		shard.EventBus(),
		objectHandle,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: true, ForceReindex: true},
		playerTransform.X,
		playerTransform.Y,
		p.logger,
	) {
		return fmt.Errorf("failed to relocate restored carried object")
	}

	shard.liftService.sendCarryState(req.PlayerID, true, state.ObjectEntityID)

	if persistMovedObject {
		if err := g.objectFactory.PersistWorldObjectNow(g.db, shard.world, objectHandle); err != nil {
			return err
		}
		p.patchChunkCachesAfterTransfer(g, shard, state, objectHandle)
	}
	return nil
}

func (p *LiftCarryTransferParticipant) patchChunkCachesAfterTransfer(
	g *Game,
	targetShard *Shard,
	state *liftCarryTransferState,
	objectHandle types.Handle,
) {
	if g == nil || targetShard == nil || state == nil || objectHandle == types.InvalidHandle {
		return
	}
	objectID := state.ObjectEntityID
	if objectID == 0 {
		return
	}
	// Remove stale source raw caches so future inactive saves do not resurrect the moved object.
	if sourceShard := g.shardManager.GetShard(state.SourceLayer); sourceShard != nil {
		if sourceChunk := sourceShard.chunkManager.GetChunkFast(state.SourceChunk); sourceChunk != nil {
			// These helpers intentionally mark rawDataDirty so cache repairs persist.
			sourceChunk.RemoveRawObjectByID(objectID)
			sourceChunk.RemoveRawInventoriesByOwner(objectID)
		}
	}

	rawObj, err := g.objectFactory.Serialize(targetShard.world, objectHandle)
	if err != nil || rawObj == nil {
		return
	}
	var rawInvs []repository.Inventory
	if info, hasInfo := ecs.GetComponent[components.EntityInfo](targetShard.world, objectHandle); hasInfo && g.objectFactory.HasPersistentInventories(info.TypeID, info.Behaviors) {
		rawInvs, _ = g.objectFactory.SerializeObjectInventories(targetShard.world, objectHandle)
	}
	if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](targetShard.world, objectHandle); hasChunkRef {
		if targetChunk := targetShard.chunkManager.GetChunkFast(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}); targetChunk != nil {
			targetChunk.UpsertRawObject(rawObj)
			targetChunk.SetRawInventoriesForOwner(objectID, rawInvs)
		}
	}
}
