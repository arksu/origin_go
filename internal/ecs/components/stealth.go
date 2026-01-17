package components

import "origin/internal/ecs"

// Stealth represents an entity's stealth capabilities
type Stealth struct {
	Value float64 // Stealth value (0.0 = fully visible, higher values = more stealth)
}

const StealthComponentID ecs.ComponentID = 17

func init() {
	ecs.RegisterComponent[Stealth](StealthComponentID)
}
