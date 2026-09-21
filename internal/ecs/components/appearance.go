package components

import "origin/internal/ecs"

// NameColor is a semantic role for a named entity's label; the client owns
// the visual palette. Zero value is the default role.
type NameColor uint8

const (
	NameColorDefault NameColor = iota
	// Reserved for future roles: administrator, NPC, ...
)

// Appearance represents an entity's visual appearance and identity
type Appearance struct {
	Name      *string   // Display name for the entity (player or NPC)
	NameColor NameColor // Role for the client's label palette; zero = default
	Resource  string
}

const AppearanceComponentID ecs.ComponentID = 18

func init() {
	ecs.RegisterComponent[Appearance](AppearanceComponentID)
}
