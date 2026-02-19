package systems

import (
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"go.uber.org/zap"
)

type testContextMenuSender struct {
	sent []*netproto.S2C_ContextMenu
}

func (s *testContextMenuSender) SendContextMenu(_ types.EntityID, menu *netproto.S2C_ContextMenu) {
	s.sent = append(s.sent, menu)
}

type testContextActionResolver struct {
	actions []ContextAction
}

func (r testContextActionResolver) ComputeActions(
	_ *ecs.World,
	_ types.EntityID,
	_ types.Handle,
	_ types.EntityID,
	_ types.Handle,
) []ContextAction {
	return r.actions
}

func (r testContextActionResolver) ExecuteAction(
	_ *ecs.World,
	_ types.EntityID,
	_ types.Handle,
	_ types.EntityID,
	_ types.Handle,
	_ string,
) bool {
	return true
}

func TestNetworkCommandSystem_InteractSingleAction_AutoExecWhenDefDisablesSingleMenu(t *testing.T) {
	world := ecs.NewWorldForTesting()
	prevRegistry := objectdefs.Global()
	defer objectdefs.SetGlobalForTesting(prevRegistry)
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID:                          101,
			Key:                            "tree",
			Resource:                       "tree",
			ContextMenuEvenForOneItemValue: false,
		},
	}))

	system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	menuSender := &testContextMenuSender{}
	system.SetContextMenuSender(menuSender)
	system.SetContextActionService(testContextActionResolver{
		actions: []ContextAction{{ActionID: "chop", Title: "Chop"}},
	})

	const (
		playerID = types.EntityID(1001)
		targetID = types.EntityID(2002)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
	})
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 10})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 1, HalfHeight: 1, Layer: 1, Mask: 1})
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 101})
	})

	system.handleInteract(world, playerHandle, &network.PlayerCommand{
		CharacterID: playerID,
		Payload:     &netproto.Interact{EntityId: uint64(targetID)},
	})

	if len(menuSender.sent) != 0 {
		t.Fatalf("expected no context menu, got %d", len(menuSender.sent))
	}
	pending, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle)
	if !hasPending {
		t.Fatalf("expected pending context action")
	}
	if pending.TargetEntityID != targetID {
		t.Fatalf("expected pending target %d, got %d", targetID, pending.TargetEntityID)
	}
	if pending.ActionID != "chop" {
		t.Fatalf("expected pending action chop, got %q", pending.ActionID)
	}
	if pending.TargetHandle != targetHandle {
		t.Fatalf("expected pending target handle %d, got %d", targetHandle, pending.TargetHandle)
	}
}

func TestNetworkCommandSystem_InteractSingleAction_OpensMenuWhenDefEnablesSingleMenu(t *testing.T) {
	world := ecs.NewWorldForTesting()
	prevRegistry := objectdefs.Global()
	defer objectdefs.SetGlobalForTesting(prevRegistry)
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{
			DefID:                          102,
			Key:                            "box",
			Resource:                       "box",
			ContextMenuEvenForOneItemValue: true,
		},
	}))

	system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	menuSender := &testContextMenuSender{}
	system.SetContextMenuSender(menuSender)
	system.SetContextActionService(testContextActionResolver{
		actions: []ContextAction{{ActionID: "open", Title: "Open"}},
	})

	const (
		playerID = types.EntityID(3003)
		targetID = types.EntityID(4004)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
	})
	world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 10})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 1, HalfHeight: 1, Layer: 1, Mask: 1})
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 102})
	})

	system.handleInteract(world, playerHandle, &network.PlayerCommand{
		CharacterID: playerID,
		Payload:     &netproto.Interact{EntityId: uint64(targetID)},
	})

	if len(menuSender.sent) != 1 {
		t.Fatalf("expected one context menu, got %d", len(menuSender.sent))
	}
	menu := menuSender.sent[0]
	if len(menu.Actions) != 1 {
		t.Fatalf("expected one context menu action, got %d", len(menu.Actions))
	}
	if menu.Actions[0].ActionId != "open" {
		t.Fatalf("expected action open, got %q", menu.Actions[0].ActionId)
	}
	if _, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle); hasPending {
		t.Fatalf("did not expect pending context action when menu is opened")
	}
}

func TestNetworkCommandSystem_CleanupPendingContextActions_ClearsMatchingLinkIntent(t *testing.T) {
	world := ecs.NewWorldForTesting()
	system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())

	const (
		playerID = types.EntityID(5001)
		targetID = types.EntityID(5002)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
		ecs.AddComponent(w, h, components.PendingContextAction{
			TargetEntityID: targetID,
			ActionID:       "chop",
		})
	})
	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 10})
	})
	if playerHandle == types.InvalidHandle || targetHandle == types.InvalidHandle {
		t.Fatalf("expected valid handles")
	}

	linkState := ecs.GetResource[ecs.LinkState](world)
	linkState.SetIntent(playerID, targetID, targetHandle, time.Now())

	system.cleanupPendingContextActions(world)

	if _, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle); hasPending {
		t.Fatalf("expected pending context action to be removed")
	}
	if _, hasIntent := linkState.IntentByPlayer[playerID]; hasIntent {
		t.Fatalf("expected matching link intent to be cleared with pending action")
	}
}

func TestNetworkCommandSystem_CleanupPendingContextActions_PreservesOtherLinkIntent(t *testing.T) {
	world := ecs.NewWorldForTesting()
	system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())

	const (
		playerID   = types.EntityID(6001)
		pendingID  = types.EntityID(6002)
		retargetID = types.EntityID(6003)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
		ecs.AddComponent(w, h, components.PendingContextAction{
			TargetEntityID: pendingID,
			ActionID:       "chop",
		})
	})
	world.Spawn(pendingID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 10})
	})
	retargetHandle := world.Spawn(retargetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 12, Y: 10})
	})
	if playerHandle == types.InvalidHandle || retargetHandle == types.InvalidHandle {
		t.Fatalf("expected valid handles")
	}

	linkState := ecs.GetResource[ecs.LinkState](world)
	linkState.SetIntent(playerID, retargetID, retargetHandle, time.Now())

	system.cleanupPendingContextActions(world)

	if _, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle); hasPending {
		t.Fatalf("expected pending context action to be removed")
	}
	intent, hasIntent := linkState.IntentByPlayer[playerID]
	if !hasIntent {
		t.Fatalf("expected retarget link intent to be preserved")
	}
	if intent.TargetID != retargetID {
		t.Fatalf("expected intent target %d, got %d", retargetID, intent.TargetID)
	}
}
