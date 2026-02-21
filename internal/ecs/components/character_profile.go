package components

import (
	"origin/internal/characterattrs"
	"origin/internal/ecs"
)

// CharacterExperience stores character-only progression values.
type CharacterExperience struct {
	LP       int64
	Nature   int64
	Industry int64
	Combat   int64
}

// CharacterProfile stores player-specific data attached only to character entities.
// It is intentionally broader than attributes and should include all character-only state.
type CharacterProfile struct {
	Attributes characterattrs.Values
	Experience CharacterExperience
	Skills     []string
	Discovery  []string
}

const CharacterProfileComponentID ecs.ComponentID = 26

func init() {
	ecs.RegisterComponent[CharacterProfile](CharacterProfileComponentID)
}
