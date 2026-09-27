package game

import (
	"testing"

	"go.uber.org/zap"
	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type repeatTestHandler struct {
	effects int
	stopAt  int
}

func (*repeatTestHandler) UnavailableReason(*ecs.World, types.EntityID, types.Handle) string {
	return ""
}

func (*repeatTestHandler) ValidateTarget(*ecs.World, types.EntityID, types.Handle, ActionTarget) string {
	return ""
}

func (handler *repeatTestHandler) Start(*ecs.World, types.EntityID, types.Handle, ActionTarget, uint64) ActionResult {
	handler.effects++
	return ActionResult{Outcome: ActionSucceeded, StopAfterCycle: handler.effects == handler.stopAt}
}

func (*repeatTestHandler) Cancel(*ecs.World, types.EntityID, types.Handle, components.ActiveGameAction) {
}

func newRepeatActionTest(t *testing.T, autoRepeat, selectAgain bool, stopAt int) (*ecs.World, types.Handle, *ActionService, *repeatTestHandler, *testActionSender) {
	t.Helper()
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: 6, Y: 6})
		ecs.AddComponent(world, handle, components.Movement{Mode: constt.Walk, Speed: constt.PlayerSpeed})
		ecs.AddComponent(world, handle, components.EntityStats{Stamina: 1000})
	})
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{
		ID: "repeat_test", Target: actiondefs.Target{Kind: actiondefs.TargetTile, Cursor: "dig", Approach: actiondefs.ApproachTileCenter},
		Execution: actiondefs.Execution{Ticks: 2, Stamina: 100, Repeat: autoRepeat}, IsRepeatable: &selectAgain,
	}})
	handler, sender := &repeatTestHandler{stopAt: stopAt}, &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"repeat_test": handler}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "repeat_test")
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	return world, player, service, handler, sender
}

func advanceRepeatTestCycle(t *testing.T, world *ecs.World, player types.Handle, service *ActionService, sender *testActionSender) {
	t.Helper()
	for tick := 0; tick < 2; tick++ {
		cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		if !exists {
			t.Fatal("missing active cycle")
		}
		service.AdvanceCycle(world, 1, player, cycle, sender)
	}
}

func TestActionExecutionRepeatContinuesOneTarget(t *testing.T) {
	world, player, service, handler, sender := newRepeatActionTest(t, true, false, 0)
	advanceRepeatTestCycle(t, world, player, service, sender)
	cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	if !exists || cycle.CycleIndex != 2 || cycle.CycleElapsedTicks != 0 || cycle.TargetX != 6 || cycle.TargetY != 6 || len(sender.finished) != 0 {
		t.Fatalf("first cycle did not continue cleanly: %#v, finished=%d", cycle, len(sender.finished))
	}
	advanceRepeatTestCycle(t, world, player, service, sender)
	cycle, exists = ecs.GetComponent[components.ActiveCyclicAction](world, player)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if !exists || cycle.CycleIndex != 3 || cycle.CycleElapsedTicks != 0 || handler.effects != 2 || stats.Stamina != 800 || service.State(world, player).Phase != "executing" || len(sender.finished) != 0 {
		t.Fatalf("second cycle did not continue: cycle=%#v effects=%d stamina=%v finished=%d", cycle, handler.effects, stats.Stamina, len(sender.finished))
	}
	if len(sender.progress) != 4 || sender.progress[1].CycleIndex != 1 || sender.progress[3].CycleIndex != 2 {
		t.Fatalf("cycle progress did not advance identity: %#v", sender.progress)
	}
}

func TestActionExecutionRepeatStopsAfterSuccessfulCycle(t *testing.T) {
	world, player, service, handler, sender := newRepeatActionTest(t, true, false, 2)
	advanceRepeatTestCycle(t, world, player, service, sender)
	advanceRepeatTestCycle(t, world, player, service, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	if handler.effects != 2 || stats.Stamina != 800 || service.State(world, player).Phase != "idle" || len(sender.finished) != 1 {
		t.Fatalf("terminal success was not charged and stopped: effects=%d stamina=%v state=%#v finished=%#v", handler.effects, stats.Stamina, service.State(world, player), sender.finished)
	}
	if _, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
		t.Fatal("terminal success started another cycle")
	}
}

func TestActionRepeatFlagsRemainIndependent(t *testing.T) {
	for _, test := range []struct {
		name         string
		autoRepeat   bool
		selectAgain  bool
		wantPhase    string
		wantFinished int
	}{
		{name: "single and idle", wantPhase: "idle", wantFinished: 1},
		{name: "single and select", selectAgain: true, wantPhase: "selecting", wantFinished: 1},
		{name: "automatic and idle", autoRepeat: true, wantPhase: "idle", wantFinished: 1},
		{name: "automatic and select", autoRepeat: true, selectAgain: true, wantPhase: "selecting", wantFinished: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, player, service, handler, sender := newRepeatActionTest(t, test.autoRepeat, test.selectAgain, 1)
			advanceRepeatTestCycle(t, world, player, service, sender)
			if handler.effects != 1 || service.State(world, player).Phase != test.wantPhase || len(sender.finished) != test.wantFinished {
				t.Fatalf("wrong repeat behavior: effects=%d state=%#v finished=%d", handler.effects, service.State(world, player), len(sender.finished))
			}
		})
	}
}

func TestActionExecutionRepeatIgnoresStaleCycleAfterCancel(t *testing.T) {
	world, player, service, handler, sender := newRepeatActionTest(t, true, false, 0)
	stale, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	service.Cancel(world, 1, player)
	service.Activate(world, 1, player, "repeat_test")
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	current, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	service.AdvanceCycle(world, 1, player, stale, sender)
	after, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	if after != current || handler.effects != 0 {
		t.Fatal("stale cycle advanced or replaced a newer attempt")
	}
	advanceRepeatTestCycle(t, world, player, service, sender)
	if handler.effects != 1 {
		t.Fatal("new attempt did not complete")
	}
}

func TestActionExecutionRepeatCancelsFromPlayerInput(t *testing.T) {
	for _, input := range []string{"escape", "secondary click"} {
		t.Run(input, func(t *testing.T) {
			world, player, service, handler, sender := newRepeatActionTest(t, true, false, 0)
			stale, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
			inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 20, MaxCommandsPerTickPerClient: 20})
			commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20}), nil, nil, nil, nil, 0, zap.NewNop())
			commands.SetActionService(service)
			command := &network.PlayerCommand{ClientID: 1, CharacterID: 1, CommandID: 1}
			if input == "escape" {
				command.CommandType = network.CmdCancelAction
				command.Payload = &netproto.C2S_CancelAction{}
			} else {
				command.CommandType = network.CmdMapClick
				command.Payload = &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, X: 6, Y: 6}
			}
			if err := inbox.Enqueue(command); err != nil {
				t.Fatal(err)
			}
			commands.Update(world, 0)
			service.AdvanceCycle(world, 1, player, stale, sender)
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			if handler.effects != 0 || stats.Stamina != 1000 || service.State(world, player).Phase != "idle" || len(sender.finished) != 1 {
				t.Fatalf("input did not cancel repeat: effects=%d stamina=%v state=%#v finished=%d", handler.effects, stats.Stamina, service.State(world, player), len(sender.finished))
			}
		})
	}
}
