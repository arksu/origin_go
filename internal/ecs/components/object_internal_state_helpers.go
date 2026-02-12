package components

func EnsureRuntimeObjectState(internalState *ObjectInternalState) *RuntimeObjectState {
	if internalState == nil {
		panic("object internal state must not be nil")
	}

	if state, ok := internalState.State.(*RuntimeObjectState); ok && state != nil {
		if state.Behaviors == nil {
			state.Behaviors = make(map[string]any)
		}
		return state
	}

	state := &RuntimeObjectState{
		Behaviors: make(map[string]any),
	}
	internalState.State = state
	return state
}

func GetRuntimeObjectState(internalState ObjectInternalState) (*RuntimeObjectState, bool) {
	state, ok := internalState.State.(*RuntimeObjectState)
	if !ok || state == nil {
		return nil, false
	}
	return state, true
}

func GetBehaviorState[T any](internalState ObjectInternalState, behaviorKey string) (*T, bool) {
	state, ok := GetRuntimeObjectState(internalState)
	if !ok || state.Behaviors == nil || behaviorKey == "" {
		return nil, false
	}

	value, has := state.Behaviors[behaviorKey]
	if !has || value == nil {
		return nil, false
	}

	typed, castOK := value.(*T)
	return typed, castOK
}

func SetBehaviorState(internalState *ObjectInternalState, behaviorKey string, value any) bool {
	if internalState == nil || behaviorKey == "" || value == nil {
		return false
	}
	state := EnsureRuntimeObjectState(internalState)
	state.Behaviors[behaviorKey] = value
	internalState.IsDirty = true
	return true
}

func DeleteBehaviorState(internalState *ObjectInternalState, behaviorKey string) bool {
	if internalState == nil || behaviorKey == "" {
		return false
	}
	state := EnsureRuntimeObjectState(internalState)
	if _, has := state.Behaviors[behaviorKey]; !has {
		return false
	}
	delete(state.Behaviors, behaviorKey)
	internalState.IsDirty = true
	return true
}
