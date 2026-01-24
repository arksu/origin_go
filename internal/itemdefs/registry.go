package itemdefs

import (
	"sync"
)

// Registry holds all loaded item definitions.
// It is read-only after initialization and thread-safe.
type Registry struct {
	byID  map[int]*ItemDef
	byKey map[string]*ItemDef
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

// NewRegistry creates a new Registry from a slice of ItemDef.
func NewRegistry(items []ItemDef) *Registry {
	r := &Registry{
		byID:  make(map[int]*ItemDef, len(items)),
		byKey: make(map[string]*ItemDef, len(items)),
	}
	for i := range items {
		item := &items[i]
		r.byID[item.DefID] = item
		r.byKey[item.Key] = item
	}
	return r
}

// GetByID returns an item definition by its defId.
func (r *Registry) GetByID(defID int) (*ItemDef, bool) {
	item, ok := r.byID[defID]
	return item, ok
}

// GetByKey returns an item definition by its key.
func (r *Registry) GetByKey(key string) (*ItemDef, bool) {
	item, ok := r.byKey[key]
	return item, ok
}

// Count returns the total number of item definitions.
func (r *Registry) Count() int {
	return len(r.byID)
}

// All returns all item definitions.
func (r *Registry) All() []*ItemDef {
	result := make([]*ItemDef, 0, len(r.byID))
	for _, item := range r.byID {
		result = append(result, item)
	}
	return result
}

// SetGlobal sets the global registry (should be called once at startup).
func SetGlobal(r *Registry) {
	registryOnce.Do(func() {
		globalRegistry = r
	})
}

// Global returns the global registry.
func Global() *Registry {
	return globalRegistry
}
