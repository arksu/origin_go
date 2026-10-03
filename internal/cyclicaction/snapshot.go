package cyclicaction

import (
	"fmt"
	"math"
	"origin/internal/actionanimationdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"time"
)

// Snapshot owns its result and reads actual counters, including creation-tick progress.
func Snapshot(w *ecs.World, handle types.Handle) (*netproto.CharacterActionAnimationState, error) {
	if !w.Alive(handle) {
		return nil, nil
	}
	appearance, ok := ecs.GetComponent[components.Appearance](w, handle)
	if !ok || appearance.Resource != "player" {
		return nil, nil
	}
	timing := ecs.GetResource[ecs.TimeState](w)
	state := &netproto.CharacterActionAnimationState{Generation: fmt.Sprintf("%d:%d", w.Layer, handle), ServerTimeMs: timing.UnixMs}
	presentation, exists := ecs.GetComponent[components.ActionAnimation](w, handle)
	if !exists {
		return state, nil
	}
	state.Revision = presentation.Revision
	if presentation.Key == "" {
		return state, nil
	}
	if presentation.ExecutionID != 0 {
		combat, ok := ecs.GetComponent[components.CombatState](w, handle)
		if !ok || combat.Execution == nil || combat.Execution.ID != presentation.ExecutionID {
			return nil, fmt.Errorf("invalid combat animation identity for handle %d", handle)
		}
		execution := combat.Execution
		duration := float64(execution.RecoveryEnd.Sub(execution.StartedAt)) / float64(time.Millisecond)
		if duration <= 0 {
			return nil, fmt.Errorf("invalid combat animation duration")
		}
		state.AnimationKey, state.ExecutionId = presentation.Key, execution.ID
		state.DurationMs = duration
		state.ElapsedMs = math.Max(0, math.Min(duration, float64(timing.Now.Sub(execution.StartedAt))/float64(time.Millisecond)))
		state.LockedDirection = &netproto.CombatDirection{X: execution.Direction.X, Y: execution.Direction.Y}
		return state, nil
	}
	cycle, active := ecs.GetComponent[components.ActiveCyclicAction](w, handle)
	if !active || cycle.CycleDurationTicks == 0 || cycle.CycleElapsedTicks > cycle.CycleDurationTicks || timing.TickPeriod <= 0 {
		return nil, fmt.Errorf("invalid action animation timing for handle %d", handle)
	}
	binding, mapped := actionanimationdefs.Global().Get(presentation.Key)
	if !mapped {
		return nil, fmt.Errorf("action animation binding %q disappeared", presentation.Key)
	}
	state.AnimationKey = presentation.Key
	state.TotalTicks, state.ElapsedTicks = cycle.CycleDurationTicks, cycle.CycleElapsedTicks
	state.TickDurationMs = float64(timing.TickPeriod) / float64(time.Millisecond)
	if binding.Facing == "target" {
		x, y, hasTarget := cycle.TargetX, cycle.TargetY, cycle.HasTargetPosition
		if !hasTarget && cycle.TargetKind == components.CyclicActionTargetObject && w.Alive(cycle.TargetHandle) {
			if targetID, ok := w.GetExternalID(cycle.TargetHandle); ok && targetID == cycle.TargetID {
				if transform, ok := ecs.GetComponent[components.Transform](w, cycle.TargetHandle); ok {
					x, y, hasTarget = transform.X, transform.Y, true
				}
			}
		}
		if hasTarget {
			if math.IsNaN(x) || math.IsNaN(y) || x < math.MinInt32 || x > math.MaxInt32 || y < math.MinInt32 || y > math.MaxInt32 {
				return nil, fmt.Errorf("invalid action animation target for handle %d", handle)
			}
			state.TargetPosition = &netproto.Position{X: int32(x), Y: int32(y)}
		}
	}
	return state, nil
}
