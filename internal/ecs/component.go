package ecs

import (
	"reflect"
	"sync"
)

// ComponentID is a unique identifier for a component type
type ComponentID uint8

// Component is a marker interface for ECS components
type Component interface{}

// ComponentRegistry manages component type registration and ID assignment
// Thread-safe for concurrent registration
type ComponentRegistry struct {
	typeToID map[reflect.Type]ComponentID
	idToType map[ComponentID]reflect.Type
	nextID   ComponentID
	mu       sync.RWMutex
}

// Global component registry - initialized once
var globalRegistry = &ComponentRegistry{
	typeToID: make(map[reflect.Type]ComponentID),
	idToType: make(map[ComponentID]reflect.Type),
	nextID:   0,
}

// GetComponentID returns the ComponentID for a given component type
// Registers the type if not already registered
func GetComponentID[T Component]() ComponentID {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	globalRegistry.mu.RLock()
	if id, ok := globalRegistry.typeToID[t]; ok {
		globalRegistry.mu.RUnlock()
		return id
	}
	globalRegistry.mu.RUnlock()

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	// Double-check after acquiring write lock
	if id, ok := globalRegistry.typeToID[t]; ok {
		return id
	}

	id := globalRegistry.nextID
	globalRegistry.nextID++
	globalRegistry.typeToID[t] = id
	globalRegistry.idToType[id] = t
	return id
}

// ComponentMask is a bitset representing which components an entity has
// Supports up to 64 component types (can be extended with []uint64 if needed)
type ComponentMask uint64

// MaxComponentID is the maximum supported component ID (0-63)
const MaxComponentID = 63

// Set sets the bit for the given component ID
// Panics if id > MaxComponentID
func (m *ComponentMask) Set(id ComponentID) {
	if id > MaxComponentID {
		panic("component ID exceeds maximum (63)")
	}
	*m |= 1 << id
}

// Clear clears the bit for the given component ID
// Panics if id > MaxComponentID
func (m *ComponentMask) Clear(id ComponentID) {
	if id > MaxComponentID {
		panic("component ID exceeds maximum (63)")
	}
	*m &^= 1 << id
}

// Has checks if the mask has the given component ID
// Panics if id > MaxComponentID
func (m ComponentMask) Has(id ComponentID) bool {
	if id > MaxComponentID {
		panic("component ID exceeds maximum (63)")
	}
	return m&(1<<id) != 0
}

// HasAll checks if the mask has all the given component IDs
func (m ComponentMask) HasAll(other ComponentMask) bool {
	return m&other == other
}

// MaxSparseSize is the maximum size of the sparse array
// Bounded by max active handles, not by astronomical EntityID values
const MaxSparseSize = 1 << 21

// ComponentStorage provides type-safe dense storage for a single component type
// Uses sparse-dense array pattern for O(1) access and cache-friendly iteration
// Works with Handle (uint32) for compact sparse arrays
type ComponentStorage[T Component] struct {
	dense   []T      // Dense array of components
	sparse  []int32  // Handle -> dense index (-1 if not present)
	handles []Handle // Dense index -> Handle
	mu      sync.RWMutex
}

// NewComponentStorage creates a new component storage with initial capacity
func NewComponentStorage[T Component](capacity int) *ComponentStorage[T] {
	sparse := make([]int32, capacity)
	for i := range sparse {
		sparse[i] = -1
	}
	return &ComponentStorage[T]{
		dense:   make([]T, 0, capacity),
		sparse:  sparse,
		handles: make([]Handle, 0, capacity),
	}
}

// Set adds or updates a component for an entity
// Panics if handle exceeds MaxSparseSize
func (s *ComponentStorage[T]) Set(h Handle, component T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if int(h) >= MaxSparseSize {
		panic("handle exceeds maximum sparse size")
	}

	// Grow sparse array if needed (grow by 2x or to h+1, whichever is larger)
	if int(h) >= len(s.sparse) {
		newSize := len(s.sparse) * 2
		if newSize < int(h)+1 {
			newSize = int(h) + 1
		}
		if newSize > MaxSparseSize {
			newSize = MaxSparseSize
		}
		newSparse := make([]int32, newSize)
		for i := range newSparse {
			newSparse[i] = -1
		}
		copy(newSparse, s.sparse)
		s.sparse = newSparse
	}

	idx := s.sparse[h]
	if idx >= 0 {
		// Update existing
		s.dense[idx] = component
	} else {
		// Add new
		s.sparse[h] = int32(len(s.dense))
		s.dense = append(s.dense, component)
		s.handles = append(s.handles, h)
	}
}

// Get retrieves a component for an entity
func (s *ComponentStorage[T]) Get(h Handle) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var zero T
	if int(h) >= len(s.sparse) {
		return zero, false
	}

	idx := s.sparse[h]
	if idx < 0 {
		return zero, false
	}
	return s.dense[idx], true
}

// GetPtr retrieves a pointer to a component for an entity (for mutation)
func (s *ComponentStorage[T]) GetPtr(h Handle) *T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if int(h) >= len(s.sparse) {
		return nil
	}

	idx := s.sparse[h]
	if idx < 0 {
		return nil
	}
	return &s.dense[idx]
}

// Remove removes a component from an entity
func (s *ComponentStorage[T]) Remove(h Handle) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if int(h) >= len(s.sparse) {
		return false
	}

	idx := s.sparse[h]
	if idx < 0 {
		return false
	}

	// Swap with last element (swap-remove pattern)
	lastIdx := len(s.dense) - 1
	if int(idx) != lastIdx {
		s.dense[idx] = s.dense[lastIdx]
		lastHandle := s.handles[lastIdx]
		s.handles[idx] = lastHandle
		s.sparse[lastHandle] = idx
	}

	s.dense = s.dense[:lastIdx]
	s.handles = s.handles[:lastIdx]
	s.sparse[h] = -1
	return true
}

// Has checks if an entity has this component
func (s *ComponentStorage[T]) Has(h Handle) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if int(h) >= len(s.sparse) {
		return false
	}
	return s.sparse[h] >= 0
}

// Len returns the number of components stored
func (s *ComponentStorage[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.dense)
}

// Iterate calls the callback for each handle-component pair
// Callback receives Handle and pointer to component for mutation
// IMPORTANT: Holds write lock during iteration - do not call other storage methods from callback
func (s *ComponentStorage[T]) Iterate(fn func(Handle, *T)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.dense {
		fn(s.handles[i], &s.dense[i])
	}
}

// IterateReadOnly calls the callback for each handle-component pair with copies
// Safe for concurrent read access - callback receives copies, not pointers
func (s *ComponentStorage[T]) IterateReadOnly(fn func(Handle, T)) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.dense {
		fn(s.handles[i], s.dense[i])
	}
}

// Handles returns a copy of all handles that have this component
func (s *ComponentStorage[T]) Handles() []Handle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Handle, len(s.handles))
	copy(result, s.handles)
	return result
}

// ExternalID stores the global unique EntityID (uint64) for an entity
// This component is present on ALL entities to map Handle -> EntityID
type ExternalID struct {
	ID EntityID
}

// ExternalIDComponentID is the ComponentID for ExternalID
// Registered during init
var ExternalIDComponentID ComponentID

func init() {
	ExternalIDComponentID = GetComponentID[ExternalID]()
}
