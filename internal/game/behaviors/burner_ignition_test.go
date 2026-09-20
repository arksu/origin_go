package behaviors

import (
	"context"
	"fmt"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func newIgnitionTestWorld(t *testing.T, stamina float64) (*ecs.World, types.Handle, types.Handle) {
	t.Helper()
	previous := objectdefs.Global()
	t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 1, Key: "test-hearth", Resource: "test-hearth/unlit", BehaviorOrder: []string{"burner"},
		BurnerConfig: &objectdefs.BurnerBehaviorConfig{FuelCapacity: 5, InitialFuel: 5, TicksPerFuel: 10},
	}}))
	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{Tick: 100})
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityStats{Stamina: stamina})
		ecs.AddComponent(w, h, components.Movement{})
	})
	target := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1, Behaviors: []string{"burner"}})
		ecs.AddComponent(w, h, components.StationState{CurrentState: "unlit"})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.Appearance{})
	})
	require.NoError(t, (burnerBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{
		World: w, Handle: target, EntityID: 2, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonSpawn,
	}))
	return w, player, target
}

func TestBurnerIgnitionActionLifecycle(t *testing.T) {
	for _, stamina := range []float64{49, 50, 75} {
		t.Run(fmt.Sprintf("stamina_%g", stamina), func(t *testing.T) {
			w, player, target := newIgnitionTestWorld(t, stamina)
			b := burnerBehavior{}
			actions := b.ProvideActions(&contracts.BehaviorActionListContext{World: w, PlayerHandle: player, TargetHandle: target})
			require.Equal(t, []contracts.ContextAction{{ActionID: burnerLightAction, Title: "Light my fire"}}, actions)
			require.False(t, b.ValidateAction(&contracts.BehaviorActionValidateContext{World: w, PlayerHandle: player, TargetHandle: target, ActionID: "add_fuel"}).OK)
			execution := &contracts.BehaviorActionExecuteContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target, ActionID: burnerLightAction}
			require.True(t, b.ExecuteAction(execution).OK)
			require.False(t, b.ExecuteAction(execution).OK, "duplicate request cannot replace the active cycle")
			action, ok := ecs.GetComponent[components.ActiveCyclicAction](w, player)
			require.True(t, ok)
			require.EqualValues(t, 10, action.CycleDurationTicks)
			require.Equal(t, target, action.TargetHandle)
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			require.Equal(t, constt.StateInteracting, movement.State)
			before, _ := ecs.GetComponent[components.EntityStats](w, player)
			require.Equal(t, stamina, before.Stamina)
			cycle := &contracts.BehaviorCycleContext{World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target, ActionID: burnerLightAction}
			decision := b.OnCycleComplete(cycle)
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			state, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
			burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
			appearance, _ := ecs.GetComponent[components.Appearance](w, target)
			require.EqualValues(t, 5, burner.Fuel)
			if stamina < 50 {
				require.Equal(t, contracts.BehaviorCycleDecisionCanceled, decision)
				require.Equal(t, stamina, stats.Stamina)
				require.Zero(t, burner.NextFuelBurnAtTick)
				require.Equal(t, "test-hearth/unlit", appearance.Resource)
			} else {
				require.Equal(t, contracts.BehaviorCycleDecisionComplete, decision)
				require.Equal(t, stamina-50, stats.Stamina)
				require.EqualValues(t, 110, burner.NextFuelBurnAtTick)
				require.Equal(t, "test-hearth/burning", appearance.Resource)
				require.True(t, state.IsDirty)
				require.Equal(t, contracts.BehaviorCycleDecisionCanceled, b.OnCycleComplete(cycle), "second player cannot ignite the same burner again")
				require.False(t, b.ValidateAction(&contracts.BehaviorActionValidateContext{World: w, PlayerHandle: player, TargetHandle: target, ActionID: burnerLightAction}).OK)
			}
		})
	}
}

func TestBurnerUnarmedStateSurvivesRestore(t *testing.T) {
	w, _, target := newIgnitionTestWorld(t, 50)
	ecs.SetResource(w, ecs.TimeState{Tick: 1000000})
	require.NoError(t, (burnerBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{
		World: w, Handle: target, EntityID: 2, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonRestore,
	}))
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
	burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.EqualValues(t, 5, burner.Fuel)
	require.Zero(t, burner.NextFuelBurnAtTick)
	require.False(t, burner.OutcomeCreated)
}

func TestBurnerAppearanceAndStationEventsOnlyOnChange(t *testing.T) {
	w, _, target := newIgnitionTestWorld(t, 50)
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	appearances := make(chan types.EntityID, 4)
	bus.SubscribeAsync(ecs.TopicGameplayEntityAppearance, eventbus.PriorityMedium, func(_ context.Context, e eventbus.Event) error {
		appearances <- e.(*ecs.EntityAppearanceChangedEvent).TargetID
		return nil
	})
	stationChanges := 0
	bus.SubscribeSync(ecs.TopicGameplayStationStateChanged, eventbus.PriorityMedium, func(_ context.Context, e eventbus.Event) error {
		stationChanges++
		return nil
	})
	deps := &contracts.ExecutionDeps{EventBus: bus}
	setBurnerStationState(w, target, "burning", deps)
	setBurnerStationState(w, target, "burning", deps)
	setBurnerStationState(w, target, "unlit", deps)
	setBurnerStationState(w, target, "unlit", deps)
	require.Equal(t, 2, stationChanges)
	for range 2 {
		select {
		case id := <-appearances:
			require.EqualValues(t, 2, id)
		case <-time.After(time.Second):
			t.Fatal("missing appearance event")
		}
	}
	require.NoError(t, bus.Shutdown(context.Background()))
	require.Empty(t, appearances)
}
