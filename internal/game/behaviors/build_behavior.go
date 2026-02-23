package behaviors

import (
	"fmt"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
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
	if deps.Alerts != nil {
		deps.Alerts.SendMiniAlert(ctx.PlayerID, &netproto.S2C_MiniAlert{
			Severity:   netproto.AlertSeverity_ALERT_SEVERITY_INFO,
			ReasonCode: "BUILD_UI_COMING_SOON",
			TtlMs:      1500,
		})
	}
	return contracts.BehaviorResult{OK: true}
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
