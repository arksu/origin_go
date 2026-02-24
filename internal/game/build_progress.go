package game

import (
	"slices"
	"strings"

	"origin/internal/builddefs"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

type buildCycleContext struct {
	targetID     types.EntityID
	targetHandle types.Handle
	buildDef     *builddefs.BuildDef
	buildState   *components.BuildBehaviorState
}

type processedBuildItem struct {
	SlotIndex         int
	StackIndex        int
	ItemKey           string
	Quality           uint32
	RemovedEmptyStack bool
	CompletedBuildNow bool
}

func (s *BuildService) IsSyntheticBuildAction(action components.ActiveCyclicAction) bool {
	return action.BehaviorKey == "" && action.ActionID == buildSyntheticActionID
}

func (s *BuildService) IsActiveBuildStillValid(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) bool {
	if s == nil || w == nil || w != s.world || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return false
	}
	if !s.IsSyntheticBuildAction(action) || action.TargetID == 0 {
		return false
	}
	_, ok := s.resolveBuildCycleContext(w, playerID, playerHandle, action.TargetID)
	return ok
}

func (s *BuildService) HandleBuildCycleComplete(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	action components.ActiveCyclicAction,
) contracts.BehaviorCycleDecision {
	if s == nil || w == nil || w != s.world || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}
	ctx, ok := s.resolveBuildCycleContext(w, playerID, playerHandle, action.TargetID)
	if !ok {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if isBuildProgressComplete(ctx.buildState) {
		if s.finalizeCompletedBuild(w, playerID, ctx.targetID, ctx.targetHandle, ctx.buildDef) {
			return contracts.BehaviorCycleDecisionComplete
		}
		return contracts.BehaviorCycleDecisionCanceled
	}

	processed, ok := s.processOneBuildItem(w, ctx.targetHandle)
	if !ok {
		s.sendWarning(playerID, "BUILD_PROGRESS_NO_MATERIALS")
		return contracts.BehaviorCycleDecisionCanceled
	}

	// Process target first and roll back on low stamina so multi-builder contention
	// cannot burn stamina for a no-progress cycle.
	if !s.hasBuildStamina(w, playerHandle, ctx.buildDef.StaminaCost) || !behaviors.ConsumePlayerLongActionStamina(w, playerHandle, ctx.buildDef.StaminaCost) {
		s.rollbackProcessedBuildItem(w, ctx.targetHandle, processed)
		s.sendWarning(playerID, "LOW_STAMINA")
		return contracts.BehaviorCycleDecisionCanceled
	}

	if processed.CompletedBuildNow {
		if s.finalizeCompletedBuild(w, playerID, ctx.targetID, ctx.targetHandle, ctx.buildDef) {
			return contracts.BehaviorCycleDecisionComplete
		}
		return contracts.BehaviorCycleDecisionCanceled
	}

	if s.totalBuildPutItemCountForTarget(w, ctx.targetHandle) == 0 {
		s.SendBuildStateSnapshotToLinkedPlayers(w, ctx.targetID)
		s.sendWarning(playerID, "BUILD_PROGRESS_NO_MATERIALS")
		return contracts.BehaviorCycleDecisionCanceled
	}

	s.SendBuildStateSnapshotToLinkedPlayers(w, ctx.targetID)
	return contracts.BehaviorCycleDecisionContinue
}

func (s *BuildService) startBuildCyclicAction(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
	targetHandle types.Handle,
) {
	if s == nil || w == nil || w != s.world || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) {
		return
	}
	if _, hasAction := ecs.GetComponent[components.ActiveCyclicAction](w, playerHandle); hasAction {
		s.sendWarning(playerID, "ACTION_BUSY")
		return
	}

	ctx, ok := s.resolveBuildCycleContext(w, playerID, playerHandle, targetID)
	if !ok {
		s.sendWarning(playerID, "BUILD_PROGRESS_INVALID_TARGET")
		return
	}
	if ctx.buildDef == nil || ctx.buildDef.TicksRequired == 0 {
		s.sendWarning(playerID, "BUILD_PROGRESS_INVALID_DEF")
		return
	}
	if _, found := findFirstProcessableBuildSlot(ctx.buildState); !found {
		if isBuildProgressComplete(ctx.buildState) {
			if s.finalizeCompletedBuild(w, playerID, ctx.targetID, ctx.targetHandle, ctx.buildDef) {
				return
			}
		}
		s.sendWarning(playerID, "BUILD_PROGRESS_NO_MATERIALS")
		return
	}
	if !s.hasBuildStamina(w, playerHandle, ctx.buildDef.StaminaCost) {
		s.sendWarning(playerID, "LOW_STAMINA")
		return
	}

	nowTick := ecs.GetResource[ecs.TimeState](w).Tick
	ecs.AddComponent(w, playerHandle, components.ActiveCyclicAction{
		ActionID:           buildSyntheticActionID,
		TargetKind:         components.CyclicActionTargetObject,
		TargetID:           ctx.targetID,
		TargetHandle:       ctx.targetHandle,
		CycleDurationTicks: ctx.buildDef.TicksRequired,
		CycleElapsedTicks:  0,
		CycleIndex:         1,
		StartedTick:        nowTick,
	})
	ecs.MutateComponent[components.Movement](w, playerHandle, func(m *components.Movement) bool {
		if m.State == constt.StateInteracting {
			return false
		}
		m.State = constt.StateInteracting
		return true
	})
}

