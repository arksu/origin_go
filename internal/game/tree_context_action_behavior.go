package game

import (
	"math"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const contextActionChop = "chop"

type treeContextActionBehavior struct {
	eventBus    *eventbus.EventBus
	chunks      chunkProvider
	idAllocator entityIDAllocator
	logger      *zap.Logger
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
		treeState, has := components.GetBehaviorState[components.TreeBehaviorState](*state, "tree")
		if !has || treeState == nil {
			treeState = &components.TreeBehaviorState{ChopPoints: treeConfig.ChopPointsTotal}
			components.SetBehaviorState(state, "tree", treeState)
		}

		if treeState.ChopPoints <= 0 {
			remaining = 0
			return
		}

		treeState.ChopPoints--
		state.IsDirty = true
		remaining = treeState.ChopPoints
		if remaining == 0 {
			components.DeleteBehaviorState(state, "tree")
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
	return cyclicActionDecisionComplete
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

	logDef, ok := objectdefs.Global().GetByKey(treeCfg.LogsSpawnDefKey)
	if !ok {
		b.logger.Warn("tree chop: log def not found", zap.String("def_key", treeCfg.LogsSpawnDefKey))
		return
	}

	dx := playerTransform.X - treeTransform.X
	dy := playerTransform.Y - treeTransform.Y
	// Fall in direction opposite to player.
	oppX := -dx
	oppY := -dy

	useXAxis := math.Abs(dx) >= math.Abs(dy) // tie => X
	dirX := 0.0
	dirY := 0.0
	if useXAxis {
		dirX = 1.0
		if oppX < 0 {
			dirX = -1.0
		}
	} else {
		dirY = 1.0
		if oppY < 0 {
			dirY = -1.0
		}
	}

	for index := 0; index < treeCfg.LogsSpawnCount; index++ {
		distance := float64(treeCfg.LogsSpawnInitialOffset + index*treeCfg.LogsSpawnStepOffset)
		logX := treeTransform.X + dirX*distance
		logY := treeTransform.Y + dirY*distance

		chunkX := int(logX) / constt.ChunkWorldSize
		chunkY := int(logY) / constt.ChunkWorldSize
		chunk := b.chunks.GetChunkFast(types.ChunkCoord{X: chunkX, Y: chunkY})
		if chunk == nil {
			continue
		}

		logID := b.idAllocator.GetFreeID()
		h := w.Spawn(logID, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Transform{
				X: logX,
				Y: logY,
			})
			ecs.AddComponent(w, h, components.EntityInfo{
				TypeID:    uint32(logDef.DefID),
				Behaviors: append([]string(nil), logDef.BehaviorOrder...),
				IsStatic:  logDef.IsStatic,
				Region:    treeInfo.Region,
				Layer:     treeInfo.Layer,
			})
			ecs.AddComponent(w, h, components.Appearance{
				Resource: logDef.Resource,
			})
			if logDef.Components != nil && logDef.Components.Collider != nil {
				c := logDef.Components.Collider
				ecs.AddComponent(w, h, components.Collider{
					HalfWidth:  c.W / 2.0,
					HalfHeight: c.H / 2.0,
					Layer:      c.Layer,
					Mask:       c.Mask,
				})
			}
			ecs.AddComponent(w, h, components.ChunkRef{
				CurrentChunkX: chunkX,
				CurrentChunkY: chunkY,
				PrevChunkX:    chunkX,
				PrevChunkY:    chunkY,
			})
			ecs.AddComponent(w, h, components.ObjectInternalState{IsDirty: true})
		})
		if h == types.InvalidHandle {
			continue
		}

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
		info.Behaviors = append(info.Behaviors[:0], stumpDef.BehaviorOrder...)
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
		c := stumpDef.Components.Collider
		if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); hasCollider {
			ecs.WithComponent(w, targetHandle, func(collider *components.Collider) {
				collider.HalfWidth = c.W / 2.0
				collider.HalfHeight = c.H / 2.0
				collider.Layer = c.Layer
				collider.Mask = c.Mask
			})
		} else {
			ecs.AddComponent(w, targetHandle, components.Collider{
				HalfWidth:  c.W / 2.0,
				HalfHeight: c.H / 2.0,
				Layer:      c.Layer,
				Mask:       c.Mask,
			})
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
