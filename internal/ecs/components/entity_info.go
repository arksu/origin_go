package components

import (
	"origin/internal/ecs"
)

// EntityInfo stores basic entity metadata
type EntityInfo struct {
	TypeID    uint32   // defId from object definitions
	Behaviors []string // behavior keys from object definitions
	IsStatic  bool     // resolved from definition at spawn time, never changes
	Quality   uint32
	Region    int
	Layer     int
}

const EntityInfoComponentID ecs.ComponentID = 13

func init() {
	ecs.RegisterComponent[EntityInfo](EntityInfoComponentID)
}
