package systems

import (
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func resolveConForHandle(w *ecs.World, handle types.Handle) int {
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
		return characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	return characterattrs.DefaultValue
}
