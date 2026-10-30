package behaviors

import (
	"fmt"
	"math"
	"strings"

	constt "origin/internal/const"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"
)

const (
	cropBehaviorKey     = "crop"
	cropHarvestActionID = "harvest"
)

type cropBehavior struct{}

func (cropBehavior) Key() string                      { return cropBehaviorKey }
func (cropBehavior) RequiresItemMutation(string) bool { return true }

func (cropBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("crop config target def is nil")
	}
	def, ok := ctx.Def.(*objectdefs.ObjectDef)
	if !ok || def == nil {
		return 0, fmt.Errorf("crop requires an object definition")
	}
	var cfg contracts.CropBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid crop config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	if cfg.PlantingSuccessChance == nil {
		chance := 1.0
		cfg.PlantingSuccessChance = &chance
	}
	if chance := *cfg.PlantingSuccessChance; math.IsNaN(chance) || math.IsInf(chance, 0) || chance < 0 || chance > 1 {
		return 0, fmt.Errorf("crop plantingSuccessChance must be between 0 and 1")
	}
	if cfg.HarvestCycleDurationTicks == 0 {
		cfg.HarvestCycleDurationTicks = 10
	}
	if cfg.HarvestStaminaCost == 0 {
		cfg.HarvestStaminaCost = 10
	}
	if math.IsNaN(cfg.HarvestStaminaCost) || math.IsInf(cfg.HarvestStaminaCost, 0) || cfg.HarvestStaminaCost < 0 {
		return 0, fmt.Errorf("crop harvestStaminaCost must be finite and positive")
	}
	items := itemdefs.Global()
	if items == nil {
		return 0, fmt.Errorf("crop requires loaded item definitions")
	}
	cfg.SeedItemKey = strings.TrimSpace(cfg.SeedItemKey)
	if err := validateCropItem(items, cfg.SeedItemKey); err != nil {
		return 0, fmt.Errorf("crop seed: %w", err)
	}
	if len(cfg.Stages) != contracts.CropStageCount {
		return 0, fmt.Errorf("crop requires exactly %d stages", contracts.CropStageCount)
	}
	var totalDuration int64
	for i := range cfg.Stages {
		stage := &cfg.Stages[i]
		if i == contracts.CropStageCount-1 {
			if stage.DurationSeconds != 0 || len(stage.Harvest) == 0 {
				return 0, fmt.Errorf("crop final stage requires harvest and zero durationSeconds")
			}
		} else if stage.DurationSeconds <= 0 || stage.DurationSeconds > math.MaxInt64-totalDuration {
			return 0, fmt.Errorf("crop stage %d durationSeconds must be positive without total duration overflow", i)
		}
		totalDuration += stage.DurationSeconds
		if i < 2 && len(stage.Harvest) != 0 {
			return 0, fmt.Errorf("crop stage %d cannot be harvested", i)
		}
		totalItems := 0
		seen := make(map[string]struct{}, len(stage.Harvest))
		for j := range stage.Harvest {
			product := &stage.Harvest[j]
			product.ItemKey = strings.TrimSpace(product.ItemKey)
			if err := validateCropItem(items, product.ItemKey); err != nil {
				return 0, fmt.Errorf("crop stage %d harvest: %w", i, err)
			}
			if product.Min < 1 || product.Max < product.Min || product.Max > contracts.CropMaxHarvestItems-totalItems {
				return 0, fmt.Errorf("crop stage %d harvest counts must be positive, ordered and total at most %d", i, contracts.CropMaxHarvestItems)
			}
			if _, exists := seen[product.ItemKey]; exists {
				return 0, fmt.Errorf("crop stage %d repeats harvest item %q", i, product.ItemKey)
			}
			seen[product.ItemKey] = struct{}{}
			totalItems += product.Max
		}
	}
	if def.Components == nil || def.Components.Collider == nil || def.Components.Collider.W != 4 || def.Components.Collider.H != 4 {
		return 0, fmt.Errorf("crop requires a 4x4 collider")
	}
	def.SetCropBehaviorConfig(cfg)
	return cfg.Priority, nil
}

