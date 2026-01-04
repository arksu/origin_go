package components

import (
	"origin/internal/ecs"
)

// ObjectType represents different types of entities
type ObjectType int32

const (
	ObjectTypeUnknown ObjectType = iota
	ObjectTypeTree
	ObjectTypeRock
	ObjectTypeBuilding
	ObjectTypeItem
	ObjectTypeNPC
)

// EntityInfo stores basic entity metadata
type EntityInfo struct {
	ObjectType ObjectType
	IsStatic   bool
	Region     int32
	Layer      int32
}

const EntityInfoComponentID ecs.ComponentID = 13

func init() {
	ecs.RegisterComponent[EntityInfo](EntityInfoComponentID)
}
