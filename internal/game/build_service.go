package game

import (
	"context"
	"strings"
	"time"

	"origin/internal/builddefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/mathutil"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	buildPendingTTL       = 15 * time.Second
	buildBehaviorStateKey = "build"
)

type buildRuntimeSender interface {
	SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert)
	SendBuildStateClosed(entityID types.EntityID, msg *netproto.S2C_BuildStateClosed)
}

type buildPendingContextStarter interface {
	StartPendingContextActionFromServer(
		w *ecs.World,
		playerHandle types.Handle,
		playerID types.EntityID,
		targetEntityID types.EntityID,
		targetHandle types.Handle,
		actionID string,
	)
}

type BuildService struct {
	world            *ecs.World
	chunkManager     *gameworld.ChunkManager
	eventBus         *eventbus.EventBus
	idAllocator      contracts.EntityIDAllocator
	behaviorRegistry contracts.BehaviorRegistry
	visionForcer     contracts.VisionUpdateForcer
	alerts           buildRuntimeSender
	pendingStarter   buildPendingContextStarter
	logger           *zap.Logger
}

var _ systems.BuildCommandService = (*BuildService)(nil)
var _ systems.BuildPlacementFinalizer = (*BuildService)(nil)

func NewBuildService(
	world *ecs.World,
	eventBus *eventbus.EventBus,
	chunkManager *gameworld.ChunkManager,
	idAllocator contracts.EntityIDAllocator,
	behaviorRegistry contracts.BehaviorRegistry,
	visionForcer contracts.VisionUpdateForcer,
	alerts buildRuntimeSender,
	pendingStarter buildPendingContextStarter,
	logger *zap.Logger,
) *BuildService {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &BuildService{
		world:            world,
		chunkManager:     chunkManager,
		eventBus:         eventBus,
		idAllocator:      idAllocator,
		behaviorRegistry: behaviorRegistry,
		visionForcer:     visionForcer,
		alerts:           alerts,
		pendingStarter:   pendingStarter,
		logger:           logger,
	}
	if eventBus != nil {
		eventBus.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityLow, s.onLinkBroken)
	}
	return s
}

func (s *BuildService) HandleStartBuild(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	msg *netproto.C2S_BuildStart,
) {
	if s == nil || w == nil || w != s.world || msg == nil || msg.Pos == nil || playerID == 0 {
		return
	}
	if playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}

	buildKey := strings.TrimSpace(msg.BuildKey)
	if buildKey == "" {
		s.sendWarning(playerID, "BUILD_INVALID_DEF")
		return
	}

	buildDef, resultDef, _, resultColliderDef, resolveErr := s.resolveBuildDefs(buildKey)
	if resolveErr != "" {
		s.sendResolveError(playerID, resolveErr)
		return
	}
	if resultColliderDef == nil {
		s.sendWarning(playerID, "BUILD_RESULT_NO_COLLIDER")
		return
	}

	targetX := int(msg.Pos.X)
	targetY := int(msg.Pos.Y)
	if !s.validateTileRules(buildDef, targetX, targetY, playerID) {
		return
	}

	mov, hasMovement := ecs.GetComponent[components.Movement](w, playerHandle)
	if !hasMovement || mov.State == constt.StateStunned {
		s.sendWarning(playerID, "BUILD_PENDING_FAILED")
		return
	}
	if _, hasCollider := ecs.GetComponent[components.Collider](w, playerHandle); !hasCollider {
		s.sendWarning(playerID, "BUILD_PENDING_FAILED")
		return
	}

	// Replace any previous pending build placement.
	s.CancelPendingBuildPlacement(w, playerID, playerHandle)

	// Break an active link immediately so phantom placement starts from a clean interaction state.
	// LinkSystem would also break it after movement, but doing it here avoids stale linked state until then.
	s.breakActiveLink(w, playerID)

	resultCollider := objectdefs.BuildColliderComponent(resultColliderDef)
	ecs.WithComponent(w, playerHandle, func(col *components.Collider) {
		col.Phantom = &components.PhantomCollider{
			WorldX:     float64(targetX),
			WorldY:     float64(targetY),
			HalfWidth:  resultCollider.HalfWidth,
			HalfHeight: resultCollider.HalfHeight,
			TypeID:     uint32(resultDef.DefID),
		}
	})

	expireAt := ecs.GetResource[ecs.TimeState](w).UnixMs + buildPendingTTL.Milliseconds()
	ecs.AddComponent(w, playerHandle, components.PendingBuildPlacement{
		BuildKey:           buildDef.Key,
		BuildDefID:         buildDef.DefID,
		ResultObjectKey:    buildDef.ObjectKey,
		ResultObjectTypeID: uint32(resultDef.DefID),
		TargetX:            targetX,
		TargetY:            targetY,
		PhantomHalfWidth:   resultCollider.HalfWidth,
		PhantomHalfHeight:  resultCollider.HalfHeight,
		ExpireAtUnixMs:     expireAt,
	})

	ecs.WithComponent(w, playerHandle, func(m *components.Movement) {
		m.SetTargetPoint(targetX, targetY)
	})
}

