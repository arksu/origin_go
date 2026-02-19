package behaviors

import (
	"fmt"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"
)

const (
	takeBehaviorKey = "take"
)

type takeBehavior struct{}

func (takeBehavior) Key() string { return takeBehaviorKey }

func (takeBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("take def config context is nil")
	}

	var cfg contracts.TakeBehaviorConfig
	if err := decodeStrictJSON(ctx.RawConfig, &cfg); err != nil {
		return 0, fmt.Errorf("invalid take config: %w", err)
	}
	if cfg.Priority <= 0 {
		cfg.Priority = defaultBehaviorPriority
	}
	if len(cfg.Items) == 0 {
		return 0, fmt.Errorf("take.items is required")
	}

	seenIDs := make(map[string]int, len(cfg.Items))
	for index, item := range cfg.Items {
		if err := validateTakeBehaviorItemConfig(index, item); err != nil {
			return 0, err
		}
		itemID := strings.TrimSpace(item.ID)
		if first, exists := seenIDs[itemID]; exists {
			return 0, fmt.Errorf("take.items[%d].id duplicate %q (first at index %d)", index, itemID, first)
		}
		seenIDs[itemID] = index
	}

	if ctx.Def == nil {
		return 0, fmt.Errorf("take config target def is nil")
	}
	ctx.Def.SetTakeBehaviorConfig(cfg)
	return cfg.Priority, nil
}

func validateTakeBehaviorItemConfig(index int, item contracts.TakeConfig) error {
	itemID := strings.TrimSpace(item.ID)
	if itemID == "" {
		return fmt.Errorf("take.items[%d].id must not be empty", index)
	}
	if strings.TrimSpace(item.Name) == "" {
		return fmt.Errorf("take.items[%d].name must not be empty", index)
	}
	if item.Count <= 0 {
		return fmt.Errorf("take.items[%d].count must be > 0", index)
	}
	itemKey := strings.TrimSpace(item.ItemDefKey)
	if itemKey == "" {
		return fmt.Errorf("take.items[%d].itemDefKey must not be empty", index)
	}

	itemRegistry := itemdefs.Global()
	if itemRegistry == nil {
		return fmt.Errorf("take.items[%d].itemDefKey validation requires loaded item defs", index)
	}
	if _, ok := itemRegistry.GetByKey(itemKey); !ok {
		return fmt.Errorf("take.items[%d].itemDefKey unknown item key %q", index, itemKey)
	}
	return nil
}

func (takeBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	if ctx.TargetHandle == types.InvalidHandle || !ctx.World.Alive(ctx.TargetHandle) {
		return nil
	}

	takeCfg := resolveTargetTakeConfig(ctx.World, ctx.TargetHandle)
	if takeCfg == nil || len(takeCfg.Items) == 0 {
		return nil
	}

	taken := takeCountsFromState(ctx.World, ctx.TargetHandle)
	actions := make([]contracts.ContextAction, 0, len(takeCfg.Items))
	for _, item := range takeCfg.Items {
		itemID := strings.TrimSpace(item.ID)
		if itemID == "" {
			continue
		}
		if takenCountForAction(taken, itemID) >= item.Count {
			continue
		}
		actions = append(actions, contracts.ContextAction{
			ActionID: itemID,
			Title:    strings.TrimSpace(item.Name),
		})
	}
	return actions
}

