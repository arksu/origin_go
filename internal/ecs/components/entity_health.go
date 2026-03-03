package components

import "origin/internal/ecs"

// EntityHealth stores runtime SHP/HHP pools and transient KO/death timers.
type EntityHealth struct {
	SHP            float64
	HHP            float64
	KOUntilTick    uint64
	RespawnDueTick uint64
}

const EntityHealthComponentID ecs.ComponentID = 33

func init() {
	ecs.RegisterComponent[EntityHealth](EntityHealthComponentID)
}