func (s *BuildService) HandleBuildProgress(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	msg *netproto.C2S_BuildProgress,
) {
	if s == nil || w == nil || w != s.world || msg == nil || playerID == 0 {
		return
	}
	if playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}
	targetID := types.EntityID(msg.EntityId)
	if targetID == 0 {
		s.sendWarning(playerID, "BUILD_PROGRESS_INVALID_TARGET")
		return
	}
	targetHandle := w.GetHandleByEntityID(targetID)
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		s.sendWarning(playerID, "BUILD_PROGRESS_INVALID_TARGET")
		return
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo || info.TypeID != constt.BuildObjectTypeID {
		s.sendWarning(playerID, "BUILD_PROGRESS_INVALID_TARGET")
		return
	}
	linkState := ecs.GetResource[ecs.LinkState](w)
	link, linked := linkState.GetLink(playerID)
	if !linked || link.TargetID != targetID {
		s.sendWarning(playerID, "BUILD_PROGRESS_NOT_LINKED")
		return
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](w, targetHandle)
	if !hasState {
		s.sendWarning(playerID, "BUILD_PROGRESS_INVALID_TARGET")
		return
	}
	if _, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, buildBehaviorStateKey); !ok {
		s.sendWarning(playerID, "BUILD_PROGRESS_INVALID_TARGET")
		return
	}
	s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_INFO, "BUILD_PROGRESS_COMING_SOON")
}