func (takeBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
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

	takeCfg := resolveTargetTakeConfig(ctx.World, ctx.TargetHandle)
	if takeCfg == nil {
		return contracts.BehaviorResult{OK: false}
	}
	item := findTakeItemByActionID(takeCfg, actionID)
	if item == nil {
		return contracts.BehaviorResult{OK: false}
	}
	if takenCountForAction(takeCountsFromState(ctx.World, ctx.TargetHandle), actionID) >= item.Count {
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

func (takeBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
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

	takeCfg := resolveTargetTakeConfig(ctx.World, ctx.TargetHandle)
	if takeCfg == nil {
		return contracts.BehaviorResult{OK: false}
	}
	item := findTakeItemByActionID(takeCfg, actionID)
	if item == nil {
		return contracts.BehaviorResult{OK: false}
	}
	if takenCountForAction(takeCountsFromState(ctx.World, ctx.TargetHandle), actionID) >= item.Count {
		return contracts.BehaviorResult{OK: false}
	}

	nowTick := ecs.GetResource[ecs.TimeState](ctx.World).Tick
	ecs.AddComponent(ctx.World, ctx.PlayerHandle, components.ActiveCyclicAction{
		BehaviorKey:        takeBehaviorKey,
		ActionID:           actionID,
		TargetKind:         components.CyclicActionTargetObject,
		TargetID:           ctx.TargetID,
		TargetHandle:       ctx.TargetHandle,
		CycleDurationTicks: uint32(takeCycleDurationTicks),
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

func (takeBehavior) OnCycleComplete(ctx *contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	if ctx == nil || ctx.World == nil || ctx.TargetHandle == types.InvalidHandle || !ctx.World.Alive(ctx.TargetHandle) {
		return contracts.BehaviorCycleDecisionCanceled
	}

	deps := resolveExecutionDeps(ctx.Deps)
	if deps.GiveItem == nil {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "TAKE_UNAVAILABLE")
		return contracts.BehaviorCycleDecisionCanceled
	}

	actionID := strings.TrimSpace(ctx.ActionID)
	if actionID == "" {
		return contracts.BehaviorCycleDecisionCanceled
	}

	takeCfg := resolveTargetTakeConfig(ctx.World, ctx.TargetHandle)
	if takeCfg == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}
	item := findTakeItemByActionID(takeCfg, actionID)
	if item == nil {
		return contracts.BehaviorCycleDecisionCanceled
	}

	taken := takenCountForAction(takeCountsFromState(ctx.World, ctx.TargetHandle), actionID)
	if taken >= item.Count {
		return contracts.BehaviorCycleDecisionComplete
	}
	if !consumePlayerStaminaForTreeCycle(ctx.World, ctx.PlayerHandle, takeCycleStaminaCost) {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "LOW_STAMINA")
		return contracts.BehaviorCycleDecisionCanceled
	}

	itemKey := strings.TrimSpace(item.ItemDefKey)
	if itemKey == "" {
		return contracts.BehaviorCycleDecisionCanceled
	}

	outcome := deps.GiveItem(ctx.World, ctx.PlayerID, ctx.PlayerHandle, itemKey, 1, treeSpawnQuality)
	if !outcome.Success {
		sendWarningMiniAlert(ctx.PlayerID, deps.Alerts, "TAKE_GIVE_FAILED")
		return contracts.BehaviorCycleDecisionCanceled
	}

	newTaken := 0
	ecs.WithComponent(ctx.World, ctx.TargetHandle, func(state *components.ObjectInternalState) {
		takeState, hasTakeState := components.GetBehaviorState[components.TakeBehaviorState](*state, takeBehaviorKey)
		takenMap := map[string]int(nil)
		if hasTakeState && takeState != nil {
			takenMap = cloneTakenCounts(takeState.Taken)
		}
		takenMap, newTaken = incrementTakenCount(takenMap, actionID)
		components.SetBehaviorState(state, takeBehaviorKey, &components.TakeBehaviorState{
			Taken: takenMap,
		})
	})

	if outcome.PlacedInHand {
		return contracts.BehaviorCycleDecisionComplete
	}
	if newTaken >= item.Count {
		return contracts.BehaviorCycleDecisionComplete
	}
	return contracts.BehaviorCycleDecisionContinue
}

func resolveTargetTakeConfig(world *ecs.World, targetHandle types.Handle) *objectdefs.TakeBehaviorConfig {
	if world == nil || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return nil
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](world, targetHandle)
	if !hasInfo {
		return nil
	}
	def, found := objectdefs.Global().GetByID(int(info.TypeID))
	if !found || def.TakeConfig == nil {
		return nil
	}
	return def.TakeConfig
}

func findTakeItemByActionID(cfg *objectdefs.TakeBehaviorConfig, actionID string) *objectdefs.TakeConfig {
	if cfg == nil || actionID == "" {
		return nil
	}
	for index := range cfg.Items {
		if strings.TrimSpace(cfg.Items[index].ID) == actionID {
			return &cfg.Items[index]
		}
	}
	return nil
}

func takeCountsFromState(world *ecs.World, targetHandle types.Handle) map[string]int {
	if world == nil || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return nil
	}
	internalState, hasState := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !hasState {
		return nil
	}
	takeState, hasTakeState := components.GetBehaviorState[components.TakeBehaviorState](internalState, takeBehaviorKey)
	if !hasTakeState || takeState == nil {
		return nil
	}
	return takeState.Taken
}
