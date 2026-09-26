package game

import (
	"fmt"
	"testing"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestDirectActionNeverSelectsOrRepeats(t *testing.T) {
	for _, outcome := range []ActionOutcome{ActionSucceeded, ActionRejected, ActionFailed, ActionApproaching, ActionDeferred} {
		t.Run(fmt.Sprintf("outcome_%d", outcome), func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			player := world.Spawn(1, nil)
			repeatable := true
			definitions := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "direct", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile, Cursor: "cursor"}, IsRepeatable: &repeatable}})
			handler, sender := &testActionHandler{status: outcome}, &testActionSender{}
			actions, err := NewActionService(world, definitions, map[string]ActionHandler{"direct": handler}, sender)
			if err != nil {
				t.Fatal(err)
			}
			actions.StartTargetedOnce(world, 1, player, "direct", 0, types.InvalidHandle, 43, 99)
			if outcome == ActionApproaching || outcome == ActionDeferred {
				active, ok := ecs.GetComponent[components.ActiveGameAction](world, player)
				if !ok || active.Generation == 0 || !active.DirectAttempt || active.TargetX != 43 || active.TargetY != 99 {
					t.Fatalf("direct state missing: %+v", active)
				}
				if len(sender.states) != 1 || sender.states[0].Cursor != "" {
					t.Fatal("direct request did not publish only its stable phase")
				}
				actions.Complete(world, 1, player, active.Generation, outcome == ActionDeferred, "")
			}
			if actions.State(world, player).Phase != "idle" || handler.startCount != 1 {
				t.Fatal("direct request did not end after one attempt")
			}
			for _, state := range sender.states {
				if state.Phase == "selecting" || state.Cursor != "" {
					t.Fatalf("direct request armed selection: %+v", state)
				}
			}
		})
	}
}

func TestDirectActionValidatesBeforeStarting(t *testing.T) {
	for _, reason := range []string{"ACTION_UNKNOWN", "ACTION_UNAVAILABLE", "BAD_TARGET"} {
		t.Run(reason, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			player := world.Spawn(1, nil)
			definitions := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "direct", SourceFile: "test.json", Target: actiondefs.Target{Kind: actiondefs.TargetTile}}})
			handler, sender := &testActionHandler{status: ActionSucceeded}, &testActionSender{}
			actions, err := NewActionService(world, definitions, map[string]ActionHandler{"direct": handler}, sender)
			if err != nil {
				t.Fatal(err)
			}
			id := "direct"
			switch reason {
			case "ACTION_UNKNOWN":
				id = "missing"
			case "ACTION_UNAVAILABLE":
				handler.unavailable = reason
			case "BAD_TARGET":
				handler.targetReason = func(ActionTarget) string { return reason }
			}
			actions.StartTargetedOnce(world, 1, player, id, 0, types.InvalidHandle, 43, 99)
			if handler.startCount != 0 || actions.State(world, player).Phase != "idle" || len(sender.alerts) != 1 || sender.alerts[0].ReasonCode != reason {
				t.Fatal("invalid direct request started or omitted its error")
			}
		})
	}
}
