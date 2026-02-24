package behaviors

import (
	"fmt"
	"math"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	actionChop          = "chop"
	treeBehaviorKey     = "tree"
	treeStageFlagPrefix = "tree.stage"
	chopRequiredTag     = "axe"

	treeChopCycleDurationTicks = 20
	treeChopStaminaCost        = 17.0
	takeCycleDurationTicks     = 10
	takeCycleStaminaCost       = 10
	treeLogsSpawnInitialOffset = 16
	treeLogsSpawnStepOffset    = 20
	treeActionSound            = "chop"
	treeFinishSound            = "tree_fall"
)

type treeBehavior struct{}

func (treeBehavior) Key() string { return treeBehaviorKey }

func (treeBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("tree def config context is nil")
	}

	var cfg contracts.TreeBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid tree config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	if len(cfg.Stages) == 0 {
		return 0, fmt.Errorf("tree.stages is required")
	}
	for idx, stage := range cfg.Stages {
		if stage.ChopPointsTotal <= 0 {
			return 0, fmt.Errorf("tree.stages[%d].chopPointsTotal must be > 0", idx)
		}
		if idx < len(cfg.Stages)-1 && stage.StageDuration <= 0 {
			return 0, fmt.Errorf("tree.stages[%d].stageDurationTicks must be > 0 for non-final stage", idx)
		}
		for itemIdx, objectKey := range stage.SpawnChopObject {
			if strings.TrimSpace(objectKey) == "" {
				return 0, fmt.Errorf("tree.stages[%d].spawnChopObject[%d] must not be empty", idx, itemIdx)
			}
		}
		for itemIdx, itemKey := range stage.SpawnChopItem {
			if strings.TrimSpace(itemKey) == "" {
				return 0, fmt.Errorf("tree.stages[%d].spawnChopItem[%d] must not be empty", idx, itemIdx)
			}
		}
		seenTakeIDs := make(map[string]int, len(stage.Take))
		for takeIdx, takeCfg := range stage.Take {
			if err := validateTakeConfig(idx, takeIdx, takeCfg); err != nil {
				return 0, err
			}
			takeID := strings.TrimSpace(takeCfg.ID)
			if firstIndex, exists := seenTakeIDs[takeID]; exists {
				return 0, fmt.Errorf("tree.stages[%d].take[%d].id duplicate %q (first at index %d)", idx, takeIdx, takeID, firstIndex)
			}
			seenTakeIDs[takeID] = takeIdx
		}
	}

	if ctx.Def == nil {
		return 0, fmt.Errorf("tree config target def is nil")
	}
	ctx.Def.SetTreeBehaviorConfig(cfg)
	return cfg.Priority, nil
}

func validateTakeConfig(stageIndex int, takeIndex int, takeCfg contracts.TakeConfig) error {
	takeID := strings.TrimSpace(takeCfg.ID)
	if takeID == "" {
		return fmt.Errorf("tree.stages[%d].take[%d].id must not be empty", stageIndex, takeIndex)
	}
	if strings.TrimSpace(takeCfg.Name) == "" {
		return fmt.Errorf("tree.stages[%d].take[%d].name must not be empty", stageIndex, takeIndex)
	}
	if takeCfg.Count <= 0 {
		return fmt.Errorf("tree.stages[%d].take[%d].count must be > 0", stageIndex, takeIndex)
	}
	itemKey := strings.TrimSpace(takeCfg.ItemDefKey)
	if itemKey == "" {
		return fmt.Errorf("tree.stages[%d].take[%d].itemDefKey must not be empty", stageIndex, takeIndex)
	}
	itemRegistry := itemdefs.Global()
	if itemRegistry == nil {
		return fmt.Errorf("tree.stages[%d].take[%d].itemDefKey validation requires loaded item defs", stageIndex, takeIndex)
	}
	if _, ok := itemRegistry.GetByKey(itemKey); !ok {
		return fmt.Errorf("tree.stages[%d].take[%d].itemDefKey unknown item key %q", stageIndex, takeIndex, itemKey)
	}
	return nil
}

func (treeBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil {
		return nil
	}
	if ctx.World == nil {
		return nil
	}
	if ctx.Handle == types.InvalidHandle || !ctx.World.Alive(ctx.Handle) {
		return nil
	}

	def, found := objectdefs.Global().GetByID(int(ctx.EntityType))
	if !found || def.TreeConfig == nil {
		return nil
	}
	treeConfig := def.TreeConfig
	nowTick := ecs.GetResource[ecs.TimeState](ctx.World).Tick

	switch ctx.Reason {
	case contracts.ObjectBehaviorInitReasonSpawn:
		initializeSpawnTreeState(ctx.World, ctx.Handle, ctx.EntityID, treeConfig, nowTick)
	case contracts.ObjectBehaviorInitReasonRestore:
		initializeRestoredTreeState(ctx.World, ctx.Handle, ctx.EntityID, treeConfig, nowTick)
	}

	return nil
}

