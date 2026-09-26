package systems

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type testCarryStateService struct{}

func (testCarryStateService) IsPlayerCarrying(w *ecs.World, player types.Handle) bool {
	_, carrying := ecs.GetComponent[components.LiftCarryState](w, player)
	return carrying
}
func (testCarryStateService) SendCarryLockedWarning(types.EntityID) {}

func TestSecondaryCarryPrecedesEveryTargetAndHasNoFallback(t *testing.T) {
	for _, target := range []string{"ground", "stale", "dropped", "collider"} {
		t.Run(target, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.Movement{})
				ecs.AddComponent(w, h, components.ActiveGameAction{Phase: components.GameActionSelecting})
				ecs.AddComponent(w, h, components.LiftCarryState{ObjectEntityID: 3})
			})
			targetID := uint64(2)
			if target == "ground" {
				targetID = 0
			} else if target != "stale" {
				w.Spawn(2, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.Transform{X: 900, Y: 900})
					ecs.AddComponent(w, h, components.Collider{})
					if target == "dropped" {
						ecs.AddComponent(w, h, components.DroppedItem{})
					}
				})
			}
			router, menu := &testActionClickRouter{consume: true}, &testContextMenuSender{}
			system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, nil)
			system.SetActionService(router)
			system.SetLiftCommandService(testCarryStateService{})
			system.SetContextMenuSender(menu)
			system.SetContextActionService(testContextActionResolver{actions: []ContextAction{{ActionID: "one"}, {ActionID: "two"}}})
			system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, X: 43, Y: 99, TargetEntityId: targetID}})
			if router.cancelCalls != 1 || router.calls != 0 || router.directCalls != 1 || router.directID != "lift_down" || router.directX != 43 || router.directY != 99 || router.directTarget != 0 || router.directHandle != types.InvalidHandle || router.activeAtDirectStart {
				t.Fatalf("incorrect carry routing: %+v", router)
			}
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			_, pickup := ecs.GetComponent[components.PendingInteraction](w, player)
			if movement.TargetType != constt.TargetNone || pickup || len(menu.sent) != 0 {
				t.Fatal("rejected direct placement fell through to another action")
			}
		})
	}
}

func TestSecondaryCancelsEveryPhaseAndLeavesAdminCommandsPending(t *testing.T) {
	for _, phase := range []components.GameActionPhase{components.GameActionSelecting, components.GameActionApproaching, components.GameActionExecuting} {
		t.Run(string(phase), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Movement{})
				ecs.AddComponent(w, h, components.ActiveGameAction{Phase: phase})
			})
			ecs.GetResource[ecs.PendingAdminDestroy](w).Set(1)
			ecs.GetResource[ecs.PendingAdminObjectInfo](w).Set(1)
			ecs.GetResource[ecs.PendingAdminTeleport](w).Set(1)
			ecs.GetResource[ecs.PendingAdminSpawn](w).Set(1, ecs.AdminSpawnEntry{})
			admin, router := &testAdminObjectInfoHandler{}, &testActionClickRouter{consume: true}
			system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, nil)
			system.SetAdminHandler(admin)
			system.SetActionService(router)
			for _, button := range []netproto.MapClickButton{1, 99, -1} {
				system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{Button: button, X: 43, Y: 99}})
			}
			if router.cancelCalls != 0 || router.calls != 0 {
				t.Fatal("unsupported button affected action state")
			}
			system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, X: 43, Y: 99}})
			_, spawnPending := ecs.GetResource[ecs.PendingAdminSpawn](w).Get(1)
			if admin.calls != 0 || admin.destroyCalls != 0 || admin.coordinateCalls != 0 || !ecs.GetResource[ecs.PendingAdminDestroy](w).Get(1) || !ecs.GetResource[ecs.PendingAdminObjectInfo](w).Get(1) || !ecs.GetResource[ecs.PendingAdminTeleport](w).Get(1) || !spawnPending {
				t.Fatal("secondary or unsupported input consumed an admin command")
			}
			if router.cancelCalls != 1 || router.calls != 0 {
				t.Fatal("secondary input did not exclusively cancel armed routing")
			}
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			if movement.TargetType != constt.TargetNone {
				t.Fatal("secondary ground click started movement")
			}
		})
	}
}

func TestSecondaryGroundAndStaleTargetsPreserveUnrelatedMovement(t *testing.T) {
	for _, targetID := range []uint64{0, 999} {
		w := ecs.NewWorldForTesting()
		player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			movement := components.Movement{}
			movement.SetTargetPoint(10, 20)
			ecs.AddComponent(w, h, movement)
			ecs.AddComponent(w, h, components.PendingContextAction{TargetEntityID: 8})
		})
		system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, nil)
		system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, X: 43, Y: 99, TargetEntityId: targetID}})
		movement, _ := ecs.GetComponent[components.Movement](w, player)
		_, pending := ecs.GetComponent[components.PendingContextAction](w, player)
		if movement.TargetX != 10 || movement.TargetY != 20 || !pending {
			t.Fatal("secondary empty input interrupted unrelated movement")
		}
	}
}

func TestSecondaryContextActionCounts(t *testing.T) {
	for _, count := range []int{0, 2} {
		w := ecs.NewWorldForTesting()
		player := w.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Movement{}) })
		w.Spawn(2, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Collider{}) })
		menu := &testContextMenuSender{}
		system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, nil)
		system.SetContextMenuSender(menu)
		system.SetContextActionService(testContextActionResolver{actions: []ContextAction{{ActionID: "one"}, {ActionID: "two"}}[:count]})
		system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, TargetEntityId: 2}})
		if count == 0 && len(menu.sent) != 0 || count == 2 && (len(menu.sent) != 1 || len(menu.sent[0].Actions) != 2) {
			t.Fatal("secondary context menu did not preserve action count rules")
		}
		movement, _ := ecs.GetComponent[components.Movement](w, player)
		if movement.TargetType != constt.TargetNone {
			t.Fatal("context menu started movement")
		}
	}
}

func TestSecondaryPickupAfterCancelRunsOnce(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.Movement{})
		ecs.AddComponent(w, h, components.ActiveGameAction{Phase: components.GameActionSelecting})
	})
	w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.DroppedItem{})
	})
	hand := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, hand, components.InventoryContainer{Kind: constt.InventoryHand, OwnerID: 1})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryHand, 1, 0, hand)
	router := &testActionClickRouter{consume: true}
	system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, nil)
	system.SetActionService(router)
	system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, TargetEntityId: 2}})
	executor := &autoPickupExecutorStub{results: []InventoryOpResult{{Success: true}}}
	pickup := NewAutoInteractSystem(executor, nil, nil, nil)
	pickup.Update(w, 0)
	pickup.Update(w, 0)
	if router.cancelCalls != 1 || router.calls != 0 || len(executor.destinations) != 1 {
		t.Fatal("secondary pickup did not run once after canceling the action")
	}
}
