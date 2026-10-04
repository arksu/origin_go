package behaviors

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestItemBehaviorsRejectBeforeChangingPlayerOrObject(t *testing.T) {
	for _, knockout := range []bool{false, true} {
		for _, operation := range []string{"gather", "tree_take", "chop", "add_fuel"} {
			t.Run(operation+map[bool]string{false: "/lying", true: "/ko"}[knockout], func(t *testing.T) {
				w := ecs.NewWorldForTesting()
				health := components.EntityHealth{HHP: 20, SHP: 3, IsLying: true}
				if knockout {
					health.KOUntilUnixMs = 61000
				}
				player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, health)
					ecs.AddComponent(w, h, components.EntityStats{Stamina: 150, Energy: 1000})
					ecs.AddComponent(w, h, components.CharacterProfile{Experience: components.CharacterExperience{LP: 10}})
					ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 42})
				})
				tree := &components.TreeBehaviorState{Stage: 1, ChopPoints: 7}
				burner := &components.BurnerBehaviorState{Fuel: 4, NextFuelBurnAtTick: 100}
				objectState := components.ObjectInternalState{}
				components.SetBehaviorState(&objectState, "tree", tree)
				components.SetBehaviorState(&objectState, "burner", burner)
				objectState.IsDirty = false
				target := w.Spawn(2, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, objectState)
				})
				beforeTree, beforeBurner := *tree, *burner
				giveCalls := 0
				deps := &contracts.ExecutionDeps{GiveItem: func(*ecs.World, types.EntityID, types.Handle, string, uint32, uint32) contracts.GiveItemOutcome {
					giveCalls++
					return contracts.GiveItemOutcome{Success: true, GrantedCount: 1}
				}}
				actionID := map[string]string{"gather": "chip_stone", "tree_take": "take_branch", "chop": "chop", "add_fuel": "add_fuel"}[operation]
				ctx := &contracts.BehaviorActionExecuteContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target, ActionID: actionID, Deps: deps}
				var result contracts.BehaviorResult
				switch operation {
				case "gather":
					result = (takeBehavior{}).ExecuteAction(ctx)
				case "tree_take", "chop":
					result = (treeBehavior{}).ExecuteAction(ctx)
				case "add_fuel":
					result = (burnerBehavior{}).ExecuteAction(ctx)
				}
				require.False(t, result.OK)
				require.Equal(t, playerstate.ItemsLockedReason, result.ReasonCode)
				// An already started cycle must have the same guard even if GiveItem succeeds.
				cycle := &contracts.BehaviorCycleContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target, ActionID: actionID, Deps: deps}
				if operation == "gather" {
					require.Equal(t, contracts.BehaviorCycleDecisionCanceled, (takeBehavior{}).OnCycleComplete(cycle))
				} else if operation != "add_fuel" {
					require.Equal(t, contracts.BehaviorCycleDecisionCanceled, (treeBehavior{}).OnCycleComplete(cycle))
				}
				require.Zero(t, giveCalls)
				require.Equal(t, beforeTree, *tree)
				require.Equal(t, beforeBurner, *burner)
				state, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
				require.False(t, state.IsDirty)
				stats, _ := ecs.GetComponent[components.EntityStats](w, player)
				require.Equal(t, 150.0, stats.Stamina)
				movement, _ := ecs.GetComponent[components.Movement](w, player)
				require.Equal(t, constt.StateMoving, movement.State)
				require.Equal(t, 42.0, movement.TargetX)
				profile, _ := ecs.GetComponent[components.CharacterProfile](w, player)
				require.EqualValues(t, 10, profile.Experience.LP)
				require.Empty(t, profile.Discovery)
				_, active := ecs.GetComponent[components.ActiveCyclicAction](w, player)
				require.False(t, active)
			})
		}
	}
}
