package behaviors

import (
	"fmt"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const actionOpen = "open"

type containerBehavior struct{}

func (containerBehavior) Key() string { return "container" }

func (containerBehavior) ValidateAndApplyDefConfig(ctx *contracts.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("container def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "container")
}

func (containerBehavior) DeclaredActions() []contracts.BehaviorActionSpec {
	return []contracts.BehaviorActionSpec{
		{
			ActionID: actionOpen,
		},
	}
}

func (containerBehavior) ApplyRuntime(ctx *contracts.BehaviorRuntimeContext) contracts.BehaviorRuntimeResult {
	if ctx == nil || ctx.World == nil {
		return contracts.BehaviorRuntimeResult{}
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](ctx.World)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, ctx.EntityID, 0)
	if !found || !ctx.World.Alive(rootHandle) {
		return contracts.BehaviorRuntimeResult{}
	}

	rootContainer, hasContainer := ecs.GetComponent[components.InventoryContainer](ctx.World, rootHandle)
	if !hasContainer || len(rootContainer.Items) == 0 {
		return contracts.BehaviorRuntimeResult{}
	}

	return contracts.BehaviorRuntimeResult{
		Flags: []string{"container.has_items"},
	}
}

func (containerBehavior) ProvideActions(ctx *contracts.BehaviorActionListContext) []contracts.ContextAction {
	if ctx == nil || ctx.World == nil {
		return nil
	}
	if !hasContainerRoot(ctx.World, ctx.TargetID) {
		return nil
	}
	return []contracts.ContextAction{
		{
			ActionID: actionOpen,
			Title:    "Open",
		},
	}
}

func (containerBehavior) ValidateAction(ctx *contracts.BehaviorActionValidateContext) contracts.BehaviorResult {
	if ctx.ActionID != actionOpen {
		return contracts.BehaviorResult{OK: false}
	}
	if ctx == nil || ctx.World == nil {
		return contracts.BehaviorResult{OK: false}
	}
	if !hasContainerRoot(ctx.World, ctx.TargetID) {
		return contracts.BehaviorResult{OK: false}
	}
	return contracts.BehaviorResult{OK: true}
}

func (containerBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	if ctx.ActionID != actionOpen {
		return contracts.BehaviorResult{OK: false}
	}

	if ctx == nil || ctx.World == nil {
		return contracts.BehaviorResult{OK: false}
	}

	deps := resolveExecutionDeps(ctx.Deps)
	if deps.OpenContainer == nil {
		return contracts.BehaviorResult{OK: false}
	}

	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
		OwnerId:      uint64(ctx.TargetID),
		InventoryKey: 0,
	}

	openErr := deps.OpenContainer(ctx.World, ctx.PlayerID, ctx.PlayerHandle, ref)
	if openErr != nil {
		return contracts.BehaviorResult{
			OK:          false,
			UserVisible: true,
			ReasonCode:  reasonFromErrorCode(openErr.Code),
			Severity:    contracts.BehaviorAlertSeverityError,
		}
	}

	return contracts.BehaviorResult{OK: true}
}

func hasContainerRoot(world *ecs.World, targetID types.EntityID) bool {
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, targetID, 0)
	return found && world.Alive(rootHandle)
}

func reasonFromErrorCode(code netproto.ErrorCode) string {
	name := strings.TrimPrefix(code.String(), "ERROR_CODE_")
	if name == "" {
		return "internal_error"
	}
	return strings.ToLower(name)
}
