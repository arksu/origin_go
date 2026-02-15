package components

import (
	"origin/internal/characterattrs"
	"origin/internal/ecs"
)

type CharacterAttributes struct {
	Values characterattrs.Values
}

const CharacterAttributesComponentID ecs.ComponentID = 26

func init() {
	ecs.RegisterComponent[CharacterAttributes](CharacterAttributesComponentID)
}
