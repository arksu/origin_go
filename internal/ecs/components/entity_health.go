package components

import "origin/internal/ecs"

// EntityHealth stores runtime SHP/HHP pools and KO/death runtime state.
type EntityHealth struct {
	SHP            float64
	HHP            float64
	KOUntilTick    uint64 // 0 means not KO; non-zero stores the tick when KO started.
	RespawnDueTick uint64
}

const EntityHealthComponentID ecs.ComponentID = 33

func init() {
	ecs.RegisterComponent[EntityHealth](EntityHealthComponentID)
}
