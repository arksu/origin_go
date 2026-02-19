package behaviors

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func resolveTargetObjectQuality(world *ecs.World, targetHandle types.Handle) (uint32, bool) {
	if world == nil || targetHandle == types.InvalidHandle || !world.Alive(targetHandle) {
		return 0, false
	}
	info, hasInfo := ecs.GetComponent[components.EntityInfo](world, targetHandle)
	if !hasInfo {
		return 0, false
	}
	return info.Quality, true
}

