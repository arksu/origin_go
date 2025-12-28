package components

import "origin/internal/ecs"

// Player marks an entity as a player character
type Player struct {
	CharacterID int64  // Database character ID
	Name        string // Character name
}

// Component ID
var PlayerID ecs.ComponentID

func init() {
	PlayerID = ecs.GetComponentID[Player]()
}
