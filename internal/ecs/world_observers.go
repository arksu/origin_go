package ecs

import "origin/internal/types"

// Observers run synchronously under the world owner's lock. They must not
// mutate observed components or despawn entities recursively.
func (world *World) AddComponentObserver(id ComponentID, observer func(types.Handle)) {
	if id > MaxComponentID || observer == nil {
		panic("invalid component observer")
	}
	world.componentObservers[id] = append(world.componentObservers[id], observer)
}

func (world *World) AddDespawnObserver(observer func(types.Handle)) {
	if observer == nil {
		panic("invalid despawn observer")
	}
	world.despawnObservers = append(world.despawnObservers, observer)
}

func (world *World) notifyComponentObservers(id ComponentID, handle types.Handle) {
	for _, observer := range world.componentObservers[id] {
		observer(handle)
	}
}