func (treeBehavior) ApplyRuntime(ctx *contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	if ctx == nil || ctx.World == nil {
		return contracts.BehaviorRuntimeResult{}
	}

	def, found := objectdefs.Global().GetByID(int(ctx.EntityType))
	if !found || def.TreeConfig == nil {
		return contracts.BehaviorRuntimeResult{}
	}
	stage := stageFromRuntimeState(ctx.PrevState, def.TreeConfig, stageMissingPolicyFinal)
	return contracts.BehaviorRuntimeResult{
		Flags: []string{treeStageFlag(stage)},
	}
}

func (treeBehavior) OnScheduledTick(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil || ctx.World == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	if ctx.Handle == types.InvalidHandle || !ctx.World.Alive(ctx.Handle) {
		return contracts.BehaviorTickResult{}, nil
	}

	def, found := objectdefs.Global().GetByID(int(ctx.EntityType))
	if !found || def.TreeConfig == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	treeConfig := def.TreeConfig
	stageChanged := false

	ecs.WithComponent(ctx.World, ctx.Handle, func(state *components.ObjectInternalState) {
		treeState, hasTreeState := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey)
		if !hasTreeState || treeState == nil {
			ecs.CancelBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey)
			return
		}

		currentStage := normalizeStage(treeState.Stage, treeConfig, stageMissingPolicyStart)
		currentNextTick := treeState.NextGrowthTick
		maxStage := growthStageMax(treeConfig)
		if currentStage >= maxStage {
			if currentNextTick != 0 {
				setTreeBehaviorState(state, treeState.ChopPoints, cloneTakenCounts(treeState.Taken), maxStage, 0)
			}
			ecs.CancelBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey)
			return
		}

		if currentNextTick == 0 {
			transitionDuration := stageTransitionDuration(treeConfig, currentStage)
			if transitionDuration == 0 {
				setTreeBehaviorState(state, treeState.ChopPoints, cloneTakenCounts(treeState.Taken), currentStage, 0)
				ecs.CancelBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey)
				return
			}
			currentNextTick = ctx.CurrentTick + transitionDuration
			setTreeBehaviorState(state, treeState.ChopPoints, cloneTakenCounts(treeState.Taken), currentStage, currentNextTick)
			ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey, currentNextTick)
			return
		}
		if ctx.CurrentTick < currentNextTick {
			ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey, currentNextTick)
			return
		}

		nextStage, nextTick, changed := applyGrowthCatchup(treeConfig, currentStage, currentNextTick, ctx.CurrentTick, 0)
		if !changed {
			ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey, currentNextTick)
			return
		}

		stageChanged = nextStage != currentStage
		taken := cloneTakenCounts(treeState.Taken)
		if stageChanged {
			taken = nil
		}
		setTreeBehaviorState(state, treeState.ChopPoints, taken, nextStage, nextTick)
		if nextStage >= maxStage {
			ecs.CancelBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey)
			return
		}
		if nextTick == 0 {
			ecs.CancelBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey)
			return
		}
		ecs.ScheduleBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey, nextTick)
	})

	return contracts.BehaviorTickResult{
		StateChanged: stageChanged,
	}, nil
}

func (treeBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	if ctx.TargetHandle == types.InvalidHandle || !ctx.World.Alive(ctx.TargetHandle) {
		return nil
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](ctx.World, ctx.TargetHandle)
	if !hasInfo {
		return nil
	}
	def, found := objectdefs.Global().GetByID(int(info.TypeID))
	if !found || def.TreeConfig == nil {
		return nil
	}
	stage := currentTreeStage(ctx.World, ctx.TargetHandle, def.TreeConfig, stageMissingPolicyFinal)
	stageCfg := stageConfigFor(def.TreeConfig, stage)
	if stageCfg == nil {
		return nil
	}
	actions := make([]contracts.ContextAction, 0, 1+len(stageCfg.Take))
	if isChopAllowedAtStage(def.TreeConfig, stage) &&
		playerHasEquippedTag(ctx.World, ctx.PlayerID, chopRequiredTag) {
		actions = append(actions, contracts.ContextAction{
			ActionID: actionChop,
			Title:    "Chop",
		})
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](ctx.World, ctx.TargetHandle)
	var taken map[string]int
	if hasState {
		if treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](internalState, treeBehaviorKey); hasTree && treeState != nil {
			taken = treeState.Taken
		}
	}
	for _, takeCfg := range stageCfg.Take {
		takeID := strings.TrimSpace(takeCfg.ID)
		if takeID == "" {
			continue
		}
		if takenCountForAction(taken, takeID) >= takeCfg.Count {
			continue
		}
		actions = append(actions, contracts.ContextAction{
			ActionID: takeID,
			Title:    strings.TrimSpace(takeCfg.Name),
		})
	}
	return actions
}

