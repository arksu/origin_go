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

type plantingClickSpy struct {
	seed  bool
	calls int
	x, y  float64
}

func (spy *plantingClickSpy) CanPlantFromHand(*ecs.World, types.EntityID, types.Handle) bool {
	return spy.seed
}
func (spy *plantingClickSpy) StartPlanting(_ *ecs.World, _ types.EntityID, _ types.Handle, x, y float64) {
	spy.calls++
	spy.x, spy.y = x, y
}

func TestPlantingShortcutOnlyConsumesSeedOnSecondaryGroundClick(t *testing.T) {
	for _, scenario := range []string{"seed", "empty_hand", "primary", "stale", "object", "carrying", "incapacitated"} {
		t.Run(scenario, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.Movement{State: constt.StateIdle})
			})
			spy := &plantingClickSpy{seed: scenario != "empty_hand"}
			system := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, nil, 0, nil)
			system.SetSeedPlantingService(spy)
			click := &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY, X: 43, Y: 99}
			switch scenario {
			case "primary":
				click.Button = netproto.MapClickButton_MAP_CLICK_BUTTON_PRIMARY
			case "stale":
				click.TargetEntityId = 99
			case "object":
				click.TargetEntityId = 2
				w.Spawn(2, nil)
			case "carrying":
				ecs.AddComponent(w, player, components.LiftCarryState{ObjectEntityID: 3})
				system.SetLiftCommandService(testCarryStateService{})
			case "incapacitated":
				ecs.AddComponent(w, player, components.EntityHealth{HHP: 20, SHP: 5, IsLying: true})
			}
			system.handleMapClick(w, player, &network.PlayerCommand{CharacterID: 1, Payload: click})
			if scenario == "seed" {
				if spy.calls != 1 || spy.x != 43 || spy.y != 99 {
					t.Fatalf("seed shortcut not dispatched correctly: %+v", spy)
				}
			} else if spy.calls != 0 {
				t.Fatalf("planting took precedence for %s", scenario)
			}
		})
	}
}
