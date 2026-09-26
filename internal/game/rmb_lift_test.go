package game

import (
	"testing"

	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

func (*testActionSender) SendLiftCarryState(types.EntityID, *netproto.S2C_LiftCarryState) {}

type rmbLiftTest struct {
	world    *ecs.World
	player   types.Handle
	object   types.Handle
	lift     *LiftService
	actions  *ActionService
	sender   *testActionSender
	commands *systems.NetworkCommandSystem
	inbox    *network.PlayerCommandInbox
	sequence uint64
}

func newRMBLiftTest(t *testing.T) *rmbLiftTest {
	t.Helper()
	w, player, object, lift, actions, sender := newNoColliderLiftActionTest(t)
	completeNoColliderLift(t, w, player, object, lift, actions)
	ecs.AddComponent(w, player, components.EntityStats{Stamina: 1000})
	lift.chunkActive = func(types.ChunkCoord) bool { return true }
	lift.alerts = sender
	sender.states = nil
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 20, MaxCommandsPerTickPerClient: 20})
	commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20}), nil, nil, nil, nil, 0, zap.NewNop())
	commands.SetActionService(actions)
	commands.SetLiftCommandService(lift)
	return &rmbLiftTest{world: w, player: player, object: object, lift: lift, actions: actions, sender: sender, commands: commands, inbox: inbox}
}

func (test *rmbLiftTest) send(t *testing.T, command network.CommandType, payload any) {
	t.Helper()
	test.sequence++
	if err := test.inbox.Enqueue(&network.PlayerCommand{ClientID: 1, CharacterID: 1, CommandID: test.sequence, CommandType: command, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	test.commands.Update(test.world, 0)
}

func (test *rmbLiftTest) click(t *testing.T, target uint64, x, y int32) components.PendingLiftTransition {
	t.Helper()
	test.send(t, network.CmdMapClick, &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, TargetEntityId: target, X: x, Y: y})
	pending, _ := ecs.GetComponent[components.PendingLiftTransition](test.world, test.player)
	return pending
}

func (test *rmbLiftTest) assertIdleCarry(t *testing.T, carrying bool) {
	t.Helper()
	_, hasCarry := ecs.GetComponent[components.LiftCarryState](test.world, test.player)
	_, pending := ecs.GetComponent[components.PendingLiftTransition](test.world, test.player)
	_, cycle := ecs.GetComponent[components.ActiveCyclicAction](test.world, test.player)
	collider, _ := ecs.GetComponent[components.Collider](test.world, test.player)
	movement, _ := ecs.GetComponent[components.Movement](test.world, test.player)
	stats, _ := ecs.GetComponent[components.EntityStats](test.world, test.player)
	state := test.actions.State(test.world, test.player)
	if hasCarry != carrying || pending || cycle || collider.Phantom != nil || state.Phase != "idle" || state.Cursor != "" || movement.TargetType != constt.TargetNone || stats.Stamina != 1000 {
		t.Fatalf("attempt cleanup: carry=%v pending=%v cycle=%v collider=%+v state=%+v movement=%+v stamina=%v", hasCarry, pending, cycle, collider, state, movement, stats.Stamina)
	}
}

func TestRMBLiftPlacementUsesCoordinatesBeforeTarget(t *testing.T) {
	for _, kind := range []string{"object", "dropped", "ground", "stale"} {
		t.Run(kind, func(t *testing.T) {
			test := newRMBLiftTest(t)
			targetID := uint64(9)
			if kind == "ground" {
				targetID = 0
			} else if kind != "stale" {
				test.world.Spawn(9, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.Transform{X: 900, Y: 900})
					ecs.AddComponent(w, h, components.Collider{})
					if kind == "dropped" {
						ecs.AddComponent(w, h, components.DroppedItem{})
					}
				})
			}
			test.actions.Activate(test.world, 1, test.player, "lift_down")
			old, _ := ecs.GetComponent[components.ActiveGameAction](test.world, test.player)
			test.sender.states = nil
			pending := test.click(t, targetID, 1043, 1087)
			active, ok := ecs.GetComponent[components.ActiveGameAction](test.world, test.player)
			if !ok || !active.DirectAttempt || active.Generation <= old.Generation || active.Generation != pending.ActionGeneration || pending.TargetX != 1043 || pending.TargetY != 1087 || active.TargetID != 0 {
				t.Fatalf("incorrect direct placement: active=%+v pending=%+v", active, pending)
			}
			if len(test.sender.states) != 2 || test.sender.states[0].Phase != "idle" || test.sender.states[1].Phase != "approaching" || test.sender.states[1].Cursor != "" {
				t.Fatalf("RMB did not cancel before starting exactly one approach: %+v", test.sender.states)
			}
			if _, pickup := ecs.GetComponent[components.PendingInteraction](test.world, test.player); pickup {
				t.Fatal("carry click also queued pickup")
			}
			ecs.AddComponent(test.world, test.player, components.CollisionResult{IsPhantom: true})
			systems.NewLiftPlacementSystem(test.world, test.lift, nil).Update(test.world, 0)
			test.assertIdleCarry(t, false)
			position, _ := ecs.GetComponent[components.Transform](test.world, test.object)
			if position.X != 1043 || position.Y != 1087 {
				t.Fatalf("placement snapped away from click: %+v", position)
			}
		})
	}
}

