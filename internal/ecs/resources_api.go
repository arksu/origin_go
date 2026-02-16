package ecs

import (
	"fmt"
	"reflect"
)

// ---------------------------------------------------------------------------
// Typed Resource API
// ---------------------------------------------------------------------------
// Resources are singleton data shared across systems, stored by type.
// Usage:
//   ecs.InitResource(w, MovedEntities{...})   // at world init
//   res := ecs.GetResource[MovedEntities](w)  // in systems — returns *T, panics if missing
//   ecs.SetResource(w, value)                 // replace value
//   ok  := ecs.HasResource[MovedEntities](w)  // check existence
// ---------------------------------------------------------------------------

// resourceKey returns the reflect.Type used as map key for type T.
func resourceKey[T any]() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}

// InitResource stores an initial resource value in the World.
// Intended for use during World construction. Returns a pointer to the stored value.
func InitResource[T any](w *World, value T) *T {
	key := resourceKey[T]()
	ptr := new(T)
	*ptr = value
	w.resources[key] = ptr
	return ptr
}

// SetResource replaces (or inserts) a resource value in the World.
func SetResource[T any](w *World, value T) {
	key := resourceKey[T]()
	ptr := new(T)
	*ptr = value
	w.resources[key] = ptr
}

// GetResource returns a pointer to the resource of type T.
// Panics if the resource was never initialised — this is a programming error.
func GetResource[T any](w *World) *T {
	key := resourceKey[T]()
	v, ok := w.resources[key]
	if !ok {
		panic(fmt.Sprintf("ecs.GetResource: resource %v not initialised", key))
	}
	return v.(*T)
}

// TryGetResource returns a pointer and a bool indicating existence.
func TryGetResource[T any](w *World) (*T, bool) {
	key := resourceKey[T]()
	v, ok := w.resources[key]
	if !ok {
		return nil, false
	}
	return v.(*T), true
}

// HasResource checks whether a resource of type T exists.
func HasResource[T any](w *World) bool {
	key := resourceKey[T]()
	_, ok := w.resources[key]
	return ok
}
