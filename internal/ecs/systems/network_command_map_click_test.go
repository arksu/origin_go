package systems

import (
	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"testing"
)

func TestMapClickOrdinaryMovementAndPickup(t *testing.T) {
	for _, mode := range []string{"ground", "object", "stale", "dropped", "stunned"} {
		t.Run(mode, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
			})
			var targetID uint64
			if mode != "ground" {
				targetID = 2
				target := w.Spawn(2, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.Transform{X: 500, Y: 600})
				})
				if mode == "dropped" {
					ecs.AddComponent(w, target, components.DroppedItem{})
				}
				if mode == "stale" {
					w.Despawn(target)
				}
			}
			if mode == "stunned" {
				ecs.WithComponent(w, player, func(m *components.Movement) { m.State = constt.StateStunned })
			}
			s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
			s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 42, Y: 99, TargetEntityId: targetID}})
			m, _ := ecs.GetComponent[components.Movement](w, player)
			pending, pickup := ecs.GetComponent[components.PendingInteraction](w, player)
			switch mode {
			case "dropped":
				if !pickup || pending.TargetEntityID != 2 || pending.Type != netproto.InteractionType_PICKUP {
					t.Fatal("pickup was not queued")
				}
			case "stunned":
				if m.TargetX != 0 || m.TargetY != 0 {
					t.Fatal("stunned player moved")
				}
			default:
				if pickup || m.TargetX != 42 || m.TargetY != 99 {
					t.Fatalf("wrong ordinary movement: %+v", m)
				}
			}
		})
	}
}

func TestMapClickOnObjectStartsLinkIntentAndLinksOnCollision(t *testing.T) {
	eb := newTestEventBus(t)
	defer shutdownTestEventBus(t, eb)

	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
		ecs.AddComponent(w, h, components.CollisionResult{})
	})
	w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 500, Y: 600})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: 1, Mask: 1})
	})
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 505, Y: 600, TargetEntityId: 2}})

	linkState := ecs.GetResource[ecs.LinkState](w)
	intent, hasIntent := linkState.IntentByPlayer[1]
	if !hasIntent || intent.TargetID != 2 {
		t.Fatal("map click on object did not set link intent")
	}
	m, _ := ecs.GetComponent[components.Movement](w, player)
	if m.TargetType != constt.TargetEntity || m.TargetHandle == types.InvalidHandle {
		t.Fatalf("movement must track the clicked object: %+v", m)
	}
	pending, hasPending := ecs.GetComponent[components.PendingContextAction](w, player)
	if !hasPending || pending.TargetEntityID != 2 || pending.ActionID != "" {
		t.Fatalf("expected link-only pending entry, got %+v", pending)
	}

	// Confirmed collision with the target while the intent is live creates the
	// link. The link-only pending entry is dropped by the LinkCreated
	// subscribers in the game layer, which execute no action for empty ActionID.
	ecs.WithComponent(w, player, func(cr *components.CollisionResult) { cr.PrevCollidedWith = 2 })
	NewLinkSystem(eb, zap.NewNop()).Update(w, 0.05)

	link, ok := linkState.GetLink(1)
	if !ok || link.TargetID != 2 {
		t.Fatal("link was not created on collision")
	}
	if _, staleIntent := linkState.IntentByPlayer[1]; staleIntent {
		t.Fatal("intent must be cleared after link creation")
	}
}

func TestMapClickGroundClearsObjectLinkIntent(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 0, Y: 0})
		ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
	})
	w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 500, Y: 600})
		ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: 1, Mask: 1})
	})
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 505, Y: 600, TargetEntityId: 2}})
	if _, hasIntent := ecs.GetResource[ecs.LinkState](w).IntentByPlayer[1]; !hasIntent {
		t.Fatal("precondition failed: object click did not set intent")
	}

	// A click on empty ground cancels the outstanding link intent.
	s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 42, Y: 99}})
	if _, hasIntent := ecs.GetResource[ecs.LinkState](w).IntentByPlayer[1]; hasIntent {
		t.Fatal("ground click must clear the link intent")
	}
	if _, hasPending := ecs.GetComponent[components.PendingContextAction](w, player); hasPending {
		t.Fatal("ground click must remove the link-only pending entry")
	}
	m, _ := ecs.GetComponent[components.Movement](w, player)
	if m.TargetType != constt.TargetPoint || m.TargetX != 42 || m.TargetY != 99 {
		t.Fatalf("ground click must move to coordinates: %+v", m)
	}
}

func TestInteractDoesNotConsumeAdminSelection(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, nil)
	admin := &testAdminObjectInfoHandler{}
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.SetAdminHandler(admin)
	ecs.GetResource[ecs.PendingAdminObjectInfo](w).Set(1)
	ecs.GetResource[ecs.PendingAdminDestroy](w).Set(1)
	s.handleInteract(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.Interact{EntityId: 99}})
	if admin.calls != 0 || admin.destroyCalls != 0 || !ecs.GetResource[ecs.PendingAdminObjectInfo](w).Get(1) || !ecs.GetResource[ecs.PendingAdminDestroy](w).Get(1) {
		t.Fatal("context interaction consumed selection")
	}
}

func TestDestroyMapClickInterceptsDroppedItem(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.Movement{})
	})
	w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 1, Y: 1})
		ecs.AddComponent(w, h, components.DroppedItem{})
	})
	admin := &testAdminObjectInfoHandler{}
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.SetAdminHandler(admin)
	ecs.GetResource[ecs.PendingAdminDestroy](w).Set(1)
	s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 42, Y: 99, TargetEntityId: 2}})
	if admin.destroyCalls != 1 || admin.destroyTargetID != 2 {
		t.Fatal("destroy did not receive target")
	}
	if _, pickup := ecs.GetComponent[components.PendingInteraction](w, player); pickup {
		t.Fatal("destroy click also picked up item")
	}
	m, _ := ecs.GetComponent[components.Movement](w, player)
	if m.TargetX != 0 || m.TargetY != 0 {
		t.Fatal("destroy click also moved")
	}
}

func TestMapClickCoordinateCommandsUseClickNotObjectPosition(t *testing.T) {
	for _, command := range []string{"spawn", "tp"} {
		t.Run(command, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Movement{}) })
			w.Spawn(2, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 500, Y: 600}) })
			admin := &testAdminObjectInfoHandler{}
			s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
			s.SetAdminHandler(admin)
			if command == "spawn" {
				ecs.GetResource[ecs.PendingAdminSpawn](w).Set(1, ecs.AdminSpawnEntry{})
			} else {
				ecs.GetResource[ecs.PendingAdminTeleport](w).Set(1)
			}
			s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 42, Y: 99, TargetEntityId: 2}})
			if admin.coordinateCalls != 1 || admin.clickX != 42 || admin.clickY != 99 {
				t.Fatalf("wrong coordinates: %+v", admin)
			}
			m, _ := ecs.GetComponent[components.Movement](w, player)
			if m.State == constt.StateMoving {
				t.Fatal("consumed click also moved")
			}
		})
	}
}
