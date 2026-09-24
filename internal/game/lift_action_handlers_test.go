package game

import (
	"context"
	"testing"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"go.uber.org/zap"
)

func newNoColliderLiftActionTest(t *testing.T) (*ecs.World, types.Handle, types.Handle, *LiftService, *ActionService, *testActionSender) {
	t.Helper()
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{})
		ecs.AddComponent(w, h, components.Collider{})
	})
	target := world.Spawn(3, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 1000, Y: 1000})
		ecs.AddComponent(w, h, components.EntityInfo{Behaviors: []string{"lift"}})
	})
	lift := NewLiftService(world, nil, nil, nil, zap.NewNop())
	lift.relocateObject = func(w *ecs.World, h types.Handle, _ gameworld.RelocateWorldObjectImmediateOptions, x, y float64) bool {
		ecs.WithComponent(w, h, func(transform *components.Transform) { transform.X, transform.Y = x, y })
		return true
	}
	registry := actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "lift", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetObject, Cursor: "lift"}},
		{ID: "lift_down", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile, Cursor: "lift_down"}},
	})
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{
		"lift": &liftActionHandler{lift: lift}, "lift_down": &liftDownActionHandler{lift: lift},
	}, sender)
	if err != nil {
		t.Fatal(err)
	}
	lift.SetActionCompletion(service.Complete)
	lift.SetActionCanCommit(service.CanCommit)
	return world, player, target, lift, service, sender
}

func TestNoColliderLiftRequiresArmingAndCancelClearsApproach(t *testing.T) {
	world, player, target, _, actions, _ := newNoColliderLiftActionTest(t)
	if _, pending := ecs.GetComponent[components.PendingLiftTransition](world, player); pending {
		t.Fatal("unarmed object created lift transition")
	}
	actions.Activate(world, 1, player, "lift")
	actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
	active, activeOK := ecs.GetComponent[components.ActiveGameAction](world, player)
	pending, pendingOK := ecs.GetComponent[components.PendingLiftTransition](world, player)
	if !activeOK || !pendingOK || active.Phase != components.GameActionApproaching || pending.ActionGeneration != active.Generation {
		t.Fatalf("armed lift did not own no-collider approach: active=%#v pending=%#v", active, pending)
	}
	actions.Cancel(world, 1, player)
	if _, stillPending := ecs.GetComponent[components.PendingLiftTransition](world, player); stillPending {
		t.Fatal("cancel left pending lift")
	}
	movement, _ := ecs.GetComponent[components.Movement](world, player)
	collider, _ := ecs.GetComponent[components.Collider](world, player)
	if movement.TargetHandle == target || collider.Phantom != nil || actions.State(world, player).Phase != "idle" {
		t.Fatal("cancel left movement, phantom, or action state")
	}
}

func TestLiftIsAbsentFromContextMenu(t *testing.T) {
	world, player, target, _, _, _ := newNoColliderLiftActionTest(t)
	contextActions := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), zap.NewNop())
	for _, action := range contextActions.ComputeActions(world, 1, player, 3, target) {
		if action.ActionID == "lift" {
			t.Fatal("lift remained in object context actions")
		}
	}
}

func TestNoColliderLiftTimeoutReturnsToSelection(t *testing.T) {
	world, player, target, lift, actions, sender := newNoColliderLiftActionTest(t)
	actions.Activate(world, 1, player, "lift")
	actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
	pending, _ := ecs.GetComponent[components.PendingLiftTransition](world, player)
	ecs.GetResource[ecs.TimeState](world).UnixMs = pending.ExpireAtUnixMs
	lift.CancelPendingLiftTransition(world, 1, player)
	if state := actions.State(world, player); state.Phase != "selecting" || state.Cursor != "lift" {
		t.Fatalf("timeout did not preserve retry: %#v", state)
	}
	if len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "LIFT_PICKUP_TIMEOUT" {
		t.Fatalf("missing timeout alert: %#v", sender.alerts)
	}
	lift.FinalizePendingLiftTransition(world, 1, player, pending)
	if actions.State(world, player).Phase != "selecting" || len(sender.alerts) != 1 {
		t.Fatal("late finalization changed action state")
	}
}

