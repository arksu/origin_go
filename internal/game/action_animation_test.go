package game

import (
	"github.com/stretchr/testify/require"
	"origin/internal/actionanimationdefs"
	"origin/internal/actiondefs"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/types"
	"os"
	"testing"
	"time"
)

type animationCycleBehavior struct {
	key      string
	decision contracts.BehaviorCycleDecision
}

func (behavior *animationCycleBehavior) Key() string { return behavior.key }
func (behavior *animationCycleBehavior) OnCycleComplete(*contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	return behavior.decision
}

func animationFixture(t *testing.T) (*ecs.World, types.Handle, []actionanimationdefs.Definition) {
	t.Helper()
	contents, err := os.ReadFile("../../tests/fixtures/action_animations/bindings.json")
	require.NoError(t, err)
	definitions, err := actionanimationdefs.Parse(contents, "fixture.json")
	require.NoError(t, err)
	registry, err := actionanimationdefs.NewRegistry(definitions)
	require.NoError(t, err)
	previous := actionanimationdefs.Global()
	actionanimationdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { actionanimationdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldForTesting()
	handle := w.Spawn(101, nil)
	ecs.AddComponent(w, handle, components.Appearance{Resource: "player"})
	ecs.AddComponent(w, handle, components.Transform{})
	timing := ecs.GetResource[ecs.TimeState](w)
	timing.TickPeriod = 100 * time.Millisecond
	timing.UnixMs = 10000
	return w, handle, definitions
}

func TestCyclePresentationStartsAtEveryExecutionPhaseAndConfirmsSuccessors(t *testing.T) {
	for _, afterSystem := range []bool{false, true} {
		w, handle, bindings := animationFixture(t)
		binding := bindings[0]
		behavior := &animationCycleBehavior{key: binding.Source.Namespace, decision: contracts.BehaviorCycleDecisionContinue}
		service := NewContextActionService(w, nil, nil, nil, nil, nil, nil, nil, nil, testSingleBehaviorRegistry{behavior: behavior}, nil)
		system := NewCyclicActionSystem(service, nil, nil)
		if afterSystem {
			system.Update(w, 0)
		}
		cyclicaction.StartContext(w, handle, components.ActiveCyclicAction{BehaviorKey: binding.Source.Namespace, ActionID: binding.Source.ID, TargetKind: components.CyclicActionTargetSelf, CycleDurationTicks: 20, CycleIndex: 1})
		initial, err := cyclicaction.Snapshot(w, handle)
		require.NoError(t, err)
		require.Equal(t, binding.Key, initial.AnimationKey)
		require.Zero(t, initial.ElapsedTicks)
		for range 20 {
			system.Update(w, .1)
		}
		next, err := cyclicaction.Snapshot(w, handle)
		require.NoError(t, err)
		require.Equal(t, initial.Revision+1, next.Revision)
		require.Zero(t, next.ElapsedTicks)
		behavior.decision = contracts.BehaviorCycleDecisionComplete
		for range 20 {
			system.Update(w, .1)
		}
		idle, err := cyclicaction.Snapshot(w, handle)
		require.NoError(t, err)
		require.Empty(t, idle.AnimationKey)
		require.Equal(t, next.Revision+1, idle.Revision)
	}
}

func TestCancellationAndDeathCleanupClearPublicState(t *testing.T) {
	for _, reason := range []string{"link", "knockout", "death"} {
		t.Run(reason, func(t *testing.T) {
			w, handle, bindings := animationFixture(t)
			binding := bindings[0]
			cyclicaction.StartContext(w, handle, components.ActiveCyclicAction{BehaviorKey: binding.Source.Namespace, ActionID: binding.Source.ID, TargetKind: components.CyclicActionTargetObject, TargetID: 202, CycleDurationTicks: 20, MutatesItems: true})
			service := NewContextActionService(w, nil, nil, nil, nil, nil, nil, nil, nil, testSingleBehaviorRegistry{}, nil)
			switch reason {
			case "link":
				NewCyclicActionSystem(service, nil, nil).Update(w, .1)
			case "knockout":
				(&Shard{contextActions: service}).HandlePlayerIncapacitated(w, 101, handle)
			case "death":
				(&Shard{}).clearPlayerTransientStateForDeath(w, 101, handle)
			}
			idle, err := cyclicaction.Snapshot(w, handle)
			require.NoError(t, err)
			require.Empty(t, idle.AnimationKey)
			require.Equal(t, uint64(2), idle.Revision)
		})
	}
}

func TestMenuExecutionUsesDefinitionIdentityAndSharedRemoval(t *testing.T) {
	w, handle, bindings := animationFixture(t)
	binding := bindings[1]
	binding.Source.Kind = "menu"
	registry, err := actionanimationdefs.NewRegistry([]actionanimationdefs.Definition{binding})
	require.NoError(t, err)
	actionanimationdefs.SetGlobalForTesting(registry)
	actions := actiondefs.NewRegistry([]actiondefs.Definition{{ID: binding.Source.ID, Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Execution: actiondefs.Execution{Ticks: 20}}})
	sender := &testActionSender{}
	service, err := NewActionService(w, actions, map[string]ActionHandler{binding.Source.ID: &testActionHandler{status: ActionSucceeded}}, sender)
	require.NoError(t, err)
	service.Activate(w, 101, handle, binding.Source.ID)
	active, err := cyclicaction.Snapshot(w, handle)
	require.NoError(t, err)
	require.Equal(t, binding.Key, active.AnimationKey)
	service.Cancel(w, 101, handle)
	idle, err := cyclicaction.Snapshot(w, handle)
	require.NoError(t, err)
	require.Empty(t, idle.AnimationKey)
	require.Equal(t, active.Revision+1, idle.Revision)
}
