package game

import (
	"origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/types"
)

type liftActionHandler struct {
	lift     *LiftService
	commands *systems.NetworkCommandSystem
}

func (handler *liftActionHandler) UnavailableReason(world *ecs.World, _ types.EntityID, playerHandle types.Handle) string {
	if handler.lift.IsPlayerCarrying(world, playerHandle) {
		return "LIFT_ALREADY_CARRYING"
	}
	return ""
}

func (handler *liftActionHandler) ValidateTarget(world *ecs.World, _ types.EntityID, playerHandle types.Handle, target ActionTarget) string {
	if handler.lift.IsPlayerCarrying(world, playerHandle) {
		return "LIFT_ALREADY_CARRYING"
	}
	if target.ObjectID == 0 || !handler.lift.isLiftableTarget(world, target.ObjectHandle) {
		return "LIFT_INVALID_TARGET"
	}
	if _, carried := ecs.GetComponent[components.LiftedObjectState](world, target.ObjectHandle); carried {
		return "LIFT_TARGET_ALREADY_CARRIED"
	}
	if _, exists := ecs.GetComponent[components.Transform](world, target.ObjectHandle); !exists {
		return "LIFT_INVALID_TARGET"
	}
	return ""
}

func (handler *liftActionHandler) Start(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, target ActionTarget, generation uint64) ActionResult {
	if reason := handler.ValidateTarget(world, playerID, playerHandle, target); reason != "" {
		return ActionResult{Outcome: ActionRejected, Reason: reason}
	}
	if _, collider := ecs.GetComponent[components.Collider](world, target.ObjectHandle); !collider {
		return handler.lift.StartNoColliderLift(world, playerID, playerHandle, target, generation)
	}
	if link, linked := ecs.GetResource[ecs.LinkState](world).GetLink(playerID); linked && link.TargetID == target.ObjectID {
		return handler.lift.StartLift(world, playerID, playerHandle, target.ObjectID, target.ObjectHandle)
	}
	if !handler.commands.BeginActionMoveToLink(world, playerHandle, playerID, target.ObjectID, target.ObjectHandle) {
		return ActionResult{Outcome: ActionRejected, Reason: "LIFT_INVALID_TARGET"}
	}
	return ActionResult{Outcome: ActionApproaching}
}

func (handler *liftActionHandler) Cancel(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, _ components.ActiveGameAction) {
	handler.lift.CancelPendingLiftTransition(world, playerID, playerHandle)
}

type liftDownActionHandler struct {
	lift *LiftService
}

func (handler *liftDownActionHandler) UnavailableReason(world *ecs.World, _ types.EntityID, playerHandle types.Handle) string {
	if !handler.lift.IsPlayerCarrying(world, playerHandle) {
		return "LIFT_NOT_CARRYING"
	}
	return ""
}

func (handler *liftDownActionHandler) ValidateTarget(world *ecs.World, _ types.EntityID, playerHandle types.Handle, _ ActionTarget) string {
	if !handler.lift.IsPlayerCarrying(world, playerHandle) {
		return "LIFT_NOT_CARRYING"
	}
	return ""
}

func (handler *liftDownActionHandler) Start(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, target ActionTarget, generation uint64) ActionResult {
	return handler.lift.StartPutDownAt(world, playerID, playerHandle, target.X, target.Y, generation)
}

func (handler *liftDownActionHandler) Cancel(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, _ components.ActiveGameAction) {
	handler.lift.CancelPendingLiftTransition(world, playerID, playerHandle)
}

func (service *LiftService) StartNoColliderLift(world *ecs.World, playerID types.EntityID, playerHandle types.Handle, target ActionTarget, generation uint64) ActionResult {
	if world != service.world || !world.Alive(playerHandle) || !service.isLiftableTarget(world, target.ObjectHandle) {
		return ActionResult{Outcome: ActionRejected, Reason: "LIFT_INVALID_TARGET"}
	}
	if _, collider := ecs.GetComponent[components.Collider](world, target.ObjectHandle); collider {
		return ActionResult{Outcome: ActionRejected, Reason: "LIFT_INVALID_TARGET"}
	}
	playerPosition, hasPlayerPosition := ecs.GetComponent[components.Transform](world, playerHandle)
	targetPosition, hasTargetPosition := ecs.GetComponent[components.Transform](world, target.ObjectHandle)
	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	_, hasPlayerCollider := ecs.GetComponent[components.Collider](world, playerHandle)
	if !hasPlayerPosition || !hasTargetPosition || !hasMovement || !hasPlayerCollider || movement.State == _const.StateStunned {
		return ActionResult{Outcome: ActionRejected, Reason: "LIFT_INVALID_TARGET"}
	}
	service.clearPendingLiftTransitionState(world, playerID, playerHandle, false)
	service.clearPendingInteractionIntents(world, playerID, playerHandle)
	service.breakActiveLink(world, playerID)
	if isWithinLiftPickupStopDistance(playerPosition, targetPosition) {
		if service.startCarryingObject(world, playerID, playerHandle, target.ObjectID, target.ObjectHandle) {
			return ActionResult{Outcome: ActionSucceeded}
		}
		return ActionResult{Outcome: ActionFailed, Reason: "LIFT_INVALID_TARGET"}
	}
	ecs.WithComponent(world, playerHandle, func(collider *components.Collider) {
		collider.Phantom = &components.PhantomCollider{
			WorldX: targetPosition.X, WorldY: targetPosition.Y,
			HalfWidth: liftPointPhantomHalfLen, HalfHeight: liftPointPhantomHalfLen,
		}
	})
	ecs.WithComponent(world, playerHandle, func(movement *components.Movement) {
		movement.SetTargetHandle(target.ObjectHandle, int(targetPosition.X), int(targetPosition.Y))
	})
	expires := ecs.GetResource[ecs.TimeState](world).UnixMs + liftPendingTTL.Milliseconds()
	ecs.AddComponent(world, playerHandle, components.PendingLiftTransition{
		Mode:           components.LiftTransitionModePickupNoCollider,
		ObjectEntityID: target.ObjectID, ObjectHandle: target.ObjectHandle,
		TargetX: targetPosition.X, TargetY: targetPosition.Y,
		PhantomHalfW: liftPointPhantomHalfLen, PhantomHalfH: liftPointPhantomHalfLen,
		ExpireAtUnixMs: expires, ActionGeneration: generation,
	})
	return ActionResult{Outcome: ActionApproaching}
}
