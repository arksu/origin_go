package behaviors

import (
	"fmt"
	"strings"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const actionOpen = "open"

type containerBehavior struct{}

func (containerBehavior) Key() string { return "container" }

func (containerBehavior) ValidateAndApplyDefConfig(ctx *types.BehaviorDefConfigContext) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("container def config context is nil")
	}
	return parsePriorityOnlyConfig(ctx.RawConfig, "container")
}

func (containerBehavior) DeclaredActions() []types.BehaviorActionSpec {
	return []types.BehaviorActionSpec{
		{
			ActionID: actionOpen,
		},
	}
}

func (containerBehavior) ApplyRuntime(ctx *types.BehaviorRuntimeContext) types.BehaviorRuntimeResult {
	world, ok := ctx.World.(*ecs.World)
	if !ok || world == nil {
		return types.BehaviorRuntimeResult{}
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	rootHandle, found := refIndex.Lookup(constt.InventoryGrid, ctx.EntityID, 0)
	if !found || !world.Alive(rootHandle) {
		return types.BehaviorRuntimeResult{}
	}

	rootContainer, hasContainer := ecs.GetComponent[components.InventoryContainer](world, rootHandle)
	if !hasContainer || len(rootContainer.Items) == 0 {
		return types.BehaviorRuntimeResult{}
	}

	return types.BehaviorRuntimeResult{
		Flags: []string{"container.has_items"},
	}
}

func (containerBehavior) ProvideActions(ctx *types.BehaviorActionListContext) []types.ContextAction {
	world, ok := ctx.World.(*ecs.World)
	if !ok || world == nil {
		return nil
	}
	if !hasContainerRoot(world, ctx.TargetID) {
		return nil
	}
	return []types.ContextAction{
		{
			ActionID: actionOpen,
			Title:    "Open",
		},
	}
}

func (containerBehavior) ValidateAction(ctx *types.BehaviorActionValidateContext) types.BehaviorResult {
	if ctx.ActionID != actionOpen {
		return types.BehaviorResult{OK: false}
	}
	world, ok := ctx.World.(*ecs.World)
	if !ok || world == nil {
		return types.BehaviorResult{OK: false}
	}
	if !hasContainerRoot(world, ctx.TargetID) {
		return types.BehaviorResult{OK: false}
	}
	return types.BehaviorResult{OK: true}
}

func (containerBehavior) ExecuteAction(ctx *types.BehaviorActionExecuteContext) types.BehaviorResult {
	if ctx.ActionID != actionOpen {
		return types.BehaviorResult{OK: false}
	}

	world, ok := ctx.World.(*ecs.World)
	if !ok || world == nil {
		return types.BehaviorResult{OK: false}
	}

	openSvc := resolveOpenCoordinator(ctx.Extra)
	if openSvc == nil {
		return types.BehaviorResult{OK: false}
	}

	ref := &netproto.InventoryRef{
		Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
		OwnerId:      uint64(ctx.TargetID),
		InventoryKey: 0,
	}

	openErr := openSvc.HandleOpenRequest(world, ctx.PlayerID, ctx.PlayerHandle, ref)
	if openErr != nil {
		return types.BehaviorResult{
			OK:          false,
			UserVisible: true,
			ReasonCode:  reasonFromErrorCode(openErr.Code),
			Severity:    types.BehaviorAlertSeverityError,
		}
	}

	return types.BehaviorResult{OK: true}
}

func resolveOpenCoordinator(extra any) systems.OpenContainerCoordinator {
	deps := resolveActionExecutionDeps(extra)
	openSvc, _ := deps.OpenService.(systems.OpenContainerCoordinator)
	return openSvc
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