func (s *BuildService) resolveBuildCycleContext(
	w *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	targetID types.EntityID,
) (buildCycleContext, bool) {
	if s == nil || w == nil || w != s.world || playerID == 0 || playerHandle == types.InvalidHandle || !w.Alive(playerHandle) || targetID == 0 {
		return buildCycleContext{}, false
	}
	targetHandle := w.GetHandleByEntityID(targetID)
	if targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return buildCycleContext{}, false
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo || info.TypeID != constt.BuildObjectTypeID {
		return buildCycleContext{}, false
	}
	link, linked := ecs.GetResource[ecs.LinkState](w).GetLink(playerID)
	if !linked || link.TargetID != targetID {
		return buildCycleContext{}, false
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](w, targetHandle)
	if !hasState {
		return buildCycleContext{}, false
	}
	buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, buildBehaviorStateKey)
	if !ok || buildState == nil {
		return buildCycleContext{}, false
	}
	buildDef, ok := resolveBuildDefFromState(buildState)
	if !ok || buildDef == nil {
		return buildCycleContext{}, false
	}
	return buildCycleContext{
		targetID:     targetID,
		targetHandle: targetHandle,
		buildDef:     buildDef,
		buildState:   buildState,
	}, true
}

func resolveBuildDefFromState(buildState *components.BuildBehaviorState) (*builddefs.BuildDef, bool) {
	if buildState == nil {
		return nil, false
	}
	reg := builddefs.Global()
	if reg == nil {
		return nil, false
	}
	if buildState.BuildDefID > 0 {
		if def, ok := reg.GetByID(buildState.BuildDefID); ok && def != nil {
			return def, true
		}
	}
	if key := strings.TrimSpace(buildState.BuildKey); key != "" {
		if def, ok := reg.GetByKey(key); ok && def != nil {
			return def, true
		}
	}
	return nil, false
}

func findFirstProcessableBuildSlot(buildState *components.BuildBehaviorState) (int, bool) {
	if buildState == nil {
		return 0, false
	}
	for i := range buildState.Items {
		slot := &buildState.Items[i]
		if slot.BuildCount >= slot.RequiredCount {
			continue
		}
		if findFirstNonEmptyBuildPutStack(slot.PutItems) >= 0 {
			return i, true
		}
	}
	return 0, false
}

func findFirstNonEmptyBuildPutStack(stacks []components.BuildPutItemState) int {
	for i := range stacks {
		if stacks[i].Count > 0 {
			return i
		}
	}
	return -1
}

func isBuildProgressComplete(buildState *components.BuildBehaviorState) bool {
	if buildState == nil {
		return false
	}
	for i := range buildState.Items {
		if buildState.Items[i].BuildCount < buildState.Items[i].RequiredCount {
			return false
		}
	}
	return len(buildState.Items) > 0
}

func totalBuildPutItemCount(buildState *components.BuildBehaviorState) uint32 {
	if buildState == nil {
		return 0
	}
	var total uint32
	for i := range buildState.Items {
		total += buildState.Items[i].PutCount()
	}
	return total
}

func (s *BuildService) totalBuildPutItemCountForTarget(w *ecs.World, targetHandle types.Handle) uint32 {
	if s == nil || w == nil || w != s.world || targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return 0
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](w, targetHandle)
	if !hasState {
		return 0
	}
	buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, buildBehaviorStateKey)
	if !ok || buildState == nil {
		return 0
	}
	return totalBuildPutItemCount(buildState)
}

