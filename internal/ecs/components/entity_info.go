package components

import (
	"origin/internal/ecs"
	"origin/internal/types"
)

// EntityInfo stores basic entity metadata
type EntityInfo struct {
	ObjectType types.ObjectType
	IsStatic   bool
	Region     int
	Layer      int
}

const EntityInfoComponentID ecs.ComponentID = 13

func init() {
	ecs.RegisterComponent[EntityInfo](EntityInfoComponentID)
}