func (s *BuildService) FinalizePendingBuildPlacement(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	pending components.PendingBuildPlacement,
) {
	if s == nil || w == nil || w != s.world || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}

	currentPending, hasPending := ecs.GetComponent[components.PendingBuildPlacement](w, playerHandle)
	if !hasPending {
		return
	}
	// Guard against stale callback arguments if pending was replaced earlier in the tick.
	if currentPending.TargetX != pending.TargetX || currentPending.TargetY != pending.TargetY ||
		currentPending.BuildKey != pending.BuildKey {
		pending = currentPending
	}

	buildDef, resultDef, buildSiteDef, resultColliderDef, resolveErr := s.resolveBuildDefs(pending.BuildKey)
	if resolveErr != "" {
		s.CancelPendingBuildPlacement(w, playerID, playerHandle)
		s.sendResolveError(playerID, resolveErr)
		return
	}
	if resultColliderDef == nil {
		s.CancelPendingBuildPlacement(w, playerID, playerHandle)
		s.sendWarning(playerID, "BUILD_RESULT_NO_COLLIDER")
		return
	}
	chunkX := mathutil.FloorDiv(pending.TargetX, constt.ChunkWorldSize)
	chunkY := mathutil.FloorDiv(pending.TargetY, constt.ChunkWorldSize)
	chunkCoord := types.ChunkCoord{X: chunkX, Y: chunkY}
	chunk := s.chunkManager.GetChunkFast(chunkCoord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		s.CancelPendingBuildPlacement(w, playerID, playerHandle)
		s.sendWarning(playerID, "BUILD_OUTSIDE_LOADED_CHUNK")
		return
	}
	if s.idAllocator == nil || s.behaviorRegistry == nil {
		s.CancelPendingBuildPlacement(w, playerID, playerHandle)
		s.sendError(playerID, "BUILD_SPAWN_FAILED")
		return
	}

	newID := s.idAllocator.GetFreeID()
	targetXf := float64(pending.TargetX)
	targetYf := float64(pending.TargetY)
	handle := gameworld.SpawnEntityFromDef(w, buildSiteDef, gameworld.DefSpawnParams{
		EntityID:         newID,
		X:                targetXf,
		Y:                targetYf,
		Quality:          0,
		Region:           chunk.Region,
		Layer:            chunk.Layer,
		InitReason:       contracts.ObjectBehaviorInitReasonSpawn,
		BehaviorRegistry: s.behaviorRegistry,
	})
	if handle == types.InvalidHandle {
		s.CancelPendingBuildPlacement(w, playerID, playerHandle)
		s.sendError(playerID, "BUILD_SPAWN_FAILED")
		return
	}

	ecs.AddComponent(w, handle, components.ChunkRef{
		CurrentChunkX: chunkX,
		CurrentChunkY: chunkY,
		PrevChunkX:    chunkX,
		PrevChunkY:    chunkY,
	})
	ecs.AddComponent(w, handle, objectdefs.BuildColliderComponent(resultColliderDef))

	ecs.WithComponent(w, handle, func(internalState *components.ObjectInternalState) {
		components.SetBehaviorState(internalState, buildBehaviorStateKey, buildStateFromDef(buildDef, resultDef, pending.TargetX, pending.TargetY))
	})
	ecs.MarkObjectBehaviorDirty(w, handle)

	if buildSiteDef.IsStatic {
		chunk.Spatial().AddStatic(handle, pending.TargetX, pending.TargetY)
	} else {
		chunk.Spatial().AddDynamic(handle, pending.TargetX, pending.TargetY)
	}
	chunk.MarkRawDataDirty()

	s.forceVisionRefreshAll(w)
	s.CancelPendingBuildPlacement(w, playerID, playerHandle)

	if s.pendingStarter != nil {
		s.pendingStarter.StartPendingContextActionFromServer(w, playerHandle, playerID, newID, handle, "open")
	}
}

func (s *BuildService) CancelPendingBuildPlacement(w *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	if s == nil || w == nil || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}
	ecs.RemoveComponent[components.PendingBuildPlacement](w, playerHandle)
	ecs.RemoveComponent[components.PendingInteraction](w, playerHandle)
	ecs.RemoveComponent[components.PendingContextAction](w, playerHandle)
	ecs.WithComponent(w, playerHandle, func(col *components.Collider) {
		col.Phantom = nil
	})
	if playerID != 0 {
		ecs.GetResource[ecs.LinkState](w).ClearIntent(playerID)
	}
}

func (s *BuildService) onLinkBroken(_ context.Context, event eventbus.Event) error {
	ev, ok := event.(*ecs.LinkBrokenEvent)
	if !ok || s == nil || s.world == nil || ev.Layer != s.world.Layer || ev.TargetID == 0 {
		return nil
	}
	if s.alerts != nil {
		targetHandle := s.world.GetHandleByEntityID(ev.TargetID)
		if targetHandle != types.InvalidHandle && s.world.Alive(targetHandle) {
			if info, hasInfo := ecs.GetComponent[components.EntityInfo](s.world, targetHandle); hasInfo && info.TypeID == constt.BuildObjectTypeID {
				s.alerts.SendBuildStateClosed(ev.PlayerID, &netproto.S2C_BuildStateClosed{
					EntityId: uint64(ev.TargetID),
				})
			}
		}
	}

	linkState := ecs.GetResource[ecs.LinkState](s.world)
	if players := linkState.PlayersByTarget[ev.TargetID]; len(players) > 0 {
		return nil
	}

	targetHandle := s.world.GetHandleByEntityID(ev.TargetID)
	if targetHandle == types.InvalidHandle || !s.world.Alive(targetHandle) {
		return nil
	}
	if !s.isEmptyBuildObject(targetHandle) {
		return nil
	}
	s.despawnBuildObject(s.world, ev.TargetID, targetHandle)
	return nil
}