func (treeBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.World == nil {
		return contracts.BehaviorResult{OK: false}
	}
	actionID := strings.TrimSpace(ctx.ActionID)
	if actionID == "" {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.PlayerHandle == types.InvalidHandle || !ctx.World.Alive(ctx.PlayerHandle) {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.TargetHandle == types.InvalidHandle || !ctx.World.Alive(ctx.TargetHandle) {
		return contracts.BehaviorResult{OK: false}
	}
	targetInfo, hasInfo := ecs.GetComponent[components.EntityInfo](ctx.World, ctx.TargetHandle)
	if !hasInfo {
		return contracts.BehaviorResult{OK: false}
	}
	targetDef, found := objectdefs.Global().GetByID(int(targetInfo.TypeID))
	if !found || targetDef.TreeConfig == nil {
		return contracts.BehaviorResult{OK: false}
	}
	stage := currentTreeStage(ctx.World, ctx.TargetHandle, targetDef.TreeConfig, stageMissingPolicyFinal)
	stageCfg := stageConfigFor(targetDef.TreeConfig, stage)
	if stageCfg == nil {
		return contracts.BehaviorResult{OK: false}
	}
	if actionID == actionChop {
		if !isChopAllowedAtStage(targetDef.TreeConfig, stage) {
			return contracts.BehaviorResult{OK: false}
		}
		if !playerHasEquippedTag(ctx.World, ctx.PlayerID, chopRequiredTag) {
			return contracts.BehaviorResult{OK: false}
		}
	} else {
		takeCfg := findTakeConfigByActionID(stageCfg, actionID)
		if takeCfg == nil {
			return contracts.BehaviorResult{OK: false}
		}
		taken := takeCountFromState(ctx.World, ctx.TargetHandle, actionID)
		if taken >= takeCfg.Count {
			return contracts.BehaviorResult{OK: false}
		}
	}
	if ctx.Phase == contracts.BehaviorValidationPhaseExecute {
		if _, exists := ecs.GetComponent[components.ActiveCyclicAction](ctx.World, ctx.PlayerHandle); exists {
			return contracts.BehaviorResult{
				OK:          false,
				UserVisible: true,
				ReasonCode:  "action_already_active",
				Severity:    contracts.BehaviorAlertSeverityWarning,
			}
		}
	}
	return contracts.BehaviorResult{OK: true}
}

func (treeBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || ctx.World == nil {
		return contracts.BehaviorResult{OK: false}
	}
	actionID := strings.TrimSpace(ctx.ActionID)
	if actionID == "" {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.PlayerHandle == types.InvalidHandle || !ctx.World.Alive(ctx.PlayerHandle) {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.TargetHandle == types.InvalidHandle || !ctx.World.Alive(ctx.TargetHandle) {
		return contracts.BehaviorResult{OK: false}
	}

	targetInfo, hasInfo := ecs.GetComponent[components.EntityInfo](ctx.World, ctx.TargetHandle)
	if !hasInfo {
		return contracts.BehaviorResult{OK: false}
	}
	targetDef, found := objectdefs.Global().GetByID(int(targetInfo.TypeID))
	if !found || targetDef.TreeConfig == nil {
		return contracts.BehaviorResult{OK: false}
	}
	stage := currentTreeStage(ctx.World, ctx.TargetHandle, targetDef.TreeConfig, stageMissingPolicyFinal)
	stageCfg := stageConfigFor(targetDef.TreeConfig, stage)
	if stageCfg == nil {
		return contracts.BehaviorResult{OK: false}
	}
	if actionID == actionChop {
		if !isChopAllowedAtStage(targetDef.TreeConfig, stage) {
			return contracts.BehaviorResult{OK: false}
		}
		if !playerHasEquippedTag(ctx.World, ctx.PlayerID, chopRequiredTag) {
			return contracts.BehaviorResult{OK: false}
		}
	}
	if actionID != actionChop {
		takeCfg := findTakeConfigByActionID(stageCfg, actionID)
		if takeCfg == nil {
			return contracts.BehaviorResult{OK: false}
		}
		taken := takeCountFromState(ctx.World, ctx.TargetHandle, actionID)
		if taken >= takeCfg.Count {
			return contracts.BehaviorResult{OK: false}
		}
	}

	// Init chop points on first chop.
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		stage := currentTreeStageFromInternalState(*state, targetDef.TreeConfig, stageMissingPolicyFinal)
		nextGrowthTick := uint64(0)
		var taken map[string]int
		chopPoints := 0
		treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey)
		if hasTree && treeState != nil {
			nextGrowthTick = treeState.NextGrowthTick
			taken = cloneTakenCounts(treeState.Taken)
			chopPoints = treeState.ChopPoints
		}
		if actionID == actionChop && chopPoints > 0 {
			return
		}
		stageCfg := stageConfigFor(targetDef.TreeConfig, stage)
		if stageCfg == nil {
			return
		}
		if chopPoints <= 0 {
			chopPoints = stageCfg.ChopPointsTotal
		}
		setTreeBehaviorState(state, chopPoints, taken, stage, nextGrowthTick)
	})

	nowTick := ecs.GetResource[ecs.TimeState](ctx.World).Tick
	cycleDuration := uint32(treeChopCycleDurationTicks)
	cycleSoundKey := strings.TrimSpace(treeActionSound)
	completeSoundKey := strings.TrimSpace(treeFinishSound)
	if actionID != actionChop {
		cycleDuration = uint32(takeCycleDurationTicks)
		cycleSoundKey = ""
		completeSoundKey = ""
	}
	ecs.AddComponent(ctx.World, ctx.PlayerHandle, components.ActiveCyclicAction{
		BehaviorKey:        treeBehaviorKey,
		ActionID:           actionID,
		CycleSoundKey:      cycleSoundKey,
		CompleteSoundKey:   completeSoundKey,
		TargetKind:         components.CyclicActionTargetObject,
		TargetID:           ctx.TargetID,
		TargetHandle:       ctx.TargetHandle,
		CycleDurationTicks: cycleDuration,
		CycleElapsedTicks:  0,
		CycleIndex:         1,
		StartedTick:        nowTick,
	})

	ecs.MutateComponent[components.Movement](ctx.World, ctx.PlayerHandle, func(m *components.Movement) bool {
		m.State = constt.StateInteracting
		return true
	})
	return contracts.BehaviorResult{OK: true}
}

func (treeBehavior) OnCycleComplete(ctx *contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	if ctx == nil || ctx.World == nil || ctx.TargetHandle == types.InvalidHandle || !ctx.World.Alive(ctx.TargetHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}

	deps := resolveExecutionDeps(ctx.Deps)
	targetInfo, hasTargetInfo := ecs.GetComponent[components.EntityInfo](ctx.World, ctx.TargetHandle)
	if !hasTargetInfo {
		return contracts.BehaviorCycleDecisionCanceled
	}
	targetDef, found := objectdefs.Global().GetByID(int(targetInfo.TypeID))
	if !found || targetDef.TreeConfig == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}
	treeConfig := targetDef.TreeConfig
	if ctx.ActionID != actionChop {
		return onTakeCycleComplete(ctx, deps, treeConfig, targetInfo.Quality)
	}
	if !consumePlayerStaminaForTreeCycle(ctx.World, ctx.PlayerHandle, treeChopStaminaCost) {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "LOW_STAMINA")
		return contracts.BehaviorCycleDecisionCanceled
	}

	remaining := 0
	completed := false
	var completedStage *objectdefs.TreeStageConfig
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		currentStage := currentTreeStageFromInternalState(*state, treeConfig, stageMissingPolicyFinal)
		if !isChopAllowedAtStage(treeConfig, currentStage) {
			remaining = 0
			return
		}
		stageCfg := stageConfigFor(treeConfig, currentStage)
		if stageCfg == nil {
			remaining = 0
			return
		}

		currentChopPoints := stageCfg.ChopPointsTotal
		nextGrowthTick := uint64(0)
		var taken map[string]int
		if treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey); hasTree && treeState != nil {
			currentChopPoints = treeState.ChopPoints
			nextGrowthTick = treeState.NextGrowthTick
			taken = cloneTakenCounts(treeState.Taken)
		}

		if currentChopPoints <= 0 {
			remaining = 0
			return
		}

		currentChopPoints--
		remaining = currentChopPoints
		setTreeBehaviorState(state, currentChopPoints, taken, currentStage, nextGrowthTick)
		if remaining == 0 {
			completed = true
			completedStage = stageCfg
		}
	})

	if !completed {
		if remaining > 0 {
			return contracts.BehaviorCycleDecisionContinue
		}
		return contracts.BehaviorCycleDecisionCanceled
	}

	playerTransform, hasPlayerTransform := ecs.GetComponent[components.Transform](ctx.World, ctx.PlayerHandle)
	targetTransform, hasTargetTransform := ecs.GetComponent[components.Transform](ctx.World, ctx.TargetHandle)
	targetChunkRef, hasTargetChunkRef := ecs.GetComponent[components.ChunkRef](ctx.World, ctx.TargetHandle)
	if !hasPlayerTransform || !hasTargetTransform || !hasTargetChunkRef {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if completedStage == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}

	spawnStageObjects(
		ctx.World,
		targetTransform,
		playerTransform,
		targetInfo,
		completedStage.SpawnChopObject,
		deps,
	)
	spawnStageItems(
		ctx.World,
		ctx.PlayerID,
		ctx.PlayerHandle,
		targetInfo.Quality,
		completedStage.SpawnChopItem,
		deps,
	)
	if strings.TrimSpace(completedStage.TransformToDefKey) != "" {
		transformTargetToDef(
			ctx.World,
			ctx.TargetID,
			ctx.TargetHandle,
			completedStage.TransformToDefKey,
			deps,
		)
	} else {
		deleteTreeTarget(
			ctx.World,
			ctx.TargetID,
			ctx.TargetHandle,
			targetInfo,
			targetChunkRef,
			targetTransform,
			deps,
		)
	}
	forceVisionUpdates(ctx.World, deps.VisionForcer)
	return contracts.BehaviorCycleDecisionComplete
}

