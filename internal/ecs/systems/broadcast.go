package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// MovementEvent represents a position update for network broadcast
type MovementEvent struct {
	Observer ecs.Handle   // Who should receive this update
	EntityID ecs.EntityID // Global entity ID for the packet
	X        int32        // New X position (rounded from float)
	Y        int32        // New Y position (rounded from float)
}

// MovementBroadcastSystem generates movement events for network synchronization
// Runs at priority 400 (after visibility, before network flush)
type MovementBroadcastSystem struct {
	ecs.BaseSystem
	visibilitySystem *VisibilitySystem
	dirtyPositions   map[ecs.Handle]bool
	events           []MovementEvent
}

// NewMovementBroadcastSystem creates a new movement broadcast system
func NewMovementBroadcastSystem(visSystem *VisibilitySystem) *MovementBroadcastSystem {
	return &MovementBroadcastSystem{
		BaseSystem:       ecs.NewBaseSystem("MovementBroadcastSystem", 400),
		visibilitySystem: visSystem,
		dirtyPositions:   make(map[ecs.Handle]bool),
		events:           make([]MovementEvent, 0, 128),
	}
}

// MarkDirty marks an entity's position as changed
func (s *MovementBroadcastSystem) MarkDirty(h ecs.Handle) {
	s.dirtyPositions[h] = true
}

// Events returns movement events from the last update
func (s *MovementBroadcastSystem) Events() []MovementEvent {
	return s.events
}

// Update processes dirty positions and generates movement events
func (s *MovementBroadcastSystem) Update(w *ecs.World, dt float64) {
	s.events = s.events[:0]

	// Get component storages
	posStorage := ecs.GetOrCreateStorage[components.Position](w)
	metaStorage := ecs.GetOrCreateStorage[components.EntityMeta](w)

	// Process each dirty position
	for h := range s.dirtyPositions {
		pos, ok := posStorage.Get(h)
		if !ok {
			continue
		}

		meta, ok := metaStorage.Get(h)
		if !ok {
			continue
		}

		// Get all observers who can see this entity
		observers := s.visibilitySystem.GetObservers(h)
		if observers == nil {
			continue
		}

		// Round position to int32 for network packet
		x := int32(pos.X)
		y := int32(pos.Y)

		// Generate movement event for each observer
		for observer := range observers {
			s.events = append(s.events, MovementEvent{
				Observer: observer,
				EntityID: meta.EntityID,
				X:        x,
				Y:        y,
			})
		}
	}

	// Clear dirty positions for next frame
	for h := range s.dirtyPositions {
		delete(s.dirtyPositions, h)
	}
}
