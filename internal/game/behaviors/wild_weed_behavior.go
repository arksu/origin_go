package behaviors

import (
	"fmt"

	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"
)

const (
	wildWeedBehaviorKey         = "wild_weed"
	wildWeedItemKey             = "wild_windsown_weed"
	wildWeedActionID            = "pick"
	wildWeedMatureStage         = 3
	wildWeedGrowthSeconds int64 = contracts.WildWeedGrowthSeconds
)

type wildWeedBehavior struct{}

func (wildWeedBehavior) Key() string                      { return wildWeedBehaviorKey }
func (wildWeedBehavior) RequiresItemMutation(string) bool { return true }

func (wildWeedBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("wild_weed config target def is nil")
	}
	def, ok := ctx.Def.(*objectdefs.ObjectDef)
	if !ok || def == nil {
		return 0, fmt.Errorf("wild_weed requires an object definition")
	}
	var cfg contracts.WildWeedBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid wild_weed config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	items := itemdefs.Global()
	if items == nil {
		return 0, fmt.Errorf("wild_weed requires loaded item definitions")
	}
	if _, found := items.GetByKey(wildWeedItemKey); !found {
		return 0, fmt.Errorf("wild_weed item %q does not exist", wildWeedItemKey)
	}
	def.SetWildWeedBehaviorConfig(cfg)
	return cfg.Priority, nil
}

func (wildWeedBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil {
		return nil
	}
	def, valid := wildWeedDefinition(ctx.World, ctx.Handle, ctx.EntityID)
	if !valid || uint32(def.DefID) != ctx.EntityType {
		return nil
	}
	state := wildWeedState(ctx.World, ctx.Handle)
	if state == nil {
		if _, exists := ecs.GetComponent[components.ObjectInternalState](ctx.World, ctx.Handle); !exists {
			return fmt.Errorf("wild_weed object has no internal state")
		}
		state = &components.WildWeedBehaviorState{Stage: wildWeedMatureStage}
		setWildWeedState(ctx.World, ctx.Handle, state)
	}
	applyWildWeedCollider(ctx.World, ctx.Handle, def, state.Stage)
	// Restore schedules the persisted deadline; catch-up runs in the existing
	// runtime dispatcher after activation, without using wall time.
	scheduleWildWeed(ctx.World, ctx.Handle, ctx.EntityID, state)
	return nil
}

func (wildWeedBehavior) ApplyRuntime(ctx *contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	if ctx == nil || ctx.World == nil || !ctx.World.Alive(ctx.Handle) {
		return contracts.BehaviorRuntimeResult{}
	}
	state := wildWeedState(ctx.World, ctx.Handle)
	if state == nil {
		return contracts.BehaviorRuntimeResult{}
	}
	return contracts.BehaviorRuntimeResult{Flags: []string{fmt.Sprintf("wild_weed.stage%d", state.Stage)}}
}

func (wildWeedBehavior) OnScheduledRuntimeTick(ctx *contracts.BehaviorRuntimeTickContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	def, valid := wildWeedDefinition(ctx.World, ctx.Handle, ctx.EntityID)
	if !valid || uint32(def.DefID) != ctx.EntityType {
		return contracts.BehaviorTickResult{}, nil
	}
	state := wildWeedState(ctx.World, ctx.Handle)
	if state == nil {
		return contracts.BehaviorTickResult{}, fmt.Errorf("wild_weed object has no behavior state")
	}
	next := *state
	// There are only three transitions, so unloaded-time catch-up is bounded.
	for next.Stage < wildWeedMatureStage && next.NextGrowthRuntimeSeconds > 0 && ctx.CurrentRuntimeSeconds >= next.NextGrowthRuntimeSeconds {
		next.Stage++
		if next.Stage == wildWeedMatureStage {
			next.NextGrowthRuntimeSeconds = 0
		} else {
			next.NextGrowthRuntimeSeconds += wildWeedGrowthSeconds
		}
	}
	changed := next != *state
	if changed {
		setWildWeedState(ctx.World, ctx.Handle, &next)
		applyWildWeedCollider(ctx.World, ctx.Handle, def, next.Stage)
	}
	scheduleWildWeed(ctx.World, ctx.Handle, ctx.EntityID, &next)
	return contracts.BehaviorTickResult{StateChanged: changed}, nil
}