func validateCropItem(items *itemdefs.Registry, key string) error {
	item, found := items.GetByKey(key)
	if key == "" || !found {
		return fmt.Errorf("item %q does not exist", key)
	}
	if item.Size.W != 1 || item.Size.H != 1 || (item.Stack != nil && item.Stack.Mode != itemdefs.StackModeNone) || item.Container != nil {
		return fmt.Errorf("item %q must be a nonstackable 1x1 item without a container", key)
	}
	return nil
}

func (cropBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil {
		return nil
	}
	def, valid := CropDefinition(ctx.World, ctx.Handle, ctx.EntityID)
	if !valid || uint32(def.DefID) != ctx.EntityType {
		return nil
	}
	return InitializeCropState(ctx.World, ctx.Handle, ctx.EntityID)
}

// InitializeCropState preserves valid persisted state, and starts absent state
// at stage zero using the durable server-runtime clock. Caller owns the shard lock.
func InitializeCropState(w *ecs.World, handle types.Handle, entityID types.EntityID) error {
	def, valid := CropDefinition(w, handle, entityID)
	if !valid {
		return fmt.Errorf("invalid crop object")
	}
	state := CropState(w, handle)
	if state == nil {
		if _, exists := ecs.GetComponent[components.ObjectInternalState](w, handle); !exists {
			return fmt.Errorf("crop object has no internal state")
		}
		now := ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal
		duration := def.CropConfig.Stages[0].DurationSeconds
		if now < 0 || duration <= 0 || duration > math.MaxInt64-now {
			return fmt.Errorf("crop initial growth deadline overflows runtime")
		}
		state = &components.CropBehaviorState{NextGrowthRuntimeSeconds: now + duration}
		setCropState(w, handle, state)
	} else if err := state.Validate(); err != nil {
		return err
	}
	// The collider remains present for ordinary object damage at every stage.
	ecs.MutateComponent(w, handle, func(collider *components.Collider) bool {
		if collider.Layer == 0 && collider.Mask == 0 {
			return false
		}
		collider.Layer, collider.Mask = 0, 0
		return true
	})
	// Activation recomputes appearance immediately after init. Reconcile here
	// too, so scheduler backlog cannot publish a stale restored growth stage.
	_, err := AdvanceCropGrowth(w, handle, entityID, ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal)
	return err
}

func (cropBehavior) ApplyRuntime(ctx *contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	if ctx == nil || ctx.World == nil || !ctx.World.Alive(ctx.Handle) {
		return contracts.BehaviorRuntimeResult{}
	}
	state := CropState(ctx.World, ctx.Handle)
	if state == nil {
		return contracts.BehaviorRuntimeResult{}
	}
	return contracts.BehaviorRuntimeResult{Flags: []string{fmt.Sprintf("crop.stage%d", state.Stage)}}
}

func (cropBehavior) OnScheduledRuntimeTick(ctx *contracts.BehaviorRuntimeTickContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	def, valid := CropDefinition(ctx.World, ctx.Handle, ctx.EntityID)
	if !valid || uint32(def.DefID) != ctx.EntityType {
		return contracts.BehaviorTickResult{}, nil
	}
	changed, err := AdvanceCropGrowth(ctx.World, ctx.Handle, ctx.EntityID, ctx.CurrentRuntimeSeconds)
	return contracts.BehaviorTickResult{StateChanged: changed}, err
}

