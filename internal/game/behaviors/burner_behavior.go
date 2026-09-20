package behaviors

import (
	"fmt"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

const (
	burnerLightAction      = "light_fire"
	burnerLightCycleTicks  = 10
	burnerLightStaminaCost = 50.0
)

type burnerBehavior struct{}

func (burnerBehavior) Key() string { return "burner" }

func (burnerBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	if burnerCanIgnite(ctx.World, ctx.TargetHandle) {
		return []contracts.ContextAction{{ActionID: burnerLightAction, Title: "Light my fire"}}
	}
	if burnerFuelValue(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetHandle) == 0 {
		return nil
	}
	return []contracts.ContextAction{{ActionID: "add_fuel", Title: "Add fuel"}}
}

func (burnerBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.World == nil || !ctx.World.Alive(ctx.PlayerHandle) {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.ActionID == burnerLightAction {
		if ctx.Phase == contracts.BehaviorValidationPhaseExecute {
			if _, active := ecs.GetComponent[components.ActiveCyclicAction](ctx.World, ctx.PlayerHandle); active {
				return contracts.BehaviorResult{OK: false}
			}
		}
		return contracts.BehaviorResult{OK: burnerCanIgnite(ctx.World, ctx.TargetHandle)}
	}
	return contracts.BehaviorResult{OK: ctx.ActionID == "add_fuel" &&
		!burnerCanIgnite(ctx.World, ctx.TargetHandle) &&
		burnerFuelValue(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetHandle) > 0}
}

func (b burnerBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || !b.ValidateAction(&contracts.BehaviorActionValidateContext{
		World: ctx.World, PlayerID: ctx.PlayerID, PlayerHandle: ctx.PlayerHandle,
		TargetHandle: ctx.TargetHandle, ActionID: ctx.ActionID, Phase: contracts.BehaviorValidationPhaseExecute,
	}).OK {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.ActionID == burnerLightAction {
		ecs.AddComponent(ctx.World, ctx.PlayerHandle, components.ActiveCyclicAction{
			BehaviorKey: b.Key(), ActionID: burnerLightAction,
			TargetKind: components.CyclicActionTargetObject, TargetID: ctx.TargetID, TargetHandle: ctx.TargetHandle,
			CycleDurationTicks: burnerLightCycleTicks, CycleIndex: 1,
			StartedTick: ecs.GetResource[ecs.TimeState](ctx.World).Tick,
		})
		ecs.MutateComponent[components.Movement](ctx.World, ctx.PlayerHandle, func(m *components.Movement) bool {
			m.State = constt.StateInteracting
			return true
		})
		return contracts.BehaviorResult{OK: true}
	}
	value := burnerFuelValue(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetHandle)
	if value == 0 {
		return contracts.BehaviorResult{OK: false}
	}
	info, ok := ecs.GetComponent[components.EntityInfo](ctx.World, ctx.TargetHandle)
	if !ok {
		return contracts.BehaviorResult{OK: false}
	}
	def, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || def.BurnerConfig == nil {
		return contracts.BehaviorResult{OK: false}
	}
	owner, ok := ecs.GetComponent[components.InventoryOwner](ctx.World, ctx.PlayerHandle)
	if !ok {
		return contracts.BehaviorResult{OK: false}
	}
	for _, link := range owner.Inventories {
		if link.Kind == constt.InventoryHand && link.OwnerID == ctx.PlayerID {
			ecs.MutateComponent[components.InventoryContainer](ctx.World, link.Handle, func(hand *components.InventoryContainer) bool {
				if len(hand.Items) != 1 {
					return false
				}
				hand.Items = nil
				hand.Version++
				return true
			})
			break
		}
	}
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		b, ok := components.GetBehaviorState[components.BurnerBehaviorState](*state, "burner")
		if !ok || b == nil {
			return
		}
		if value > def.BurnerConfig.FuelCapacity-b.Fuel {
			b.Fuel = def.BurnerConfig.FuelCapacity
		} else {
			b.Fuel += value
		}
		state.IsDirty = true
	})
	setBurnerStationState(ctx.World, ctx.TargetHandle, "burning", ctx.Deps)
	if ctx.Deps != nil && ctx.Deps.InventoryUpdate != nil {
		ctx.Deps.InventoryUpdate(ctx.World, ctx.PlayerID, ctx.PlayerHandle)
	}
	return contracts.BehaviorResult{OK: true}
}

func burnerCanIgnite(w *ecs.World, handle types.Handle) bool {
	if w == nil || !w.Alive(handle) {
		return false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, handle)
	if !ok {
		return false
	}
	def, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || def.BurnerConfig == nil {
		return false
	}
	station, ok := ecs.GetComponent[components.StationState](w, handle)
	if !ok || station.CurrentState != "unlit" {
		return false
	}
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !ok {
		return false
	}
	burner, ok := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	return ok && burner != nil && burner.Fuel > 0 && burner.NextFuelBurnAtTick == 0 && !burner.OutcomeCreated
}

func (burnerBehavior) OnCycleComplete(ctx *contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	if ctx == nil || ctx.ActionID != burnerLightAction || !burnerCanIgnite(ctx.World, ctx.TargetHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}
	if !consumePlayerActionStamina(ctx.World, ctx.PlayerHandle, burnerLightStaminaCost, false) {
		if ctx.Deps != nil {
			sendWarningMiniAlert(ctx.PlayerID, ctx.Deps.Alerts, "LOW_STAMINA")
		}
		return contracts.BehaviorCycleDecisionCanceled
	}
	info, _ := ecs.GetComponent[components.EntityInfo](ctx.World, ctx.TargetHandle)
	def, _ := objectdefs.Global().GetByID(int(info.TypeID))
	now := ecs.GetResource[ecs.TimeState](ctx.World).Tick
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](*state, "burner")
		burner.NextFuelBurnAtTick = now + uint64(def.BurnerConfig.TicksPerFuel)
		ecs.ScheduleBehaviorTick(ctx.World, ctx.TargetID, "burner", burner.NextFuelBurnAtTick)
		state.IsDirty = true
	})
	setBurnerStationState(ctx.World, ctx.TargetHandle, "burning", ctx.Deps)
	return contracts.BehaviorCycleDecisionComplete
}