func (wildWeedBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || !wildWeedMature(ctx.World, ctx.TargetHandle, ctx.TargetID) {
		return nil
	}
	return []contracts.ContextAction{{ActionID: wildWeedActionID, Title: "Pick"}}
}

func (wildWeedBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != wildWeedActionID || !wildWeedActor(ctx.World, ctx.PlayerHandle, ctx.PlayerID) || !wildWeedMature(ctx.World, ctx.TargetHandle, ctx.TargetID) {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.Phase == contracts.BehaviorValidationPhaseExecute {
		if _, exists := ecs.GetComponent[components.ActiveCyclicAction](ctx.World, ctx.PlayerHandle); exists {
			return contracts.BehaviorResult{OK: false, UserVisible: true, ReasonCode: "action_already_active", Severity: contracts.BehaviorAlertSeverityWarning}
		}
	}
	return contracts.BehaviorResult{OK: true}
}

func (behavior wildWeedBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil {
		return contracts.BehaviorResult{OK: false}
	}
	result := behavior.ValidateAction(&contracts.BehaviorActionValidateContext{
		World: ctx.World, PlayerID: ctx.PlayerID, PlayerHandle: ctx.PlayerHandle,
		TargetID: ctx.TargetID, TargetHandle: ctx.TargetHandle, ActionID: ctx.ActionID,
		Phase: contracts.BehaviorValidationPhaseExecute,
	})
	if !result.OK {
		return result
	}
	cyclicaction.StartContext(ctx.World, ctx.PlayerHandle, components.ActiveCyclicAction{
		MutatesItems: true, BehaviorKey: wildWeedBehaviorKey, ActionID: wildWeedActionID,
		TargetKind: components.CyclicActionTargetObject, TargetID: ctx.TargetID, TargetHandle: ctx.TargetHandle,
		CycleDurationTicks: uint32(takeCycleDurationTicks), CycleIndex: 1,
		StartedTick: ecs.GetResource[ecs.TimeState](ctx.World).Tick,
	})
	ecs.MutateComponent[components.Movement](ctx.World, ctx.PlayerHandle, func(movement *components.Movement) bool {
		movement.State = constt.StateInteracting
		return true
	})
	return contracts.BehaviorResult{OK: true}
}

func (wildWeedBehavior) OnCycleComplete(ctx *contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	if ctx == nil || ctx.ActionID != wildWeedActionID || !wildWeedActor(ctx.World, ctx.PlayerHandle, ctx.PlayerID) || !wildWeedMature(ctx.World, ctx.TargetHandle, ctx.TargetID) {
		return contracts.BehaviorCycleDecisionCanceled
	}
	active, exists := ecs.GetComponent[components.ActiveCyclicAction](ctx.World, ctx.PlayerHandle)
	if !exists || active != ctx.Action || active.BehaviorKey != wildWeedBehaviorKey || active.ActionID != wildWeedActionID || active.TargetID != ctx.TargetID || active.TargetHandle != ctx.TargetHandle || active.CycleDurationTicks == 0 || active.CycleElapsedTicks < active.CycleDurationTicks {
		return contracts.BehaviorCycleDecisionCanceled
	}
	deps := resolveExecutionDeps(ctx.Deps)
	if deps.GiveItem == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if !canPayWildWeedStamina(ctx.World, ctx.PlayerHandle) {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "LOW_STAMINA")
		return contracts.BehaviorCycleDecisionCanceled
	}
	quality, found := resolveTargetObjectQuality(ctx.World, ctx.TargetHandle)
	if !found {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if outcome := deps.GiveItem(ctx.World, ctx.PlayerID, ctx.PlayerHandle, wildWeedItemKey, 1, quality); !outcome.Success {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "TAKE_GIVE_FAILED")
		return contracts.BehaviorCycleDecisionCanceled
	}
	// The precheck and payment execute under the same shard lock. Giving an
	// inventory item does not change stamina, so payment has no remaining admission.
	ConsumePlayerLongActionStamina(ctx.World, ctx.PlayerHandle, takeCycleStaminaCost)
	next := *wildWeedState(ctx.World, ctx.TargetHandle)
	next.Stage = 0
	next.NextGrowthRuntimeSeconds = ecs.GetResource[ecs.TimeState](ctx.World).RuntimeSecondsTotal + wildWeedGrowthSeconds
	setWildWeedState(ctx.World, ctx.TargetHandle, &next)
	def, _ := wildWeedDefinition(ctx.World, ctx.TargetHandle, ctx.TargetID)
	applyWildWeedCollider(ctx.World, ctx.TargetHandle, def, next.Stage)
	scheduleWildWeed(ctx.World, ctx.TargetHandle, ctx.TargetID, &next)
	ecs.MarkObjectBehaviorDirty(ctx.World, ctx.TargetHandle)
	return contracts.BehaviorCycleDecisionComplete
}