func (s *BuildService) processOneBuildItem(
	w *ecs.World,
	targetHandle types.Handle,
) (processedBuildItem, bool) {
	var result processedBuildItem
	processed := false

	ecs.MutateComponent[components.ObjectInternalState](w, targetHandle, func(state *components.ObjectInternalState) bool {
		buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](*state, buildBehaviorStateKey)
		if !ok || buildState == nil {
			return false
		}
		slotIndex, ok := findFirstProcessableBuildSlot(buildState)
		if !ok {
			return false
		}
		slot := &buildState.Items[slotIndex]
		stackIndex := findFirstNonEmptyBuildPutStack(slot.PutItems)
		if stackIndex < 0 {
			return false
		}
		stack := &slot.PutItems[stackIndex]
		if stack.Count == 0 {
			return false
		}

		result = processedBuildItem{
			SlotIndex:  slotIndex,
			StackIndex: stackIndex,
			ItemKey:    stack.ItemKey,
			Quality:    stack.Quality,
		}

		stack.Count--
		if stack.Count == 0 {
			slot.PutItems = append(slot.PutItems[:stackIndex], slot.PutItems[stackIndex+1:]...)
			result.RemovedEmptyStack = true
		}
		slot.BuildCount++
		slot.BuildQualityTotal += result.Quality
		result.CompletedBuildNow = isBuildProgressComplete(buildState)

		state.IsDirty = true
		processed = true
		return true
	})

	return result, processed
}

func (s *BuildService) rollbackProcessedBuildItem(
	w *ecs.World,
	targetHandle types.Handle,
	item processedBuildItem,
) {
	if w == nil || targetHandle == types.InvalidHandle || !w.Alive(targetHandle) || item.ItemKey == "" {
		return
	}
	ecs.MutateComponent[components.ObjectInternalState](w, targetHandle, func(state *components.ObjectInternalState) bool {
		buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](*state, buildBehaviorStateKey)
		if !ok || buildState == nil || item.SlotIndex < 0 || item.SlotIndex >= len(buildState.Items) {
			return false
		}
		slot := &buildState.Items[item.SlotIndex]
		if slot.BuildCount > 0 {
			slot.BuildCount--
		}
		if slot.BuildQualityTotal >= item.Quality {
			slot.BuildQualityTotal -= item.Quality
		} else {
			slot.BuildQualityTotal = 0
		}

		if item.RemovedEmptyStack {
			insertBuildPutStackAt(slot, item.StackIndex, components.BuildPutItemState{
				ItemKey: item.ItemKey,
				Quality: item.Quality,
				Count:   1,
			})
		} else if item.StackIndex >= 0 && item.StackIndex < len(slot.PutItems) &&
			slot.PutItems[item.StackIndex].ItemKey == item.ItemKey &&
			slot.PutItems[item.StackIndex].Quality == item.Quality {
			slot.PutItems[item.StackIndex].Count++
		} else {
			// Fallback preserves material correctness if slot content changed unexpectedly.
			// Merge/append is safer than inserting at a stale index.
			slot.MergePutItem(item.ItemKey, item.Quality, 1)
		}
		state.IsDirty = true
		return true
	})
}

func insertBuildPutStackAt(slot *components.BuildRequiredItemState, insertAt int, stack components.BuildPutItemState) {
	if slot == nil || stack.ItemKey == "" || stack.Count == 0 {
		return
	}
	if insertAt < 0 {
		insertAt = 0
	}
	if insertAt > len(slot.PutItems) {
		insertAt = len(slot.PutItems)
	}
	slot.PutItems = append(slot.PutItems, components.BuildPutItemState{})
	copy(slot.PutItems[insertAt+1:], slot.PutItems[insertAt:])
	slot.PutItems[insertAt] = stack
}

func (s *BuildService) hasBuildStamina(w *ecs.World, playerHandle types.Handle, cost float64) bool {
	if cost <= 0 {
		return true
	}
	stats, hasStats := ecs.GetComponent[components.EntityStats](w, playerHandle)
	if !hasStats {
		return true
	}
	con := characterattrs.DefaultValue
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, playerHandle); hasProfile {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	maxStamina := entitystats.MaxStaminaFromCon(con)
	currentStamina := entitystats.ClampStamina(stats.Stamina, maxStamina)
	return entitystats.CanConsumeLongActionStamina(currentStamina, maxStamina, cost)
}

