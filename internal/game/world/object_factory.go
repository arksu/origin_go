package world

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

// ObjectFactory builds and serializes world objects using object definitions.
type ObjectFactory struct {
	registry *objectdefs.Registry
}

// NewObjectFactory creates a factory backed by the given object definitions registry.
func NewObjectFactory(registry *objectdefs.Registry) *ObjectFactory {
	return &ObjectFactory{registry: registry}
}

// Registry returns the underlying object definitions registry.
func (f *ObjectFactory) Registry() *objectdefs.Registry {
	return f.registry
}

// Build creates an ECS entity from a raw database object using its definition.
func (f *ObjectFactory) Build(w *ecs.World, raw *repository.Object) (types.Handle, error) {
	def, ok := f.registry.GetByID(raw.TypeID)
	if !ok {
		return types.InvalidHandle, fmt.Errorf("%w: type_id=%d", ErrDefNotFound, raw.TypeID)
	}

	h := w.Spawn(types.EntityID(raw.ID), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CreateTransform(raw.X, raw.Y, int(raw.Heading.Int16)))

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    uint32(def.DefID),
			Behaviors: def.Behavior,
			IsStatic:  def.IsStatic,
			Region:    raw.Region,
			Layer:     raw.Layer,
		})

		// Collider from definition
		if def.Components != nil && def.Components.Collider != nil {
			c := def.Components.Collider
			ecs.AddComponent(w, h, components.Collider{
				HalfWidth:  c.W / 2.0,
				HalfHeight: c.H / 2.0,
				Layer:      c.Layer,
				Mask:       c.Mask,
			})
		}

		// Appearance: base resource from definition
		ecs.AddComponent(w, h, components.Appearance{
			Resource: def.Resource,
		})
	})
	if h == types.InvalidHandle {
		return types.InvalidHandle, ErrEntitySpawnFailed
	}

	return h, nil
}

// Serialize converts an ECS entity back to a database object for persistence.
func (f *ObjectFactory) Serialize(w *ecs.World, h types.Handle) (*repository.Object, error) {
	externalID, ok := ecs.GetComponent[ecs.ExternalID](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	info, ok := ecs.GetComponent[components.EntityInfo](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	transform, ok := ecs.GetComponent[components.Transform](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	chunkRef, ok := ecs.GetComponent[components.ChunkRef](w, h)
	if !ok {
		return nil, ErrEntityNotFound
	}

	obj := &repository.Object{
		ID:     int64(externalID.ID),
		TypeID: int(info.TypeID),
		Region: info.Region,
		X:      int(transform.X),
		Y:      int(transform.Y),
		Layer:  info.Layer,
		ChunkX: chunkRef.CurrentChunkX,
		ChunkY: chunkRef.CurrentChunkY,
	}

	return obj, nil
}

// IsStatic returns whether a raw object is static based on its definition.
func (f *ObjectFactory) IsStatic(raw *repository.Object) bool {
	def, ok := f.registry.GetByID(raw.TypeID)
	if !ok {
		return true // safe default
	}
	return def.IsStatic
}
