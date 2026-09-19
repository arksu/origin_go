package systems

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestBurnerSystemConsumesOverdueFuelByRuntimeSeconds(t *testing.T) {
	w := ecs.NewWorld(nil, 0)
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, BurnerConfig: &objectdefs.BurnerBehaviorConfig{SecondsPerFuel: 1440}}}))
	h := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1})
		ecs.AddComponent(w, h, components.ObjectInternalState{State: &components.RuntimeObjectState{Behaviors: map[string]any{"burner": &components.BurnerBehaviorState{Fuel: 3, NextFuelBurnAtRuntimeSecond: 1540}}}})
	})
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 2981})
	w.AddSystem(NewBurnerSystem())
	w.Update(0)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.Equal(t, uint32(1), burner.Fuel)
	require.Equal(t, int64(4420), burner.NextFuelBurnAtRuntimeSecond)
}

func TestBurnerSystemExtinguishesStationWhenFinalFuelBurns(t *testing.T) {
	w := ecs.NewWorld(nil, 0)
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 2, BurnerConfig: &objectdefs.BurnerBehaviorConfig{SecondsPerFuel: 10}}}))
	h := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 2})
		ecs.AddComponent(w, h, components.StationState{CurrentState: "burning"})
		ecs.AddComponent(w, h, components.ObjectInternalState{State: &components.RuntimeObjectState{Behaviors: map[string]any{"burner": &components.BurnerBehaviorState{Fuel: 1, NextFuelBurnAtRuntimeSecond: 10}}}})
	})
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 10})
	w.AddSystem(NewBurnerSystem())
	w.Update(0)
	station, _ := ecs.GetComponent[components.StationState](w, h)
	require.Equal(t, "unlit", station.CurrentState)
}

func TestBurnerSystemRetriesUnfinishedExhaustionOutcomeOnlyUntilItSucceeds(t *testing.T) {
	w := ecs.NewWorld(nil, 0)
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 3, BurnerConfig: &objectdefs.BurnerBehaviorConfig{SecondsPerFuel: 10, DropItem: "ash"}}}))
	h := w.Spawn(3, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 3})
		ecs.AddComponent(w, h, components.ObjectInternalState{State: &components.RuntimeObjectState{Behaviors: map[string]any{"burner": &components.BurnerBehaviorState{Fuel: 1, NextFuelBurnAtRuntimeSecond: 10}}}})
	})
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 10})
	attempts := 0
	w.AddSystem(NewBurnerSystem(func(_ *ecs.World, _ types.Handle, _ *objectdefs.BurnerBehaviorConfig) bool {
		attempts++
		return attempts == 2
	}))
	w.Update(0)
	w.Update(0)
	w.Update(0)

	require.Equal(t, 2, attempts)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.True(t, burner.OutcomeCreated)
}
