package components

import (
	"origin/internal/ecs"
)

// ObjectType represents different types of entities
type ObjectType int

const (
	ObjectTypeUnknown ObjectType = iota
	ObjectTypeTree
	ObjectTypeRock
	ObjectTypeBuilding
	ObjectTypeItem
	ObjectTypeNPC
	ObjectTypePlayer
)

// EntityInfo stores basic entity metadata
type EntityInfo struct {
	ObjectType ObjectType
	IsStatic   bool
	Region     int
	Layer      int
}

const EntityInfoComponentID ecs.ComponentID = 13

func init() {
	ecs.RegisterComponent[EntityInfo](EntityInfoComponentID)
}
