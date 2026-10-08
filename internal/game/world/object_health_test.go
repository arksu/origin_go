package world

import (
	"math"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestValidateObjectHP(t *testing.T) {
	for _, hp := range []float64{0, 0.6, 0.49, 81.0 / 13, float64(math.MaxInt32) + 100, math.MaxFloat64} {
		require.NoError(t, ValidateObjectHP(hp))
		require.Zero(t, testing.AllocsPerRun(1000, func() { _ = ValidateObjectHP(hp) }))
	}
	for _, hp := range []float64{-0.1, math.NaN(), math.Inf(-1), math.Inf(1)} {
		require.ErrorIs(t, ValidateObjectHP(hp), ErrInvalidObjectHP)
		require.Zero(t, testing.AllocsPerRun(1000, func() { _ = ValidateObjectHP(hp) }))
	}
}

func TestSetObjectHPObserverDirtyAndNoOp(t *testing.T) {
	w := ecs.NewWorldForTesting()
	h := w.Spawn(1, nil)
	payload := &components.RuntimeObjectState{Behaviors: map[string]any{"tree": &components.TreeBehaviorState{ChopPoints: 3}}}
	ecs.AddComponent(w, h, components.ObjectInternalState{HP: 100, HasHP: true, State: payload, Flags: []string{"tree.active"}})
	stateWrites := 0
	var observedState components.ObjectInternalState
	w.AddComponentObserver(components.ObjectInternalStateComponentID, func(observed types.Handle) {
		require.Equal(t, h, observed)
		state, ok := ecs.GetComponent[components.ObjectInternalState](w, observed)
		require.True(t, ok)
		require.True(t, state.HasHP)
		observedState = state
		stateWrites++
	})
	require.NoError(t, SetObjectHP(w, h, 0.6))
	state, ok := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, ok)
	require.Equal(t, 0.6, state.HP)
	require.True(t, state.HasHP)
	require.True(t, state.IsDirty)
	require.Same(t, payload, state.State)
	require.Equal(t, []string{"tree.active"}, state.Flags)
	require.Equal(t, 1, stateWrites, "HP and dirty flag must use one observer-aware write")
	require.Equal(t, 0.6, observedState.HP)
	require.True(t, observedState.IsDirty, "observer must see new HP and dirty flag together")

	// Saving may clear the flag; writing the same HP must not re-dirty it.
	ecs.WithComponent(w, h, func(state *components.ObjectInternalState) { state.IsDirty = false })
	stateWrites = 0
	require.NoError(t, SetObjectHP(w, h, 0.6))
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.False(t, state.IsDirty)
	require.Zero(t, stateWrites)
	before := state
	for _, hp := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		require.ErrorIs(t, SetObjectHP(w, h, hp), ErrInvalidObjectHP)
	}
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.Equal(t, before, state)
	require.Zero(t, stateWrites)
	require.NoError(t, SetObjectHP(w, h, 0))
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, state.HasHP, "zero is present health, not missing health")
	require.Zero(t, state.HP)
	require.True(t, state.IsDirty)
	require.Equal(t, 1, stateWrites)
}

func TestSetObjectHPRejectsMissingAndStaleTargets(t *testing.T) {
	w := ecs.NewWorldForTesting()
	h := w.Spawn(1, nil)
	require.ErrorIs(t, SetObjectHP(nil, h, 1), ErrEntityNotFound)
	require.ErrorIs(t, SetObjectHP(w, types.InvalidHandle, 1), ErrEntityNotFound)
	require.ErrorIs(t, SetObjectHP(w, h, 1), ErrObjectStateMissing)
	require.Nil(t, w.GetStorage(components.ObjectInternalStateComponentID), "failed lookup must not create a storage")
	ecs.AddComponent(w, h, components.ObjectInternalState{HP: 0.6, Flags: []string{"untouched"}})
	require.ErrorIs(t, SetObjectHP(w, h, 1), ErrObjectHealthMissing)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.Equal(t, 0.6, state.HP)
	require.False(t, state.HasHP)
	require.False(t, state.IsDirty)
	require.Equal(t, []string{"untouched"}, state.Flags)
	require.True(t, w.Despawn(h))
	replacement := w.Spawn(2, nil)
	ecs.AddComponent(w, replacement, components.ObjectInternalState{HP: 20, HasHP: true})
	require.ErrorIs(t, SetObjectHP(w, h, 0), ErrEntityNotFound)
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, replacement)
	require.Equal(t, 20.0, state.HP)
	require.False(t, state.IsDirty)

	// A valid setter call can repair invalid runtime HP for a persistence retry.
	ecs.WithComponent(w, replacement, func(state *components.ObjectInternalState) { state.HP = math.NaN() })
	require.NoError(t, SetObjectHP(w, replacement, 0.49))
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, replacement)
	require.Equal(t, 0.49, state.HP)
	require.True(t, state.HasHP)
}