func TestRMBLiftFailureKeepsCarryWithoutArming(t *testing.T) {
	for _, failure := range []string{"rejection", "timeout", "chunk lost", "relocation"} {
		t.Run(failure, func(t *testing.T) {
			test := newRMBLiftTest(t)
			if failure == "rejection" {
				test.lift.chunkActive = func(types.ChunkCoord) bool { return false }
			}
			pending := test.click(t, 0, 1043, 1087)
			if failure == "timeout" {
				ecs.GetResource[ecs.TimeState](test.world).UnixMs = pending.ExpireAtUnixMs
				systems.NewLiftPlacementSystem(test.world, test.lift, nil).Update(test.world, 0)
			} else if failure != "rejection" {
				if failure == "chunk lost" {
					test.lift.chunkActive = func(types.ChunkCoord) bool { return false }
				} else {
					test.lift.relocateObject = func(*ecs.World, types.Handle, gameworld.RelocateWorldObjectImmediateOptions, float64, float64) bool {
						return false
					}
				}
				test.lift.FinalizePendingLiftTransition(test.world, 1, test.player, pending)
			}
			test.assertIdleCarry(t, true)
			wantReason := "LIFT_PUTDOWN_INVALID"
			if failure == "timeout" {
				wantReason = "LIFT_PUTDOWN_TIMEOUT"
			}
			if len(test.sender.alerts) != 1 || test.sender.alerts[0].ReasonCode != wantReason {
				t.Fatalf("missing placement error: %+v", test.sender.alerts)
			}
			for _, state := range test.sender.states {
				if state.Phase == "selecting" || state.Cursor != "" {
					t.Fatal("failed secondary placement armed retry")
				}
			}
		})
	}
}

func TestRMBLiftReplacementRejectsStaleCompletion(t *testing.T) {
	test := newRMBLiftTest(t)
	first := test.click(t, 0, 1043, 1087)
	second := test.click(t, 0, 1131, 1173)
	if first.ActionGeneration == 0 || second.ActionGeneration <= first.ActionGeneration {
		t.Fatal("replacement reused the old generation")
	}
	test.lift.FinalizePendingLiftTransition(test.world, 1, test.player, first)
	test.actions.Complete(test.world, 1, test.player, first.ActionGeneration, false, "STALE")
	current, exists := ecs.GetComponent[components.PendingLiftTransition](test.world, test.player)
	position, _ := ecs.GetComponent[components.Transform](test.world, test.object)
	collider, _ := ecs.GetComponent[components.Collider](test.world, test.player)
	if !exists || current != second || position.X != 1000 || position.Y != 1000 || collider.Phantom == nil || collider.Phantom.WorldX != 1131 || len(test.sender.alerts) != 0 {
		t.Fatal("stale completion placed the object or changed the replacement")
	}
	test.lift.FinalizePendingLiftTransition(test.world, 1, test.player, second)
	test.assertIdleCarry(t, false)
	position, _ = ecs.GetComponent[components.Transform](test.world, test.object)
	if position.X != 1131 || position.Y != 1173 {
		t.Fatal("replacement did not place at B")
	}
}

