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

const actionChop = "chop"

type treeBehavior struct{}

func (treeBehavior) Key() string { return "tree" }

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

	// Keep existing behavior state untouched. Hook exists for explicit lifecycle wiring.
	return nil
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

	// Init chop points on first chop.
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](*state, "tree")
		if hasTree && treeState != nil && treeState.ChopPoints > 0 {
			return
		}
		components.SetBehaviorState(state, "tree", &components.TreeBehaviorState{
			ChopPoints: targetDef.TreeConfig.ChopPointsTotal,
		})
	})

	nowTick := ecs.GetResource[ecs.TimeState](ctx.World).Tick
	ecs.AddComponent(ctx.World, ctx.PlayerHandle, components.ActiveCyclicAction{
		BehaviorKey:        "tree",
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
		currentChopPoints := treeConfig.ChopPointsTotal
		if treeState, hasTree := components.GetBehaviorState[components.TreeBehaviorState](*state, "tree"); hasTree && treeState != nil {
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
		components.DeleteBehaviorState(state, "tree")
		state.Flags = nil
		state.IsDirty = true
	})

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
