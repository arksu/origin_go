package systems

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestStationSystemConsumesFuelAndMarksObjectDirty(t *testing.T) {
	world := ecs.NewWorldForTesting()
	handle := world.Spawn(types.EntityID(1), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.StationState{
			CurrentState: "burning",
			Resources:    map[string]uint32{"fuel": 1},
			AutonomousConsumption: []components.StationAutonomousConsumption{{
				ResourceKey:       "fuel",
				AmountPerTick:     1,
				RequiredState:     "burning",
				StateWhenDepleted: "unlit",
			}},
		})
	})

	world.AddSystem(NewStationSystem(nil))
	world.Update(0)

	station, ok := ecs.GetComponent[components.StationState](world, handle)
	if !ok {
		t.Fatal("station state missing")
	}
	if station.Resources["fuel"] != 0 || station.CurrentState != "unlit" {
		t.Fatalf("unexpected station after tick: %#v", station)
	}
	objectState, ok := ecs.GetComponent[components.ObjectInternalState](world, handle)
	if !ok || !objectState.IsDirty {
		t.Fatal("station mutation did not mark object state dirty")
	}
}
