package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func actionSoundCueDue(cycle components.ActiveCyclicAction) bool {
	if cycle.SoundBinding == nil || cycle.CycleDurationTicks == 0 || cycle.NextSoundCue >= len(cycle.SoundBinding.WorldSoundCues) {
		return false
	}
	cue := cycle.SoundBinding.WorldSoundCues[cycle.NextSoundCue]
	return float64(cycle.CycleElapsedTicks)/float64(cycle.CycleDurationTicks) >= cue.Phase
}

func advanceActionSoundCues(world *ecs.World, actor types.Handle, cycle *components.ActiveCyclicAction, service *SoundEventService) {
	for actionSoundCueDue(*cycle) {
		cue := cycle.SoundBinding.WorldSoundCues[cycle.NextSoundCue]
		// Budget loss or an unavailable source must not replay a passed contact.
		cycle.NextSoundCue++
		if point, exists := actionSoundSourcePoint(world, actor, *cycle, cue.Source); exists && service != nil {
			service.EmitPoint(world, point, cue.SoundKey)
		}
	}
}

func actionSoundSourcePoint(world *ecs.World, actor types.Handle, cycle components.ActiveCyclicAction, source string) (soundPoint, bool) {
	handle := actor
	if source == "target" {
		if cycle.HasTargetPosition {
			return soundPoint{x: cycle.TargetX, y: cycle.TargetY}, true
		}
		if cycle.TargetKind != components.CyclicActionTargetSelf {
			handle = cycle.TargetHandle
			if handle == types.InvalidHandle {
				handle = world.GetHandleByEntityID(cycle.TargetID)
			}
			if entityID, exists := world.GetExternalID(handle); !exists || entityID != cycle.TargetID {
				return soundPoint{}, false
			}
		}
	} else if source != "actor" {
		return soundPoint{}, false
	}
	transform, exists := ecs.GetComponent[components.Transform](world, handle)
	if !exists {
		return soundPoint{}, false
	}
	return soundPoint{x: transform.X, y: transform.Y}, true
}
