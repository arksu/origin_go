package components

import "origin/internal/ecs"

// Appearance represents an entity's visual appearance and identity
type Appearance struct {
	Name string // Display name for the entity (player or NPC)
}

const AppearanceComponentID ecs.ComponentID = 18

func init() {
	ecs.RegisterComponent[Appearance](AppearanceComponentID)
}
