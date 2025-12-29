package systems

import (
	"math"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// VisibilityEvent represents a change in what an observer can see
type VisibilityEvent struct {
	Observer ecs.Handle // Who is observing
	Target   ecs.Handle // What entity entered/left visibility
	Enter    bool       // true = entered visibility, false = left visibility
}

// VisibilitySystem manages what entities can see each other
// Runs at priority 300 (after collision, before network broadcast)
type VisibilitySystem struct {
	ecs.BaseSystem
	spatialHash *SpatialHashGrid
	observers   map[ecs.Handle]map[ecs.Handle]struct{} // Reverse index: target -> set of observers
	events      []VisibilityEvent
}

// NewVisibilitySystem creates a new visibility system
func NewVisibilitySystem() *VisibilitySystem {
	return &VisibilitySystem{
		BaseSystem:  ecs.NewBaseSystem("VisibilitySystem", 300),
		spatialHash: NewSpatialHash(DefaultCollisionCellSize), // Reuse collision cell size
		observers:   make(map[ecs.Handle]map[ecs.Handle]struct{}),
		events:      make([]VisibilityEvent, 0, 64),
	}
}

// Events returns visibility events from the last update
func (s *VisibilitySystem) Events() []VisibilityEvent {
	return s.events
}

// GetObservers returns all observers that can see the given target
func (s *VisibilitySystem) GetObservers(target ecs.Handle) map[ecs.Handle]struct{} {
	return s.observers[target]
}

// Update processes visibility for all observers
func (s *VisibilitySystem) Update(w *ecs.World, dt float64) {
	s.events = s.events[:0]
	s.spatialHash.Clear()

	// Get component storages
	posStorage := ecs.GetOrCreateStorage[components.Position](w)
	perceptionStorage := ecs.GetOrCreateStorage[components.Perception](w)
	stealthStorage := ecs.GetOrCreateStorage[components.Stealth](w)
	visStateStorage := ecs.GetOrCreateStorage[components.VisibilityState](w)

	// Phase 1: Insert all visible entities into spatial hash
	// (entities with EntityMeta are potentially visible)
	metaQuery := w.Query().
		With(components.PositionID).
		With(components.EntityMetaID)

	for _, h := range metaQuery.Handles() {
		pos, ok := posStorage.Get(h)
		if !ok {
			continue
		}
		// Insert with a small AABB for point-based queries
		const entityRadius = 1.0
		s.spatialHash.Insert(h, pos.X-entityRadius, pos.Y-entityRadius, pos.X+entityRadius, pos.Y+entityRadius)
	}

	// Phase 2: Process each observer
	observerQuery := w.Query().
		With(components.PositionID).
		With(components.PerceptionID).
		With(components.VisibilityStateID)

	for _, observer := range observerQuery.Handles() {
		observerPos, ok := posStorage.Get(observer)
		if !ok {
			continue
		}

		perception, ok := perceptionStorage.Get(observer)
		if !ok {
			continue
		}

		visState := visStateStorage.GetPtr(observer)
		if visState == nil {
			continue
		}

		// Initialize visible entities map if needed
		if visState.VisibleEntities == nil {
			visState.VisibleEntities = make(map[ecs.Handle]bool)
		}

		// Query spatial hash for candidates within perception range
		visionRadius := perception.Range
		candidates := make([]ecs.Handle, 0, 32)
		candidates = s.spatialHash.Query(
			observerPos.X-visionRadius, observerPos.Y-visionRadius,
			observerPos.X+visionRadius, observerPos.Y+visionRadius,
			candidates,
		)

		// Track what we can see this frame
		currentlyVisible := make(map[ecs.Handle]bool)

		for _, target := range candidates {
			// Skip self
			if target == observer {
				continue
			}

			targetPos, ok := posStorage.Get(target)
			if !ok {
				continue
			}

			// Calculate distance
			dx := targetPos.X - observerPos.X
			dy := targetPos.Y - observerPos.Y
			dist := math.Sqrt(dx*dx + dy*dy)

			// Get stealth value (default 0 if not present)
			stealthValue := 0.0
			if stealth, ok := stealthStorage.Get(target); ok {
				stealthValue = stealth.Value
			}

			// Check if target is visible: visionRadius >= dist + stealth
			effectiveRange := visionRadius - stealthValue
			if dist <= effectiveRange {
				currentlyVisible[target] = true

				// Check if this is a new visibility
				if !visState.VisibleEntities[target] {
					// Object entered visibility
					s.events = append(s.events, VisibilityEvent{
						Observer: observer,
						Target:   target,
						Enter:    true,
					})

					// Add to reverse index
					if s.observers[target] == nil {
						s.observers[target] = make(map[ecs.Handle]struct{})
					}
					s.observers[target][observer] = struct{}{}
				}
			}
		}

		// Check for entities that left visibility
		for target := range visState.VisibleEntities {
			if !currentlyVisible[target] {
				// Object left visibility
				s.events = append(s.events, VisibilityEvent{
					Observer: observer,
					Target:   target,
					Enter:    false,
				})

				// Remove from reverse index
				if observers, ok := s.observers[target]; ok {
					delete(observers, observer)
					if len(observers) == 0 {
						delete(s.observers, target)
					}
				}
			}
		}

		// Update visibility state
		visState.VisibleEntities = currentlyVisible
	}
}