func (s *BuildService) isEmptyBuildObject(targetHandle types.Handle) bool {
	info, hasInfo := ecs.GetComponent[components.EntityInfo](s.world, targetHandle)
	if !hasInfo || info.TypeID != constt.BuildObjectTypeID {
		return false
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](s.world, targetHandle)
	if !hasState {
		return false
	}
	buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, buildBehaviorStateKey)
	if !ok || buildState == nil {
		return false
	}
	return buildState.IsEmpty()
}

func (s *BuildService) despawnBuildObject(w *ecs.World, targetID types.EntityID, targetHandle types.Handle) {
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	transform, hasTransform := ecs.GetComponent[components.Transform](w, targetHandle)
	chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, targetHandle)
	if hasInfo && hasTransform && hasChunkRef {
		chunk := s.chunkManager.GetChunkFast(types.ChunkCoord{
			X: chunkRef.CurrentChunkX,
			Y: chunkRef.CurrentChunkY,
		})
		if chunk != nil {
			if info.IsStatic {
				chunk.Spatial().RemoveStatic(targetHandle, int(transform.X), int(transform.Y))
			} else {
				chunk.Spatial().RemoveDynamic(targetHandle, int(transform.X), int(transform.Y))
			}
			chunk.MarkRawDataDirty()
		}
	}
	ecs.CancelBehaviorTicksByEntityID(w, targetID)
	w.Despawn(targetHandle)
	s.forceVisionRefreshAll(w)
}

func (s *BuildService) resolveBuildDefs(buildKey string) (*builddefs.BuildDef, *objectdefs.ObjectDef, *objectdefs.ObjectDef, *objectdefs.ColliderDef, string) {
	buildReg := builddefs.Global()
	if buildReg == nil {
		return nil, nil, nil, nil, "BUILD_INVALID_DEF"
	}
	buildDef, ok := buildReg.GetByKey(strings.TrimSpace(buildKey))
	if !ok || buildDef == nil {
		return nil, nil, nil, nil, "BUILD_INVALID_DEF"
	}

	objReg := objectdefs.Global()
	if objReg == nil {
		return nil, nil, nil, nil, "BUILD_INVALID_OBJECT"
	}
	resultDef, ok := objReg.GetByKey(buildDef.ObjectKey)
	if !ok || resultDef == nil {
		return nil, nil, nil, nil, "BUILD_INVALID_OBJECT"
	}
	buildSiteDef, ok := objReg.GetByKey("build")
	if !ok || buildSiteDef == nil {
		return nil, nil, nil, nil, "BUILD_SPECIAL_DEF_MISSING"
	}
	if buildSiteDef.DefID != constt.BuildObjectTypeID {
		return nil, nil, nil, nil, "BUILD_SPECIAL_DEF_MISMATCH"
	}
	if resultDef.Components == nil || resultDef.Components.Collider == nil {
		return buildDef, resultDef, buildSiteDef, nil, ""
	}
	return buildDef, resultDef, buildSiteDef, resultDef.Components.Collider, ""
}

func (s *BuildService) validateTileRules(buildDef *builddefs.BuildDef, worldX, worldY int, playerID types.EntityID) bool {
	if s == nil || s.chunkManager == nil || buildDef == nil {
		s.sendError(playerID, "BUILD_PENDING_FAILED")
		return false
	}
	tileX := mathutil.FloorDiv(worldX, constt.CoordPerTile)
	tileY := mathutil.FloorDiv(worldY, constt.CoordPerTile)
	tileID, ok := s.chunkManager.GetTileID(tileX, tileY)
	if !ok {
		s.sendWarning(playerID, "BUILD_OUTSIDE_LOADED_CHUNK")
		return false
	}

	if len(buildDef.AllowedTiles) > 0 {
		allowed := false
		for _, allowedID := range buildDef.AllowedTiles {
			if int(tileID) == allowedID {
				allowed = true
				break
			}
		}
		if !allowed {
			s.sendWarning(playerID, "BUILD_TILE_NOT_ALLOWED")
			return false
		}
	}
	for _, blockedID := range buildDef.DisallowedTiles {
		if int(tileID) == blockedID {
			s.sendWarning(playerID, "BUILD_TILE_BLOCKED")
			return false
		}
	}
	return true
}