func completeNoColliderLift(t *testing.T, world *ecs.World, player, target types.Handle, lift *LiftService, actions *ActionService) {
	t.Helper()
	actions.Activate(world, 1, player, "lift")
	actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
	pending, exists := ecs.GetComponent[components.PendingLiftTransition](world, player)
	if !exists {
		t.Fatal("lift approach did not create pending transition")
	}
	ecs.WithComponent(world, player, func(transform *components.Transform) { transform.X, transform.Y = 1000, 1000 })
	lift.FinalizePendingLiftTransition(world, 1, player, pending)
	if _, carrying := ecs.GetComponent[components.LiftCarryState](world, player); !carrying || actions.State(world, player).Phase != "idle" {
		t.Fatal("lift did not finish after reaching no-collider object")
	}
}

func TestNoColliderLiftCompletesAndLiftDownPlacementLifecycle(t *testing.T) {
	world, player, target, lift, actions, sender := newNoColliderLiftActionTest(t)
	completeNoColliderLift(t, world, player, target, lift, actions)
	actions.Recheck(world, 1, player)
	if len(sender.lists) != 0 {
		t.Fatal("carry state refreshed action catalog")
	}
	actions.Activate(world, 1, player, "lift")
	if actions.State(world, player).Phase != "idle" || len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "LIFT_ALREADY_CARRYING" {
		t.Fatal("carrying player could activate lift or missed rejection alert")
	}
	actions.Activate(world, 1, player, "lift_down")
	actions.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 1010, 1010)
	if actions.State(world, player).Phase != "selecting" {
		t.Fatal("rejected placement did not return to target selection")
	}
	if _, carrying := ecs.GetComponent[components.LiftCarryState](world, player); !carrying {
		t.Fatal("rejected placement dropped object")
	}
	if sender.alerts[len(sender.alerts)-1].ReasonCode != "LIFT_PUTDOWN_INVALID" {
		t.Fatal("rejected placement did not alert")
	}

	lift.chunkActive = func(types.ChunkCoord) bool { return true }
	actions.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 1010, 1010)
	pending, exists := ecs.GetComponent[components.PendingLiftTransition](world, player)
	if !exists || pending.Mode != components.LiftTransitionModePutDown {
		t.Fatal("valid placement did not begin deferred transition")
	}
	lift.FinalizePendingLiftTransition(world, 1, player, pending)
	if _, carrying := ecs.GetComponent[components.LiftCarryState](world, player); carrying || actions.State(world, player).Phase != "idle" {
		t.Fatal("successful placement did not end carry and action")
	}
	position, _ := ecs.GetComponent[components.Transform](world, target)
	if position.X != 1010 || position.Y != 1010 {
		t.Fatalf("placement used wrong target: %#v", position)
	}
}

func TestLiftDownTimeoutAllowsRetryWhileCarrying(t *testing.T) {
	world, player, target, lift, actions, sender := newNoColliderLiftActionTest(t)
	completeNoColliderLift(t, world, player, target, lift, actions)
	lift.chunkActive = func(types.ChunkCoord) bool { return true }
	actions.Activate(world, 1, player, "lift_down")
	actions.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 1010, 1010)
	pending, _ := ecs.GetComponent[components.PendingLiftTransition](world, player)
	ecs.GetResource[ecs.TimeState](world).UnixMs = pending.ExpireAtUnixMs
	lift.CancelPendingLiftTransition(world, 1, player)
	if actions.State(world, player).Phase != "selecting" || sender.alerts[len(sender.alerts)-1].ReasonCode != "LIFT_PUTDOWN_TIMEOUT" {
		t.Fatal("placement timeout did not allow retry")
	}
	if _, carrying := ecs.GetComponent[components.LiftCarryState](world, player); !carrying {
		t.Fatal("timeout dropped object")
	}
}

func TestLiftDownForcedCarryLossCancelsPendingPlacement(t *testing.T) {
	world, player, target, lift, actions, _ := newNoColliderLiftActionTest(t)
	completeNoColliderLift(t, world, player, target, lift, actions)
	lift.chunkActive = func(types.ChunkCoord) bool { return true }
	actions.Activate(world, 1, player, "lift_down")
	actions.HandleArmedClick(world, 1, player, 0, types.InvalidHandle, 1010, 1010)
	pending, exists := ecs.GetComponent[components.PendingLiftTransition](world, player)
	if !exists {
		t.Fatal("placement precondition failed")
	}
	ecs.RemoveComponent[components.LiftCarryState](world, player)
	actions.Recheck(world, 1, player)
	if actions.State(world, player).Phase != "idle" {
		t.Fatal("carry loss left action active")
	}
	if _, stillPending := ecs.GetComponent[components.PendingLiftTransition](world, player); stillPending {
		t.Fatal("carry loss left pending placement")
	}
	lift.FinalizePendingLiftTransition(world, 1, player, pending)
	if actions.State(world, player).Phase != "idle" {
		t.Fatal("late placement changed state")
	}
}

