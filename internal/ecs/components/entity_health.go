package components

import "origin/internal/ecs"

// EntityHealth stores legacy HP pools used for permanent death flow.
type EntityHealth struct {
	SHP    int16
	HHP    int16
	SHPMax int16
	HHPMax int16
}

const EntityHealthComponentID ecs.ComponentID = 33

func init() {
	ecs.RegisterComponent[EntityHealth](EntityHealthComponentID)
}
