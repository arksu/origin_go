package game

import (
	"testing"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestActionValidationSystemSkipsIdlePlayersAndChecksEveryActivePlayer(t *testing.T) {
	world := ecs.NewWorldForTesting()
	players := []types.Handle{
		world.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.CharacterProfile{Skills: []string{"woodcutting"}})
		}),
		world.Spawn(2, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.CharacterProfile{Skills: []string{"woodcutting"}})
		}),
	}
	for playerID := types.EntityID(3); playerID < 23; playerID++ {
		world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.CharacterProfile{Skills: []string{"woodcutting"}})
		})
	}
	handler := &testActionHandler{status: ActionSucceeded}
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{
		ID: "test_tile", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile},
		Requirements: actiondefs.Requirements{Skills: []string{"woodcutting"}},
	}})
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_tile": handler}, &testActionSender{})
	if err != nil {
		t.Fatal(err)
	}
	system := NewActionValidationSystem(world, service)
	system.Update(world, 0)
	if handler.reasonCalls != 0 {
		t.Fatalf("idle players caused %d handler checks", handler.reasonCalls)
	}
	for playerID, handle := range players {
		service.Activate(world, types.EntityID(playerID+1), handle, "test_tile")
	}
	handler.reasonCalls = 0
	system.Update(world, 0)
	if handler.reasonCalls != len(players) {
		t.Fatalf("active players caused %d checks, want %d", handler.reasonCalls, len(players))
	}
	for _, handle := range players {
		ecs.WithComponent(world, handle, func(profile *components.CharacterProfile) { profile.Skills = nil })
	}
	system.Update(world, 0)
	for _, handle := range players {
		if state := service.State(world, handle); state.Phase != "idle" {
			t.Fatalf("requirement loss left action active: %#v", state)
		}
	}
	system.Update(world, 0)
	if handler.reasonCalls != len(players) {
		t.Fatalf("canceled actions were still checked: %d", handler.reasonCalls)
	}
}

func TestActionValidationSystemChecksApproachTimeoutAndTargetLoss(t *testing.T) {
	for _, test := range []struct {
		name        string
		reason      string
		breakTarget bool
	}{
		{name: "timeout", reason: "ACTION_TARGET_TIMEOUT"},
		{name: "target loss", reason: "ACTION_INVALID_TARGET", breakTarget: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			player := world.Spawn(1, nil)
			target := world.Spawn(3, nil)
			handler := &testActionHandler{status: ActionApproaching}
			sender := &testActionSender{}
			registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_object", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetObject, Cursor: "lift"}}})
			service, err := NewActionService(world, registry, map[string]ActionHandler{"test_object": handler}, sender)
			if err != nil {
				t.Fatal(err)
			}
			system := NewActionValidationSystem(world, service)
			service.Activate(world, 1, player, "test_object")
			service.HandleArmedClick(world, 1, player, 3, target, 7, 8)
			if state := service.State(world, player); state.Phase != "approaching" {
				t.Fatalf("action did not approach: %#v", state)
			}
			if test.breakTarget {
				world.Despawn(target)
			} else {
				active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
				ecs.GetResource[ecs.TimeState](world).UnixMs = active.ExpireAtUnixMs
			}
			system.Update(world, 0)
			if state := service.State(world, player); state.Phase != "selecting" {
				t.Fatalf("approach did not return to selection: %#v", state)
			}
			if len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != test.reason || handler.canceled != 1 {
				t.Fatalf("approach failure was not reported or canceled: %#v", sender.alerts)
			}
		})
	}
}

func TestActionValidationSystemCancelsTimedCycleWithoutStaminaCost(t *testing.T) {
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CharacterProfile{Skills: []string{"woodcutting"}})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 10})
	})
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{
		ID: "test_none", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetNone},
		Requirements: actiondefs.Requirements{Skills: []string{"woodcutting"}},
		Execution:    actiondefs.Execution{Ticks: 3, Stamina: 2},
	}})
	handler := &testActionHandler{status: ActionSucceeded}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_none": handler}, &testActionSender{})
	if err != nil {
		t.Fatal(err)
	}
	system := NewActionValidationSystem(world, service)
	service.Activate(world, 1, player, "test_none")
	ecs.WithComponent(world, player, func(profile *components.CharacterProfile) { profile.Skills = nil })
	system.Update(world, 0)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	_, cycleActive := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	if service.State(world, player).Phase != "idle" || cycleActive || handler.startCount != 0 || stats.Stamina != 10 {
		t.Fatal("requirement loss did not cancel timed action without effect or stamina cost")
	}
}
