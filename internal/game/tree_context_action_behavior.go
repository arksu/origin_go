package game

import (
	"math"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const contextActionChop = "chop"

type treeContextActionBehavior struct {
	eventBus     *eventbus.EventBus
	chunks       chunkProvider
	idAllocator  entityIDAllocator
	visionForcer visionUpdateForcer
	logger       *zap.Logger
}

func (b treeContextActionBehavior) Actions(
	w *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
) []systems.ContextAction {
	_ = targetID
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return nil
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo {
		return nil
	}
	def, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || def.TreeConfig == nil {
		return nil
	}

	return []systems.ContextAction{
		{
			ActionID: contextActionChop,
			Title:    "Chop",
		},
	}
}

func (b treeContextActionBehavior) Execute(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
	actionID string,
	openSvc systems.OpenContainerCoordinator,
) contextActionExecuteResult {
	_ = openSvc
	_ = playerID

	if actionID != contextActionChop {
		return contextActionExecuteResult{ok: false}
	}
	if playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return contextActionExecuteResult{ok: false}
	}
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return contextActionExecuteResult{ok: false}
	}

	if _, exists := ecs.GetComponent[components.ActiveCyclicAction](w, playerHandle); exists {
		return contextActionExecuteResult{
			ok:          false,
			userVisible: true,
			reasonCode:  "action_already_active",
			severity:    netproto.AlertSeverity_ALERT_SEVERITY_WARNING,
		}
	}

	targetInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo {
		return contextActionExecuteResult{ok: false}
	}
	targetDef, ok := objectdefs.Global().GetByID(int(targetInfo.TypeID))
	if !ok || targetDef.TreeConfig == nil {
		return contextActionExecuteResult{ok: false}
	}

	// Init chop points on first chop.
	ecs.WithComponent(w, targetHandle, func(state *components.ObjectInternalState) {
		treeState, has := components.GetBehaviorState[components.TreeBehaviorState](*state, "tree")
		if has && treeState != nil && treeState.ChopPoints > 0 {
			return
		}
		components.SetBehaviorState(state, "tree", &components.TreeBehaviorState{
			ChopPoints: targetDef.TreeConfig.ChopPointsTotal,
		})
	})

	nowTick := ecs.GetResource[ecs.TimeState](w).Tick
	ecs.AddComponent(w, playerHandle, components.ActiveCyclicAction{
		BehaviorKey:        "tree",
		ActionID:           contextActionChop,
		CycleSoundKey:      strings.TrimSpace(targetDef.TreeConfig.ActionSound),
		CompleteSoundKey:   strings.TrimSpace(targetDef.TreeConfig.FinishSound),
		TargetKind:         components.CyclicActionTargetObject,
		TargetID:           targetID,
		TargetHandle:       targetHandle,
		CycleDurationTicks: uint32(targetDef.TreeConfig.ChopCycleDurationTicks),
		CycleElapsedTicks:  0,
		CycleIndex:         1,
		StartedTick:        nowTick,
	})

	ecs.MutateComponent[components.Movement](w, playerHandle, func(m *components.Movement) bool {
		m.State = constt.StateInteracting
		return true
	})

	return contextActionExecuteResult{ok: true}
}

