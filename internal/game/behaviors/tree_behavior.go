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
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	actionChop          = "chop"
	treeBehaviorKey     = "tree"
	treeStageFlagPrefix = "tree.stage"
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
	if cfg.ChopPointsTotal <= 0 {
		return 0, fmt.Errorf("tree.chopPointsTotal must be > 0")
	}
	if cfg.ChopCycleDurationTicks <= 0 {
		return 0, fmt.Errorf("tree.chopCycleDurationTicks must be > 0")
	}
	if cfg.LogsSpawnDefKey == "" {
		return 0, fmt.Errorf("tree.logsSpawnDefKey is required")
	}
	if cfg.LogsSpawnCount <= 0 {
		return 0, fmt.Errorf("tree.logsSpawnCount must be > 0")
	}
	if cfg.LogsSpawnInitialOffset < 0 {
		return 0, fmt.Errorf("tree.logsSpawnInitialOffset must be >= 0")
	}
	if cfg.LogsSpawnStepOffset <= 0 {
		return 0, fmt.Errorf("tree.logsSpawnStepOffset must be > 0")
	}
	if cfg.TransformToDefKey == "" {
		return 0, fmt.Errorf("tree.transformToDefKey is required")
	}
	if cfg.GrowthStageMax < 1 {
		return 0, fmt.Errorf("tree.growthStageMax must be >= 1")
	}
	if cfg.GrowthStartStage <= 0 {
		cfg.GrowthStartStage = 1
	}
	if cfg.GrowthStartStage > cfg.GrowthStageMax {
		return 0, fmt.Errorf("tree.growthStartStage must be in range 1..growthStageMax")
	}
	if cfg.GrowthStageDurations == nil {
		return 0, fmt.Errorf("tree.growthStageDurationsTicks is required")
	}
	if len(cfg.GrowthStageDurations) != cfg.GrowthStageMax-1 {
		return 0, fmt.Errorf("tree.growthStageDurationsTicks length must be growthStageMax-1")
	}
	for idx, duration := range cfg.GrowthStageDurations {
		if duration <= 0 {
			return 0, fmt.Errorf("tree.growthStageDurationsTicks[%d] must be > 0", idx)
		}
	}
	if cfg.AllowedChopStages == nil {
		return 0, fmt.Errorf("tree.allowedChopStages is required")
	}
	seenStages := make(map[int]struct{}, len(cfg.AllowedChopStages))
	for _, stage := range cfg.AllowedChopStages {
		if stage < 1 || stage > cfg.GrowthStageMax {
			return 0, fmt.Errorf("tree.allowedChopStages values must be in range 1..growthStageMax")
		}
		if _, exists := seenStages[stage]; exists {
			return 0, fmt.Errorf("tree.allowedChopStages contains duplicate stage %d", stage)
		}
		seenStages[stage] = struct{}{}
	}

	if ctx.Def == nil {
		return 0, fmt.Errorf("tree config target def is nil")
	}
	ctx.Def.SetTreeBehaviorConfig(cfg)
	return cfg.Priority, nil
}

func (treeBehavior) DeclaredActions() []contracts.BehaviorActionSpec {
	return []contracts.BehaviorActionSpec{
		{
			ActionID:     actionChop,
			StartsCyclic: true,
		},
	}
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
		if currentStage >= treeConfig.GrowthStageMax {
			if currentNextTick != 0 {
				components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
					ChopPoints: treeState.ChopPoints,
					Stage:      treeConfig.GrowthStageMax,
				})
			}
			ecs.CancelBehaviorTick(ctx.World, ctx.EntityID, treeBehaviorKey)
			return
		}

		if currentNextTick == 0 {
			currentNextTick = ctx.CurrentTick + stageTransitionDuration(treeConfig, currentStage)
			components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
				ChopPoints:     treeState.ChopPoints,
				Stage:          currentStage,
				NextGrowthTick: currentNextTick,
			})
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
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     treeState.ChopPoints,
			Stage:          nextStage,
			NextGrowthTick: nextTick,
		})
		if nextStage >= treeConfig.GrowthStageMax {
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
	if !isChopAllowedAtStage(def.TreeConfig, stage) {
		return nil
	}

	return []contracts.ContextAction{
		{
			ActionID: actionChop,
			Title:    "Chop",
		},
	}
}