func burnerAppearanceResource(w *ecs.World, handle types.Handle, def *objectdefs.ObjectDef) string {
	station, ok := ecs.GetComponent[components.StationState](w, handle)
	if !ok || def == nil || def.Key == "" || (station.CurrentState != "unlit" && station.CurrentState != "burning") {
		return ""
	}
	return def.Key + "/" + station.CurrentState
}

func (burnerBehavior) ApplyRuntime(ctx *contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	def, _ := objectdefs.Global().GetByID(int(ctx.EntityType))
	return contracts.BehaviorRuntimeResult{AppearanceResource: burnerAppearanceResource(ctx.World, ctx.Handle, def)}
}

func setBurnerStationState(w *ecs.World, handle types.Handle, nextState string, deps *contracts.ExecutionDeps) {
	changed := ecs.MutateComponent[components.StationState](w, handle, func(station *components.StationState) bool {
		if station.CurrentState == nextState {
			return false
		}
		station.CurrentState = nextState
		return true
	})
	ecs.MarkObjectBehaviorDirty(w, handle)
	info, _ := ecs.GetComponent[components.EntityInfo](w, handle)
	def, _ := objectdefs.Global().GetByID(int(info.TypeID))
	resource := burnerAppearanceResource(w, handle, def)
	appearanceChanged := false
	if resource != "" {
		appearanceChanged = ecs.MutateComponent[components.Appearance](w, handle, func(appearance *components.Appearance) bool {
			if appearance.Resource == resource {
				return false
			}
			appearance.Resource = resource
			return true
		})
	}
	if deps == nil || deps.EventBus == nil {
		return
	}
	entityID, _ := w.GetExternalID(handle)
	if appearanceChanged {
		deps.EventBus.PublishAsync(ecs.NewEntityAppearanceChangedEvent(w.Layer, entityID, handle), eventbus.PriorityMedium)
	}
	if changed {
		if err := deps.EventBus.PublishSync(ecs.NewStationStateChangedEvent(w.Layer, entityID, handle)); err != nil {
			resolveLogger(deps.Logger).Error("burner: failed to publish station state change", zap.Uint64("entity_id", uint64(entityID)), zap.Error(err))
		}
	}
}

