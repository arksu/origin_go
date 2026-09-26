package game

import (
	"context"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func newTileApproachTest(t *testing.T) (*ecs.World, types.Handle, *ActionService, *testActionHandler, *testActionSender) {
	t.Helper()
	world := ecs.NewWorldForTesting()
	player := world.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 1, Y: 1})
		ecs.AddComponent(w, h, components.Movement{Speed: constt.PlayerSpeed, Mode: constt.Walk})
		ecs.AddComponent(w, h, components.EntityStats{Stamina: 250})
	})
	repeat := true
	registry := actiondefs.NewRegistry([]actiondefs.Definition{{ID: "test_tile", Target: actiondefs.Target{Kind: actiondefs.TargetTile, Cursor: "dig", Approach: actiondefs.ApproachTileCenter}, IsRepeatable: &repeat, Execution: actiondefs.Execution{Ticks: 20, Stamina: 250}}})
	handler, sender := &testActionHandler{status: ActionSucceeded}, &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{"test_tile": handler}, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.Activate(world, 1, player, "test_tile")
	return world, player, service, handler, sender
}

func TestTileCenterNormalizationAndApproach(t *testing.T) {
	for _, test := range []struct{ x, y, cx, cy float64 }{{0, 0, 6, 6}, {11, 11, 6, 6}, {-1, -12, -6, -6}, {-13, 12, -18, 18}} {
		world, player, service, _, _ := newTileApproachTest(t)
		object := world.Spawn(2, nil)
		if !service.HandleArmedClick(world, 1, player, 2, object, test.x, test.y) {
			t.Fatal("unconsumed tile click")
		}
		active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
		movement, _ := ecs.GetComponent[components.Movement](world, player)
		if active.Phase != components.GameActionApproaching || active.TargetX != test.cx || active.TargetY != test.cy || active.TargetID != 0 || movement.TargetX != test.cx || movement.TargetY != test.cy {
			t.Fatalf("wrong tile approach: %#v, %#v", active, movement)
		}
		if _, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
			t.Fatal("cycle started before arrival")
		}
	}
}

func TestTileApproachRetargetsAndPreservesRejectedTarget(t *testing.T) {
	world, player, service, handler, sender := newTileApproachTest(t)
	service.HandleArmedClick(world, 1, player, 0, 0, 100, 1)
	if !service.HandleArmedClick(world, 1, player, 0, 0, 200, 1) {
		t.Fatal("retarget fell through")
	}
	active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
	if active.TargetX != 198 {
		t.Fatalf("wrong retarget: %#v", active)
	}
	handler.targetReason = func(ActionTarget) string { return "BAD_TERRAIN" }
	movement, _ := ecs.GetComponent[components.Movement](world, player)
	if !service.HandleArmedClick(world, 1, player, 0, 0, 300, 1) {
		t.Fatal("invalid retarget fell through")
	}
	after, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
	afterMovement, _ := ecs.GetComponent[components.Movement](world, player)
	if after != active || afterMovement != movement || sender.alerts[len(sender.alerts)-1].ReasonCode != "BAD_TERRAIN" {
		t.Fatal("invalid retarget changed original approach")
	}
	handler.targetReason = nil
	service.HandleArmedClick(world, 1, player, 0, 0, 6, 6)
	if service.State(world, player).Phase != "approaching" {
		t.Fatal("same tile skipped center")
	}
	ecs.WithComponent(world, player, func(p *components.Transform) { p.X, p.Y = 6, 6 })
	service.HandleArmedClick(world, 1, player, 0, 0, 1, 1)
	if service.State(world, player).Phase != "executing" {
		t.Fatal("retarget at center did not start cycle")
	}
}

func TestRepeatableTileLosesAffordabilityDuringApproach(t *testing.T) {
	world, player, service, handler, _ := newTileApproachTest(t)
	service.HandleArmedClick(world, 1, player, 0, 0, 100, 1)
	ecs.WithComponent(world, player, func(s *components.EntityStats) { s.Stamina = 249 })
	service.Recheck(world, 1, player)
	movement, _ := ecs.GetComponent[components.Movement](world, player)
	if service.State(world, player).Phase != "selecting" || movement.TargetType != constt.TargetNone || handler.startCount != 0 {
		t.Fatal("stamina loss did not stop attempt and rearm")
	}
	handler.unavailable = "ACTION_UNAVAILABLE"
	service.Recheck(world, 1, player)
	if service.State(world, player).Phase != "idle" {
		t.Fatal("low stamina masked non-stamina requirement loss")
	}
}