// AdvanceCropGrowth reconciles at most three transitions, including when the
// scheduler is backlogged. Deadlines advance from the previous deadline, never
// from now. Caller owns the shard lock and must not retain CropState pointers.
func AdvanceCropGrowth(w *ecs.World, handle types.Handle, entityID types.EntityID, now int64) (bool, error) {
	def, valid := CropDefinition(w, handle, entityID)
	if !valid {
		return false, fmt.Errorf("invalid crop object")
	}
	state := CropState(w, handle)
	if state == nil {
		return false, fmt.Errorf("crop object has no behavior state")
	}
	if err := state.Validate(); err != nil {
		return false, err
	}
	next := *state
	for steps := 0; steps < contracts.CropStageCount-1 && next.Stage < contracts.CropStageCount-1 && now >= next.NextGrowthRuntimeSeconds; steps++ {
		next.Stage++
		if next.Stage == contracts.CropStageCount-1 {
			next.NextGrowthRuntimeSeconds = 0
		} else {
			duration := def.CropConfig.Stages[next.Stage].DurationSeconds
			if duration <= 0 || duration > math.MaxInt64-next.NextGrowthRuntimeSeconds {
				return false, fmt.Errorf("crop growth deadline overflows runtime")
			}
			next.NextGrowthRuntimeSeconds += duration
		}
	}
	changed := next != *state
	if changed {
		setCropState(w, handle, &next)
		ecs.MarkObjectBehaviorDirty(w, handle)
	}
	scheduleCropGrowth(w, handle, entityID, &next)
	return changed, nil
}

func (cropBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || !cropHarvestable(ctx.World, ctx.TargetHandle, ctx.TargetID) {
		return nil
	}
	return []contracts.ContextAction{{ActionID: cropHarvestActionID, Title: "Harvest"}}
}

func (cropBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != cropHarvestActionID || !cropActor(ctx.World, ctx.PlayerHandle, ctx.PlayerID) || !cropHarvestable(ctx.World, ctx.TargetHandle, ctx.TargetID) {
		return contracts.BehaviorResult{}
	}
	if ctx.Phase == contracts.BehaviorValidationPhaseExecute {
		if !cropLinked(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetID, ctx.TargetHandle) {
			return contracts.BehaviorResult{}
		}
		if _, exists := ecs.GetComponent[components.ActiveCyclicAction](ctx.World, ctx.PlayerHandle); exists {
			return contracts.BehaviorResult{UserVisible: true, ReasonCode: "action_already_active", Severity: contracts.BehaviorAlertSeverityWarning}
		}
	}
	return contracts.BehaviorResult{OK: true}
}

func (behavior cropBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil {
		return contracts.BehaviorResult{}
	}
	result := behavior.ValidateAction(&contracts.BehaviorActionValidateContext{
		World: ctx.World, PlayerID: ctx.PlayerID, PlayerHandle: ctx.PlayerHandle,
		TargetID: ctx.TargetID, TargetHandle: ctx.TargetHandle, ActionID: ctx.ActionID,
		Phase: contracts.BehaviorValidationPhaseExecute,
	})
	if !result.OK {
		return result
	}
	if ctx.Deps == nil || ctx.Deps.CropHarvest == nil {
		return contracts.BehaviorResult{UserVisible: true, ReasonCode: "CROP_HARVEST_UNAVAILABLE", Severity: contracts.BehaviorAlertSeverityWarning}
	}
	def, _ := CropDefinition(ctx.World, ctx.TargetHandle, ctx.TargetID)
	cyclicaction.StartContext(ctx.World, ctx.PlayerHandle, components.ActiveCyclicAction{
		MutatesItems: true, BehaviorKey: cropBehaviorKey, ActionID: cropHarvestActionID,
		TargetKind: components.CyclicActionTargetObject, TargetID: ctx.TargetID, TargetHandle: ctx.TargetHandle,
		CycleDurationTicks: def.CropConfig.HarvestCycleDurationTicks, CycleIndex: 1,
		StartedTick: ecs.GetResource[ecs.TimeState](ctx.World).Tick,
	})
	ecs.MutateComponent(ctx.World, ctx.PlayerHandle, func(movement *components.Movement) bool {
		movement.State = constt.StateInteracting
		return true
	})
	return contracts.BehaviorResult{OK: true}
}