func burnerFuelValue(w *ecs.World, playerID types.EntityID, playerHandle, targetHandle types.Handle) uint32 {
	if w == nil {
		return 0
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, targetHandle)
	if !ok {
		return 0
	}
	def, ok := objectdefs.Global().GetByID(int(info.TypeID))
	if !ok || def.BurnerConfig == nil {
		return 0
	}
	owner, ok := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	if !ok {
		return 0
	}
	var item components.InvItem
	found := false
	for _, link := range owner.Inventories {
		if link.Kind == constt.InventoryHand && link.OwnerID == playerID {
			h, ok := ecs.GetComponent[components.InventoryContainer](w, link.Handle)
			if ok && len(h.Items) == 1 {
				item = h.Items[0]
				found = true
			}
			break
		}
	}
	if !found {
		return 0
	}
	itemDef, ok := itemdefs.Global().GetByID(int(item.TypeID))
	if !ok {
		return 0
	}
	accepted := map[string]bool{}
	for _, key := range def.BurnerConfig.FuelAbilities {
		accepted[key] = true
	}
	var total uint32
	for key, value := range itemDef.Abilities {
		if accepted[key] {
			total += value
		}
	}
	return total
}

func (burnerBehavior) InitObject(ctx *contracts.BehaviorObjectInitContext) error {
	if ctx == nil || ctx.World == nil || !ctx.World.Alive(ctx.Handle) || (ctx.Reason != contracts.ObjectBehaviorInitReasonSpawn && ctx.Reason != contracts.ObjectBehaviorInitReasonRestore && ctx.Reason != contracts.ObjectBehaviorInitReasonTransform) {
		return nil
	}
	def, found := objectdefs.Global().GetByID(int(ctx.EntityType))
	if !found || def.BurnerConfig == nil {
		return nil
	}
	now := ecs.GetResource[ecs.TimeState](ctx.World).Tick
	var burner *components.BurnerBehaviorState
	ecs.WithComponent(ctx.World, ctx.Handle, func(state *components.ObjectInternalState) {
		if existing, ok := components.GetBehaviorState[components.BurnerBehaviorState](*state, "burner"); ok && existing != nil {
			burner = existing
			if ctx.Reason == contracts.ObjectBehaviorInitReasonRestore && consumeDueBurnerFuel(burner, now, def.BurnerConfig.TicksPerFuel) {
				state.IsDirty = true
			}
			return
		}
		deadline := now + uint64(def.BurnerConfig.TicksPerFuel)
		if station, ok := ecs.GetComponent[components.StationState](ctx.World, ctx.Handle); ok && station.CurrentState == "unlit" {
			deadline = 0
		}
		burner = &components.BurnerBehaviorState{Fuel: def.BurnerConfig.InitialFuel, NextFuelBurnAtTick: deadline}
		components.SetBehaviorState(state, "burner", burner)
		state.IsDirty = true
	})
	if burner != nil {
		if burner.NextFuelBurnAtTick != 0 && burner.Fuel == 0 {
			setBurnerStationState(ctx.World, ctx.Handle, "unlit", nil)
		}
		scheduleBurner(ctx.World, ctx.EntityID, burner, now)
	}
	resource := burnerAppearanceResource(ctx.World, ctx.Handle, def)
	if resource != "" {
		ecs.WithComponent(ctx.World, ctx.Handle, func(appearance *components.Appearance) { appearance.Resource = resource })
	}
	return nil
}

// Catch up arithmetically so a long unloaded period does not require a loop per fuel unit.
func consumeDueBurnerFuel(burner *components.BurnerBehaviorState, now uint64, ticksPerFuel uint32) bool {
	if burner.NextFuelBurnAtTick == 0 || burner.OutcomeCreated || burner.Fuel == 0 || ticksPerFuel == 0 || now < burner.NextFuelBurnAtTick {
		return false
	}
	due := 1 + (now-burner.NextFuelBurnAtTick)/uint64(ticksPerFuel)
	if due >= uint64(burner.Fuel) {
		burner.Fuel = 0
	} else {
		burner.Fuel -= uint32(due)
		burner.NextFuelBurnAtTick += due * uint64(ticksPerFuel)
	}
	return true
}