func wildWeedDefinition(w *ecs.World, handle types.Handle, entityID types.EntityID) (*objectdefs.ObjectDef, bool) {
	if w == nil || entityID == 0 || !w.Alive(handle) || ecs.ObjectDestructionPending(w, handle) {
		return nil, false
	}
	actualID, exists := w.GetExternalID(handle)
	if !exists || actualID != entityID {
		return nil, false
	}
	info, exists := ecs.GetComponent[components.EntityInfo](w, handle)
	registry := objectdefs.Global()
	if !exists || registry == nil {
		return nil, false
	}
	def, found := registry.GetByID(int(info.TypeID))
	return def, found && def.WildWeedConfig != nil && def.HasBehavior(wildWeedBehaviorKey)
}

func wildWeedState(w *ecs.World, handle types.Handle) *components.WildWeedBehaviorState {
	state, exists := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !exists {
		return nil
	}
	weed, _ := components.GetBehaviorState[components.WildWeedBehaviorState](state, wildWeedBehaviorKey)
	return weed
}

func wildWeedMature(w *ecs.World, handle types.Handle, entityID types.EntityID) bool {
	if _, valid := wildWeedDefinition(w, handle, entityID); !valid {
		return false
	}
	state := wildWeedState(w, handle)
	return state != nil && state.Stage == wildWeedMatureStage
}

func wildWeedActor(w *ecs.World, handle types.Handle, entityID types.EntityID) bool {
	if w == nil || entityID == 0 || !w.Alive(handle) {
		return false
	}
	actualID, found := w.GetExternalID(handle)
	return found && actualID == entityID && !playerstate.ItemsLocked(w, handle)
}

func canPayWildWeedStamina(w *ecs.World, handle types.Handle) bool {
	stats, found := ecs.GetComponent[components.EntityStats](w, handle)
	if !found {
		return false
	}
	con := characterattrs.DefaultValue
	if profile, found := ecs.GetComponent[components.CharacterProfile](w, handle); found {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	maxStamina := entitystats.MaxStaminaFromCon(con)
	return entitystats.CanConsumeLongActionStamina(entitystats.ClampStamina(stats.Stamina, maxStamina), maxStamina, takeCycleStaminaCost)
}

func setWildWeedState(w *ecs.World, handle types.Handle, state *components.WildWeedBehaviorState) {
	ecs.WithComponent(w, handle, func(internal *components.ObjectInternalState) {
		components.SetBehaviorState(internal, wildWeedBehaviorKey, state)
	})
}

func applyWildWeedCollider(w *ecs.World, handle types.Handle, def *objectdefs.ObjectDef, stage int) {
	if def == nil || def.Components == nil || def.Components.Collider == nil {
		return
	}
	layer, mask := def.Components.Collider.Layer, def.Components.Collider.Mask
	if stage < wildWeedMatureStage {
		layer, mask = 0, 0
	}
	ecs.MutateComponent[components.Collider](w, handle, func(collider *components.Collider) bool {
		if collider.Layer == layer && collider.Mask == mask {
			return false
		}
		collider.Layer, collider.Mask = layer, mask
		return true
	})
}

func scheduleWildWeed(w *ecs.World, handle types.Handle, entityID types.EntityID, state *components.WildWeedBehaviorState) {
	if state.Stage >= wildWeedMatureStage || state.NextGrowthRuntimeSeconds <= 0 {
		ecs.CancelBehaviorRuntime(w, handle, wildWeedBehaviorKey)
		return
	}
	ecs.ScheduleBehaviorRuntime(w, handle, entityID, wildWeedBehaviorKey, state.NextGrowthRuntimeSeconds)
}