func TestColliderLiftApproachesAndCompletesAfterLink(t *testing.T) {
	world, player, target, lift, actions, _ := newNoColliderLiftActionTest(t)
	ecs.AddComponent(world, target, components.Collider{HalfWidth: 5, HalfHeight: 5})
	commands := systems.NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	actions.handlers["lift"] = &liftActionHandler{lift: lift, commands: commands}
	actions.Activate(world, 1, player, "lift")
	actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
	if state := actions.State(world, player); state.Phase != "approaching" {
		t.Fatalf("collider lift did not approach: %#v", state)
	}
	if _, hasContext := ecs.GetComponent[components.PendingContextAction](world, player); hasContext {
		t.Fatal("collider lift used context-action pending state")
	}
	ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{PlayerID: 1, TargetID: 3})
	if err := actions.onLinkCreated(context.Background(), ecs.NewLinkCreatedEvent(world.Layer, 1, 3)); err != nil {
		t.Fatal(err)
	}
	if _, carrying := ecs.GetComponent[components.LiftCarryState](world, player); !carrying || actions.State(world, player).Phase != "idle" {
		t.Fatal("collider lift did not complete on LinkCreated")
	}
}

func TestColliderLiftRejectsNonLiftableAndTargetLoss(t *testing.T) {
	world, player, target, lift, actions, sender := newNoColliderLiftActionTest(t)
	ecs.AddComponent(world, target, components.Collider{HalfWidth: 5, HalfHeight: 5})
	commands := systems.NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	actions.handlers["lift"] = &liftActionHandler{lift: lift, commands: commands}
	ecs.WithComponent(world, target, func(info *components.EntityInfo) { info.Behaviors = nil })
	actions.Activate(world, 1, player, "lift")
	actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
	if actions.State(world, player).Phase != "selecting" || sender.alerts[len(sender.alerts)-1].ReasonCode != "LIFT_INVALID_TARGET" {
		t.Fatal("non-liftable collider was accepted")
	}
	ecs.WithComponent(world, target, func(info *components.EntityInfo) { info.Behaviors = []string{"lift"} })
	actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
	world.Despawn(target)
	actions.Recheck(world, 1, player)
	if actions.State(world, player).Phase != "selecting" {
		t.Fatal("target loss did not allow new target selection")
	}
}

func TestColliderLiftCancelStopsApproach(t *testing.T) {
	world, player, target, lift, actions, _ := newNoColliderLiftActionTest(t)
	ecs.AddComponent(world, target, components.Collider{HalfWidth: 5, HalfHeight: 5})
	actions.handlers["lift"] = &liftActionHandler{lift: lift, commands: systems.NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())}
	actions.Activate(world, 1, player, "lift")
	actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
	actions.Cancel(world, 1, player)
	if _, intent := ecs.GetResource[ecs.LinkState](world).IntentByPlayer[1]; intent {
		t.Fatal("cancel left link intent")
	}
	movement, _ := ecs.GetComponent[components.Movement](world, player)
	if movement.TargetType != constt.TargetNone || actions.State(world, player).Phase != "idle" {
		t.Fatal("cancel left collider approach active")
	}
}

func TestLiftDownForcedCarryLossCancelsSelection(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.LiftCarryState{ObjectEntityID: 3})
	})
	lift := NewLiftService(world, nil, nil, nil, zap.NewNop())
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "lift_down", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile, Cursor: "lift_down"}}})
	sender := &testActionSender{}
	actions, err := NewActionService(world, registry, map[string]ActionHandler{"lift_down": &liftDownActionHandler{lift: lift}}, sender)
	if err != nil {
		t.Fatal(err)
	}
	actions.SendList(1)
	actions.Activate(world, 1, player, "lift_down")
	ecs.RemoveComponent[components.LiftCarryState](world, player)
	actions.Recheck(world, 1, player)
	if state := actions.State(world, player); state.Phase != "idle" || state.Cursor != "" {
		t.Fatalf("carry loss left action armed: %#v", state)
	}
	if len(sender.lists) != 1 {
		t.Fatal("carry loss refreshed action catalog")
	}
	actions.Activate(world, 1, player, "lift_down")
	if len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != "LIFT_NOT_CARRYING" || len(sender.lists) != 1 {
		t.Fatal("missing carry rejection alert or unexpected catalog refresh")
	}
}
