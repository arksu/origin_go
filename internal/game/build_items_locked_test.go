package game

import (
	"encoding/json"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type lockedBuildSender struct{ testActionSender }

func (*lockedBuildSender) SendBuildState(types.EntityID, *netproto.S2C_BuildState)             {}
func (*lockedBuildSender) SendBuildStateClosed(types.EntityID, *netproto.S2C_BuildStateClosed) {}
func (*lockedBuildSender) SendInventoryUpdate(types.EntityID, []*netproto.InventoryState)      {}

func TestBuildItemPathsRejectBeforeResourcesAndRetirePendingPlacement(t *testing.T) {
	for _, operation := range []string{"start", "progress", "take_back", "cycle", "pending"} {
		t.Run(operation, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			pending := components.PendingBuildPlacement{BuildKey: "campfire", TargetX: 42, TargetY: 10}
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityHealth{SHP: 5, HHP: 20, IsLying: true})
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 150, Energy: 900})
				ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 42})
				ecs.AddComponent(w, h, pending)
			})
			state := components.ObjectInternalState{}
			components.SetBehaviorState(&state, buildBehaviorStateKey, &components.BuildBehaviorState{Items: []components.BuildRequiredItemState{{ItemKey: "branch", RequiredCount: 2, PutItems: []components.BuildPutItemState{{ItemKey: "branch", Count: 1, Quality: 10}}}}})
			target := w.Spawn(2, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, state) })
			before, err := json.Marshal(state)
			require.NoError(t, err)
			sender := &lockedBuildSender{}
			service := &BuildService{world: w, alerts: sender}
			switch operation {
			case "start":
				service.HandleStartBuild(w, 1, player, &netproto.C2S_BuildStart{})
			case "progress":
				service.HandleBuildProgress(w, 1, player, &netproto.C2S_BuildProgress{EntityId: 2})
			case "take_back":
				service.HandleBuildTakeBack(w, 1, player, &netproto.C2S_BuildTakeBack{EntityId: 2})
			case "cycle":
				require.Equal(t, contracts.BehaviorCycleDecisionCanceled, service.HandleBuildCycleComplete(w, 1, player, components.ActiveCyclicAction{ActionID: buildSyntheticActionID, TargetID: 2, MutatesItems: true}))
			case "pending":
				service.FinalizePendingBuildPlacement(w, 1, player, pending)
			}
			require.Len(t, sender.alerts, 1)
			require.Equal(t, playerstate.ItemsLockedReason, sender.alerts[0].ReasonCode)
			current, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
			after, err := json.Marshal(current)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, 2, w.EntityCount())
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			require.Equal(t, 150.0, stats.Stamina)
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			require.Equal(t, constt.StateMoving, movement.State)
			require.Equal(t, 42.0, movement.TargetX)
			_, hasPending := ecs.GetComponent[components.PendingBuildPlacement](w, player)
			require.Equal(t, operation != "pending", hasPending)
		})
	}
}
