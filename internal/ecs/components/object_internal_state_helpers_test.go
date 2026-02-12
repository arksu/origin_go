package components

import "testing"

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
	if before != after {
		t.Fatalf("expected getter to avoid map mutation")
	}
}