func onTakeCycleComplete(
	ctx *contracts.BehaviorCycleContext,
	deps contracts.ExecutionDeps,
	treeConfig *objectdefs.TreeBehaviorConfig,
	parentQuality uint32,
) contracts.BehaviorCycleDecision {
	if ctx == nil || ctx.World == nil || treeConfig == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if deps.GiveItem == nil {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "TREE_TAKE_UNAVAILABLE")
		return contracts.BehaviorCycleDecisionCanceled
	}
	if !consumePlayerStaminaForTreeCycle(ctx.World, ctx.PlayerHandle, takeCycleStaminaCost) {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "LOW_STAMINA")
		return contracts.BehaviorCycleDecisionCanceled
	}

	stage := currentTreeStage(ctx.World, ctx.TargetHandle, treeConfig, stageMissingPolicyFinal)
	stageCfg := stageConfigFor(treeConfig, stage)
	if stageCfg == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}
	actionID := strings.TrimSpace(ctx.ActionID)
	takeCfg := findTakeConfigByActionID(stageCfg, actionID)
	if takeCfg == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}

	taken := 0
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		treeState, hasState := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey)
		if hasState && treeState != nil {
			taken = takenCountForAction(treeState.Taken, actionID)
		}
	})
	itemKey := strings.TrimSpace(takeCfg.ItemDefKey)
	if itemKey == "" || takeCfg.Count <= 0 {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if taken >= takeCfg.Count {
		return contracts.BehaviorCycleDecisionComplete
	}

	outcome := deps.GiveItem(ctx.World, ctx.PlayerID, ctx.PlayerHandle, itemKey, 1, parentQuality)
	if !outcome.Success {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "TREE_TAKE_GIVE_FAILED")
		return contracts.BehaviorCycleDecisionCanceled
	}

	newTaken := 0
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		treeState, hasTreeState := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey)
		if !hasTreeState || treeState == nil {
			return
		}
		stage := normalizeStage(treeState.Stage, treeConfig, stageMissingPolicyFinal)
		nextTick := treeState.NextGrowthTick
		chopPoints := treeState.ChopPoints
		takenMap := cloneTakenCounts(treeState.Taken)
		takenMap, newTaken = incrementTakenCount(takenMap, actionID)
		setTreeBehaviorState(state, chopPoints, takenMap, stage, nextTick)
	})
	if outcome.PlacedInHand {
		return contracts.BehaviorCycleDecisionComplete
	}
	if newTaken >= takeCfg.Count {
		return contracts.BehaviorCycleDecisionComplete
	}
	return contracts.BehaviorCycleDecisionContinue
}