func TestRMBLiftApproachCanBeInterrupted(t *testing.T) {
	for _, interrupt := range []string{"escape", "primary", "carry loss"} {
		t.Run(interrupt, func(t *testing.T) {
			test := newRMBLiftTest(t)
			pending := test.click(t, 0, 1043, 1087)
			switch interrupt {
			case "escape":
				test.send(t, network.CmdCancelAction, &netproto.C2S_CancelAction{})
			case "primary":
				test.send(t, network.CmdMapClick, &netproto.MapClick{X: 1201, Y: 1253})
				movement, _ := ecs.GetComponent[components.Movement](test.world, test.player)
				if movement.TargetX != 1201 || movement.TargetY != 1253 || movement.TargetType != constt.TargetPoint {
					t.Fatal("primary input did not replace approach with ordinary movement")
				}
				ecs.WithComponent(test.world, test.player, func(movement *components.Movement) { movement.ClearTarget() })
			case "carry loss":
				ecs.RemoveComponent[components.LiftCarryState](test.world, test.player)
				test.actions.Recheck(test.world, 1, test.player)
			}
			test.lift.FinalizePendingLiftTransition(test.world, 1, test.player, pending)
			test.assertIdleCarry(t, interrupt != "carry loss")
			position, _ := ecs.GetComponent[components.Transform](test.world, test.object)
			if position.X != 1000 || position.Y != 1000 {
				t.Fatal("canceled approach placed the object")
			}
		})
	}
}

func TestRMBGroundCancelsLiftSelectionAndApproach(t *testing.T) {
	for _, phase := range []string{"selecting", "approaching"} {
		t.Run(phase, func(t *testing.T) {
			world, player, target, lift, actions, sender := newNoColliderLiftActionTest(t)
			actions.Activate(world, 1, player, "lift")
			if phase == "approaching" {
				actions.HandleArmedClick(world, 1, player, 3, target, 1000, 1000)
			}
			pending, _ := ecs.GetComponent[components.PendingLiftTransition](world, player)
			inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 20, MaxCommandsPerTickPerClient: 20})
			commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20}), nil, nil, nil, nil, 0, nil)
			commands.SetActionService(actions)
			commands.SetLiftCommandService(lift)
			test := &rmbLiftTest{world: world, player: player, object: target, lift: lift, actions: actions, sender: sender, inbox: inbox, commands: commands}
			ecs.AddComponent(world, player, components.EntityStats{Stamina: 1000})
			test.click(t, 0, 43, 99)
			lift.FinalizePendingLiftTransition(world, 1, player, pending)
			test.assertIdleCarry(t, false)
		})
	}
}

func TestRMBGroundCancelsTimedActionWithoutCostOrLateEffect(t *testing.T) {
	test := newRMBLiftTest(t)
	ecs.RemoveComponent[components.LiftCarryState](test.world, test.player)
	definitions := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "work", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile}, Execution: actiondefs.Execution{Ticks: 3, Stamina: 2}}})
	handler := &testActionHandler{status: ActionSucceeded}
	actions, err := NewActionService(test.world, definitions, map[string]ActionHandler{"work": handler}, test.sender)
	if err != nil {
		t.Fatal(err)
	}
	test.actions = actions
	test.commands.SetActionService(actions)
	actions.Activate(test.world, 1, test.player, "work")
	actions.HandleArmedClick(test.world, 1, test.player, 0, types.InvalidHandle, 43, 99)
	active, _ := ecs.GetComponent[components.ActiveGameAction](test.world, test.player)
	cycle, hasCycle := ecs.GetComponent[components.ActiveCyclicAction](test.world, test.player)
	if !hasCycle || active.Phase != components.GameActionExecuting {
		t.Fatal("timed action did not start")
	}
	test.click(t, 0, 71, 83)
	actions.Complete(test.world, 1, test.player, active.Generation, true, "")
	actions.AdvanceCycle(test.world, 1, test.player, cycle, test.sender)
	test.assertIdleCarry(t, false)
	if handler.startCount != 0 || handler.canceled != 1 || len(test.sender.finished) != 1 || test.sender.finished[0].Result != netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED {
		t.Fatal("RMB did not cancel the incomplete cycle exactly once")
	}
}
