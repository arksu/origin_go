package behaviors

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestBurnerScheduledTicksIgnoreRuntimeSecondsAndCatchUp(t *testing.T) {
	w, player, target := newIgnitionTestWorld(t, 50)
	behavior := burnerBehavior{}
	require.Zero(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount(), "unlit burners have no scheduled work")
	require.Equal(t, contracts.BehaviorCycleDecisionComplete, behavior.OnCycleComplete(&contracts.BehaviorCycleContext{
		World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target, ActionID: burnerLightAction,
	}))
	require.Equal(t, 1, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
	system := systems.NewBehaviorTickSystem(nil, systems.BehaviorTickSystemConfig{BehaviorRegistry: MustDefaultRegistry()})
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
	burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	ecs.SetResource(w, ecs.TimeState{Tick: 109, RuntimeSecondsTotal: 999999})
	system.Update(w, 0)
	require.EqualValues(t, 5, burner.Fuel)
	ecs.SetResource(w, ecs.TimeState{Tick: 130, RuntimeSecondsTotal: 999999})
	system.Update(w, 0)
	require.EqualValues(t, 2, burner.Fuel, "three elapsed tick intervals consume three fuel units")
	require.EqualValues(t, 140, burner.NextFuelBurnAtTick)
	system.Update(w, 0)
	require.EqualValues(t, 2, burner.Fuel, "the same tick cannot consume fuel twice")
	ecs.GetResource[ecs.TimeState](w).Tick = 150
	system.Update(w, 0)
	require.Zero(t, burner.Fuel)
	require.True(t, burner.OutcomeCreated)
	require.Zero(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
	station, _ := ecs.GetComponent[components.StationState](w, target)
	require.Equal(t, "unlit", station.CurrentState)
	appearance, _ := ecs.GetComponent[components.Appearance](w, target)
	require.Equal(t, "test-hearth/unlit", appearance.Resource)
}

func TestBurnerScheduledExhaustionRetriesWithoutReburning(t *testing.T) {
	w, player, target := newIgnitionTestWorld(t, 50)
	def, _ := objectdefs.Global().GetByID(1)
	def.BurnerConfig.DropItem = "ash"
	require.Equal(t, contracts.BehaviorCycleDecisionComplete, (burnerBehavior{}).OnCycleComplete(&contracts.BehaviorCycleContext{
		World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target, ActionID: burnerLightAction,
	}))
	attempts := 0
	system := systems.NewBehaviorTickSystem(nil, systems.BehaviorTickSystemConfig{
		BehaviorRegistry: MustDefaultRegistry(),
		ExecutionDeps: &contracts.ExecutionDeps{ExhaustBurner: func(w *ecs.World, h types.Handle) bool {
			require.Equal(t, target, h)
			station, _ := ecs.GetComponent[components.StationState](w, h)
			require.Equal(t, "unlit", station.CurrentState)
			attempts++
			return attempts == 2
		}},
	})
	ecs.GetResource[ecs.TimeState](w).Tick = 150
	system.Update(w, 0)
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
	system.Update(w, 0)
	require.Equal(t, 1, attempts)
	ecs.GetResource[ecs.TimeState](w).Tick++
	system.Update(w, 0)
	require.Equal(t, 2, attempts)
	require.Zero(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
	ecs.GetResource[ecs.TimeState](w).Tick++
	system.Update(w, 0)
	require.Equal(t, 2, attempts)
}

func TestBurnerRestoreCatchesUpAndSchedulesOnlyRemainingWork(t *testing.T) {
	w, _, target := newIgnitionTestWorld(t, 50)
	ecs.WithComponent(w, target, func(state *components.ObjectInternalState) {
		components.SetBehaviorState(state, "burner", &components.BurnerBehaviorState{Fuel: 5, NextFuelBurnAtTick: 110})
	})
	setBurnerStationState(w, target, "burning", nil)
	ecs.GetResource[ecs.TimeState](w).Tick = 135
	ctx := &contracts.BehaviorObjectInitContext{World: w, Handle: target, EntityID: 2, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonRestore}
	require.NoError(t, (burnerBehavior{}).InitObject(ctx))
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
	burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.EqualValues(t, 2, burner.Fuel)
	require.EqualValues(t, 140, burner.NextFuelBurnAtTick)
	require.Equal(t, 1, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
	require.NoError(t, (burnerBehavior{}).InitObject(ctx))
	require.EqualValues(t, 2, burner.Fuel)
	require.Equal(t, 1, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
	ecs.GetResource[ecs.TimeState](w).Tick = 200
	require.NoError(t, (burnerBehavior{}).InitObject(ctx))
	require.Zero(t, burner.Fuel)
	station, _ := ecs.GetComponent[components.StationState](w, target)
	require.Equal(t, "unlit", station.CurrentState)
	require.Equal(t, 1, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount(), "pending exhaustion is retried after exposure if persistence fails")
	w.Despawn(target)
	require.Zero(t, ecs.GetResource[ecs.BehaviorTickSchedule](w).PendingCount())
}
