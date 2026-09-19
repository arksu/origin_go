package behaviors

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestBurnerBehaviorValidatesAndAppliesConfig(t *testing.T) {
	def := &objectdefs.ObjectDef{}
	_, err := (burnerBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{
		RawConfig: []byte(`{"fuelAbilities":["fuel"],"fuelCapacity":5,"secondsPerFuel":1440,"initialFuel":5,"onExhausted":{"dropItem":"ash","despawn":true}}`),
		Def:       def,
	})
	require.NoError(t, err)
	require.NotNil(t, def.BurnerConfig)
	require.Equal(t, []string{"fuel"}, def.BurnerConfig.FuelAbilities)
}

func TestBurnerBehaviorInitializesRuntimeStateOnSpawn(t *testing.T) {
	w := ecs.NewWorld(nil, 0)
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 100})
	h := w.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.ObjectInternalState{}) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, BurnerConfig: &objectdefs.BurnerBehaviorConfig{FuelCapacity: 5, SecondsPerFuel: 1440, InitialFuel: 5}}}))
	require.NoError(t, (burnerBehavior{}).InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 1, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonSpawn}))
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	burner, ok := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.True(t, ok)
	require.Equal(t, uint32(5), burner.Fuel)
	require.Equal(t, int64(1540), burner.NextFuelBurnAtRuntimeSecond)
}

func TestBurnerBehaviorRestoresMissingLegacyStateWithoutOverwritingPersistedState(t *testing.T) {
	w := ecs.NewWorld(nil, 0)
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 100})
	h := w.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.ObjectInternalState{}) })
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, BurnerConfig: &objectdefs.BurnerBehaviorConfig{FuelCapacity: 5, SecondsPerFuel: 1440, InitialFuel: 5}}}))
	b := burnerBehavior{}
	require.NoError(t, b.InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 1, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonRestore}))
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	burner, ok := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.True(t, ok)
	require.Equal(t, int64(1540), burner.NextFuelBurnAtRuntimeSecond)
	components.SetBehaviorState(&state, "burner", &components.BurnerBehaviorState{Fuel: 2, NextFuelBurnAtRuntimeSecond: 500})
	ecs.AddComponent(w, h, state)
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 600})
	require.NoError(t, b.InitObject(&contracts.BehaviorObjectInitContext{World: w, Handle: h, EntityID: 1, EntityType: 1, Reason: contracts.ObjectBehaviorInitReasonRestore}))
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	burner, _ = components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.Equal(t, uint32(1), burner.Fuel)
	require.Equal(t, int64(1940), burner.NextFuelBurnAtRuntimeSecond)
}

func TestBurnerBehaviorRefuelsFromHandAndCapsOverflow(t *testing.T) {
	w := ecs.NewWorld(nil, 0)
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 9, Key: "mixed", Abilities: map[string]uint32{"fuel": 1, "peat": 2}}}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, BurnerConfig: &objectdefs.BurnerBehaviorConfig{FuelAbilities: []string{"fuel", "peat"}, FuelCapacity: 5}}}))
	hand := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{OwnerID: 10, Kind: constt.InventoryHand, Items: []components.InvItem{{TypeID: 9}}})
	})
	player := w.Spawn(10, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryHand, OwnerID: 10, Handle: hand}}})
	})
	target := w.Spawn(11, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1})
		ecs.AddComponent(w, h, components.StationState{CurrentState: "unlit"})
		ecs.AddComponent(w, h, components.ObjectInternalState{State: &components.RuntimeObjectState{Behaviors: map[string]any{"burner": &components.BurnerBehaviorState{Fuel: 4}}}})
	})
	updates := 0
	result := (burnerBehavior{}).ExecuteAction(&contracts.BehaviorActionExecuteContext{World: w, PlayerID: 10, PlayerHandle: player, TargetHandle: target, ActionID: "add_fuel", Deps: &contracts.ExecutionDeps{InventoryUpdate: func(*ecs.World, types.EntityID, types.Handle) { updates++ }}})
	require.True(t, result.OK)
	handState, _ := ecs.GetComponent[components.InventoryContainer](w, hand)
	require.Empty(t, handState.Items)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
	require.True(t, state.IsDirty)
	burner, _ := components.GetBehaviorState[components.BurnerBehaviorState](state, "burner")
	require.Equal(t, uint32(5), burner.Fuel)
	station, _ := ecs.GetComponent[components.StationState](w, target)
	require.Equal(t, "burning", station.CurrentState)
	require.False(t, (burnerBehavior{}).ValidateAction(&contracts.BehaviorActionValidateContext{World: w, PlayerID: 10, PlayerHandle: player, TargetHandle: target, ActionID: "add_fuel"}).OK)
	require.Equal(t, 1, updates)
}

func TestBurnerBehaviorRejectsIncompatibleHandFuel(t *testing.T) {
	w := ecs.NewWorld(nil, 0)
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{DefID: 9, Key: "coal", Abilities: map[string]uint32{"coal": 1}}}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{DefID: 1, BurnerConfig: &objectdefs.BurnerBehaviorConfig{FuelAbilities: []string{"fuel"}}}}))
	hand := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryContainer{OwnerID: 10, Kind: constt.InventoryHand, Items: []components.InvItem{{TypeID: 9}}})
	})
	player := w.Spawn(10, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.InventoryOwner{Inventories: []components.InventoryLink{{Kind: constt.InventoryHand, OwnerID: 10, Handle: hand}}})
	})
	target := w.Spawn(11, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1}) })
	require.False(t, (burnerBehavior{}).ValidateAction(&contracts.BehaviorActionValidateContext{World: w, PlayerID: 10, PlayerHandle: player, TargetHandle: target, ActionID: "add_fuel"}).OK)
	handState, _ := ecs.GetComponent[components.InventoryContainer](w, hand)
	require.Len(t, handState.Items, 1)
}

func TestBurnerBehaviorRejectsInvalidCapacity(t *testing.T) {
	_, err := (burnerBehavior{}).ValidateAndApplyDefConfig(&contracts.BehaviorDefConfigContext{
		RawConfig: []byte(`{"fuelAbilities":["fuel"],"fuelCapacity":0,"secondsPerFuel":1440,"initialFuel":0,"onExhausted":{"dropItem":"ash","despawn":true}}`),
		Def:       &objectdefs.ObjectDef{},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "fuelCapacity")
}