func findTakeConfigByActionID(stageCfg *objectdefs.TreeStageConfig, actionID string) *objectdefs.TakeConfig {
	if stageCfg == nil {
		return nil
	}
	for index := range stageCfg.Take {
		if strings.TrimSpace(stageCfg.Take[index].ID) == actionID {
			return &stageCfg.Take[index]
		}
	}
	return nil
}

func takeCountFromState(world *ecs.World, targetHandle types.Handle, actionID string) int {
	if world == nil || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return 0
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !hasState {
		return 0
	}
	treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](internalState, treeBehaviorKey)
	if !hasTree || treeState == nil {
		return 0
	}
	return takenCountForAction(treeState.Taken, actionID)
}

func takenCountForAction(taken map[string]int, actionID string) int {
	if len(taken) == 0 || actionID == "" {
		return 0
	}
	value, ok := taken[actionID]
	if !ok || value < 0 {
		return 0
	}
	return value
}

func incrementTakenCount(taken map[string]int, actionID string) (map[string]int, int) {
	if strings.TrimSpace(actionID) == "" {
		return taken, 0
	}
	if taken == nil {
		taken = make(map[string]int)
	}
	next := takenCountForAction(taken, actionID) + 1
	taken[actionID] = next
	return taken, next
}

func cloneTakenCounts(source map[string]int) map[string]int {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(source))
	for key, value := range source {
		if key == "" || value <= 0 {
			continue
		}
		cloned[key] = value
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
}

func setTreeBehaviorState(
	state *components.ObjectInternalState,
	chopPoints int,
	taken map[string]int,
	stage int,
	nextGrowthTick uint64,
) {
	if state == nil {
		return
	}
	components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
		ChopPoints:     chopPoints,
		Taken:          taken,
		Stage:          stage,
		NextGrowthTick: nextGrowthTick,
	})
}

func consumePlayerStaminaForTreeCycle(
	world *ecs.World,
	playerHandle types.Handle,
	cost float64,
) bool {
	return ConsumePlayerLongActionStamina(world, playerHandle, cost)
}

func sendWarningMiniAlert(playerID types.EntityID, alerts contracts.MiniAlertSender, reasonCode string) {
	if alerts == nil || playerID == 0 {
		return
	}
	alerts.SendMiniAlert(playerID, &netproto.S2C_MiniAlert{
		Severity:   netproto.AlertSeverity_ALERT_SEVERITY_WARNING,
		ReasonCode: reasonCode,
		TtlMs:      2000,
	})
}

func forceVisionUpdates(world *ecs.World, visionForcer contracts.VisionUpdateForcer) {
	if visionForcer == nil || world == nil {
		return
	}

	characters := ecs.GetResource[ecs.CharacterEntities](world)
	for _, character := range characters.Map {
		if character.Handle == types.InvalidHandle || !world.Alive(character.Handle) {
			continue
		}
		visionForcer.ForceUpdateForObserver(world, character.Handle)
	}
}

type treeStageMissingPolicy uint8

const (
	stageMissingPolicyStart treeStageMissingPolicy = iota + 1
	stageMissingPolicyFinal
)