func buildStateFromDef(buildDef *builddefs.BuildDef, resultDef *objectdefs.ObjectDef, targetX, targetY int) *components.BuildBehaviorState {
	if buildDef == nil || resultDef == nil {
		return &components.BuildBehaviorState{}
	}
	items := make([]components.BuildRequiredItemState, 0, len(buildDef.Inputs))
	for i, input := range buildDef.Inputs {
		items = append(items, components.BuildRequiredItemState{
			Slot:              i,
			ItemKey:           input.ItemKey,
			ItemTag:           input.ItemTag,
			RequiredCount:     input.Count,
			QualityWeight:     input.QualityWeight,
			BuildQualityTotal: 0,
			PutItems:          nil,
		})
	}
	return &components.BuildBehaviorState{
		BuildKey:     buildDef.Key,
		BuildDefID:   buildDef.DefID,
		ObjectKey:    buildDef.ObjectKey,
		ObjectTypeID: uint32(resultDef.DefID),
		TargetX:      targetX,
		TargetY:      targetY,
		Items:        items,
	}
}

func (s *BuildService) forceVisionRefreshAll(w *ecs.World) {
	if s == nil || s.visionForcer == nil || w == nil {
		return
	}
	characters := ecs.GetResource[ecs.CharacterEntities](w)
	for _, ce := range characters.Map {
		if ce.Handle != types.InvalidHandle && w.Alive(ce.Handle) {
			s.visionForcer.ForceUpdateForObserver(w, ce.Handle)
		}
	}
}

func (s *BuildService) sendWarning(playerID types.EntityID, reasonCode string) {
	s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_WARNING, reasonCode)
}

func (s *BuildService) sendError(playerID types.EntityID, reasonCode string) {
	s.sendMiniAlert(playerID, netproto.AlertSeverity_ALERT_SEVERITY_ERROR, reasonCode)
}

func (s *BuildService) sendMiniAlert(playerID types.EntityID, severity netproto.AlertSeverity, reasonCode string) {
	if s == nil || s.alerts == nil || playerID == 0 || reasonCode == "" {
		return
	}
	s.alerts.SendMiniAlert(playerID, &netproto.S2C_MiniAlert{
		Severity:   severity,
		ReasonCode: reasonCode,
		TtlMs:      ttlBySeverity(severity),
	})
}

func (s *BuildService) sendResolveError(playerID types.EntityID, reasonCode string) {
	switch reasonCode {
	case "BUILD_SPECIAL_DEF_MISSING", "BUILD_SPECIAL_DEF_MISMATCH":
		s.sendError(playerID, reasonCode)
	default:
		s.sendWarning(playerID, reasonCode)
	}
}

func (s *BuildService) breakActiveLink(w *ecs.World, playerID types.EntityID) {
	if s == nil || w == nil || playerID == 0 {
		return
	}
	linkState := ecs.GetResource[ecs.LinkState](w)
	link, removed := linkState.RemoveLink(playerID)
	if !removed || s.eventBus == nil {
		return
	}
	if err := s.eventBus.PublishSync(ecs.NewLinkBrokenEvent(w.Layer, playerID, link.TargetID, ecs.LinkBreakMoved)); err != nil {
		s.logger.Warn("failed to publish LinkBroken(moved) for build arm",
			zap.Error(err),
			zap.Uint64("player_id", uint64(playerID)),
			zap.Uint64("target_id", uint64(link.TargetID)),
		)
	}
}