func TestTileArrivalTimeoutAndCompletionPosition(t *testing.T) {
	for _, outcome := range []string{"success", "blocked", "timeout", "away", "returned", "low stamina"} {
		t.Run(outcome, func(t *testing.T) {
			world, player, service, handler, sender := newTileApproachTest(t)
			service.HandleArmedClick(world, 1, player, 0, 0, 1000, 1)
			active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			if active.ExpireAtUnixMs <= ecs.GetResource[ecs.TimeState](world).UnixMs+15000 {
				t.Fatal("distant approach kept fixed timeout")
			}
			if outcome == "timeout" {
				ecs.GetResource[ecs.TimeState](world).UnixMs = active.ExpireAtUnixMs
				service.Recheck(world, 1, player)
			} else {
				actualX := active.TargetX
				if outcome == "blocked" {
					actualX -= 1
				}
				ecs.WithComponent(world, player, func(p *components.Transform) { p.X, p.Y = actualX, active.TargetY })
				if err := service.onPointMovementStopped(context.Background(), &ecs.PointMovementStoppedEvent{Layer: world.Layer, EntityID: 1, X: actualX, Y: active.TargetY, TargetX: active.TargetX, TargetY: active.TargetY}); err != nil {
					t.Fatal(err)
				}
				if outcome != "blocked" {
					if service.State(world, player).Phase != "executing" {
						t.Fatal("arrival did not start cycle")
					}
					if service.HandleArmedClick(world, 1, player, 0, 0, 2000, 1) {
						t.Fatal("executing click was queued/consumed")
					}
					if outcome == "away" || outcome == "returned" {
						ecs.WithComponent(world, player, func(p *components.Transform) { p.X += 1 })
						service.Recheck(world, 1, player)
						if service.State(world, player).Phase != "executing" {
							t.Fatal("departure canceled cycle")
						}
						if outcome == "returned" {
							ecs.WithComponent(world, player, func(p *components.Transform) { p.X -= 1 })
						}
					}
					if outcome == "low stamina" {
						ecs.WithComponent(world, player, func(s *components.EntityStats) { s.Stamina = 249 })
					}
					for tick := 0; tick < 20; tick++ {
						if cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player); exists {
							service.AdvanceCycle(world, 1, player, cycle, sender)
						}
					}
				}
			}
			wantStarts, wantStamina := 0, float64(250)
			if outcome == "success" || outcome == "returned" {
				wantStarts, wantStamina = 1, 0
			}
			if outcome == "low stamina" {
				wantStamina = 249
			}
			stats, _ := ecs.GetComponent[components.EntityStats](world, player)
			if handler.startCount != wantStarts || stats.Stamina != wantStamina || service.State(world, player).Cursor != "dig" {
				t.Fatalf("outcome: starts=%d stamina=%v state=%#v", handler.startCount, stats.Stamina, service.State(world, player))
			}
			service.Recheck(world, 1, player)
			if service.State(world, player).Phase != "selecting" {
				t.Fatal("recheck disarmed repeat action")
			}
			if wantStamina < 250 {
				movement, _ := ecs.GetComponent[components.Movement](world, player)
				service.HandleArmedClick(world, 1, player, 0, 0, 3000, 0)
				after, _ := ecs.GetComponent[components.Movement](world, player)
				if movement != after || service.State(world, player).Phase != "selecting" || sender.alerts[len(sender.alerts)-1].ReasonCode != "LOW_STAMINA" {
					t.Fatal("unaffordable click moved or disarmed")
				}
			}
		})
	}
}

func TestPlowCatalogUsesAbsoluteCost(t *testing.T) {
	registry, err := actiondefs.LoadFromDirectory(filepath.Join("..", "..", "data", "actions"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	handlers := map[string]ActionHandler{}
	for _, definition := range registry.All() {
		handlers[definition.ID] = &testActionHandler{}
	}
	sender := &testActionSender{}
	service, err := NewActionService(ecs.NewWorldForTesting(), registry, handlers, sender)
	if err != nil {
		t.Fatal(err)
	}
	service.SendList(1)
	for _, entry := range sender.lists[0].Actions {
		if entry.Id == "plow_tile" {
			if entry.Stamina != 250 || entry.Ticks != 20 || entry.Cursor != "dig" || !entry.IsRepeatable || len(entry.RequiredSkills) != 0 || len(entry.RequiredEquipment) != 0 {
				t.Fatalf("catalog: %#v", entry)
			}
			return
		}
	}
	t.Fatal("plow absent from catalog")
}