func scheduleBurner(w *ecs.World, entityID types.EntityID, burner *components.BurnerBehaviorState, now uint64) {
	if burner.NextFuelBurnAtTick == 0 || burner.OutcomeCreated {
		ecs.CancelBehaviorTick(w, entityID, "burner")
		return
	}
	due := burner.NextFuelBurnAtTick
	if burner.Fuel == 0 {
		// A failed exhaustion must remain retryable without replaying fuel consumption.
		due = now + 1
	}
	ecs.ScheduleBehaviorTick(w, entityID, "burner", due)
}

func (burnerBehavior) OnScheduledTick(ctx *contracts.BehaviorTickContext) (contracts.BehaviorTickResult, error) {
	if ctx == nil || ctx.World == nil || !ctx.World.Alive(ctx.Handle) {
		return contracts.BehaviorTickResult{}, nil
	}
	def, ok := objectdefs.Global().GetByID(int(ctx.EntityType))
	if !ok || def.BurnerConfig == nil {
		return contracts.BehaviorTickResult{}, nil
	}
	state, ok := ecs.GetComponent[components.ObjectInternalState](ctx.World, ctx.Handle)
	if !ok {
		return contracts.BehaviorTickResult{}, nil
	}
	burner, ok := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	if !ok || burner == nil || burner.NextFuelBurnAtTick == 0 || burner.OutcomeCreated {
		return contracts.BehaviorTickResult{}, nil
	}
	changed := consumeDueBurnerFuel(burner, ctx.CurrentTick, def.BurnerConfig.TicksPerFuel)
	if changed {
		ecs.WithComponent(ctx.World, ctx.Handle, func(state *components.ObjectInternalState) { state.IsDirty = true })
	}
	scheduleBurner(ctx.World, ctx.EntityID, burner, ctx.CurrentTick)
	result := contracts.BehaviorTickResult{StateChanged: changed}
	if burner.Fuel > 0 {
		return result, nil
	}
	setBurnerStationState(ctx.World, ctx.Handle, "unlit", ctx.Deps)
	if def.BurnerConfig.DropItem != "" {
		if ctx.Deps == nil || ctx.Deps.ExhaustBurner == nil {
			return result, fmt.Errorf("burner exhaustion dependency is unavailable")
		}
		if !ctx.Deps.ExhaustBurner(ctx.World, ctx.Handle) {
			return result, nil
		}
	}
	if ctx.World.Alive(ctx.Handle) {
		ecs.WithComponent(ctx.World, ctx.Handle, func(state *components.ObjectInternalState) {
			burner.OutcomeCreated = true
			state.IsDirty = true
		})
	}
	ecs.CancelBehaviorTick(ctx.World, ctx.EntityID, "burner")
	result.StateChanged = true
	return result, nil
}

func (burnerBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil || ctx.Def == nil {
		return 0, fmt.Errorf("burner config target def is nil")
	}
	var cfg contracts.BurnerBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid burner config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	if len(cfg.FuelAbilities) == 0 {
		return 0, fmt.Errorf("burner.fuelAbilities is required")
	}
	seen := map[string]struct{}{}
	for _, ability := range cfg.FuelAbilities {
		ability = strings.TrimSpace(ability)
		if ability == "" {
			return 0, fmt.Errorf("burner.fuelAbilities contains an empty ability")
		}
		if _, ok := seen[ability]; ok {
			return 0, fmt.Errorf("burner.fuelAbilities duplicate %q", ability)
		}
		seen[ability] = struct{}{}
	}
	if cfg.FuelCapacity == 0 {
		return 0, fmt.Errorf("burner.fuelCapacity must be > 0")
	}
	if cfg.TicksPerFuel == 0 {
		return 0, fmt.Errorf("burner.ticksPerFuel must be > 0")
	}
	if cfg.InitialFuel > cfg.FuelCapacity {
		return 0, fmt.Errorf("burner.initialFuel must not exceed fuelCapacity")
	}
	if strings.TrimSpace(cfg.OnExhausted.DropItem) == "" || !cfg.OnExhausted.Despawn {
		return 0, fmt.Errorf("burner.onExhausted must drop an item and despawn")
	}
	ctx.Def.SetBurnerBehaviorConfig(cfg)
	return cfg.Priority, nil
}