func TestSetObjectHPAllocations(t *testing.T) {
	w := ecs.NewWorldForTesting()
	h := w.Spawn(1, nil)
	ecs.AddComponent(w, h, components.ObjectInternalState{HP: 1, HasHP: true})
	missingHealth := w.Spawn(2, nil)
	ecs.AddComponent(w, missingHealth, components.ObjectInternalState{})
	missingState := w.Spawn(3, nil)
	observerCalls := 0
	w.AddComponentObserver(components.ObjectInternalStateComponentID, func(types.Handle) { observerCalls++ })
	hp := 0.6
	require.Zero(t, testing.AllocsPerRun(1000, func() {
		hp = 1 - hp
		_ = SetObjectHP(w, h, hp)
	}))
	require.Equal(t, 1001, observerCalls, "every changed setter call must invoke exactly one state observer")
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
	for _, call := range []func() error{
		func() error { return SetObjectHP(w, h, state.HP) },
		func() error { return SetObjectHP(w, h, math.NaN()) },
		func() error { return SetObjectHP(w, missingHealth, 1) },
		func() error { return SetObjectHP(w, missingState, 1) },
		func() error { return SetObjectHP(w, types.InvalidHandle, 1) },
		func() error { return SetObjectHP(nil, h, 1) },
	} {
		require.Zero(t, testing.AllocsPerRun(1000, func() { _ = call() }))
	}
	require.Equal(t, 1001, observerCalls)
}

func TestSpawnObjectHealthAndTypeTransformation(t *testing.T) {
	w := ecs.NewWorldForTesting()
	original := &objectdefs.ObjectDef{DefID: 901, Key: "rock", HP: 100}
	h := SpawnEntityFromDef(w, original, DefSpawnParams{EntityID: 1, Quality: 10})
	require.NotEqual(t, types.InvalidHandle, h)
	health, ok := ecs.GetComponent[components.ObjectInternalState](w, h)
	require.True(t, ok)
	require.True(t, health.HasHP)
	require.Equal(t, 100.0, health.HP)
	require.NoError(t, SetObjectHP(w, h, 0.6))
	quality := uint32(20)
	require.True(t, TransformObjectToDefInPlace(w, 1, h, original, TransformObjectInPlaceOptions{QualityOverride: &quality}))
	health, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.Equal(t, 0.6, health.HP)
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	require.Equal(t, quality, info.Quality)
	require.True(t, TransformObjectToDefInPlace(w, 1, h, &objectdefs.ObjectDef{DefID: 902, Key: "kiln", HP: 250}, TransformObjectInPlaceOptions{}))
	health, _ = ecs.GetComponent[components.ObjectInternalState](w, h)
	require.Equal(t, 250.0, health.HP)
	info, _ = ecs.GetComponent[components.EntityInfo](w, h)
	require.Equal(t, uint32(902), info.TypeID)

	for _, def := range []*objectdefs.ObjectDef{nil, {DefID: 903, Key: "invalid"}, {DefID: 903, Key: "invalid", HP: -1}, {DefID: 903, Key: "player", HP: 1}} {
		require.False(t, TransformObjectToDefInPlace(w, 1, h, def, TransformObjectInPlaceOptions{}))
		afterHealth, _ := ecs.GetComponent[components.ObjectInternalState](w, h)
		afterInfo, _ := ecs.GetComponent[components.EntityInfo](w, h)
		require.Equal(t, health, afterHealth)
		require.Equal(t, info, afterInfo)
	}
	before := w.EntityCount()
	require.Equal(t, types.InvalidHandle, SpawnEntityFromDef(w, &objectdefs.ObjectDef{DefID: 903, Key: "invalid"}, DefSpawnParams{EntityID: 2}))
	require.Equal(t, before, w.EntityCount())
	invalidHP := math.NaN()
	require.Equal(t, types.InvalidHandle, SpawnEntityFromDef(w, original, DefSpawnParams{EntityID: 2, HPOverride: &invalidHP}))
	require.Equal(t, before, w.EntityCount())
	zeroHP := 0.0
	restored := SpawnEntityFromDef(w, original, DefSpawnParams{EntityID: 2, HPOverride: &zeroHP})
	require.True(t, w.Alive(restored))
	health, ok = ecs.GetComponent[components.ObjectInternalState](w, restored)
	require.True(t, ok)
	require.True(t, health.HasHP)
	require.Zero(t, health.HP)
	player := SpawnEntityFromDef(w, &objectdefs.ObjectDef{DefID: 1, Key: "player"}, DefSpawnParams{EntityID: 3})
	require.True(t, w.Alive(player))
	playerState, ok := ecs.GetComponent[components.ObjectInternalState](w, player)
	require.True(t, ok)
	require.False(t, playerState.HasHP)
	inventory := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, inventory, components.InventoryContainer{})
	require.False(t, ecs.HasComponent[components.ObjectInternalState](w, inventory))
}

func TestObjectTransformRejectsMissingStateWithoutMutation(t *testing.T) {
	w := ecs.NewWorldForTesting()
	h := SpawnEntityFromDef(w, &objectdefs.ObjectDef{DefID: 901, Key: "rock", Resource: "rock", HP: 100}, DefSpawnParams{EntityID: 1, Quality: 10})
	ecs.RemoveComponent[components.ObjectInternalState](w, h)
	beforeInfo, _ := ecs.GetComponent[components.EntityInfo](w, h)
	beforeAppearance, _ := ecs.GetComponent[components.Appearance](w, h)
	beforeCount := w.EntityCount()
	quality := uint32(20)
	require.False(t, TransformObjectToDefInPlace(w, 1, h, &objectdefs.ObjectDef{DefID: 902, Key: "kiln", Resource: "kiln", HP: 250}, TransformObjectInPlaceOptions{QualityOverride: &quality}))
	info, _ := ecs.GetComponent[components.EntityInfo](w, h)
	appearance, _ := ecs.GetComponent[components.Appearance](w, h)
	require.Equal(t, beforeInfo, info)
	require.Equal(t, beforeAppearance, appearance)
	require.Equal(t, beforeCount, w.EntityCount())
	require.False(t, ecs.HasComponent[components.ObjectInternalState](w, h))
}
