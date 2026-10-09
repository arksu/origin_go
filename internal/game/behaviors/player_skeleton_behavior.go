package behaviors

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"
)

const (
	playerSkeletonBehaviorKey = "player_skeleton"
	actionTakeSkull           = "take_skull"
)

type playerSkeletonBehavior struct{}

func (playerSkeletonBehavior) Key() string { return playerSkeletonBehaviorKey }

func (playerSkeletonBehavior) RequiresItemMutation(string) bool { return true }

func (playerSkeletonBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("player_skeleton def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, playerSkeletonBehaviorKey)
}

func (playerSkeletonBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || !hasSkeletonSkull(ctx.World, ctx.TargetID, ctx.TargetHandle) {
		return nil
	}
	return []contracts.ContextAction{{ActionID: actionTakeSkull, Title: "Take Skull"}}
}

func (playerSkeletonBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != actionTakeSkull || !hasSkeletonSkull(ctx.World, ctx.TargetID, ctx.TargetHandle) {
		return contracts.BehaviorResult{}
	}
	return contracts.BehaviorResult{OK: true}
}

func (playerSkeletonBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != actionTakeSkull || ctx.World == nil || !ctx.World.Alive(ctx.PlayerHandle) {
		return contracts.BehaviorResult{}
	}
	if playerID, ok := ctx.World.GetExternalID(ctx.PlayerHandle); !ok || playerID != ctx.PlayerID {
		return contracts.BehaviorResult{}
	}
	if playerstate.ItemsLocked(ctx.World, ctx.PlayerHandle) {
		return contracts.BehaviorResult{UserVisible: true, ReasonCode: playerstate.ItemsLockedReason, Severity: contracts.BehaviorAlertSeverityWarning}
	}
	if !hasSkeletonSkull(ctx.World, ctx.TargetID, ctx.TargetHandle) {
		return contracts.BehaviorResult{}
	}
	deps := resolveExecutionDeps(ctx.Deps)
	if deps.TakeSkull == nil {
		return contracts.BehaviorResult{UserVisible: true, ReasonCode: "TAKE_SKULL_UNAVAILABLE", Severity: contracts.BehaviorAlertSeverityWarning}
	}
	return deps.TakeSkull(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ctx.TargetID, ctx.TargetHandle)
}

func hasSkeletonSkull(w *ecs.World, id types.EntityID, handle types.Handle) bool {
	if w == nil || id == 0 || !w.Alive(handle) || ecs.ObjectDestructionPending(w, handle) {
		return false
	}
	if actualID, ok := w.GetExternalID(handle); !ok || actualID != id {
		return false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](w, handle)
	registry := objectdefs.Global()
	if !ok || registry == nil {
		return false
	}
	definition, ok := registry.GetByID(int(info.TypeID))
	return ok && definition != nil && definition.Key == playerSkeletonBehaviorKey
}
