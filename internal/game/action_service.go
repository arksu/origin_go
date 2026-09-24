package game

import (
	"context"
	"fmt"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

const gameActionCycleBehaviorKey = "game_action"

type ActionTarget struct {
	ObjectID     types.EntityID
	ObjectHandle types.Handle
	X            float64
	Y            float64
}

func (service *ActionService) SubscribeEvents(bus *eventbus.EventBus) {
	if service != nil && bus != nil {
		bus.SubscribeSync(ecs.TopicGameplayLinkCreated, eventbus.PriorityHigh, service.onLinkCreated)
	}
}

func (service *ActionService) onLinkCreated(_ context.Context, event eventbus.Event) error {
	linked, ok := event.(*ecs.LinkCreatedEvent)
	if !ok || linked.Layer != service.world.Layer {
		return nil
	}
	playerHandle := service.world.GetHandleByEntityID(linked.PlayerID)
	if !service.world.Alive(playerHandle) {
		return nil
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](service.world, playerHandle)
	if !exists || active.Phase != components.GameActionApproaching || active.TargetID != linked.TargetID {
		return nil
	}
	definition, exists := service.definitions.Get(active.ActionID)
	if !exists {
		service.Cancel(service.world, linked.PlayerID, playerHandle)
		return nil
	}
	targetHandle := service.world.GetHandleByEntityID(active.TargetID)
	if !service.world.Alive(targetHandle) {
		service.Complete(service.world, linked.PlayerID, playerHandle, active.Generation, false, "ACTION_INVALID_TARGET")
		return nil
	}
	target := ActionTarget{ObjectID: active.TargetID, ObjectHandle: targetHandle, X: active.TargetX, Y: active.TargetY}
	if reason := service.handlers[active.ActionID].ValidateTarget(service.world, linked.PlayerID, playerHandle, target); reason != "" {
		service.Complete(service.world, linked.PlayerID, playerHandle, active.Generation, false, reason)
		return nil
	}
	service.executeHandler(service.world, linked.PlayerID, playerHandle, definition, active, target)
	return nil
}

type ActionOutcome uint8

const (
	ActionRejected ActionOutcome = iota
	ActionSucceeded
	ActionFailed
	ActionApproaching
	ActionDeferred
)

type ActionResult struct {
	Outcome ActionOutcome
	Reason  string
}

type ActionHandler interface {
	UnavailableReason(world *ecs.World, playerID types.EntityID, playerHandle types.Handle) string
	ValidateTarget(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, target ActionTarget) string
	Start(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, target ActionTarget, generation uint64) ActionResult
	Cancel(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, active components.ActiveGameAction)
}

type actionSender interface {
	SendActionStateChanged(types.EntityID, *netproto.S2C_ActionStateChanged)
	SendActionList(types.EntityID, *netproto.S2C_ActionList)
	SendMiniAlert(types.EntityID, *netproto.S2C_MiniAlert)
	SendCyclicActionFinished(types.EntityID, *netproto.S2C_CyclicActionFinished)
}

type ActionService struct {
	world           *ecs.World
	definitions     *actiondefs.Registry
	handlers        map[string]ActionHandler
	sender          actionSender
	nextGeneration  uint64
	approachTimeout time.Duration
}

func NewActionService(world *ecs.World, definitions *actiondefs.Registry, handlers map[string]ActionHandler, sender actionSender) (*ActionService, error) {
	if world == nil || definitions == nil {
		return nil, fmt.Errorf("action service requires world and definitions")
	}
	handlerIDs := make([]string, 0, len(handlers))
	for id, handler := range handlers {
		if handler == nil {
			return nil, fmt.Errorf("action handler %q is nil", id)
		}
		handlerIDs = append(handlerIDs, id)
	}
	if err := definitions.ValidateHandlers(handlerIDs); err != nil {
		return nil, err
	}
	return &ActionService{world: world, definitions: definitions, handlers: handlers, sender: sender, approachTimeout: 15 * time.Second}, nil
}

func (service *ActionService) SetApproachTimeout(timeout time.Duration) {
	if timeout > 0 {
		service.approachTimeout = timeout
	}
}

func (service *ActionService) State(world *ecs.World, playerHandle types.Handle) *netproto.S2C_ActionStateChanged {
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists {
		return &netproto.S2C_ActionStateChanged{Phase: "idle"}
	}
	definition, found := service.definitions.Get(active.ActionID)
	if !found {
		return &netproto.S2C_ActionStateChanged{Phase: "idle"}
	}
	cursor := ""
	if active.Phase == components.GameActionSelecting {
		cursor = definition.Target.Cursor
	}
	return &netproto.S2C_ActionStateChanged{ActionId: active.ActionID, Phase: string(active.Phase), Cursor: cursor}
}

func (service *ActionService) SendState(world *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	if service != nil && service.sender != nil {
		service.sender.SendActionStateChanged(playerID, service.State(world, playerHandle))
	}
}

func (service *ActionService) SendList(playerID types.EntityID) {
	if service == nil || service.sender == nil {
		return
	}
	list := &netproto.S2C_ActionList{Actions: make([]*netproto.ActionDefinition, 0, len(service.definitions.All()))}
	for _, definition := range service.definitions.All() {
		requirements := make([]*netproto.ActionEquipmentRequirement, 0, len(definition.Requirements.Equipment))
		for _, requirement := range definition.Requirements.Equipment {
			requirements = append(requirements, &netproto.ActionEquipmentRequirement{
				Slots: append([]string(nil), requirement.Slots...), ItemKey: requirement.ItemKey, ItemTag: requirement.ItemTag,
			})
		}
		list.Actions = append(list.Actions, &netproto.ActionDefinition{
			Id: definition.ID, Label: definition.Presentation.Label, MenuIcon: definition.Presentation.MenuIcon,
			TargetKind: string(definition.Target.Kind), Cursor: definition.Target.Cursor,
			RequiredSkills: append([]string(nil), definition.Requirements.Skills...), RequiredEquipment: requirements,
			Ticks: uint32(definition.Execution.Ticks), Stamina: definition.Execution.Stamina,
			IsRepeatable: definition.Repeatable(),
		})
	}
	service.sender.SendActionList(playerID, list)
}

func (service *ActionService) Recheck(world *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	if service == nil || world != service.world || !world.Alive(playerHandle) {
		return
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists {
		return
	}
	definition, found := service.definitions.Get(active.ActionID)
	if active.Phase == components.GameActionApproaching && active.ExpireAtUnixMs > 0 && ecs.GetResource[ecs.TimeState](world).UnixMs >= active.ExpireAtUnixMs {
		service.Complete(world, playerID, playerHandle, active.Generation, false, "ACTION_TARGET_TIMEOUT")
	} else if !found || service.UnavailableReason(world, playerID, playerHandle, definition) != "" {
		service.Cancel(world, playerID, playerHandle)
	} else if active.TargetID != 0 && !world.Alive(world.GetHandleByEntityID(active.TargetID)) {
		service.Complete(world, playerID, playerHandle, active.Generation, false, "ACTION_INVALID_TARGET")
	}
}

func (service *ActionService) Activate(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, id string) {
	if service == nil || world != service.world || !world.Alive(playerHandle) {
		return
	}
	if active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle); exists {
		service.Cancel(world, playerID, playerHandle)
		if active.ActionID == id && active.Phase == components.GameActionSelecting {
			return
		}
	}
	definition, exists := service.definitions.Get(id)
	if !exists {
		service.alert(playerID, "ACTION_UNKNOWN")
		return
	}
	if reason := service.UnavailableReason(world, playerID, playerHandle, definition); reason != "" {
		service.alert(playerID, reason)
		return
	}
	service.nextGeneration++
	active := components.ActiveGameAction{ActionID: id, Generation: service.nextGeneration}
	if definition.Target.Kind == actiondefs.TargetNone {
		active.Phase = components.GameActionExecuting
		ecs.AddComponent(world, playerHandle, active)
		if definition.Execution.Ticks > 0 {
			service.SendState(world, playerID, playerHandle)
		}
		service.beginExecution(world, playerID, playerHandle, definition, active, ActionTarget{})
		return
	}
	active.Phase = components.GameActionSelecting
	ecs.AddComponent(world, playerHandle, active)
	service.SendState(world, playerID, playerHandle)
}

func (service *ActionService) HandleArmedClick(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, targetID types.EntityID, targetHandle types.Handle, x, y float64) bool {
	if service == nil || world != service.world {
		return false
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists || active.Phase != components.GameActionSelecting {
		return false
	}
	target := ActionTarget{ObjectID: targetID, ObjectHandle: targetHandle, X: x, Y: y}
	definition, exists := service.definitions.Get(active.ActionID)
	if !exists {
		service.Cancel(world, playerID, playerHandle)
		return false
	}
	if definition.Target.Kind == actiondefs.TargetObject && (target.ObjectID == 0 || !world.Alive(target.ObjectHandle)) {
		return false
	}
	if definition.Target.Kind != actiondefs.TargetObject && definition.Target.Kind != actiondefs.TargetTile {
		return false
	}
	if definition.Target.Kind == actiondefs.TargetTile {
		target.ObjectID = 0
		target.ObjectHandle = types.InvalidHandle
	}
	if reason := service.UnavailableReason(world, playerID, playerHandle, definition); reason != "" {
		service.Cancel(world, playerID, playerHandle)
		service.alert(playerID, reason)
		return true
	}
	if reason := service.handlers[definition.ID].ValidateTarget(world, playerID, playerHandle, target); reason != "" {
		service.alert(playerID, reason)
		return true
	}
	active.TargetID, active.TargetHandle, active.TargetX, active.TargetY = target.ObjectID, target.ObjectHandle, target.X, target.Y
	active.Phase = components.GameActionExecuting
	ecs.AddComponent(world, playerHandle, active)
	if definition.Execution.Ticks > 0 {
		service.SendState(world, playerID, playerHandle)
	}
	service.beginExecution(world, playerID, playerHandle, definition, active, target)
	return true
}

func (service *ActionService) beginExecution(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, definition *actiondefs.Definition, active components.ActiveGameAction, target ActionTarget) {
	if definition.Execution.Ticks > 0 {
		kind := components.CyclicActionTargetSelf
		if definition.Target.Kind == actiondefs.TargetObject {
			kind = components.CyclicActionTargetObject
		}
		ecs.AddComponent(world, playerHandle, components.ActiveCyclicAction{
			BehaviorKey: gameActionCycleBehaviorKey, ActionID: definition.ID,
			TargetKind: kind, TargetID: target.ObjectID, TargetHandle: target.ObjectHandle,
			CycleDurationTicks: uint32(definition.Execution.Ticks), StartedTick: ecs.GetResource[ecs.TimeState](world).Tick,
		})
		return
	}
	service.executeHandler(world, playerID, playerHandle, definition, active, target)
}

func (service *ActionService) executeHandler(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, definition *actiondefs.Definition, active components.ActiveGameAction, target ActionTarget) {
	if reason := service.UnavailableReason(world, playerID, playerHandle, definition); reason != "" {
		service.Cancel(world, playerID, playerHandle)
		service.alert(playerID, reason)
		return
	}
	result := service.handlers[definition.ID].Start(world, playerID, playerHandle, target, active.Generation)
	switch result.Outcome {
	case ActionSucceeded:
		service.Complete(world, playerID, playerHandle, active.Generation, true, "")
	case ActionRejected, ActionFailed:
		service.Complete(world, playerID, playerHandle, active.Generation, false, result.Reason)
	case ActionApproaching, ActionDeferred:
		active.Phase = components.GameActionExecuting
		if result.Outcome == ActionApproaching {
			active.Phase = components.GameActionApproaching
			active.MovementOwned = true
			active.ExpireAtUnixMs = ecs.GetResource[ecs.TimeState](world).UnixMs + service.approachTimeout.Milliseconds()
		}
		ecs.AddComponent(world, playerHandle, active)
		service.SendState(world, playerID, playerHandle)
	default:
		service.Complete(world, playerID, playerHandle, active.Generation, false, "ACTION_FAILED")
	}
}

func (service *ActionService) Complete(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, generation uint64, success bool, reason string) {
	if service == nil || world != service.world || !world.Alive(playerHandle) {
		return
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists || active.Generation != generation || active.Phase == components.GameActionSelecting {
		return
	}
	definition, found := service.definitions.Get(active.ActionID)
	if !found {
		service.Cancel(world, playerID, playerHandle)
		return
	}
	if success {
		if !service.chargeStamina(world, playerHandle, definition.Execution.Stamina) {
			service.Cancel(world, playerID, playerHandle)
			service.alert(playerID, "LOW_STAMINA")
			return
		}
	}
	if !success && reason != "" {
		service.alert(playerID, reason)
	}
	if !success {
		service.handlers[active.ActionID].Cancel(world, playerID, playerHandle, active)
	}
	service.stopOwnedMovement(world, playerID, playerHandle, active)
	service.clearCycle(world, playerID, playerHandle, success, reason)
	if definition.Target.Kind != actiondefs.TargetNone && (!success || definition.Repeatable()) && service.UnavailableReason(world, playerID, playerHandle, definition) == "" {
		active.Phase = components.GameActionSelecting
		active.TargetID, active.TargetHandle = 0, types.InvalidHandle
		active.TargetX, active.TargetY = 0, 0
		active.MovementOwned = false
		active.ExpireAtUnixMs = 0
		ecs.AddComponent(world, playerHandle, active)
		service.SendState(world, playerID, playerHandle)
		return
	}
	ecs.RemoveComponent[components.ActiveGameAction](world, playerHandle)
	service.SendState(world, playerID, playerHandle)
}

// CanCommit is called immediately before a deferred handler changes the world.
func (service *ActionService) CanCommit(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, generation uint64) bool {
	if service == nil || world != service.world || !world.Alive(playerHandle) {
		return false
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists || active.Generation != generation || active.Phase == components.GameActionSelecting {
		return false
	}
	definition, found := service.definitions.Get(active.ActionID)
	return found && service.UnavailableReason(world, playerID, playerHandle, definition) == ""
}

func (service *ActionService) Cancel(world *ecs.World, playerID types.EntityID, playerHandle types.Handle) {
	if service == nil || world != service.world || !world.Alive(playerHandle) {
		return
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists {
		return
	}
	ecs.RemoveComponent[components.ActiveGameAction](world, playerHandle)
	if handler := service.handlers[active.ActionID]; handler != nil {
		handler.Cancel(world, playerID, playerHandle, active)
	}
	service.stopOwnedMovement(world, playerID, playerHandle, active)
	service.clearCycle(world, playerID, playerHandle, false, "")
	service.SendState(world, playerID, playerHandle)
}

func (service *ActionService) stopOwnedMovement(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, active components.ActiveGameAction) {
	if !active.MovementOwned {
		return
	}
	linkState := ecs.GetResource[ecs.LinkState](world)
	if intent, exists := linkState.IntentByPlayer[playerID]; exists && intent.TargetID == active.TargetID {
		linkState.ClearIntent(playerID)
	}
	ecs.WithComponent(world, playerHandle, func(movement *components.Movement) {
		if movement.TargetHandle == active.TargetHandle || (active.TargetHandle == types.InvalidHandle && movement.TargetX == active.TargetX && movement.TargetY == active.TargetY) {
			movement.ClearTarget()
		}
	})
}

func (service *ActionService) clearCycle(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, success bool, reason string) {
	cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle)
	if !exists || cycle.BehaviorKey != gameActionCycleBehaviorKey {
		return
	}
	ecs.RemoveComponent[components.ActiveCyclicAction](world, playerHandle)
	if service.sender == nil {
		return
	}
	result := netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED
	if success {
		result = netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_COMPLETED
	}
	finished := &netproto.S2C_CyclicActionFinished{ActionId: cycle.ActionID, Result: result}
	if reason != "" && !success {
		finished.ReasonCode = &reason
	}
	service.sender.SendCyclicActionFinished(playerID, finished)
}

func (service *ActionService) alert(playerID types.EntityID, reason string) {
	if service.sender != nil && reason != "" {
		service.sender.SendMiniAlert(playerID, &netproto.S2C_MiniAlert{
			Severity: netproto.AlertSeverity_ALERT_SEVERITY_WARNING, ReasonCode: reason, TtlMs: 3000,
		})
	}
}
