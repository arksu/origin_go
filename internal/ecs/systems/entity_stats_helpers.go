package systems

import (
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entitystats"
	"origin/internal/types"
)

func resolveConForHandle(w *ecs.World, handle types.Handle) int {
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
		return characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	return characterattrs.DefaultValue
}

// movementCapability bundles the tick-stable stamina inputs derived from the
// character profile. MovementSystem (pre-move gate) and TransformUpdateSystem's
// stamina tick (post-move drain) both need them for every mover; resolving
// through a cached storage avoids the registry lock + map cost of the generic
// GetComponent on each access.
type movementCapability struct {
	con        int
	maxStamina float64
}

func resolveMovementCapability(
	profileStorage *ecs.ComponentStorage[components.CharacterProfile],
	handle types.Handle,
) movementCapability {
	con := characterattrs.DefaultValue
	if profile, hasProfile := profileStorage.Get(handle); hasProfile {
		con = characterattrs.Get(profile.Attributes, characterattrs.CON)
	}
	return movementCapability{
		con:        con,
		maxStamina: entitystats.MaxStaminaFromCon(con),
	}
}
