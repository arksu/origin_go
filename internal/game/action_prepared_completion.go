package game

import (
	"math"

	"origin/internal/actiondefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// executePreparedCompletion preserves the ordinary handler path while allowing
// combat to resolve every fallible operation ahead of its first mutation.
func (s *ActionService) executePreparedCompletion(w *ecs.World, playerID types.EntityID, player types.Handle,
	definition *actiondefs.Definition, active components.ActiveGameAction, target ActionTarget, completion preparedActionCompletion,
) {
	current, exists := ecs.GetComponent[components.ActiveGameAction](w, player)
	if !exists || current != active || active.Phase != components.GameActionExecuting {
		return
	}
	now := ecs.GetResource[ecs.TimeState](w).UnixMs
	if now < 0 || now > math.MaxInt64-int64(definition.Cooldown) {
		s.Complete(w, playerID, player, active.Generation, false, "ACTION_FAILED")
		return
	}
	if reason := completion.PrepareCompletion(w, playerID, player, target, active.Generation); reason != "" {
		s.Complete(w, playerID, player, active.Generation, false, reason)
		return
	}
	if !s.chargeActionCosts(w, player, definition) {
		completion.AbortPrepared()
		s.Complete(w, playerID, player, active.Generation, false, "LOW_STAMINA")
		return
	}
	completion.CommitPrepared()
	// Complete(success=true) would charge a second time. Clear the completed
	// cycle before quarantine invokes synchronous link/lifecycle callbacks.
	s.finishAction(w, playerID, player, definition, active, true, "")
	completion.AfterCompletion()
}
