package components

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEnsureRuntimeObjectState_PanicsOnNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic for nil ObjectInternalState")
		}
	}()

	_ = EnsureRuntimeObjectState(nil)
}

func TestGetBehaviorState_NoSideEffectsForValueState(t *testing.T) {
	internalState := ObjectInternalState{
		State: &RuntimeObjectState{
			Behaviors: map[string]any{
				"tree": TreeBehaviorState{ChopPoints: 3},
			},
		},
	}

	state, ok := GetRuntimeObjectState(internalState)
	if !ok {
		t.Fatalf("expected runtime state")
	}
	before := state.Behaviors["tree"]
	if _, found := GetBehaviorState[TreeBehaviorState](internalState, "tree"); found {
		t.Fatalf("expected typed pointer lookup to fail for value state")
	}
	after := state.Behaviors["tree"]
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected getter to avoid map mutation")
	}
}

func TestTreeBehaviorState_MarshalJSONWithDynamicTakenKeys(t *testing.T) {
	state := TreeBehaviorState{
		ChopPoints:     3,
		Stage:          2,
		NextGrowthTick: 40,
		Taken: map[string]int{
			"take_branch": 2,
			"take_bark":   1,
		},
	}

	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded["take_branch_taken"] != float64(2) {
		t.Fatalf("expected take_branch_taken=2, got %#v", decoded["take_branch_taken"])
	}
	if decoded["take_bark_taken"] != float64(1) {
		t.Fatalf("expected take_bark_taken=1, got %#v", decoded["take_bark_taken"])
	}
}

func TestTreeBehaviorState_UnmarshalJSONDynamicTakenKeys(t *testing.T) {
	var state TreeBehaviorState
	if err := json.Unmarshal([]byte(`{"chop_points":2,"stage":4,"take_branch_taken":5,"take_bark_taken":1}`), &state); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if state.ChopPoints != 2 || state.Stage != 4 {
		t.Fatalf("unexpected base fields: %+v", state)
	}
	if state.Taken["take_branch"] != 5 {
		t.Fatalf("expected take_branch=5, got %d", state.Taken["take_branch"])
	}
	if state.Taken["take_bark"] != 1 {
		t.Fatalf("expected take_bark=1, got %d", state.Taken["take_bark"])
	}
}

func TestTakeBehaviorState_MarshalJSONWithDynamicTakenKeys(t *testing.T) {
	state := TakeBehaviorState{
		Taken: map[string]int{
			"chip_stone": 3,
			"chip_flint": 1,
		},
	}

	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded["chip_stone_taken"] != float64(3) {
		t.Fatalf("expected chip_stone_taken=3, got %#v", decoded["chip_stone_taken"])
	}
	if decoded["chip_flint_taken"] != float64(1) {
		t.Fatalf("expected chip_flint_taken=1, got %#v", decoded["chip_flint_taken"])
	}
}

func TestTakeBehaviorState_UnmarshalJSONDynamicTakenKeys(t *testing.T) {
	var state TakeBehaviorState
	if err := json.Unmarshal([]byte(`{"chip_stone_taken":5,"chip_flint_taken":2}`), &state); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if state.Taken["chip_stone"] != 5 {
		t.Fatalf("expected chip_stone=5, got %d", state.Taken["chip_stone"])
	}
	if state.Taken["chip_flint"] != 2 {
		t.Fatalf("expected chip_flint=2, got %d", state.Taken["chip_flint"])
	}
}