func initializeSpawnTreeState(
	world *ecs.World,
	handle types.Handle,
	entityID types.EntityID,
	treeConfig *objectdefs.TreeBehaviorConfig,
	nowTick uint64,
) {
	if world == nil || treeConfig == nil {
		return
	}

	startStage := normalizeStage(0, treeConfig, stageMissingPolicyStart)
	nextGrowthTick := uint64(0)
	if startStage < growthStageMax(treeConfig) {
		nextGrowthTick = nowTick + stageTransitionDuration(treeConfig, startStage)
		ecs.ScheduleBehaviorTick(world, entityID, treeBehaviorKey, nextGrowthTick)
	} else {
		ecs.CancelBehaviorTick(world, entityID, treeBehaviorKey)
	}

	ecs.WithComponent(world, handle, func(state *components.ObjectInternalState) {
		chopPoints := 0
		var taken map[string]int
		if existing, has := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey); has && existing != nil {
			chopPoints = existing.ChopPoints
			taken = cloneTakenCounts(existing.Taken)
		}
		setTreeBehaviorState(state, chopPoints, taken, startStage, nextGrowthTick)
	})
}

func initializeRestoredTreeState(
	world *ecs.World,
	handle types.Handle,
	entityID types.EntityID,
	treeConfig *objectdefs.TreeBehaviorConfig,
	nowTick uint64,
) {
	if world == nil || treeConfig == nil {
		return
	}

	catchupLimit := uint64(2000)
	if policy, ok := ecs.TryGetResource[ecs.BehaviorTickPolicy](world); ok && policy != nil {
		if policy.CatchUpLimitTicks > 0 {
			catchupLimit = policy.CatchUpLimitTicks
		}
	}

	ecs.WithComponent(world, handle, func(state *components.ObjectInternalState) {
		treeState, hasTreeState := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey)
		if !hasTreeState || treeState == nil {
			ecs.CancelBehaviorTick(world, entityID, treeBehaviorKey)
			return
		}

		currentStage := normalizeStage(treeState.Stage, treeConfig, stageMissingPolicyFinal)
		nextGrowthTick := treeState.NextGrowthTick
		stateChanged := currentStage != treeState.Stage

		maxStage := growthStageMax(treeConfig)
		if currentStage >= maxStage {
			nextGrowthTick = 0
			ecs.CancelBehaviorTick(world, entityID, treeBehaviorKey)
		} else {
			if nextGrowthTick == 0 {
				nextGrowthTick = nowTick + stageTransitionDuration(treeConfig, currentStage)
				stateChanged = true
			}

			nextStage, caughtUpNextTick, caughtUp := applyGrowthCatchup(
				treeConfig,
				currentStage,
				nextGrowthTick,
				nowTick,
				catchupLimit,
			)
			if caughtUp {
				currentStage = nextStage
				nextGrowthTick = caughtUpNextTick
				stateChanged = true
			}

			if currentStage >= maxStage {
				nextGrowthTick = 0
				ecs.CancelBehaviorTick(world, entityID, treeBehaviorKey)
			} else if nextGrowthTick == 0 {
				ecs.CancelBehaviorTick(world, entityID, treeBehaviorKey)
			} else {
				ecs.ScheduleBehaviorTick(world, entityID, treeBehaviorKey, nextGrowthTick)
			}
		}

		if !stateChanged {
			return
		}
		taken := cloneTakenCounts(treeState.Taken)
		if currentStage != treeState.Stage {
			taken = nil
		}
		setTreeBehaviorState(state, treeState.ChopPoints, taken, currentStage, nextGrowthTick)
	})
}

func currentTreeStage(
	world *ecs.World,
	targetHandle types.Handle,
	treeConfig *objectdefs.TreeBehaviorConfig,
	missingPolicy treeStageMissingPolicy,
) int {
	if world == nil || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) || treeConfig == nil {
		return 1
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !hasState {
		return normalizeStage(0, treeConfig, missingPolicy)
	}
	return currentTreeStageFromInternalState(internalState, treeConfig, missingPolicy)
}

func currentTreeStageFromInternalState(
	internalState components.ObjectInternalState,
	treeConfig *objectdefs.TreeBehaviorConfig,
	missingPolicy treeStageMissingPolicy,
) int {
	if treeConfig == nil {
		return 1
	}
	treeState, hasState := components.GetBehaviorState[components.TreeBehaviorState](internalState, treeBehaviorKey)
	if !hasState || treeState == nil {
		return normalizeStage(0, treeConfig, missingPolicy)
	}
	return normalizeStage(treeState.Stage, treeConfig, missingPolicy)
}

func stageFromRuntimeState(
	runtimeState *components.RuntimeObjectState,
	treeConfig *objectdefs.TreeBehaviorConfig,
	missingPolicy treeStageMissingPolicy,
) int {
	if treeConfig == nil || runtimeState == nil || runtimeState.Behaviors == nil {
		return normalizeStage(0, treeConfig, missingPolicy)
	}
	rawState, has := runtimeState.Behaviors[treeBehaviorKey]
	if !has || rawState == nil {
		return normalizeStage(0, treeConfig, missingPolicy)
	}
	// Canonical runtime state is pointer-based. Value fallback is kept for defensive compatibility.
	if typedState, ok := rawState.(*components.TreeBehaviorState); ok && typedState != nil {
		return normalizeStage(typedState.Stage, treeConfig, missingPolicy)
	}
	if typedState, ok := rawState.(components.TreeBehaviorState); ok {
		return normalizeStage(typedState.Stage, treeConfig, missingPolicy)
	}
	return normalizeStage(0, treeConfig, missingPolicy)
}