func (treeBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx.ActionID != actionChop {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx == nil || ctx.World == nil {
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
	if !isChopAllowedAtStage(targetDef.TreeConfig, stage) {
		return contracts.BehaviorResult{OK: false}
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
	if ctx.ActionID != actionChop {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx == nil || ctx.World == nil {
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
	if !isChopAllowedAtStage(
		targetDef.TreeConfig,
		currentTreeStage(ctx.World, ctx.TargetHandle, targetDef.TreeConfig, stageMissingPolicyFinal),
	) {
		return contracts.BehaviorResult{OK: false}
	}

	// Init chop points on first chop.
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		stage := targetDef.TreeConfig.GrowthStageMax
		nextGrowthTick := uint64(0)
		treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey)
		if hasTree && treeState != nil {
			stage = normalizeStage(treeState.Stage, targetDef.TreeConfig, stageMissingPolicyStart)
			nextGrowthTick = treeState.NextGrowthTick
		}
		if hasTree && treeState != nil && treeState.ChopPoints > 0 {
			return
		}
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     targetDef.TreeConfig.ChopPointsTotal,
			Stage:          stage,
			NextGrowthTick: nextGrowthTick,
		})
	})

	nowTick := ecs.GetResource[ecs.TimeState](ctx.World).Tick
	ecs.AddComponent(ctx.World, ctx.PlayerHandle, components.ActiveCyclicAction{
		BehaviorKey:        treeBehaviorKey,
		ActionID:           actionChop,
		CycleSoundKey:      strings.TrimSpace(targetDef.TreeConfig.ActionSound),
		CompleteSoundKey:   strings.TrimSpace(targetDef.TreeConfig.FinishSound),
		TargetKind:         components.CyclicActionTargetObject,
		TargetID:           ctx.TargetID,
		TargetHandle:       ctx.TargetHandle,
		CycleDurationTicks: uint32(targetDef.TreeConfig.ChopCycleDurationTicks),
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

	remaining := 0
	transitionToStump := false
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		currentStage := currentTreeStageFromInternalState(*state, treeConfig, stageMissingPolicyFinal)
		if !isChopAllowedAtStage(treeConfig, currentStage) {
			remaining = 0
			return
		}

		currentChopPoints := treeConfig.ChopPointsTotal
		nextGrowthTick := uint64(0)
		if treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey); hasTree && treeState != nil {
			currentChopPoints = treeState.ChopPoints
			nextGrowthTick = treeState.NextGrowthTick
		}

		if currentChopPoints <= 0 {
			remaining = 0
			return
		}

		currentChopPoints--
		remaining = currentChopPoints
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     currentChopPoints,
			Stage:          currentStage,
			NextGrowthTick: nextGrowthTick,
		})
		if remaining == 0 {
			transitionToStump = true
		}
	})

	if !transitionToStump {
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

	spawnLogs(ctx.World, targetTransform, playerTransform, targetInfo, treeConfig, deps)
	transformTargetToStump(
		ctx.World,
		ctx.TargetID,
		ctx.TargetHandle,
		targetInfo,
		targetChunkRef,
		treeConfig,
		deps,
	)
	forceVisionUpdates(ctx.World, deps.VisionForcer)
	return contracts.BehaviorCycleDecisionComplete
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
	if startStage < treeConfig.GrowthStageMax {
		nextGrowthTick = nowTick + stageTransitionDuration(treeConfig, startStage)
		ecs.ScheduleBehaviorTick(world, entityID, treeBehaviorKey, nextGrowthTick)
	} else {
		ecs.CancelBehaviorTick(world, entityID, treeBehaviorKey)
	}

	ecs.WithComponent(world, handle, func(state *components.ObjectInternalState) {
		chopPoints := 0
		if existing, has := components.GetBehaviorState[components.TreeBehaviorState](*state, treeBehaviorKey); has && existing != nil {
			chopPoints = existing.ChopPoints
		}
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     chopPoints,
			Stage:          startStage,
			NextGrowthTick: nextGrowthTick,
		})
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

		if currentStage >= treeConfig.GrowthStageMax {
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

			if currentStage >= treeConfig.GrowthStageMax {
				nextGrowthTick = 0
				ecs.CancelBehaviorTick(world, entityID, treeBehaviorKey)
			} else {
				ecs.ScheduleBehaviorTick(world, entityID, treeBehaviorKey, nextGrowthTick)
			}
		}

		if !stateChanged {
			return
		}
		components.SetBehaviorState(state, treeBehaviorKey, &components.TreeBehaviorState{
			ChopPoints:     treeState.ChopPoints,
			Stage:          currentStage,
			NextGrowthTick: nextGrowthTick,
		})
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
	if treeConfig == nil || treeConfig.GrowthStageMax < 1 {
		return 1
	}
	if stage > 0 {
		if stage > treeConfig.GrowthStageMax {
			return treeConfig.GrowthStageMax
		}
		return stage
	}
	if missingPolicy == stageMissingPolicyFinal {
		return treeConfig.GrowthStageMax
	}
	if treeConfig.GrowthStartStage >= 1 && treeConfig.GrowthStartStage <= treeConfig.GrowthStageMax {
		return treeConfig.GrowthStartStage
	}
	return 1
}

