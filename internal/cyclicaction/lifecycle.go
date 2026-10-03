// Package cyclicaction owns cycle installation/removal and its public presentation.
// Callers hold the world write lock; snapshots use that same lock or its read side.
package cyclicaction

import (
	"math"
	"origin/internal/actionanimationdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func StartContext(w *ecs.World, handle types.Handle, cycle components.ActiveCyclicAction) {
	Start(w, handle, cycle, actionanimationdefs.Source{Kind: "context", Namespace: cycle.BehaviorKey, ID: cycle.ActionID})
}

func Start(w *ecs.World, handle types.Handle, cycle components.ActiveCyclicAction, source actionanimationdefs.Source) {
	if !w.Alive(handle) {
		return
	}
	binding, mapped := actionanimationdefs.Global().Resolve(source)
	key := ""
	if mapped {
		key = binding.Key
	}
	cycle.SoundBinding = binding
	cycle.NextSoundCue = 0
	state, _ := ecs.GetComponent[components.ActionAnimation](w, handle)
	previous, exists := ecs.GetComponent[components.ActiveCyclicAction](w, handle)
	// Duplicate installation must not reset progress or re-arm consumed contacts.
	previous.CycleElapsedTicks = cycle.CycleElapsedTicks
	previous.ActionCompletionStarted = cycle.ActionCompletionStarted
	previous.NextSoundCue = 0
	if exists && previous == cycle && state.Key == key {
		return
	}
	ecs.AddComponent(w, handle, cycle)
	if key == "" {
		clearPresentation(w, handle, state)
		return
	}
	state.Key = key
	publishCycle(w, handle, cycle, state)
}

// Continue is called after gameplay has confirmed and reset the next cycle.
func Continue(w *ecs.World, handle types.Handle) {
	cycle, active := ecs.GetComponent[components.ActiveCyclicAction](w, handle)
	state, exists := ecs.GetComponent[components.ActionAnimation](w, handle)
	if !active || !exists || state.Key == "" {
		return
	}
	if state.CycleIndex == cycle.CycleIndex && state.StartedTick == cycle.StartedTick && state.TotalTicks == cycle.CycleDurationTicks {
		return
	}
	cycle.NextSoundCue = 0
	ecs.AddComponent(w, handle, cycle)
	publishCycle(w, handle, cycle, state)
}

func Clear(w *ecs.World, handle types.Handle) {
	if !w.Alive(handle) {
		return
	}
	ecs.RemoveComponent[components.ActiveCyclicAction](w, handle)
	state, _ := ecs.GetComponent[components.ActionAnimation](w, handle)
	clearPresentation(w, handle, state)
}

// SyncCombat shares public animation revisions while keeping combat off the
// legacy cyclic-action payment and completion path.
func SyncCombat(w *ecs.World, handle types.Handle) {
	state, _ := ecs.GetComponent[components.ActionAnimation](w, handle)
	combat, exists := ecs.GetComponent[components.CombatState](w, handle)
	if !exists || combat.Execution == nil {
		if state.ExecutionID != 0 {
			state.ExecutionID = 0
			clearPresentation(w, handle, state)
		}
		return
	}
	execution := combat.Execution
	if state.ExecutionID == execution.ID {
		return
	}
	binding, mapped := actionanimationdefs.Global().Resolve(actionanimationdefs.Source{Kind: "combat", ID: execution.ActionID})
	if !mapped {
		return
	}
	state.Key, state.ExecutionID = binding.Key, execution.ID
	state.CycleIndex, state.StartedTick, state.TotalTicks = 0, 0, 0
	publish(w, handle, state)
}

func clearPresentation(w *ecs.World, handle types.Handle, state components.ActionAnimation) {
	if state.Key == "" {
		return
	}
	state.Key = ""
	state.CycleIndex, state.StartedTick, state.TotalTicks = 0, 0, 0
	publish(w, handle, state)
}

func publishCycle(w *ecs.World, handle types.Handle, cycle components.ActiveCyclicAction, state components.ActionAnimation) {
	state.CycleIndex, state.StartedTick, state.TotalTicks = cycle.CycleIndex, cycle.StartedTick, cycle.CycleDurationTicks
	publish(w, handle, state)
}

func publish(w *ecs.World, handle types.Handle, state components.ActionAnimation) {
	if state.Revision == math.MaxUint64 {
		panic("action animation revision exhausted")
	}
	state.Revision++
	ecs.AddComponent(w, handle, state)
	ecs.GetResource[ecs.ActionAnimationDirtyQueue](w).Mark(handle)
}
