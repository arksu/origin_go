package components

import "origin/internal/ecs"

// Perception represents an entity's ability to detect other entities
type Perception struct {
	Range float64 // Vision radius in world units
}

// Stealth represents how difficult an entity is to detect
type Stealth struct {
	Value float64 // Stealth modifier (reduces effective detection range)
}

// EntityMeta contains metadata for network synchronization
type EntityMeta struct {
	EntityID   ecs.EntityID // Global unique ID for persistence/replication
	EntityType uint32       // Type of entity (player, npc, item, etc.)
}

// VisibilityState tracks what an observer can see
type VisibilityState struct {
	VisibleEntities map[ecs.Handle]bool // Set of entities this observer can see
}

// Component IDs
var (
	PerceptionID      ecs.ComponentID
	StealthID         ecs.ComponentID
	EntityMetaID      ecs.ComponentID
	VisibilityStateID ecs.ComponentID
)

func init() {
	PerceptionID = ecs.GetComponentID[Perception]()
	StealthID = ecs.GetComponentID[Stealth]()
	EntityMetaID = ecs.GetComponentID[EntityMeta]()
	VisibilityStateID = ecs.GetComponentID[VisibilityState]()
}
