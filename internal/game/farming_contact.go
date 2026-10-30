package game

import (
	"math"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	"origin/internal/types"
)

// Passable crops still need confirmed contact before the ordinary link and
// context-action lifecycle can start. Reuse LinkSystem's existing hook, which
// runs only for the intended/linked target and retains all movement guards.
func (s *Shard) additionalObjectContact(actorID types.EntityID, target types.Handle) bool {
	w := s.world
	if id, exists := w.GetExternalID(target); exists {
		if _, crop := behaviors.CropDefinition(w, target, id); crop {
			actor := w.GetHandleByEntityID(actorID)
			actorPosition, hasActor := ecs.GetComponent[components.Transform](w, actor)
			targetPosition, hasTarget := ecs.GetComponent[components.Transform](w, target)
			actorCollider, hasActorCollider := ecs.GetComponent[components.Collider](w, actor)
			targetCollider, hasTargetCollider := ecs.GetComponent[components.Collider](w, target)
			return hasActor && hasTarget && hasActorCollider && hasTargetCollider &&
				math.Abs(actorPosition.X-targetPosition.X) <= actorCollider.HalfWidth+targetCollider.HalfWidth &&
				math.Abs(actorPosition.Y-targetPosition.Y) <= actorCollider.HalfHeight+targetCollider.HalfHeight
		}
	}
	if runtime, exists := ecs.TryGetResource[hearth.Runtime](w); exists {
		return runtime.HasOwnerContact(actorID, target)
	}
	return false
}
