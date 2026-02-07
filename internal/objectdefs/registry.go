package objectdefs

import (
	"sync"
)

// Registry holds all loaded object definitions.
// It is read-only after initialization and thread-safe.
type Registry struct {
	byID  map[int]*ObjectDef
	byKey map[string]*ObjectDef
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

// NewRegistry creates a new Registry from a slice of ObjectDef.
func NewRegistry(objects []ObjectDef) *Registry {
	r := &Registry{
		byID:  make(map[int]*ObjectDef, len(objects)),
		byKey: make(map[string]*ObjectDef, len(objects)),
	}
	for i := range objects {
		obj := &objects[i]
		r.byID[obj.DefID] = obj
		r.byKey[obj.Key] = obj
	}
	return r
}

// GetByID returns an object definition by its defId.
func (r *Registry) GetByID(defID int) (*ObjectDef, bool) {
	obj, ok := r.byID[defID]
	return obj, ok
}

// GetByKey returns an object definition by its key.
func (r *Registry) GetByKey(key string) (*ObjectDef, bool) {
	obj, ok := r.byKey[key]
	return obj, ok
}

// Count returns the total number of object definitions.
func (r *Registry) Count() int {
	return len(r.byID)
}

// All returns all object definitions.
func (r *Registry) All() []*ObjectDef {
	result := make([]*ObjectDef, 0, len(r.byID))
	for _, obj := range r.byID {
		result = append(result, obj)
	}
	return result
}

// SetGlobal sets the global registry (should be called once at startup).
func SetGlobal(r *Registry) {
	registryOnce.Do(func() {
		globalRegistry = r
	})
}

// SetGlobalForTesting replaces the global registry without sync.Once protection.
// Must only be used in tests.
func SetGlobalForTesting(r *Registry) {
	globalRegistry = r
}

// Global returns the global registry.
func Global() *Registry {
	return globalRegistry
}
