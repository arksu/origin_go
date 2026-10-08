package game

import (
	"errors"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

var ErrInvalidPlayerRuntimeState = errors.New("invalid player runtime state")

// playerRuntimeState is an owned, runtime-only copy. It survives reconnect,
// transfer and rollback, but intentionally is not serialized to the database.
type playerRuntimeState struct {
	Health      components.EntityHealth
	CombatState ecs.CombatState
}

func (s *Shard) capturePlayerRuntimeState(handle types.Handle) (playerRuntimeState, bool) {
	health, exists := ecs.GetComponent[components.EntityHealth](s.world, handle)
	if !exists {
		return playerRuntimeState{}, false
	}
	combat, _ := ecs.GetResource[ecs.CombatActivityState](s.world).Capture(handle)
	return playerRuntimeState{Health: health, CombatState: combat}, true
}

// restorePlayerCombatState prepares the shared registration before publication.
// The later creature receiver preparation preserves this exact runtime state.
func restorePlayerCombatState(w *ecs.World, handle types.Handle, identity types.EntityID, state ecs.CombatState) error {
	activity := ecs.GetResource[ecs.CombatActivityState](w)
	if !activity.Prepare(handle, identity) {
		return ecs.ErrCombatActivityUnprepared
	}
	return activity.Restore(handle, state)
}
