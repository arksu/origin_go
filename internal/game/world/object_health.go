package world

import (
	"errors"
	"math"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

var (
	ErrInvalidObjectHP     = errors.New("object HP must be finite and nonnegative")
	ErrObjectHealthMissing = errors.New("object health is missing")
	ErrObjectStateMissing  = errors.New("object internal state is missing")
)

// ValidateObjectHP accepts finite, nonnegative HP, including zero.
// Definitions supply initial HP; persisted HP is never rounded or clamped.
func ValidateObjectHP(hp float64) error {
	if !(hp >= 0 && hp <= math.MaxFloat64) {
		return ErrInvalidObjectHP
	}
	return nil
}

// SetObjectHP updates health and persistence intent under the owning shard lock.
// Errors and unchanged values do not mutate the world or notify observers.
func SetObjectHP(w *ecs.World, target types.Handle, hp float64) error {
	if w == nil || !w.Alive(target) {
		return ErrEntityNotFound
	}
	state, hasState := ecs.GetComponent[components.ObjectInternalState](w, target)
	if !hasState {
		return ErrObjectStateMissing
	}
	if !state.HasHP {
		return ErrObjectHealthMissing
	}
	if err := ValidateObjectHP(hp); err != nil {
		return err
	}
	if state.HP == hp {
		return nil
	}
	// The presence check prevents lazy storage creation; observers see HP and
	// persistence intent together in one component write.
	ecs.WithComponent(w, target, func(state *components.ObjectInternalState) {
		state.HP = hp
		state.IsDirty = true
	})
	return nil
}