func stageTransitionDuration(treeConfig *objectdefs.TreeBehaviorConfig, stage int) uint64 {
	if treeConfig == nil || stage < 1 || stage >= treeConfig.GrowthStageMax {
		return 0
	}
	stageIndex := stage - 1
	if stageIndex < 0 || stageIndex >= len(treeConfig.GrowthStageDurations) {
		return 0
	}
	durationTicks := treeConfig.GrowthStageDurations[stageIndex]
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
	if treeConfig == nil || currentStage >= treeConfig.GrowthStageMax || nextGrowthTick == 0 {
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

	for stage < treeConfig.GrowthStageMax && nextTick > 0 && nextTick <= effectiveNowTick {
		stage++
		changed = true
		if stage >= treeConfig.GrowthStageMax {
			nextTick = 0
			break
		}

		transitionDuration := stageTransitionDuration(treeConfig, stage)
		if transitionDuration == 0 {
			nextTick = 0
			stage = treeConfig.GrowthStageMax
			break
		}
		nextTick += transitionDuration
	}

	return stage, nextTick, changed
}

func isChopAllowedAtStage(treeConfig *objectdefs.TreeBehaviorConfig, stage int) bool {
	if treeConfig == nil || stage <= 0 || len(treeConfig.AllowedChopStages) == 0 {
		return false
	}
	for _, allowedStage := range treeConfig.AllowedChopStages {
		if allowedStage == stage {
			return true
		}
	}
	return false
}

func treeStageFlag(stage int) string {
	return fmt.Sprintf("%s%d", treeStageFlagPrefix, stage)
}

func spawnLogs(
	world *ecs.World,
	treeTransform components.Transform,
	playerTransform components.Transform,
	treeInfo components.EntityInfo,
	treeCfg *objectdefs.TreeBehaviorConfig,
	deps contracts.ExecutionDeps,
) {
	if deps.IDAllocator == nil || deps.Chunks == nil || treeCfg == nil {
		return
	}

	logger := resolveLogger(deps.Logger)
	dirX, dirY := resolveLogFallAxisDirection(treeTransform.X, treeTransform.Y, playerTransform.X, playerTransform.Y)
	logDefKey := resolveAxisLogDefKey(treeCfg.LogsSpawnDefKey, dirX, dirY)
	logDef, ok := objectdefs.Global().GetByKey(logDefKey)
	if !ok {
		logger.Warn("tree chop: log def not found", zap.String("def_key", logDefKey))
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
		chunk := deps.Chunks.GetChunkFast(types.ChunkCoord{X: chunkX, Y: chunkY})
		if chunk == nil {
			continue
		}

		logID := deps.IDAllocator.GetFreeID()
		handle := gameworld.SpawnEntityFromDef(world, logDef, gameworld.DefSpawnParams{
			EntityID:         logID,
			X:                logX,
			Y:                logY,
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

func transformTargetToStump(
	world *ecs.World,
	targetID types.EntityID,
	targetHandle types.Handle,
	targetInfo components.EntityInfo,
	targetChunkRef components.ChunkRef,
	treeCfg *objectdefs.TreeBehaviorConfig,
	deps contracts.ExecutionDeps,
) {
	logger := resolveLogger(deps.Logger)
	stumpDef, ok := objectdefs.Global().GetByKey(treeCfg.TransformToDefKey)
	if !ok {
		logger.Warn("tree chop: stump def not found", zap.String("def_key", treeCfg.TransformToDefKey))
		return
	}

	previousTypeID := targetInfo.TypeID
	ecs.WithComponent(world, targetHandle, func(info *components.EntityInfo) {
		info.TypeID = uint32(stumpDef.DefID)
		info.Behaviors = stumpDef.CopyBehaviorOrder()
		info.IsStatic = stumpDef.IsStatic
	})
	ecs.WithComponent(world, targetHandle, func(appearance *components.Appearance) {
		resource := objectdefs.ResolveAppearanceResource(stumpDef, nil)
		if resource == "" {
			resource = stumpDef.Resource
		}
		appearance.Resource = resource
	})

	if stumpDef.Components != nil && stumpDef.Components.Collider != nil {
		collider := objectdefs.BuildColliderComponent(stumpDef.Components.Collider)
		if _, hasCollider := ecs.GetComponent[components.Collider](world, targetHandle); hasCollider {
			ecs.WithComponent(world, targetHandle, func(existing *components.Collider) {
				existing.HalfWidth = collider.HalfWidth
				existing.HalfHeight = collider.HalfHeight
				existing.Layer = collider.Layer
				existing.Mask = collider.Mask
			})
		} else {
			ecs.AddComponent(world, targetHandle, collider)
		}
	} else {
		ecs.RemoveComponent[components.Collider](world, targetHandle)
	}

	ecs.WithComponent(world, targetHandle, func(state *components.ObjectInternalState) {
		components.DeleteBehaviorState(state, treeBehaviorKey)
		state.Flags = nil
		state.IsDirty = true
	})
	ecs.CancelBehaviorTicksByEntityID(world, targetID)

	if deps.BehaviorRegistry != nil {
		currentInfo, hasInfo := ecs.GetComponent[components.EntityInfo](world, targetHandle)
		if hasInfo {
			if initErr := deps.BehaviorRegistry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
				World:        world,
				Handle:       targetHandle,
				EntityID:     targetID,
				EntityType:   currentInfo.TypeID,
				PreviousType: previousTypeID,
				Reason:       contracts.ObjectBehaviorInitReasonTransform,
			}, currentInfo.Behaviors); initErr != nil {
				logger.Error("tree chop: failed to init transformed object behaviors",
					zap.Uint64("target_id", uint64(targetID)),
					zap.Error(initErr),
				)
			}
		}
	}

	if deps.Chunks != nil {
		chunk := deps.Chunks.GetChunkFast(types.ChunkCoord{
			X: targetChunkRef.CurrentChunkX,
			Y: targetChunkRef.CurrentChunkY,
		})
		if chunk != nil {
			chunk.MarkRawDataDirty()
		}
	}

	if deps.EventBus != nil {
		deps.EventBus.PublishAsync(
			ecs.NewEntityAppearanceChangedEvent(targetInfo.Layer, targetID, targetHandle),
			eventbus.PriorityMedium,
		)
	}
	ecs.MarkObjectBehaviorDirty(world, targetHandle)
}

func resolveLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}