func (s *BuildService) SendBuildStateSnapshotToLinkedPlayers(
	w *ecs.World,
	targetID types.EntityID,
) {
	if s == nil || w == nil || w != s.world || targetID == 0 {
		return
	}
	linkState := ecs.GetResource[ecs.LinkState](w)
	players := linkState.PlayersByTarget[targetID]
	if len(players) == 0 {
		return
	}
	playerIDs := make([]types.EntityID, 0, len(players))
	for playerID := range players {
		playerIDs = append(playerIDs, playerID)
	}
	slices.Sort(playerIDs)
	for _, playerID := range playerIDs {
		s.SendBuildStateSnapshot(w, playerID, targetID)
	}
}

func (s *BuildService) finalizeCompletedBuild(
	w *ecs.World,
	actorPlayerID types.EntityID,
	targetID types.EntityID,
	targetHandle types.Handle,
	buildDef *builddefs.BuildDef,
) bool {
	if s == nil || w == nil || w != s.world || targetID == 0 || targetHandle == types.InvalidHandle || !w.Alive(targetHandle) {
		return false
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !hasInfo || info.TypeID != constt.BuildObjectTypeID {
		return false
	}
	internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](w, targetHandle)
	if !hasInternalState {
		return false
	}
	buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, buildBehaviorStateKey)
	if !ok || buildState == nil || !isBuildProgressComplete(buildState) {
		return false
	}
	resultDef, ok := resolveBuildResultObjectDef(buildState)
	if !ok || resultDef == nil {
		s.logger.Warn("build finalize failed: result object def missing",
			zap.Uint64("target_id", uint64(targetID)))
		return false
	}

	s.closeAndBreakLinksForCompletedBuild(w, actorPlayerID, targetID)
	s.transformCompletedBuildTarget(w, targetID, targetHandle, info, buildDef, buildState, resultDef)
	return true
}

func resolveBuildResultObjectDef(buildState *components.BuildBehaviorState) (*objectdefs.ObjectDef, bool) {
	if buildState == nil {
		return nil, false
	}
	reg := objectdefs.Global()
	if reg == nil {
		return nil, false
	}
	if buildState.ObjectTypeID > 0 {
		if def, ok := reg.GetByID(int(buildState.ObjectTypeID)); ok && def != nil {
			return def, true
		}
	}
	if key := strings.TrimSpace(buildState.ObjectKey); key != "" {
		if def, ok := reg.GetByKey(key); ok && def != nil {
			return def, true
		}
	}
	return nil, false
}

func (s *BuildService) closeAndBreakLinksForCompletedBuild(
	w *ecs.World,
	actorPlayerID types.EntityID,
	targetID types.EntityID,
) {
	if s == nil || w == nil || targetID == 0 {
		return
	}
	linkState := ecs.GetResource[ecs.LinkState](w)
	players := linkState.PlayersByTarget[targetID]
	if len(players) == 0 {
		return
	}
	playerIDs := make([]types.EntityID, 0, len(players))
	for playerID := range players {
		playerIDs = append(playerIDs, playerID)
	}
	slices.Sort(playerIDs)

	for _, playerID := range playerIDs {
		if playerID == actorPlayerID {
			continue
		}
		if _, _, err := ecs.BreakLinkForPlayer(w, playerID, ecs.LinkBreakClosed); err != nil {
			s.logger.Warn("failed to break link for completed build (other player)",
				zap.Uint64("player_id", uint64(playerID)),
				zap.Uint64("target_id", uint64(targetID)),
				zap.Error(err))
		}
	}
	if actorPlayerID != 0 {
		s.sendBuildStateClosedDirect(actorPlayerID, targetID)
		s.breakLinkForPlayerSilently(w, actorPlayerID)
	}
}

func (s *BuildService) sendBuildStateClosedDirect(playerID, targetID types.EntityID) {
	if s == nil || s.alerts == nil || playerID == 0 || targetID == 0 {
		return
	}
	s.alerts.SendBuildStateClosed(playerID, &netproto.S2C_BuildStateClosed{
		EntityId: uint64(targetID),
	})
}

