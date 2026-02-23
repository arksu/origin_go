package behaviors

import (
	"fmt"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const buildActionOpen = "open"

type buildBehavior struct{}

func (buildBehavior) Key() string { return "build" }

func (buildBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("build def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "build")
}

func (buildBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if !isBuildTargetUsable(ctx) {
		return nil
	}
	return []contracts.ContextAction{{
		ActionID: buildActionOpen,
		Title:    "Open",
	}}
}

func (buildBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != buildActionOpen {
		return contracts.BehaviorResult{OK: false}
	}
	if isBuildTargetStatePresent(ctx.World, ctx.TargetHandle) {
		return contracts.BehaviorResult{OK: true}
	}
	return contracts.BehaviorResult{OK: false}
}

func (buildBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != buildActionOpen {
		return contracts.BehaviorResult{OK: false}
	}
	if !isBuildTargetStatePresent(ctx.World, ctx.TargetHandle) {
		return contracts.BehaviorResult{OK: false}
	}

	deps := resolveExecutionDeps(ctx.Deps)
	if deps.BuildState == nil {
		if deps.Alerts != nil {
			deps.Alerts.SendMiniAlert(ctx.PlayerID, &netproto.S2C_MiniAlert{
				Severity:   netproto.AlertSeverity_ALERT_SEVERITY_WARNING,
				ReasonCode: "BUILD_UI_UNAVAILABLE",
				TtlMs:      1500,
			})
		}
		return contracts.BehaviorResult{OK: true}
	}

	snapshot, ok := buildStateSnapshot(ctx.World, ctx.TargetID, ctx.TargetHandle)
	if !ok || snapshot == nil {
		if deps.Alerts != nil {
			deps.Alerts.SendMiniAlert(ctx.PlayerID, &netproto.S2C_MiniAlert{
				Severity:   netproto.AlertSeverity_ALERT_SEVERITY_WARNING,
				ReasonCode: "BUILD_UI_UNAVAILABLE",
				TtlMs:      1500,
			})
		}
		return contracts.BehaviorResult{OK: true}
	}
	deps.BuildState.SendBuildState(ctx.PlayerID, snapshot)
	return contracts.BehaviorResult{OK: true}
}

func buildStateSnapshot(world *ecs.World, targetID types.EntityID, targetHandle types.Handle) (*netproto.S2C_BuildState, bool) {
	if world == nil || targetID == 0 || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return nil, false
	}
	internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !hasInternalState {
		return nil, false
	}
	buildState, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, "build")
	if !ok || buildState == nil {
		return nil, false
	}

	rows := make([]*netproto.BuildStateItem, 0, len(buildState.Items))
	for i := range buildState.Items {
		slot := &buildState.Items[i]
		row := &netproto.BuildStateItem{
			Resource:      resolveBuildStateItemResource(slot.ItemKey),
			RequiredCount: slot.RequiredCount,
			PutCount:      slot.PutCount(),
			BuildCount:    slot.BuildCount,
		}
		if slot.ItemKey != "" {
			itemKey := slot.ItemKey
			row.ItemKey = &itemKey
		}
		if slot.ItemTag != "" {
			itemTag := slot.ItemTag
			row.ItemTag = &itemTag
		}
		rows = append(rows, row)
	}
	return &netproto.S2C_BuildState{
		EntityId: uint64(targetID),
		List:     rows,
	}, true
}

func resolveBuildStateItemResource(itemKey string) string {
	if itemKey == "" {
		return ""
	}
	reg := itemdefs.Global()
	if reg == nil {
		return ""
	}
	def, ok := reg.GetByKey(itemKey)
	if !ok || def == nil {
		return ""
	}
	return def.ResolveResource(false)
}

func isBuildTargetUsable(ctx *contracts.BehaviorActionListContext) bool {
	if ctx == nil || ctx.World == nil {
		return false
	}
	return isBuildTargetStatePresent(ctx.World, ctx.TargetHandle)
}

func isBuildTargetStatePresent(world *ecs.World, targetHandle types.Handle) bool {
	if world == nil || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return false
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](world, targetHandle)
	if !hasInfo || info.TypeID != constt.BuildObjectTypeID {
		return false
	}
	internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](world, targetHandle)
	if !hasInternalState {
		return false
	}
	_, ok := components.GetBehaviorState[components.BuildBehaviorState](internalState, "build")
	return ok
}
