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

type testActionClickRouter struct {
	consume       bool
	calls         int
	targetID      types.EntityID
	x, y          float64
	lists, states int
	cancelCalls   int
	cancelTargetX float64
	cancelTargetY float64
}

func (*testActionClickRouter) Activate(*ecs.World, types.EntityID, types.Handle, string) {}
func (router *testActionClickRouter) Cancel(w *ecs.World, _ types.EntityID, player types.Handle) {
	router.cancelCalls++
	if movement, exists := ecs.GetComponent[components.Movement](w, player); exists {
		router.cancelTargetX, router.cancelTargetY = movement.TargetX, movement.TargetY
	}
	ecs.RemoveComponent[components.ActiveGameAction](w, player)
}
func (router *testActionClickRouter) SendList(types.EntityID) {
	router.lists++
}
func (router *testActionClickRouter) SendState(*ecs.World, types.EntityID, types.Handle) {
	router.states++
}
func (router *testActionClickRouter) HandleArmedClick(_ *ecs.World, _ types.EntityID, _ types.Handle, targetID types.EntityID, _ types.Handle, x, y float64) bool {
	router.calls++
	router.targetID, router.x, router.y = targetID, x, y
	return router.consume
}

func TestMapClickActionRoutingPrecedesPickupAndMovement(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.Movement{})
	})
	w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(w, h, components.DroppedItem{})
	})
	router := &testActionClickRouter{consume: true}
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.SetActionService(router)
	s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 42, Y: 99, TargetEntityId: 2}})
	if router.calls != 1 || router.targetID != 2 || router.x != 42 || router.y != 99 {
		t.Fatalf("action did not receive map click: %#v", router)
	}
	if _, pickup := ecs.GetComponent[components.PendingInteraction](w, player); pickup {
		t.Fatal("consumed action click also picked up item")
	}
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	if movement.TargetX != 0 || movement.TargetY != 0 {
		t.Fatal("consumed action click also moved")
	}

	router.consume = false
	ecs.AddComponent(w, player, components.ActiveGameAction{Phase: components.GameActionSelecting})
	s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 50, Y: 60, TargetEntityId: 999}})
	movement, _ = ecs.GetComponent[components.Movement](w, player)
	active, armed := ecs.GetComponent[components.ActiveGameAction](w, player)
	if movement.TargetX != 50 || movement.TargetY != 60 || router.cancelCalls != 0 || !armed || active.Phase != components.GameActionSelecting {
		t.Fatal("unconsumed stale click did not move while preserving selection")
	}
}

func TestMapClickCancelsApproachBeforeOrdinaryRouting(t *testing.T) {
	for _, targetKind := range []string{"ground", "object"} {
		t.Run(targetKind, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.Movement{})
				ecs.AddComponent(w, h, components.ActiveGameAction{Phase: components.GameActionApproaching})
			})
			ecs.WithComponent(w, player, func(movement *components.Movement) { movement.SetTargetPoint(10, 20) })
			targetID := uint64(0)
			if targetKind == "object" {
				targetID = 2
				w.Spawn(2, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.Transform{X: 50, Y: 60})
					ecs.AddComponent(w, h, components.Collider{HalfWidth: 5, HalfHeight: 5, Layer: 1, Mask: 1})
				})
			}
			router := &testActionClickRouter{}
			system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
			system.SetActionService(router)
			system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 50, Y: 60, TargetEntityId: targetID}})
			if router.cancelCalls != 1 || router.cancelTargetX != 10 || router.cancelTargetY != 20 {
				t.Fatalf("old approach was not canceled before routing: %#v", router)
			}
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			if targetKind == "ground" && (movement.TargetType != constt.TargetPoint || movement.TargetX != 50 || movement.TargetY != 60) {
				t.Fatalf("new ground click did not move: %#v", movement)
			}
			if targetKind == "object" {
				intent, exists := ecs.GetResource[ecs.LinkState](w).IntentByPlayer[1]
				if !exists || intent.TargetID != 2 || movement.TargetType != constt.TargetEntity {
					t.Fatalf("new object click did not start link: intent=%#v movement=%#v", intent, movement)
				}
			}
		})
	}
}

func TestMapClickAdminPrecedesArmedAction(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{})
		ecs.AddComponent(w, h, components.ActiveGameAction{Phase: components.GameActionApproaching})
	})
	w.Spawn(2, nil)
	router := &testActionClickRouter{consume: true}
	admin := &testAdminObjectInfoHandler{}
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.SetActionService(router)
	s.SetAdminHandler(admin)
	ecs.GetResource[ecs.PendingAdminDestroy](w).Set(1)
	s.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 42, Y: 99, TargetEntityId: 2}})
	if admin.destroyCalls != 1 || router.calls != 0 || router.cancelCalls != 0 {
		t.Fatal("armed action intercepted administrator click")
	}
}

func TestInteractWithoutColliderCannotStartLift(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, nil)
	w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{Behaviors: []string{"lift"}})
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
	})
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.handleInteract(w, player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.Interact{EntityId: 2}})
	if _, pending := ecs.GetComponent[components.PendingLiftTransition](w, player); pending {
		t.Fatal("ordinary Interact started lift")
	}
}

func TestActionSnapshotJobSendsListAndState(t *testing.T) {
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, nil)
	router := &testActionClickRouter{}
	s := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	s.SetActionService(router)
	s.processServerJob(w, &network.ServerJob{JobType: network.JobSendActionSnapshot, TargetID: 1, Payload: &network.ActionSnapshotJobPayload{Handle: player}})
	if router.lists != 1 || router.states != 1 {
		t.Fatalf("snapshot did not send list and state: %#v", router)
	}
}
