package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// objectTargetReferences indexes only live references, rather than scanning
// every player when an object disappears. Component observers synchronously
// replace references under the owning shard lock; ordinary movement updates
// with the same target do not allocate or modify maps.
type objectTargetReferences struct {
	byActor  map[types.Handle][6]types.Handle
	byTarget map[types.Handle]map[types.Handle]struct{}
}

func (index *objectTargetReferences) set(actor types.Handle, slot int, target types.Handle) {
	refs := index.byActor[actor]
	previous := refs[slot]
	if previous == target {
		return
	}
	refs[slot] = target
	keepPrevious, alreadyTarget, nonempty := false, false, false
	for i, ref := range refs {
		keepPrevious = keepPrevious || ref == previous && ref != types.InvalidHandle
		alreadyTarget = alreadyTarget || i != slot && ref == target
		nonempty = nonempty || ref != types.InvalidHandle
	}
	if previous != types.InvalidHandle && !keepPrevious {
		actors := index.byTarget[previous]
		delete(actors, actor)
		if len(actors) == 0 {
			delete(index.byTarget, previous)
		}
	}
	if target != types.InvalidHandle && !alreadyTarget {
		actors := index.byTarget[target]
		if actors == nil {
			actors = make(map[types.Handle]struct{}, 4)
			index.byTarget[target] = actors
		}
		actors[actor] = struct{}{}
	}
	if nonempty {
		index.byActor[actor] = refs
	} else {
		delete(index.byActor, actor)
	}
}

// attachObjectTargetReferences is a setup operation, before entities are made
// available for attacks. Repeated service setup does not register observers twice.
func attachObjectTargetReferences(w *ecs.World) *objectTargetReferences {
	if index, exists := ecs.TryGetResource[objectTargetReferences](w); exists {
		return index
	}
	ecs.SetResource(w, objectTargetReferences{
		byActor: make(map[types.Handle][6]types.Handle), byTarget: make(map[types.Handle]map[types.Handle]struct{}),
	})
	index := ecs.GetResource[objectTargetReferences](w)
	observe := func(id ecs.ComponentID, slot int, target func(types.Handle) types.Handle) {
		w.AddComponentObserver(id, func(actor types.Handle) { index.set(actor, slot, target(actor)) })
		// Components may precede service setup (e.g. tests/reattachment). Bootstrap
		// once here; destruction and normal ticks never perform this query.
		ecs.NewPreparedQuery(w, 1<<id, 0).ForEach(func(actor types.Handle) { index.set(actor, slot, target(actor)) })
	}
	observe(components.MovementComponentID, 0, func(actor types.Handle) types.Handle {
		movement, _ := ecs.GetComponent[components.Movement](w, actor)
		return movement.TargetHandle
	})
	observe(components.PendingContextActionComponentID, 1, func(actor types.Handle) types.Handle {
		pending, _ := ecs.GetComponent[components.PendingContextAction](w, actor)
		return pending.TargetHandle
	})
	observe(components.PendingLiftTransitionComponentID, 2, func(actor types.Handle) types.Handle {
		pending, _ := ecs.GetComponent[components.PendingLiftTransition](w, actor)
		return pending.ObjectHandle
	})
	observe(components.ActiveGameActionComponentID, 3, func(actor types.Handle) types.Handle {
		active, _ := ecs.GetComponent[components.ActiveGameAction](w, actor)
		return active.TargetHandle
	})
	observe(components.PendingInteractionComponentID, 4, func(actor types.Handle) types.Handle {
		pending, _ := ecs.GetComponent[components.PendingInteraction](w, actor)
		return pending.TargetHandle
	})
	observe(components.ActiveCyclicActionComponentID, 5, func(actor types.Handle) types.Handle {
		active, _ := ecs.GetComponent[components.ActiveCyclicAction](w, actor)
		return active.TargetHandle
	})
	w.AddDespawnObserver(func(handle types.Handle) {
		for slot := range index.byActor[handle] {
			index.set(handle, slot, types.InvalidHandle)
		}
		for actor := range index.byTarget[handle] {
			refs := index.byActor[actor]
			for slot, target := range refs {
				if target == handle {
					index.set(actor, slot, types.InvalidHandle)
				}
			}
		}
		links := ecs.GetResource[ecs.LinkState](w)
		if id, exists := w.GetExternalID(handle); exists {
			links.ClearIntent(id)
		}
		for player := range links.IntentPlayersByTarget[handle] {
			links.ClearIntent(player)
		}
	})
	return index
}
