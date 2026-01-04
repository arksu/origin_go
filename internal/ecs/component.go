package ecs

import (
	"reflect"
	"sync"
)

// ComponentID is a unique identifier for a component type
type ComponentID uint8

// Component is a marker interface for ECS components
type Component interface{}

// ComponentRegistry manages component type registration with explicit IDs
// Thread-safe for concurrent registration
// IDs must be explicitly assigned to ensure deterministic behavior across builds
type ComponentRegistry struct {
	typeToID map[reflect.Type]ComponentID
	idToType map[ComponentID]reflect.Type
	mu       sync.RWMutex
}

// Global component registry - initialized once
var globalRegistry = &ComponentRegistry{
	typeToID: make(map[reflect.Type]ComponentID),
	idToType: make(map[ComponentID]reflect.Type),
}

// RegisterComponent registers a component type with an explicit, stable ID
// Must be called during init() to ensure deterministic registration
// Panics if ID is already registered or exceeds MaxComponentID
func RegisterComponent[T Component](id ComponentID) {
	if id > MaxComponentID {
		panic("component ID exceeds maximum (63)")
	}

	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	if existingType, exists := globalRegistry.idToType[id]; exists {
		panic("component ID " + string(id) + " already registered for type " + existingType.String())
	}

	if existingID, exists := globalRegistry.typeToID[t]; exists {
		panic("component type " + t.String() + " already registered with ID " + string(rune(existingID)))
	}

	globalRegistry.typeToID[t] = id
	globalRegistry.idToType[id] = t
}

// GetComponentID returns the ComponentID for a given component type
// Panics if the component type is not registered
// Components must be registered via RegisterComponent during init()
func GetComponentID[T Component]() ComponentID {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	id, ok := globalRegistry.typeToID[t]
	if !ok {
		panic("component type " + t.String() + " not registered - call RegisterComponent during init()")
	}
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
// Single-threaded per shard - no locks needed
type ComponentStorage[T Component] struct {
	dense   []T      // Dense array of components
	sparse  []int32  // Handle -> dense index (-1 if not present)
	handles []Handle // Dense index -> Handle
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
// Panics if handle index exceeds MaxSparseSize
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) Set(h Handle, component T) {

	index := h.Index()
	if int(index) >= MaxSparseSize {
		panic("handle index exceeds maximum sparse size")
	}

	// Grow sparse array if needed (grow by 2x or to index+1, whichever is larger)
	if int(index) >= len(s.sparse) {
		newSize := len(s.sparse) * 2
		if newSize < int(index)+1 {
			newSize = int(index) + 1
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

	idx := s.sparse[index]
	if idx >= 0 {
		// Update existing
		s.dense[idx] = component
		s.handles[idx] = h // Update handle to latest generation
	} else {
		// Add new
		s.sparse[index] = int32(len(s.dense))
		s.dense = append(s.dense, component)
		s.handles = append(s.handles, h)
	}
}

// Get retrieves a component for an entity
// Returns false if handle generation doesn't match (stale handle)
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) Get(h Handle) (T, bool) {

	var zero T
	index := h.Index()
	if int(index) >= len(s.sparse) {
		return zero, false
	}

	idx := s.sparse[index]
	if idx < 0 {
		return zero, false
	}
	// Validate generation to prevent stale handle access
	if s.handles[idx] != h {
		return zero, false
	}
	return s.dense[idx], true
}

// Mutate executes a callback with a pointer to the component for mutation
// The callback returns bool to indicate success/failure or conditional logic
// The pointer is only valid within the callback scope
// Returns false if handle generation doesn't match (stale handle)
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) Mutate(h Handle, fn func(*T) bool) bool {

	index := h.Index()
	if int(index) >= len(s.sparse) {
		return false
	}

	idx := s.sparse[index]
	if idx < 0 {
		return false
	}
	// Validate generation to prevent stale handle access
	if s.handles[idx] != h {
		return false
	}
	return fn(&s.dense[idx])
}

// WithPtr executes a callback with a pointer to the component for mutation
// The pointer is only valid within the callback scope
// Returns false if the component doesn't exist or handle generation doesn't match
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) WithPtr(h Handle, fn func(*T)) bool {

	index := h.Index()
	if int(index) >= len(s.sparse) {
		return false
	}

	idx := s.sparse[index]
	if idx < 0 {
		return false
	}
	// Validate generation to prevent stale handle access
	if s.handles[idx] != h {
		return false
	}
	fn(&s.dense[idx])
	return true
}

// Remove removes a component from an entity
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) Remove(h Handle) bool {
	return s.remove(h)
}

// RemoveByHandle implements componentRemover interface for Despawn cleanup
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) RemoveByHandle(h Handle) {
	s.remove(h)
}

func (s *ComponentStorage[T]) remove(h Handle) bool {
	index := h.Index()
	if int(index) >= len(s.sparse) {
		return false
	}

	idx := s.sparse[index]
	if idx < 0 {
		return false
	}
	// Validate generation to prevent stale handle removal
	if s.handles[idx] != h {
		return false
	}

	lastIdx := len(s.dense) - 1
	if int(idx) != lastIdx {
		s.dense[idx] = s.dense[lastIdx]
		lastHandle := s.handles[lastIdx]
		s.handles[idx] = lastHandle
		s.sparse[lastHandle.Index()] = idx
	}

	s.dense = s.dense[:lastIdx]
	s.handles = s.handles[:lastIdx]
	s.sparse[index] = -1
	return true
}

// Has checks if an entity has this component
// Returns false if handle generation doesn't match (stale handle)
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) Has(h Handle) bool {

	index := h.Index()
	if int(index) >= len(s.sparse) {
		return false
	}
	idx := s.sparse[index]
	if idx < 0 {
		return false
	}
	// Validate generation to prevent stale handle check
	return s.handles[idx] == h
}

// Len returns the number of components stored
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) Len() int {
	return len(s.dense)
}

// Iterate calls the callback for each handle-component pair
// Callback receives Handle and pointer to component for mutation
// Single-threaded - safe to call other methods from callback
func (s *ComponentStorage[T]) Iterate(fn func(Handle, *T)) {
	for i := range s.dense {
		fn(s.handles[i], &s.dense[i])
	}
}

// IterateReadOnly calls the callback for each handle-component pair with copies
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) IterateReadOnly(fn func(Handle, T)) {
	for i := range s.dense {
		fn(s.handles[i], s.dense[i])
	}
}

// Handles returns a copy of all handles that have this component
// Single-threaded - no lock needed
func (s *ComponentStorage[T]) Handles() []Handle {
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
// ID 0 is reserved for the mandatory ExternalID component
const ExternalIDComponentID ComponentID = 0

func init() {
	RegisterComponent[ExternalID](ExternalIDComponentID)
}
