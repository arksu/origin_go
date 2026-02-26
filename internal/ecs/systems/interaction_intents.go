package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// ClearPlayerInteractionIntents clears pending interact/context intents and the link intent.
// This is shared by network-command and gameplay services to keep intent reset semantics identical.
func ClearPlayerInteractionIntents(w *ecs.World, playerHandle types.Handle, playerID types.EntityID) {
	if w == nil || playerHandle == types.InvalidHandle {
		return
	}
	ecs.RemoveComponent[components.PendingInteraction](w, playerHandle)
	ecs.RemoveComponent[components.PendingContextAction](w, playerHandle)
	if playerID != 0 {
		ecs.GetResource[ecs.LinkState](w).ClearIntent(playerID)
	}
}
