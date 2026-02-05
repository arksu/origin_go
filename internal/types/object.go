package types

// ObjectType represents different types of entities
type ObjectType int

const (
	ObjectTypeUnknown     ObjectType = iota
	ObjectTypeTree                   = 1
	ObjectTypeRock                   = 2
	ObjectTypeDroppedItem            = 3
	ObjectTypeBuilding               = 4
	ObjectTypeNPC                    = 5
	ObjectTypePlayer                 = 6
)