func (s *BuildService) breakLinkForPlayerSilently(w *ecs.World, playerID types.EntityID) bool {
	if w == nil || playerID == 0 {
		return false
	}
	linkState := ecs.GetResource[ecs.LinkState](w)
	link, removed := linkState.RemoveLink(playerID)
	if !removed {
		linkState.ClearIntent(playerID)
		return false
	}
	if intent, hasIntent := linkState.IntentByPlayer[playerID]; !hasIntent || intent.TargetID == link.TargetID {
		linkState.ClearIntent(playerID)
	}
	return true
}

func (s *BuildService) transformCompletedBuildTarget(
	w *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
	targetInfo components.EntityInfo,
	buildDef *builddefs.BuildDef,
	buildState *components.BuildBehaviorState,
	resultDef *objectdefs.ObjectDef,
) {
	if s == nil || w == nil || targetHandle == types.InvalidHandle || !w.Alive(targetHandle) || resultDef == nil {
		return
	}
	previousTypeID := targetInfo.TypeID

	var computedQuality uint32
	var hasComputedQuality bool
	if buildDef != nil && buildState != nil {
		computedQuality, hasComputedQuality = s.computeCompletedBuildObjectQuality(buildDef, buildState)
	}

	ecs.WithComponent(w, targetHandle, func(info *components.EntityInfo) {
		info.TypeID = uint32(resultDef.DefID)
		info.Behaviors = resultDef.CopyBehaviorOrder()
		info.IsStatic = resultDef.IsStatic
		if hasComputedQuality {
			info.Quality = computedQuality
		}
	})

	resource := objectdefs.ResolveAppearanceResource(resultDef, nil)
	if resource == "" {
		resource = resultDef.Resource
	}
	if _, hasAppearance := ecs.GetComponent[components.Appearance](w, targetHandle); hasAppearance {
		ecs.WithComponent(w, targetHandle, func(appearance *components.Appearance) {
			appearance.Resource = resource
		})
	}

	if resultDef.Components != nil && resultDef.Components.Collider != nil {
		nextCollider := objectdefs.BuildColliderComponent(resultDef.Components.Collider)
		if _, hasCollider := ecs.GetComponent[components.Collider](w, targetHandle); hasCollider {
			ecs.WithComponent(w, targetHandle, func(c *components.Collider) {
				c.HalfWidth = nextCollider.HalfWidth
				c.HalfHeight = nextCollider.HalfHeight
				c.Layer = nextCollider.Layer
				c.Mask = nextCollider.Mask
			})
		} else {
			ecs.AddComponent(w, targetHandle, nextCollider)
		}
	} else {
		ecs.RemoveComponent[components.Collider](w, targetHandle)
	}

	ecs.WithComponent(w, targetHandle, func(state *components.ObjectInternalState) {
		components.DeleteBehaviorState(state, buildBehaviorStateKey)
		state.Flags = nil
		state.IsDirty = true
	})

	ecs.CancelBehaviorTicksByEntityID(w, targetID)
	if s.behaviorRegistry != nil {
		currentInfo, hasInfo := ecs.GetComponent[components.EntityInfo](w, targetHandle)
		if hasInfo {
			if err := s.behaviorRegistry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
				World:        w,
				Handle:       targetHandle,
				EntityID:     targetID,
				EntityType:   currentInfo.TypeID,
				PreviousType: previousTypeID,
				Reason:       contracts.ObjectBehaviorInitReasonTransform,
			}, currentInfo.Behaviors); err != nil {
				s.logger.Error("build finalize: failed to init transformed object behaviors",
					zap.Uint64("target_id", uint64(targetID)),
					zap.Error(err))
			}
		}
	}

	if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](w, targetHandle); hasChunkRef && s.chunkManager != nil {
		chunk := s.chunkManager.GetChunkFast(types.ChunkCoord{
			X: chunkRef.CurrentChunkX,
			Y: chunkRef.CurrentChunkY,
		})
		if chunk != nil {
			chunk.MarkRawDataDirty()
		}
	}

	if s.eventBus != nil {
		s.eventBus.PublishAsync(
			ecs.NewEntityAppearanceChangedEvent(targetInfo.Layer, targetID, targetHandle),
			eventbus.PriorityMedium,
		)
	}
	ecs.MarkObjectBehaviorDirty(w, targetHandle)
}

func (s *BuildService) computeCompletedBuildObjectQuality(
	buildDef *builddefs.BuildDef,
	buildState *components.BuildBehaviorState,
) (uint32, bool) {
	_ = buildDef
	_ = buildState
	// Keep completion quality computation behind one seam so future content-driven
	// formulas can be added without changing the build cycle/transform flow.
	return 0, false
}
