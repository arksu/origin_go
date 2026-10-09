package inventory

import (
	"math"
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestObjectLootTransformationCapturePreservesHealthAndInventories(t *testing.T) {
	for _, hp := range []float64{0, 0.49, 100} {
		w, registry, target, roots := objectLootCaptureFixture()
		state := components.ObjectInternalState{HP: hp, HasHP: true, Flags: []string{"retained.flag"}}
		ecs.AddComponent(w, target, state)
		ecs.SetResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{target: true}})
		before := make([]components.InventoryContainer, len(roots))
		for i, root := range roots {
			before[i], _ = ecs.GetComponent[components.InventoryContainer](w, root.Handle)
		}
		capture := NewObjectLootCaptureForTransformation(100, roots)
		finishObjectLootCapture(t, capture, w, registry, 1)
		require.Len(t, capture.Items(), 2)
		require.Len(t, capture.ContainerRefs(), 4)
		require.Equal(t, types.EntityID(104), capture.MaxItemID())
		after, _ := ecs.GetComponent[components.ObjectInternalState](w, target)
		require.Equal(t, state, after, "capturing a lifecycle transition cannot mutate HP, dirty intent or behavior flags")
		require.True(t, ecs.ObjectDestructionPending(w, target))
		for i, root := range roots {
			after, _ := ecs.GetComponent[components.InventoryContainer](w, root.Handle)
			require.Equal(t, before[i], after)
		}
		if hp > 0 {
			_, err := NewObjectLootCapture(100, roots).CaptureBatch(w, registry, 100)
			require.ErrorIs(t, err, ErrInvalidObjectLootCapture, "ordinary destruction cannot capture a positive-HP object")
		}
	}
}

func TestObjectLootTransformationCaptureRejectsMissingQuarantineAndInvalidHealth(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*ecs.World, types.Handle)
	}{
		{"not pending", func(w *ecs.World, target types.Handle) {
			ecs.GetResource[ecs.ObjectDestructionState](w).Pending[target] = false
		}},
		{"missing health", func(w *ecs.World, target types.Handle) {
			ecs.WithComponent(w, target, func(state *components.ObjectInternalState) { state.HasHP = false })
		}},
		{"missing state", func(w *ecs.World, target types.Handle) {
			ecs.RemoveComponent[components.ObjectInternalState](w, target)
		}},
		{"negative health", func(w *ecs.World, target types.Handle) {
			ecs.WithComponent(w, target, func(state *components.ObjectInternalState) { state.HP = -0.49 })
		}},
		{"NaN health", func(w *ecs.World, target types.Handle) {
			ecs.WithComponent(w, target, func(state *components.ObjectInternalState) { state.HP = math.NaN() })
		}},
		{"infinite health", func(w *ecs.World, target types.Handle) {
			ecs.WithComponent(w, target, func(state *components.ObjectInternalState) { state.HP = math.Inf(1) })
		}},
		{"negative infinite health", func(w *ecs.World, target types.Handle) {
			ecs.WithComponent(w, target, func(state *components.ObjectInternalState) { state.HP = math.Inf(-1) })
		}},
		{"wrong target identity", func(w *ecs.World, target types.Handle) {
			ecs.AddComponent(w, target, ecs.ExternalID{ID: 200})
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			w, registry, target, roots := objectLootCaptureFixture()
			ecs.WithComponent(w, target, func(state *components.ObjectInternalState) { state.HP = 0.49 })
			ecs.SetResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{target: true}})
			scenario.change(w, target)
			beforeCount := w.EntityCount()
			capture := NewObjectLootCaptureForTransformation(100, roots)
			_, err := capture.CaptureBatch(w, registry, 100)
			require.ErrorIs(t, err, ErrInvalidObjectLootCapture)
			require.Nil(t, capture.Items())
			require.Nil(t, capture.ContainerRefs())
			require.Equal(t, beforeCount, w.EntityCount())
		})
	}
}

func TestObjectLootTransformationCaptureRechecksQuarantineAndGeneration(t *testing.T) {
	for _, replace := range []bool{false, true} {
		w, registry, target, roots := objectLootCaptureFixture()
		ecs.WithComponent(w, target, func(state *components.ObjectInternalState) { state.HP = 0.49 })
		ecs.SetResource(w, ecs.ObjectDestructionState{Pending: map[types.Handle]bool{target: true}})
		capture := NewObjectLootCaptureForTransformation(100, roots)
		done, err := capture.CaptureBatch(w, registry, 1)
		require.NoError(t, err)
		require.False(t, done)
		if replace {
			require.True(t, w.Despawn(target))
			replacement := w.Spawn(100, nil)
			ecs.AddComponent(w, replacement, components.ObjectInternalState{HP: 0.49, HasHP: true})
			ecs.GetResource[ecs.ObjectDestructionState](w).Pending[replacement] = true
		} else {
			ecs.GetResource[ecs.ObjectDestructionState](w).Pending[target] = false
		}
		_, err = capture.CaptureBatch(w, registry, 100)
		require.ErrorIs(t, err, ErrInvalidObjectLootCapture)
		require.Nil(t, capture.Items())
	}
}
