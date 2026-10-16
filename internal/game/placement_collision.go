package game

import (
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

const (
	placementMaxCandidates = 4096
	placementMaxCells      = 1024
	placementMaxVisits     = 4096
)

// colliderPlacementAllowed checks loaded colliders under the owning shard lock.
func colliderPlacementAllowed(w *ecs.World, collider components.Collider, x, y float64, actorHandle, objectHandle types.Handle, candidates []types.Handle) bool {
	bounds := core.ColliderBounds{MinX: x - collider.HalfWidth, MinY: y - collider.HalfHeight, MaxX: x + collider.HalfWidth, MaxY: y + collider.HalfHeight}
	if !(bounds.MinX < bounds.MaxX && bounds.MinY < bounds.MaxY) {
		return false
	}
	spatial, ok := ecs.TryGetResource[*core.WorldColliderSpatial](w)
	if !ok || *spatial == nil || (*spatial).Index == nil {
		return false
	}
	candidates, err := (*spatial).Index.QueryBoundedInto(bounds, candidates[:0], core.ColliderQueryBudget{
		MaxCells: placementMaxCells, MaxVisits: placementMaxVisits,
	})
	if err != nil {
		return false
	}
	for _, handle := range candidates {
		if handle == actorHandle || handle == objectHandle || !w.Alive(handle) {
			continue
		}
		target, hasCollider := ecs.GetComponent[components.Collider](w, handle)
		if !hasCollider || target.Phantom != nil || (collider.Layer&target.Mask == 0 && target.Layer&collider.Mask == 0) {
			continue
		}
		position, hasPosition := ecs.GetComponent[components.Transform](w, handle)
		if hasPosition && bounds.MinX < position.X+target.HalfWidth && bounds.MaxX > position.X-target.HalfWidth &&
			bounds.MinY < position.Y+target.HalfHeight && bounds.MaxY > position.Y-target.HalfHeight {
			return false
		}
	}
	return true
}
