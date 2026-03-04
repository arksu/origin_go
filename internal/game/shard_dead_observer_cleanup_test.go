package game

import (
	"testing"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestCleanupObserverModeStateForHandle_RemovesVisionAndVisibilityLinks(t *testing.T) {
	world := ecs.NewWorldForTesting()

	observerID := types.EntityID(91001)
	targetAID := types.EntityID(91002)
	targetBID := types.EntityID(91003)
	otherObserverID := types.EntityID(91004)

	observerHandle := world.Spawn(observerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Vision{Radius: 100, Power: 100})
	})
	targetAHandle := world.Spawn(targetAID, nil)
	targetBHandle := world.Spawn(targetBID, nil)
	otherObserverHandle := world.Spawn(otherObserverID, nil)

	visState := ecs.GetResource[ecs.VisibilityState](world)
	visState.VisibleByObserver[observerHandle] = ecs.ObserverVisibility{
		Known: map[types.Handle]types.EntityID{
			targetAHandle: targetAID,
			targetBHandle: targetBID,
		},
	}
	visState.ObserversByVisibleTarget[targetAHandle] = map[types.Handle]struct{}{
		observerHandle:      {},
		otherObserverHandle: {},
	}
	visState.ObserversByVisibleTarget[targetBHandle] = map[types.Handle]struct{}{
		observerHandle: {},
	}

	cleanupObserverModeStateForHandle(world, observerHandle)

	if _, hasVision := ecs.GetComponent[components.Vision](world, observerHandle); hasVision {
		t.Fatalf("expected observer vision to be removed")
	}
	if _, exists := visState.VisibleByObserver[observerHandle]; exists {
		t.Fatalf("expected observer visibility entry to be removed")
	}
	if observers := visState.ObserversByVisibleTarget[targetAHandle]; observers == nil {
		t.Fatalf("expected target A observer set to remain for other observer")
	} else {
		if _, hasObserver := observers[observerHandle]; hasObserver {
			t.Fatalf("expected target A to drop dead observer")
		}
		if _, hasOtherObserver := observers[otherObserverHandle]; !hasOtherObserver {
			t.Fatalf("expected target A to keep non-dead observer")
		}
	}
	if _, exists := visState.ObserversByVisibleTarget[targetBHandle]; exists {
		t.Fatalf("expected target B observer set to be removed when empty")
	}
}

func TestCleanupObserverModeStateForHandle_CleansStaleReverseLinksWithoutObserverEntry(t *testing.T) {
	world := ecs.NewWorldForTesting()

	observerID := types.EntityID(92001)
	targetID := types.EntityID(92002)

	observerHandle := world.Spawn(observerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Vision{Radius: 100, Power: 100})
	})
	targetHandle := world.Spawn(targetID, nil)

	visState := ecs.GetResource[ecs.VisibilityState](world)
	visState.ObserversByVisibleTarget[targetHandle] = map[types.Handle]struct{}{
		observerHandle: {},
	}

	cleanupObserverModeStateForHandle(world, observerHandle)

	if _, hasVision := ecs.GetComponent[components.Vision](world, observerHandle); hasVision {
		t.Fatalf("expected observer vision to be removed")
	}
	if _, exists := visState.ObserversByVisibleTarget[targetHandle]; exists {
		t.Fatalf("expected stale reverse observer link to be removed")
	}
}
