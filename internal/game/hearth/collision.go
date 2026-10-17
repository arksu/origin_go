package hearth

import (
	"math"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// IgnoreObjectCollision permits only an active hearth's owner to pass through it.
// The caller must hold the owning surface shard lock while reading the world.
func (r *Runtime) IgnoreObjectCollision(actorID types.EntityID, targetHandle types.Handle) bool {
	if actorID == 0 {
		return false
	}
	owner, _, _, owned := r.Owner(targetHandle)
	return owned && owner == actorID
}

// HasOwnerContact confirms interaction contact without making the owner's hearth
// physically solid. LinkSystem owns intent, movement and link lifetime checks.
func (r *Runtime) HasOwnerContact(actorID types.EntityID, targetHandle types.Handle) bool {
	if !r.IgnoreObjectCollision(actorID, targetHandle) {
		return false
	}
	actorHandle := r.w.GetHandleByEntityID(actorID)
	if !r.w.Alive(actorHandle) {
		return false
	}
	actor, hasActor := ecs.GetComponent[components.Transform](r.w, actorHandle)
	target, hasTarget := ecs.GetComponent[components.Transform](r.w, targetHandle)
	actorCollider, hasActorCollider := ecs.GetComponent[components.Collider](r.w, actorHandle)
	targetCollider, hasTargetCollider := ecs.GetComponent[components.Collider](r.w, targetHandle)
	return hasActor && hasTarget && hasActorCollider && hasTargetCollider &&
		math.Abs(actor.X-target.X) <= actorCollider.HalfWidth+targetCollider.HalfWidth &&
		math.Abs(actor.Y-target.Y) <= actorCollider.HalfHeight+targetCollider.HalfHeight
}