func normalizeStage(
	stage int,
	treeConfig *objectdefs.TreeBehaviorConfig,
	missingPolicy treeStageMissingPolicy,
) int {
	maxStage := growthStageMax(treeConfig)
	if maxStage < 1 {
		return 1
	}
	if stage > 0 {
		if stage > maxStage {
			return maxStage
		}
		return stage
	}
	if missingPolicy == stageMissingPolicyFinal {
		return maxStage
	}
	return 1
}

func stageTransitionDuration(treeConfig *objectdefs.TreeBehaviorConfig, stage int) uint64 {
	if treeConfig == nil {
		return 0
	}
	maxStage := growthStageMax(treeConfig)
	if stage < 1 || stage >= maxStage {
		return 0
	}
	stageCfg := stageConfigFor(treeConfig, stage)
	if stageCfg == nil {
		return 0
	}
	durationTicks := stageCfg.StageDuration
	if durationTicks <= 0 {
		return 0
	}
	return uint64(durationTicks)
}

func applyGrowthCatchup(
	treeConfig *objectdefs.TreeBehaviorConfig,
	currentStage int,
	nextGrowthTick uint64,
	nowTick uint64,
	catchupLimit uint64,
) (int, uint64, bool) {
	if treeConfig == nil {
		return currentStage, nextGrowthTick, false
	}
	maxStage := growthStageMax(treeConfig)
	if currentStage >= maxStage || nextGrowthTick == 0 {
		return currentStage, nextGrowthTick, false
	}

	effectiveNowTick := nowTick
	if nowTick > nextGrowthTick && catchupLimit > 0 {
		limitedNowTick := nextGrowthTick + catchupLimit
		if limitedNowTick < effectiveNowTick {
			effectiveNowTick = limitedNowTick
		}
	}

	stage := currentStage
	nextTick := nextGrowthTick
	changed := false

	for stage < maxStage && nextTick > 0 && nextTick <= effectiveNowTick {
		stage++
		changed = true
		if stage >= maxStage {
			nextTick = 0
			break
		}

		transitionDuration := stageTransitionDuration(treeConfig, stage)
		if transitionDuration == 0 {
			nextTick = 0
			break
		}
		nextTick += transitionDuration
	}

	return stage, nextTick, changed
}

func isChopAllowedAtStage(treeConfig *objectdefs.TreeBehaviorConfig, stage int) bool {
	stageCfg := stageConfigFor(treeConfig, stage)
	return stageCfg != nil && stageCfg.AllowChop
}

