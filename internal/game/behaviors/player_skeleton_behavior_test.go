package behaviors

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/playerstate"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func skeletonActionFixture(t *testing.T) (*ecs.World, types.Handle, types.Handle) {
	t.Helper()
	previous := objectdefs.Global()
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{
		{DefID: 17, Key: "player_skeleton"},
		{DefID: 18, Key: "player_skeleton_without_skull"},
	}))
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, nil)
	ecs.AddComponent(w, player, components.EntityHealth{HHP: 100, SHP: 100})
	ecs.AddComponent(w, player, components.EntityStats{Stamina: 150})
	skeleton := w.Spawn(2, nil)
	ecs.AddComponent(w, skeleton, components.EntityInfo{TypeID: 17})
	return w, player, skeleton
}

func TestSkeletonActionDelegatesOnceWithoutCycleOrStaminaCost(t *testing.T) {
	w, player, skeleton := skeletonActionFixture(t)
	behavior := playerSkeletonBehavior{}
	list := &contracts.BehaviorActionListContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: skeleton}
	require.Equal(t, []contracts.ContextAction{{ActionID: "take_skull", Title: "Take Skull"}}, behavior.ProvideActions(list))
	require.True(t, behavior.RequiresItemMutation(actionTakeSkull))
	calls := 0
	want := contracts.BehaviorResult{OK: true}
	deps := &contracts.ExecutionDeps{TakeSkull: func(actual *ecs.World, playerID types.EntityID, ph types.Handle, targetID types.EntityID, th types.Handle) contracts.BehaviorResult {
		calls++
		require.Same(t, w, actual)
		require.EqualValues(t, 1, playerID)
		require.Equal(t, player, ph)
		require.EqualValues(t, 2, targetID)
		require.Equal(t, skeleton, th)
		return want
	}}
	ctx := &contracts.BehaviorActionExecuteContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: skeleton, ActionID: actionTakeSkull, Deps: deps}
	require.Equal(t, want, behavior.ExecuteAction(ctx))
	require.Equal(t, 1, calls)
	stats, _ := ecs.GetComponent[components.EntityStats](w, player)
	require.Equal(t, 150.0, stats.Stamina)
	_, cycling := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	require.False(t, cycling)

	// The service's durable completion swaps the def; stale menu actions stop here.
	ecs.WithComponent(w, skeleton, func(info *components.EntityInfo) { info.TypeID = 18 })
	require.Empty(t, behavior.ProvideActions(list))
	require.False(t, behavior.ExecuteAction(ctx).OK)
	require.Equal(t, 1, calls)
}

func TestSkeletonActionRechecksTargetIdentityAndQuarantine(t *testing.T) {
	for _, scenario := range []string{"headless", "wrong_id", "stale_handle", "quarantined", "missing_registry"} {
		t.Run(scenario, func(t *testing.T) {
			w, player, skeleton := skeletonActionFixture(t)
			id := types.EntityID(2)
			switch scenario {
			case "headless":
				ecs.WithComponent(w, skeleton, func(info *components.EntityInfo) { info.TypeID = 18 })
			case "wrong_id":
				id = 3
			case "stale_handle":
				w.Despawn(skeleton)
				replacement := w.Spawn(2, nil)
				ecs.AddComponent(w, replacement, components.EntityInfo{TypeID: 17})
			case "quarantined":
				ecs.SetResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{skeleton: true}})
			case "missing_registry":
				objectdefs.SetGlobalForTesting(nil)
			}
			behavior := playerSkeletonBehavior{}
			require.Empty(t, behavior.ProvideActions(&contracts.BehaviorActionListContext{World: w, TargetID: id, TargetHandle: skeleton}))
			require.False(t, behavior.ValidateAction(&contracts.BehaviorActionValidateContext{World: w, ActionID: actionTakeSkull, TargetID: id, TargetHandle: skeleton}).OK)
			calls := 0
			deps := &contracts.ExecutionDeps{TakeSkull: func(*ecs.World, types.EntityID, types.Handle, types.EntityID, types.Handle) contracts.BehaviorResult {
				calls++
				return contracts.BehaviorResult{OK: true}
			}}
			require.False(t, behavior.ExecuteAction(&contracts.BehaviorActionExecuteContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: id, TargetHandle: skeleton, ActionID: actionTakeSkull, Deps: deps}).OK)
			require.Zero(t, calls)
		})
	}
}

func TestSkeletonActionLockedPlayerAndMissingService(t *testing.T) {
	w, player, skeleton := skeletonActionFixture(t)
	behavior := playerSkeletonBehavior{}
	ctx := &contracts.BehaviorActionExecuteContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: skeleton, ActionID: actionTakeSkull}
	result := behavior.ExecuteAction(ctx)
	require.Equal(t, "TAKE_SKULL_UNAVAILABLE", result.ReasonCode)
	require.True(t, result.UserVisible)
	ecs.WithComponent(w, player, func(health *components.EntityHealth) { health.IsLying = true })
	result = behavior.ExecuteAction(ctx)
	require.Equal(t, playerstate.ItemsLockedReason, result.ReasonCode)
	require.False(t, result.OK)
	require.True(t, result.UserVisible)
}
