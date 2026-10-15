package hearth

import (
	"slices"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// IgnoreObjectCollision permits only an active hearth's owner to pass through it.
// The caller must hold the owning surface shard lock while reading the world.
func (r *Runtime) IgnoreObjectCollision(actorID types.EntityID, targetHandle types.Handle) bool {
	if r == nil || r.w == nil || r.Index == nil || actorID == 0 || r.w.Layer != SurfaceLayer || !r.w.Alive(targetHandle) {
		return false
	}
	info, ok := ecs.GetComponent[components.EntityInfo](r.w, targetHandle)
	if !ok || info.Layer != SurfaceLayer || !slices.Contains(info.Behaviors, "hearth") {
		return false
	}
	targetID, ok := r.w.GetExternalID(targetHandle)
	if !ok {
		return false
	}
	owner, owned := r.Index.Owner(ObjectKey{Region: info.Region, ID: targetID})
	return owned && owner == actorID
}
