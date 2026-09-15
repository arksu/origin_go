package ecs

import "origin/internal/types"

// CharacterVisualDirtyQueue shares the existing deduplicated queue mechanics.
// Inventory mutations mark owners; rendering never scans all characters per tick.
type CharacterVisualDirtyQueue struct{ ObjectBehaviorDirtyQueue }

func MarkCharacterVisualDirty(w *World, ownerID types.EntityID) {
	handle := w.GetHandleByEntityID(ownerID)
	if handle != types.InvalidHandle && w.Alive(handle) {
		GetResource[CharacterVisualDirtyQueue](w).Mark(handle)
	}
}
