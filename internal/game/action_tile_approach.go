package game

import (
	"context"
	"math"
	"time"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/types"
)

func tileCenter(x, y float64) (float64, float64, bool) {
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) ||
		x < math.MinInt32 || x > math.MaxInt32 || y < math.MinInt32 || y > math.MaxInt32 {
		return 0, 0, false
	}
	size := float64(constt.CoordPerTile)
	return math.Floor(x/size)*size + size/2, math.Floor(y/size)*size + size/2, true
}

func withinArrivalTolerance(x, y, targetX, targetY float64) bool {
	dx, dy := targetX-x, targetY-y
	return dx*dx+dy*dy <= constt.StopDistance*constt.StopDistance
}

func atTileCenter(world *ecs.World, player types.Handle, x, y float64) bool {
	position, exists := ecs.GetComponent[components.Transform](world, player)
	return exists && withinArrivalTolerance(position.X, position.Y, x, y)
}

func (service *ActionService) approachTileCenter(world *ecs.World, playerID types.EntityID, player types.Handle, definition *actiondefs.Definition, active components.ActiveGameAction, target ActionTarget) {
	active.Phase = components.GameActionApproaching
	ecs.AddComponent(world, player, active)
	position, hasPosition := ecs.GetComponent[components.Transform](world, player)
	movement, hasMovement := ecs.GetComponent[components.Movement](world, player)
	if !hasPosition || !hasMovement || movement.State == constt.StateStunned || movement.Speed <= 0 {
		service.Complete(world, playerID, player, active.Generation, false, "ACTION_INVALID_TARGET")
		return
	}
	systems.ClearPlayerInteractionIntents(world, player, playerID)
	if _, _, err := ecs.BreakLinkForPlayer(world, playerID, ecs.LinkBreakMoved); err != nil {
		service.Complete(world, playerID, player, active.Generation, false, "ACTION_FAILED")
		return
	}
	if withinArrivalTolerance(position.X, position.Y, target.X, target.Y) {
		movement.ClearTarget()
		ecs.AddComponent(world, player, movement)
		service.beginTileCycle(world, playerID, player, definition, active, target)
		return
	}
	movement.SetTargetPoint(int(target.X), int(target.Y))
	ecs.AddComponent(world, player, movement)
	// Crawl is the slowest normal mode. Allow a detour and a fixed stall margin.
	travelSeconds := 2 * math.Hypot(target.X-position.X, target.Y-position.Y) / (movement.Speed * 0.5)
	active.ExpireAtUnixMs = ecs.GetResource[ecs.TimeState](world).UnixMs + int64(math.Ceil(travelSeconds*1000)) + (15 * time.Second).Milliseconds()
	active.MovementOwned = true
	ecs.AddComponent(world, player, active)
	service.SendState(world, playerID, player)
}

func (service *ActionService) beginTileCycle(world *ecs.World, playerID types.EntityID, player types.Handle, definition *actiondefs.Definition, active components.ActiveGameAction, target ActionTarget) {
	if reason := service.UnavailableReason(world, playerID, player, definition); reason != "" {
		service.failRequirements(world, playerID, player, definition, active, reason)
		return
	}
	if reason := service.handlers[definition.ID].ValidateTarget(world, playerID, player, target); reason != "" {
		service.Complete(world, playerID, player, active.Generation, false, reason)
		return
	}
	active.Phase = components.GameActionExecuting
	active.MovementOwned, active.ExpireAtUnixMs = false, 0
	ecs.AddComponent(world, player, active)
	if definition.Execution.Ticks > 0 {
		service.SendState(world, playerID, player)
	}
	service.beginExecution(world, playerID, player, definition, active, target)
}

func (service *ActionService) onPointMovementStopped(_ context.Context, event eventbus.Event) error {
	stopped, ok := event.(*ecs.PointMovementStoppedEvent)
	if !ok || stopped.Layer != service.world.Layer {
		return nil
	}
	player := service.world.GetHandleByEntityID(stopped.EntityID)
	active, exists := ecs.GetComponent[components.ActiveGameAction](service.world, player)
	if !exists || active.Phase != components.GameActionApproaching || active.TargetX != stopped.TargetX || active.TargetY != stopped.TargetY {
		return nil
	}
	definition, found := service.definitions.Get(active.ActionID)
	if !found || definition.Target.Approach != actiondefs.ApproachTileCenter {
		return nil
	}
	if !withinArrivalTolerance(stopped.X, stopped.Y, active.TargetX, active.TargetY) {
		service.Complete(service.world, stopped.EntityID, player, active.Generation, false, "ACTION_INVALID_TARGET")
		return nil
	}
	service.beginTileCycle(service.world, stopped.EntityID, player, definition, active, ActionTarget{X: active.TargetX, Y: active.TargetY})
	return nil
}
