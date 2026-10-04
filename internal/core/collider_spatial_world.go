package core

import (
	"fmt"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// Half a base cell keeps small movements out of the membership hot path.
// The index still filters candidates by exact bounds before sector geometry.
const colliderReindexMargin = colliderCellSize / 2

// WorldColliderSpatial owns the index and the cached storages needed to keep
// it aligned with committed transforms. Attach it before loading the world.
type WorldColliderSpatial struct {
	Index      *ColliderSpatialIndex
	transforms *ecs.ComponentStorage[components.Transform]
	colliders  *ecs.ComponentStorage[components.Collider]
}

func AttachColliderSpatial(world *ecs.World) *WorldColliderSpatial {
	if existing, ok := ecs.TryGetResource[*WorldColliderSpatial](world); ok {
		return *existing
	}
	spatial := &WorldColliderSpatial{
		Index:      NewColliderSpatialIndex(),
		transforms: ecs.GetOrCreateStorage[components.Transform](world),
		colliders:  ecs.GetOrCreateStorage[components.Collider](world),
	}
	ecs.SetResource(world, spatial)
	world.AddComponentObserver(components.TransformComponentID, spatial.Sync)
	world.AddComponentObserver(components.ColliderComponentID, spatial.Sync)
	world.AddDespawnObserver(spatial.Index.Remove)
	// This also permits attachment to an already-populated test/tool world.
	world.Query().With(components.TransformComponentID).With(components.ColliderComponentID).ForEach(spatial.Sync)
	return spatial
}

func (spatial *WorldColliderSpatial) Sync(handle types.Handle) {
	transform, exists := spatial.transforms.Get(handle)
	if !exists {
		spatial.Index.Remove(handle)
		return
	}
	spatial.OnPositionCommitted(handle, transform.X, transform.Y)
}

func (spatial *WorldColliderSpatial) OnPositionCommitted(handle types.Handle, positionX, positionY float64) {
	collider, exists := spatial.colliders.Get(handle)
	if !exists {
		spatial.Index.Remove(handle)
		return
	}
	if collider.HalfWidth <= 0 || collider.HalfHeight <= 0 {
		panic(fmt.Sprintf("invalid collider dimensions for handle %d", handle))
	}
	if err := spatial.Index.upsert(handle, ColliderBounds{
		MinX: positionX - collider.HalfWidth, MinY: positionY - collider.HalfHeight,
		MaxX: positionX + collider.HalfWidth, MaxY: positionY + collider.HalfHeight,
	}, colliderReindexMargin); err != nil {
		panic(err)
	}
}
