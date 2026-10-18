package behaviors

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/hearth"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type hearthBehavior struct{}

func (hearthBehavior) Key() string { return "hearth" }

func (hearthBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("hearth def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "hearth")
}

func (hearthBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil {
		return nil
	}
	if _, _, ok := hearthOwnerDetails(ctx.World, ctx.TargetID, ctx.TargetHandle); !ok {
		return nil
	}
	return []contracts.ContextAction{{ActionID: actionOpen, Title: "Open"}}
}

func (hearthBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx == nil || ctx.ActionID != actionOpen {
		return contracts.BehaviorResult{}
	}
	_, _, ok := hearthOwnerDetails(ctx.World, ctx.TargetID, ctx.TargetHandle)
	return contracts.BehaviorResult{OK: ok}
}

func (b hearthBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	window := b.BuildCustomWindow(ctx)
	if window == nil || ctx.Deps == nil || ctx.Deps.CustomWindow == nil {
		return contracts.BehaviorResult{}
	}
	ctx.Deps.CustomWindow.SendCustomWindow(ctx.PlayerID, window)
	return contracts.BehaviorResult{OK: true}
}

func (hearthBehavior) BuildCustomWindow(ctx *contracts.BehaviorActionExecuteContext) *netproto.S2C_CustomWindow {
	if ctx == nil || ctx.ActionID != actionOpen || ctx.World == nil || !ctx.World.Alive(ctx.PlayerHandle) ||
		ctx.World.GetHandleByEntityID(ctx.PlayerID) != ctx.PlayerHandle {
		return nil
	}
	name, lastPingAt, ok := hearthOwnerDetails(ctx.World, ctx.TargetID, ctx.TargetHandle)
	if !ok {
		return nil
	}
	link, linked := ecs.GetResource[ecs.LinkState](ctx.World).GetLink(ctx.PlayerID)
	if !linked || link.PlayerHandle != ctx.PlayerHandle || link.TargetID != ctx.TargetID || link.TargetHandle != ctx.TargetHandle {
		return nil
	}
	return &netproto.S2C_CustomWindow{
		EntityId: uint64(ctx.TargetID), Title: "Hearth Fire",
		Fields: []*netproto.CustomWindowField{
			{Label: "Owner", Value: name},
			{Label: "Last seen at", Value: formatLastSeen(lastPingAt, ecs.GetResource[ecs.TimeState](ctx.World).UnixMs)},
		},
	}
}

func hearthOwnerDetails(w *ecs.World, id types.EntityID, target types.Handle) (string, int64, bool) {
	if w == nil || id == 0 || !w.Alive(target) || w.GetHandleByEntityID(id) != target || ecs.ObjectDestructionPending(w, target) {
		return "", 0, false
	}
	runtime, ok := ecs.TryGetResource[hearth.Runtime](w)
	if !ok {
		return "", 0, false
	}
	_, name, lastPingAt, owned := runtime.Owner(target)
	return name, lastPingAt, owned && name != ""
}

func formatLastSeen(lastPingAt, now int64) string {
	if lastPingAt <= 0 {
		return "Unknown"
	}
	if now <= lastPingAt || now-lastPingAt < 60_000 {
		return "just now"
	}
	minutes := (now - lastPingAt) / 60_000
	value, unit := minutes, "minute"
	if minutes >= 24*60 {
		value, unit = minutes/(24*60), "day"
	} else if minutes >= 60 {
		value, unit = minutes/60, "hour"
	}
	if value != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s ago", value, unit)
}

func (hearthBehavior) CompleteConstruction(ctx *contracts.ConstructionContext, complete func() bool) contracts.BehaviorResult {
	if ctx != nil && ctx.World != nil && ctx.World.Layer == hearth.SurfaceLayer {
		if runtime, ok := ecs.TryGetResource[hearth.Runtime](ctx.World); ok {
			return runtime.CompleteConstruction(ctx, complete)
		}
	}
	return contracts.BehaviorResult{UserVisible: true, ReasonCode: "Cannot complete this hearth here.", Severity: contracts.BehaviorAlertSeverityWarning}
}