func (cropBehavior) OnCycleComplete(ctx *contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	if ctx == nil || ctx.ActionID != cropHarvestActionID || !cropActor(ctx.World, ctx.PlayerHandle, ctx.PlayerID) || !cropLinked(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetID, ctx.TargetHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}
	active, exists := ecs.GetComponent[components.ActiveCyclicAction](ctx.World, ctx.PlayerHandle)
	if !exists || active != ctx.Action || active.BehaviorKey != cropBehaviorKey || active.ActionID != cropHarvestActionID || active.TargetKind != components.CyclicActionTargetObject || active.TargetID != ctx.TargetID || active.TargetHandle != ctx.TargetHandle || active.CycleDurationTicks == 0 || active.CycleElapsedTicks < active.CycleDurationTicks || !active.MutatesItems {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if !cropHarvestable(ctx.World, ctx.TargetHandle, ctx.TargetID) {
		return contracts.BehaviorCycleDecisionCanceled
	}
	deps := resolveExecutionDeps(ctx.Deps)
	if deps.CropHarvest == nil {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "CROP_HARVEST_UNAVAILABLE")
		return contracts.BehaviorCycleDecisionCanceled
	}
	result := deps.CropHarvest(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetID, ctx.TargetHandle)
	if !result.OK {
		if result.UserVisible {
			sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, result.ReasonCode)
		}
		return contracts.BehaviorCycleDecisionCanceled
	}
	return contracts.BehaviorCycleDecisionComplete
}

// CropDefinition resolves a live crop, including exact persistent identity.
func CropDefinition(w *ecs.World, handle types.Handle, entityID types.EntityID) (*objectdefs.ObjectDef, bool) {
	if w == nil || entityID == 0 || !w.Alive(handle) || ecs.ObjectDestructionPending(w, handle) || w.GetHandleByEntityID(entityID) != handle {
		return nil, false
	}
	info, exists := ecs.GetComponent[components.EntityInfo](w, handle)
	registry := objectdefs.Global()
	if !exists || registry == nil {
		return nil, false
	}
	def, found := registry.GetByID(int(info.TypeID))
	return def, found && def.CropConfig != nil && len(def.CropConfig.Stages) == contracts.CropStageCount && def.HasBehavior(cropBehaviorKey)
}

// CropState is borrowed only for the caller's synchronous shard-locked work.
func CropState(w *ecs.World, handle types.Handle) *components.CropBehaviorState {
	if w == nil || !w.Alive(handle) {
		return nil
	}
	state, exists := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !exists {
		return nil
	}
	crop, _ := components.GetBehaviorState[components.CropBehaviorState](state, cropBehaviorKey)
	return crop
}

func cropHarvestable(w *ecs.World, handle types.Handle, entityID types.EntityID) bool {
	if w == nil {
		return false
	}
	if _, err := AdvanceCropGrowth(w, handle, entityID, ecs.GetResource[ecs.TimeState](w).RuntimeSecondsTotal); err != nil {
		return false
	}
	def, valid := CropDefinition(w, handle, entityID)
	state := CropState(w, handle)
	return valid && state != nil && len(def.CropConfig.Stages[state.Stage].Harvest) > 0
}

func cropLinked(w *ecs.World, playerID types.EntityID, playerHandle types.Handle, targetID types.EntityID, targetHandle types.Handle) bool {
	links, exists := ecs.TryGetResource[ecs.LinkState](w)
	if !exists {
		return false
	}
	link, linked := links.GetLink(playerID)
	return linked && link.PlayerHandle == playerHandle && link.TargetID == targetID && link.TargetHandle == targetHandle
}

func cropActor(w *ecs.World, handle types.Handle, entityID types.EntityID) bool {
	return w != nil && entityID != 0 && w.Alive(handle) && w.GetHandleByEntityID(entityID) == handle && !playerstate.ItemsLocked(w, handle)
}

func setCropState(w *ecs.World, handle types.Handle, state *components.CropBehaviorState) {
	ecs.WithComponent(w, handle, func(internal *components.ObjectInternalState) {
		components.SetBehaviorState(internal, cropBehaviorKey, state)
	})
}

func scheduleCropGrowth(w *ecs.World, handle types.Handle, entityID types.EntityID, state *components.CropBehaviorState) {
	if state.Stage == contracts.CropStageCount-1 {
		ecs.CancelBehaviorRuntime(w, handle, cropBehaviorKey)
		return
	}
	ecs.ScheduleBehaviorRuntime(w, handle, entityID, cropBehaviorKey, state.NextGrowthRuntimeSeconds)
}