func (b treeContextActionBehavior) OnCycleComplete(ctx cyclicActionCycleContext) cyclicActionDecision {
	w := ctx.World
	if w == nil || ctx.TargetHandle == types.InvalidHandle || !w.Alive(ctx.TargetHandle) {
		return cyclicActionDecisionCanceled
	}

	targetInfo, hasTargetInfo := ecs.GetComponent[components.EntityInfo](w, ctx.TargetHandle)
	if !hasTargetInfo {
		return cyclicActionDecisionCanceled
	}
	targetDef, ok := objectdefs.Global().GetByID(int(targetInfo.TypeID))
	if !ok || targetDef.TreeConfig == nil {
		return cyclicActionDecisionCanceled
	}
	treeConfig := targetDef.TreeConfig

	remaining := 0
	transitionToStump := false
	ecs.WithComponent(w, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		currentChopPoints := treeConfig.ChopPointsTotal
		if treeState, has := components.GetBehaviorState[components.TreeBehaviorState](*state, "tree"); has && treeState != nil {
			currentChopPoints = treeState.ChopPoints
		}

		if currentChopPoints <= 0 {
			remaining = 0
			return
		}

		currentChopPoints--
		remaining = currentChopPoints
		components.SetBehaviorState(state, "tree", &components.TreeBehaviorState{
			ChopPoints: currentChopPoints,
		})
		if remaining == 0 {
			transitionToStump = true
		}
	})

	if !transitionToStump {
		if remaining > 0 {
			return cyclicActionDecisionContinue
		}
		return cyclicActionDecisionCanceled
	}

	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](w, ctx.PlayerHandle)
	targetTransform, hasTargetTransform := ecs.GetComponent[components.Transform](w, ctx.TargetHandle)
	targetChunkRef, hasTargetChunkRef := ecs.GetComponent[components.ChunkRef](w, ctx.TargetHandle)
	if !hasPlayerTransform || !hasTargetTransform || !hasTargetChunkRef {
		return cyclicActionDecisionCanceled
	}

	b.spawnLogs(
		w,
		targetTransform,
		playerTransform,
		targetInfo,
		treeConfig,
	)
	b.transformTargetToStump(
		w,
		ctx.TargetID,
		ctx.TargetHandle,
		targetInfo,
		targetChunkRef,
		treeConfig,
	)
	b.forceVisionUpdates(w)
	return cyclicActionDecisionComplete
}

func (b treeContextActionBehavior) forceVisionUpdates(w *ecs.World) {
	if b.visionForcer == nil || w == nil {
		return
	}

	characters := ecs.GetResource[ecs.CharacterEntities](w)
	for _, character := range characters.Map {
		if character.Handle == types.InvalidHandle || !w.Alive(character.Handle) {
			continue
		}
		b.visionForcer.ForceUpdateForObserver(w, character.Handle)
	}
}

func (b treeContextActionBehavior) spawnLogs(
	w *ecs.World,
	treeTransform components.Transform,
	playerTransform components.Transform,
	treeInfo components.EntityInfo,
	treeCfg *objectdefs.TreeBehaviorConfig,
) {
	if b.idAllocator == nil || b.chunks == nil || treeCfg == nil {
		return
	}

	dirX, dirY := resolveLogFallAxisDirection(treeTransform.X, treeTransform.Y, playerTransform.X, playerTransform.Y)
	logDefKey := resolveAxisLogDefKey(treeCfg.LogsSpawnDefKey, dirX, dirY)
	logDef, ok := objectdefs.Global().GetByKey(logDefKey)
	if !ok {
		b.logger.Warn("tree chop: log def not found", zap.String("def_key", logDefKey))
		return
	}

	for index := 0; index < treeCfg.LogsSpawnCount; index++ {
		logX, logY := logSpawnPosition(
			treeTransform.X,
			treeTransform.Y,
			dirX,
			dirY,
			treeCfg.LogsSpawnInitialOffset,
			treeCfg.LogsSpawnStepOffset,
			index,
		)

		chunkX := worldCoordToChunkIndex(logX)
		chunkY := worldCoordToChunkIndex(logY)
		chunk := b.chunks.GetChunkFast(types.ChunkCoord{X: chunkX, Y: chunkY})
		if chunk == nil {
			continue
		}

		logID := b.idAllocator.GetFreeID()
		h := gameworld.SpawnEntityFromDef(w, logDef, gameworld.DefSpawnParams{
			EntityID: logID,
			X:        logX,
			Y:        logY,
			Region:   treeInfo.Region,
			Layer:    treeInfo.Layer,
		})
		if h == types.InvalidHandle {
			continue
		}
		ecs.AddComponent(w, h, components.ChunkRef{
			CurrentChunkX: chunkX,
			CurrentChunkY: chunkY,
			PrevChunkX:    chunkX,
			PrevChunkY:    chunkY,
		})
		ecs.AddComponent(w, h, components.ObjectInternalState{IsDirty: true})

		if logDef.IsStatic {
			chunk.Spatial().AddStatic(h, int(logX), int(logY))
		} else {
			chunk.Spatial().AddDynamic(h, int(logX), int(logY))
		}
		chunk.MarkRawDataDirty()
		if len(logDef.BehaviorOrder) > 0 {
			ecs.MarkObjectBehaviorDirty(w, h)
		}
	}
}

