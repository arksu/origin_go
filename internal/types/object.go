package types

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