func playerHasEquippedTag(world *ecs.World, playerID types.EntityID, requiredTag string) bool {
	if world == nil || playerID == 0 {
		return false
	}
	requiredTag = strings.TrimSpace(requiredTag)
	if requiredTag == "" {
		return false
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	equipmentHandle, found := refIndex.Lookup(constt.InventoryEquipment, playerID, 0)
	if !found || equipmentHandle == types.InvalidHandle || !world.Alive(equipmentHandle) {
		return false
	}
	container, hasContainer := ecs.GetComponent[components.InventoryContainer](world, equipmentHandle)
	if !hasContainer || container.Kind != constt.InventoryEquipment {
		return false
	}

	itemRegistry := itemdefs.Global()
	if itemRegistry == nil {
		return false
	}
	for _, item := range container.Items {
		itemDef, ok := itemRegistry.GetByID(int(item.TypeID))
		if !ok {
			continue
		}
		if hasItemTag(itemDef.Tags, requiredTag) {
			return true
		}
	}
	return false
}

func hasItemTag(tags []string, requiredTag string) bool {
	for _, tag := range tags {
		if tag == requiredTag {
			return true
		}
	}
	return false
}

func treeStageFlag(stage int) string {
	return fmt.Sprintf("%s%d", treeStageFlagPrefix, stage)
}

func growthStageMax(treeConfig *objectdefs.TreeBehaviorConfig) int {
	if treeConfig == nil || len(treeConfig.Stages) == 0 {
		return 1
	}
	return len(treeConfig.Stages)
}

func stageConfigFor(treeConfig *objectdefs.TreeBehaviorConfig, stage int) *objectdefs.TreeStageConfig {
	if treeConfig == nil || stage < 1 || stage > len(treeConfig.Stages) {
		return nil
	}
	return &treeConfig.Stages[stage-1]
}

func spawnStageObjects(
	world *ecs.World,
	treeTransform components.Transform,
	playerTransform components.Transform,
	treeInfo components.EntityInfo,
	objectKeys []string,
	deps contracts.ExecutionDeps,
) {
	if deps.IDAllocator == nil || deps.Chunks == nil || len(objectKeys) == 0 {
		return
	}

	logger := resolveLogger(deps.Logger)
	dirX, dirY := resolveLogFallAxisDirection(treeTransform.X, treeTransform.Y, playerTransform.X, playerTransform.Y)
	for index, rawObjectKey := range objectKeys {
		objectKey := resolveAxisLogDefKey(strings.TrimSpace(rawObjectKey), dirX, dirY)
		logDef, ok := objectdefs.Global().GetByKey(objectKey)
		if !ok {
			logger.Warn("tree chop: spawned object def not found", zap.String("def_key", objectKey))
			continue
		}
		logX, logY := logSpawnPosition(
			treeTransform.X,
			treeTransform.Y,
			dirX,
			dirY,
			treeLogsSpawnInitialOffset,
			treeLogsSpawnStepOffset,
			index,
		)

		chunkX := worldCoordToChunkIndex(logX)
		chunkY := worldCoordToChunkIndex(logY)
		chunk := deps.Chunks.GetChunkFast(types.ChunkCoord{X: chunkX, Y: chunkY})
		if chunk == nil {
			continue
		}

		logID := deps.IDAllocator.GetFreeID()
		handle := gameworld.SpawnEntityFromDef(world, logDef, gameworld.DefSpawnParams{
			EntityID:         logID,
			X:                logX,
			Y:                logY,
			Quality:          treeInfo.Quality,
			Region:           treeInfo.Region,
			Layer:            treeInfo.Layer,
			InitReason:       contracts.ObjectBehaviorInitReasonSpawn,
			BehaviorRegistry: deps.BehaviorRegistry,
		})
		if handle == types.InvalidHandle {
			continue
		}

		ecs.AddComponent(world, handle, components.ChunkRef{
			CurrentChunkX: chunkX,
			CurrentChunkY: chunkY,
			PrevChunkX:    chunkX,
			PrevChunkY:    chunkY,
		})

		if logDef.IsStatic {
			chunk.Spatial().AddStatic(handle, int(logX), int(logY))
		} else {
			chunk.Spatial().AddDynamic(handle, int(logX), int(logY))
		}
		chunk.MarkRawDataDirty()
		if len(logDef.BehaviorOrder) > 0 {
			ecs.MarkObjectBehaviorDirty(world, handle)
		}
	}
}

func spawnStageItems(
	world *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	parentQuality uint32,
	itemKeys []string,
	deps contracts.ExecutionDeps,
) {
	if world == nil || len(itemKeys) == 0 || deps.GiveItem == nil {
		return
	}
	logger := resolveLogger(deps.Logger)
	for _, rawItemKey := range itemKeys {
		itemKey := strings.TrimSpace(rawItemKey)
		if itemKey == "" {
			continue
		}
		outcome := deps.GiveItem(world, playerID, playerHandle, itemKey, 1, parentQuality)
		if !outcome.Success {
			logger.Warn("tree chop: failed to give stage item",
				zap.String("item_key", itemKey),
				zap.String("reason", outcome.Message),
			)
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
	if strings.HasSuffix(baseDefKey, "_y") {
		return baseDefKey
	}
	return baseDefKey + "_y"
}

func transformTargetToDef(
	world *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
	transformDefKey string,
	deps contracts.ExecutionDeps,
) {
	logger := resolveLogger(deps.Logger)
	stumpDef, ok := objectdefs.Global().GetByKey(transformDefKey)
	if !ok {
		logger.Warn("tree chop: stump def not found", zap.String("def_key", transformDefKey))
		return
	}
	gameworld.TransformObjectToDefInPlace(world, targetID, targetHandle, stumpDef, gameworld.TransformObjectInPlaceOptions{
		DeleteBehaviorStateKeys: []string{treeBehaviorKey},
		ClearFlags:              true,
		BehaviorRegistry:        deps.BehaviorRegistry,
		Chunks:                  deps.Chunks,
		EventBus:                deps.EventBus,
		Logger:                  logger,
	})
}

func deleteTreeTarget(
	world *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
	targetInfo components.EntityInfo,
	targetChunkRef components.ChunkRef,
	targetTransform components.Transform,
	deps contracts.ExecutionDeps,
) {
	if world == nil || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return
	}

	if deps.Chunks != nil {
		chunk := deps.Chunks.GetChunkFast(types.ChunkCoord{
			X: targetChunkRef.CurrentChunkX,
			Y: targetChunkRef.CurrentChunkY,
		})
		if chunk != nil {
			if targetInfo.IsStatic {
				chunk.Spatial().RemoveStatic(targetHandle, int(targetTransform.X), int(targetTransform.Y))
			} else {
				chunk.Spatial().RemoveDynamic(targetHandle, int(targetTransform.X), int(targetTransform.Y))
			}
			chunk.MarkRawDataDirty()
		}
	}

	ecs.CancelBehaviorTicksByEntityID(world, targetID)
	world.Despawn(targetHandle)

	if deps.EventBus != nil {
		deps.EventBus.PublishAsync(
			ecs.NewEntityAppearanceChangedEvent(targetInfo.Layer, targetID, targetHandle),
			eventbus.PriorityMedium,
		)
	}
}

func resolveLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}
