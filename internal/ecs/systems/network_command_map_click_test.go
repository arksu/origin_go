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
