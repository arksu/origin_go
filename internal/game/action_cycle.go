package game

import (
	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func (service *ActionService) AdvanceCycle(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, cycle components.ActiveCyclicAction, progress cyclicActionProgressSender) {
	if service == nil || world != service.world || cycle.BehaviorKey != gameActionCycleBehaviorKey {
		return
	}
	current, hasCycle := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle)
	if !hasCycle || current != cycle {
		return
	}
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, playerHandle)
	if !exists || active.ActionID != cycle.ActionID || active.Generation != cycle.ActionGeneration || (active.Phase != components.GameActionExecuting && !cycle.ActionCompletionStarted) {
		service.clearCycle(world, playerID, playerHandle, false, "ACTION_CANCELED")
		return
	}
	definition, found := service.definitions.Get(active.ActionID)
	if !found || cycle.CycleDurationTicks == 0 {
		service.Cancel(world, playerID, playerHandle)
		return
	}
	if cycle.ActionCompletionStarted {
		return
	}
	if reason := service.UnavailableReason(world, playerID, playerHandle, definition); reason != "" {
		service.failRequirements(world, playerID, playerHandle, definition, active, reason)
		return
	}
	if definition.Target.Kind == actiondefs.TargetObject && !world.Alive(active.TargetHandle) {
		service.Complete(world, playerID, playerHandle, active.Generation, false, "ACTION_INVALID_TARGET")
		return
	}
	cycle.CycleElapsedTicks++
	if cycle.CycleElapsedTicks > cycle.CycleDurationTicks {
		cycle.CycleElapsedTicks = cycle.CycleDurationTicks
	}
	target := actionTarget(active)
	cueReady := actionSoundCueDue(cycle)
	if cueReady {
		if reason := service.validateCycleTarget(world, playerID, playerHandle, definition, target); reason != "" {
			service.Complete(world, playerID, playerHandle, active.Generation, false, reason)
			return
		}
		advanceActionSoundCues(world, playerHandle, &cycle, service.soundEvents)
	}
	if progress != nil {
		progress.SendCyclicActionProgress(playerID, &netproto.S2C_CyclicActionProgress{
			ActionId: cycle.ActionID, TargetEntityId: uint64(cycle.TargetID),
			CycleIndex: cycle.CycleIndex, ElapsedTicks: cycle.CycleElapsedTicks, TotalTicks: cycle.CycleDurationTicks,
		})
	}
	if cycle.CycleElapsedTicks < cycle.CycleDurationTicks {
		ecs.AddComponent(world, playerHandle, cycle)
		return
	}
	cycle.ActionCompletionStarted = true
	ecs.AddComponent(world, playerHandle, cycle)
	if !cueReady {
		if reason := service.validateCycleTarget(world, playerID, playerHandle, definition, target); reason != "" {
			service.Complete(world, playerID, playerHandle, active.Generation, false, reason)
			return
		}
	}
	service.executeHandler(world, playerID, playerHandle, definition, active, target)
}

func (service *ActionService) validateCycleTarget(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, definition *actiondefs.Definition, target ActionTarget) string {
	if definition.Target.Kind == actiondefs.TargetDirection && !validActionAim(target.AimAngle) {
		return "ACTION_INVALID_TARGET"
	}
	if definition.Target.Approach == actiondefs.ApproachTileCenter && !atTileCenter(world, playerHandle, target.X, target.Y) {
		return "ACTION_INVALID_TARGET"
	}
	return service.handlers[definition.ID].ValidateTarget(world, playerID, playerHandle, target)
}
