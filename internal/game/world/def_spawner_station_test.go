package world

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestSpawnEntityFromDef_InitializesAndRestoresStationState(t *testing.T) {
	world := ecs.NewWorldForTesting()
	stationDef := &objectdefs.ObjectDef{
		DefID: 1,
		Key:   "campfire",
		Name:  "Campfire",
		Station: &objectdefs.StationDef{
			Capabilities: []string{"cooking"},
			States:       []string{"unlit", "burning"},
			InitialState: "unlit",
			Values:       map[string]float64{"temperature": 20},
			Resources:    []objectdefs.StationResourceDef{{Key: "fuel", Amount: 5}},
			AutonomousConsumption: []objectdefs.StationAutonomousConsumption{{
				ResourceKey:       "fuel",
				AmountPerTick:     1,
				RequiredState:     "burning",
				StateWhenDepleted: "unlit",
			}},
		},
	}

	freshHandle := SpawnEntityFromDef(world, stationDef, DefSpawnParams{EntityID: types.EntityID(1)})
	require.NotEqual(t, types.InvalidHandle, freshHandle)
	freshState, ok := ecs.GetComponent[components.StationState](world, freshHandle)
	require.True(t, ok)
	require.Equal(t, "unlit", freshState.CurrentState)
	require.Equal(t, float64(20), freshState.Values["temperature"])
	require.Equal(t, uint32(5), freshState.Resources["fuel"])
	require.True(t, freshState.HasCapability("cooking"))
	require.Len(t, freshState.AutonomousConsumption, 1)

	nonStationHandle := SpawnEntityFromDef(world, &objectdefs.ObjectDef{DefID: 2, Key: "rock", Name: "Rock"}, DefSpawnParams{EntityID: types.EntityID(2)})
	_, hasStationState := ecs.GetComponent[components.StationState](world, nonStationHandle)
	require.False(t, hasStationState)

	ecs.MutateComponent[components.ObjectInternalState](world, freshHandle, func(state *components.ObjectInternalState) bool {
		state.State = &components.RuntimeObjectState{Station: &components.StationPersistentState{
			CurrentState: "burning",
			Values:       map[string]float64{"temperature": 800},
			Resources:    map[string]uint32{"fuel": 2},
		}}
		return true
	})
	(&ObjectFactory{}).RestoreDerivedComponentsFromState(world, freshHandle)

	restoredState, ok := ecs.GetComponent[components.StationState](world, freshHandle)
	require.True(t, ok)
	require.Equal(t, "burning", restoredState.CurrentState)
	require.Equal(t, float64(800), restoredState.Values["temperature"])
	require.Equal(t, uint32(2), restoredState.Resources["fuel"])
	require.True(t, restoredState.HasCapability("cooking"))
	require.Len(t, restoredState.AutonomousConsumption, 1)
}
