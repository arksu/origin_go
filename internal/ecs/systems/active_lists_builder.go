package systems

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
)

// ActiveListsBuilderSystem builds precomputed entity lists from active chunks
// Runs at priority 20 (after ActiveChunksSystem, before hot systems)
type ActiveListsBuilderSystem struct {
	ecs.BaseSystem
}

// NewActiveListsBuilderSystem creates a new active lists builder system
func NewActiveListsBuilderSystem() *ActiveListsBuilderSystem {
	return &ActiveListsBuilderSystem{
		BaseSystem: ecs.NewBaseSystem("ActiveListsBuilderSystem", 20),
	}
}

// Update builds entity lists from active chunks
func (s *ActiveListsBuilderSystem) Update(w *ecs.World, dt float64) {
	activeLists := ecs.GetActiveLists(w)
	if activeLists == nil {
		return
	}

	chunkIndex := ecs.GetChunkIndex(w)
	if chunkIndex == nil {
		return
	}

	// Get component storages for filtering
	posStorage := ecs.GetOrCreateStorage[components.Position](w)
	velStorage := ecs.GetOrCreateStorage[components.Velocity](w)
	staticStorage := ecs.GetOrCreateStorage[components.Static](w)
	colliderStorage := ecs.GetOrCreateStorage[components.Collider](w)
	perceptionStorage := ecs.GetOrCreateStorage[components.Perception](w)
	visStateStorage := ecs.GetOrCreateStorage[components.VisibilityState](w)
	metaStorage := ecs.GetOrCreateStorage[components.EntityMeta](w)

	// Collect all entities from active chunks
	activeLists.All = chunkIndex.GetEntitiesInChunksSet(activeLists.ActiveChunks, activeLists.All)

	// Categorize entities
	for _, h := range activeLists.All {
		hasPosition := posStorage.Has(h)
		hasVelocity := velStorage.Has(h)
		isStatic := staticStorage.Has(h)
		hasCollider := colliderStorage.Has(h)
		hasPerception := perceptionStorage.Has(h)
		hasVisState := visStateStorage.Has(h)
		hasMeta := metaStorage.Has(h)

		// Dynamic: has Position + Velocity, not Static
		if hasPosition && hasVelocity && !isStatic {
			activeLists.Dynamic = append(activeLists.Dynamic, h)
		}

		// Static: is Static and has Position + Collider
		if hasPosition && isStatic && hasCollider {
			activeLists.Static = append(activeLists.Static, h)
		}

		// Vision: has Position + Perception + VisibilityState (observers)
		if hasPosition && hasPerception && hasVisState {
			activeLists.Vision = append(activeLists.Vision, h)
		}

		// Visible: has Position + EntityMeta (can be seen)
		if hasPosition && hasMeta {
			activeLists.Visible = append(activeLists.Visible, h)
		}
	}
}
