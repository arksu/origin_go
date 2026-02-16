package components

import "origin/internal/ecs"

// EntityStats stores runtime stamina and energy for all living entities that use these mechanics.
type EntityStats struct {
	Stamina float64
	Energy  float64
}

const EntityStatsComponentID ecs.ComponentID = 27

func init() {
	ecs.RegisterComponent[EntityStats](EntityStatsComponentID)
}
