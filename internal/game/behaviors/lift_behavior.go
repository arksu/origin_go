package behaviors

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
)

const liftActionID = "lift"

type liftBehavior struct{}

func (liftBehavior) Key() string { return "lift" }

func (liftBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("lift def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "lift")
}

func (liftBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	// No-collider lift uses a special interact path; context actions remain collider-only.
	if _, hasCollider := ecs.GetComponent[components.Collider](ctx.World, ctx.TargetHandle); !hasCollider {
		return nil
	}
	if _, carried := ecs.GetComponent[components.LiftedObjectState](ctx.World, ctx.TargetHandle); carried {
		return nil
	}
	return []contracts.ContextAction{{
		ActionID: liftActionID,
		Title:    "Lift",
	}}
}

func (liftBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != liftActionID {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx.World == nil || ctx.TargetHandle == 0 || !ctx.World.Alive(ctx.TargetHandle) {
		return contracts.BehaviorResult{OK: false}
	}
	if _, hasCollider := ecs.GetComponent[components.Collider](ctx.World, ctx.TargetHandle); !hasCollider {
		return contracts.BehaviorResult{OK: false}
	}
	if _, carried := ecs.GetComponent[components.LiftedObjectState](ctx.World, ctx.TargetHandle); carried {
		return contracts.BehaviorResult{OK: false}
	}
	return contracts.BehaviorResult{OK: true}
}

func (liftBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != liftActionID {
		return contracts.BehaviorResult{OK: false}
	}
	deps := resolveExecutionDeps(ctx.Deps)
	if deps.LiftObject == nil {
		return contracts.BehaviorResult{
			OK:          false,
			UserVisible: true,
			ReasonCode:  "LIFT_UNAVAILABLE",
			Severity:    contracts.BehaviorAlertSeverityWarning,
		}
	}
	return deps.LiftObject(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetID, ctx.TargetHandle)
}