func worldCoordToChunkIndex(coord float64) int {
	return int(math.Floor(coord / float64(constt.ChunkWorldSize)))
}

func resolveLogFallAxisDirection(treeX, treeY, playerX, playerY float64) (float64, float64) {
	dx := playerX - treeX
	dy := playerY - treeY
	oppositeX := -dx
	oppositeY := -dy

	if math.Abs(dx) >= math.Abs(dy) { // tie => X
		if oppositeX < 0 {
			return -1, 0
		}
		return 1, 0
	}

	if oppositeY < 0 {
		return 0, -1
	}
	return 0, 1
}

func logSpawnPosition(
	treeX, treeY, dirX, dirY float64,
	initialOffset, stepOffset, index int,
) (float64, float64) {
	distance := float64(initialOffset + index*stepOffset)
	return treeX + dirX*distance, treeY + dirY*distance
}

func resolveAxisLogDefKey(baseDefKey string, dirX, dirY float64) string {
	if baseDefKey == "" {
		return baseDefKey
	}
	axisX := math.Abs(dirX) >= math.Abs(dirY)
	if axisX {
		if strings.HasSuffix(baseDefKey, "_y") {
			return strings.TrimSuffix(baseDefKey, "_y") + "_x"
		}
		return baseDefKey + "_x"
	}
	if strings.HasSuffix(baseDefKey, "_x") {
		return strings.TrimSuffix(baseDefKey, "_x") + "_y"
	}
	return baseDefKey
}

func (b treeContextActionBehavior) transformTargetToStump(
	w *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
	targetInfo components.EntityInfo,
	targetChunkRef components.ChunkRef,
	treeCfg *objectdefs.TreeBehaviorConfig,
) {
	stumpDef, ok := objectdefs.Global().GetByKey(treeCfg.TransformToDefKey)
	if !ok {
		b.logger.Warn("tree chop: stump def not found", zap.String("def_key", treeCfg.TransformToDefKey))
		return
	}

	ecs.WithComponent(w, targetHandle, func(info *components.EntityInfo) {
		info.TypeID = uint32(stumpDef.DefID)
		info.Behaviors = stumpDef.CopyBehaviorOrder()
		info.IsStatic = stumpDef.IsStatic
	})
	ecs.WithComponent(w, targetHandle, func(appearance *components.Appearance) {
		resource := objectdefs.ResolveAppearanceResource(stumpDef, nil)
		if resource == "" {
			resource = stumpDef.Resource
		}
		appearance.Resource = resource
	})

	if stumpDef.Components != nil && stumpDef.Components.Collider != nil {
		collider := objectdefs.BuildColliderComponent(stumpDef.Components.Collider)
		if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); hasCollider {
			ecs.WithComponent(w, targetHandle, func(existing *components.Collider) {
				existing.HalfWidth = collider.HalfWidth
				existing.HalfHeight = collider.HalfHeight
				existing.Layer = collider.Layer
				existing.Mask = collider.Mask
			})
		} else {
			ecs.AddComponent(w, targetHandle, collider)
		}
	} else {
		ecs.RemoveComponent[components.Collider](w, targetHandle)
	}

	ecs.WithComponent(w, targetHandle, func(state *components.ObjectInternalState) {
		components.DeleteBehaviorState(state, "tree")
		state.Flags = nil
		state.IsDirty = true
	})

	if b.chunks != nil {
		chunk := b.chunks.GetChunkFast(types.ChunkCoord{
			X: targetChunkRef.CurrentChunkX,
			Y: targetChunkRef.CurrentChunkY,
		})
		if chunk != nil {
			chunk.MarkRawDataDirty()
		}
	}

	if b.eventBus != nil {
		b.eventBus.PublishAsync(
			ecs.NewEntityAppearanceChangedEvent(targetInfo.Layer, targetID, targetHandle),
			eventbus.PriorityMedium,
		)
	}
	ecs.MarkObjectBehaviorDirty(w, targetHandle)
}
