package stationreq

import (
	"testing"

	"origin/internal/craftdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestEvaluatorChecksStationWithoutMutation(t *testing.T) {
	world := ecs.NewWorldForTesting()
	stationID := types.EntityID(2)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.StationState{
			Capabilities: []string{"cooking"},
			CurrentState: "burning",
			Values:       map[string]float64{"temperature": 800},
			Resources:    map[string]uint32{"thread": 2},
		})
	})

	result := NewEvaluator().Evaluate(Context{World: world, StationID: stationID}, []craftdefs.StationRequirement{{
		Capability: "cooking",
		State:      "burning",
		Conditions: []craftdefs.StationCondition{{Source: "station", Kind: "value", Key: "temperature", Operator: "gte", Value: 600}},
		Consume:    []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}},
	}})

	if !result.Passed {
		t.Fatalf("evaluation failed: %s", result.FailureCode)
	}
	if got := result.Consumptions["thread"]; got != 1 {
		t.Fatalf("thread consumption = %d, want 1", got)
	}
	station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
	if station.Resources["thread"] != 2 {
		t.Fatal("read-only evaluation mutated station resources")
	}
}

func TestEvaluatorFailsClosedForUnknownProvider(t *testing.T) {
	result := NewEvaluator().Evaluate(Context{}, []craftdefs.StationRequirement{{
		Conditions: []craftdefs.StationCondition{{Source: "terrain", Kind: "water", Key: "water", Operator: "eq", Value: 1}},
	}})
	if result.Passed || result.FailureCode != FailureUnsupportedProvider {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestEvaluatorStationRequirements(t *testing.T) {
	world := ecs.NewWorldForTesting()
	stationID := types.EntityID(2)
	stationHandle := world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.StationState{
			Capabilities: []string{"cooking"},
			CurrentState: "burning",
			Values:       map[string]float64{"temperature": 800},
			Resources:    map[string]uint32{"thread": 2},
		})
	})

	tests := []struct {
		name         string
		requirements []craftdefs.StationRequirement
		wantCode     string
	}{
		{
			name:         "missing capability",
			requirements: []craftdefs.StationRequirement{{Capability: "smithing"}},
			wantCode:     FailureCapabilityMissing,
		},
		{
			name:         "wrong state",
			requirements: []craftdefs.StationRequirement{{State: "unlit"}},
			wantCode:     FailureStateMismatch,
		},
		{
			name: "malformed station condition",
			requirements: []craftdefs.StationRequirement{{Conditions: []craftdefs.StationCondition{{
				Source: "station", Kind: "unknown", Key: "temperature", Operator: "gte", Value: 600,
			}}}},
			wantCode: FailureConditionFailed,
		},
		{
			name: "aggregate resource shortage",
			requirements: []craftdefs.StationRequirement{
				{Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 2}}},
				{Consume: []craftdefs.StationResourceConsumption{{ResourceKey: "thread", Amount: 1}}},
			},
			wantCode: FailureResourceMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := NewEvaluator().Evaluate(Context{World: world, StationID: stationID}, test.requirements)
			if result.Passed || result.FailureCode != test.wantCode {
				t.Fatalf("unexpected result: %#v", result)
			}
			station, _ := ecs.GetComponent[components.StationState](world, stationHandle)
			if station.Resources["thread"] != 2 {
				t.Fatal("evaluation mutated station resources")
			}
		})
	}
}

func TestEvaluatorStationValueOperators(t *testing.T) {
	world := ecs.NewWorldForTesting()
	stationID := types.EntityID(3)
	world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.StationState{Values: map[string]float64{"temperature": 800}})
	})

	tests := []struct {
		name     string
		operator string
		value    float64
		passed   bool
	}{
		{name: "equal", operator: "eq", value: 800, passed: true},
		{name: "greater than or equal", operator: "gte", value: 600, passed: true},
		{name: "less than or equal", operator: "lte", value: 800, passed: true},
		{name: "failing comparison", operator: "lte", value: 799, passed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := NewEvaluator().Evaluate(Context{World: world, StationID: stationID}, []craftdefs.StationRequirement{{
				Conditions: []craftdefs.StationCondition{{
					Source: "station", Kind: "value", Key: "temperature", Operator: test.operator, Value: test.value,
				}},
			}})
			if result.Passed != test.passed {
				t.Fatalf("passed = %v, want %v: %#v", result.Passed, test.passed, result)
			}
		})
	}
}

func TestEvaluatorSupportsRegisteredFutureProvider(t *testing.T) {
	result := NewEvaluator(alwaysPassingProvider{}).Evaluate(Context{}, []craftdefs.StationRequirement{{
		Conditions: []craftdefs.StationCondition{{Source: "operator", Kind: "skill", Key: "cooking", Operator: "gte", Value: 1}},
	}})
	if !result.Passed {
		t.Fatalf("registered provider did not run: %#v", result)
	}
}

func TestEvaluatorRejectsNestedStationCycleAndDepthOverflow(t *testing.T) {
	world := ecs.NewWorldForTesting()
	stationID := types.EntityID(2)
	world.Spawn(stationID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.StationState{Capabilities: []string{"cooking"}})
	})

	evaluator := NewEvaluator()
	evaluator.Register(sameStationProvider{evaluator: evaluator})
	cycleResult := evaluator.Evaluate(Context{World: world, StationID: stationID, MaxDepth: 2}, []craftdefs.StationRequirement{{
		Conditions: []craftdefs.StationCondition{{Source: "nearby", Kind: "station", Key: "same", Operator: "eq", Value: 1}},
	}})
	if cycleResult.Passed || cycleResult.FailureCode != FailureDependencyCycle {
		t.Fatalf("expected cycle rejection, got %#v", cycleResult)
	}

	depthResult := evaluator.Evaluate(Context{World: world, StationID: stationID, Depth: 3, MaxDepth: 2}, []craftdefs.StationRequirement{{Capability: "cooking"}})
	if depthResult.Passed || depthResult.FailureCode != FailureDepthExceeded {
		t.Fatalf("expected depth rejection, got %#v", depthResult)
	}
}

type alwaysPassingProvider struct{}

func (alwaysPassingProvider) Source() string { return "operator" }

func (alwaysPassingProvider) Evaluate(Context, craftdefs.StationCondition) Evaluation {
	return Evaluation{Passed: true}
}

type sameStationProvider struct {
	evaluator *Evaluator
}

func (sameStationProvider) Source() string { return "nearby" }

func (p sameStationProvider) Evaluate(ctx Context, _ craftdefs.StationCondition) Evaluation {
	return p.evaluator.Evaluate(Context{
		World:           ctx.World,
		StationID:       ctx.StationID,
		VisitedEntities: ctx.VisitedEntities,
		Depth:           ctx.Depth + 1,
		MaxDepth:        ctx.MaxDepth,
	}, []craftdefs.StationRequirement{{Capability: "cooking"}})
}
