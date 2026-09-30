package ecs

import (
	"origin/internal/eventbus"
	"origin/internal/types"
)

// PublishEntityAppearanceChanged runs after the appearance mutation on the ECS thread.
// Unobserved objects will expose their final state in the next visibility spawn;
// enqueueing now could otherwise duplicate that spawn after observers are added.
func PublishEntityAppearanceChanged(w *World, bus *eventbus.EventBus, targetID types.EntityID, targetHandle types.Handle) {
	if bus == nil {
		return
	}
	visibility := GetResource[VisibilityState](w)
	visibility.Mu.RLock()
	observed := len(visibility.ObserversByVisibleTarget[targetHandle]) > 0
	visibility.Mu.RUnlock()
	if !observed {
		return
	}
	bus.PublishAsync(NewEntityAppearanceChangedEvent(w.Layer, targetID, targetHandle), eventbus.PriorityMedium)
}
