package ecs

import (
	"sort"
	"sync"

	"origin/internal/types"
)

// DefaultMaxHandles is the default maximum number of active entities
const DefaultMaxHandles uint32 = 1 << 20 // 1M entities

// EntityLocation tracks where an entity is stored in its archetype
type EntityLocation struct {
	archetype *Archetype
	index     int // Index within archetype.handles
}

// World manages all entities and systems for one shard
// Single-threaded ECS per shard - no internal locks
// External synchronization via command queue for cross-thread access
type World struct {
	// Entity management
	entities   map[types.Handle]ComponentMask
	locations  map[types.Handle]EntityLocation // O(1) archetype removal
	archetypes *ArchetypeGraph
	handles    *HandleAllocator

	// System scheduling
	systems       []System
	systemsSorted bool

	// Component storages (type-erased, accessed via typed helpers)
	storages map[ComponentID]any

	// Resources (singleton data shared across systems)
	resources map[any]any

	// Delta time for current tick
	DeltaTime float64

	// Optional: mutex for external command queue synchronization
	// Only used if World is accessed from multiple threads (not recommended)
	mu sync.RWMutex
}

// NewWorld creates a new ECS world with default capacity
func NewWorld() *World {
	return NewWorldWithCapacity(DefaultMaxHandles)
}

// NewWorldWithCapacity creates a new ECS world with specified max handles
func NewWorldWithCapacity(maxHandles uint32) *World {
	w := &World{
		entities:   make(map[types.Handle]ComponentMask, 1024),
		locations:  make(map[types.Handle]EntityLocation, 1024),
		archetypes: NewArchetypeGraph(),
		handles:    NewHandleAllocator(maxHandles),
		systems:    make([]System, 0, 16),
		storages:   make(map[ComponentID]any),
		resources:  make(map[any]any),
	}
	return w
}

// System interface for ECS systems
type System interface {
	Update(w *World, dt float64)
	Priority() int // Lower priority runs first
	Name() string
}

// BaseSystem provides default implementations for System interface
type BaseSystem struct {
	priority int
	name     string
}

func NewBaseSystem(name string, priority int) BaseSystem {
	return BaseSystem{name: name, priority: priority}
}

func (s BaseSystem) Priority() int { return s.priority }
func (s BaseSystem) Name() string  { return s.name }

// Spawn creates a new entity with the given external EntityID
// Returns the Handle for internal ECS operations
// The ExternalID component is automatically added to map Handle -> EntityID
// Single-threaded - no lock needed
func (w *World) Spawn(externalID types.EntityID) types.Handle {
	h := w.handles.Alloc()
	if h == types.InvalidHandle {
		return types.InvalidHandle
	}

	w.entities[h] = 0 // Empty component mask
	arch := w.archetypes.GetOrCreate(0)
	index := arch.AddEntity(h)
	w.locations[h] = EntityLocation{archetype: arch, index: index}

	// Add ExternalID component
	AddComponent(w, h, ExternalID{ID: externalID})

	return h
}

// SpawnWithoutExternalID creates a new entity without external ID
// Useful for temporary/runtime-only entities
// Single-threaded - no lock needed
func (w *World) SpawnWithoutExternalID() types.Handle {
	h := w.handles.Alloc()
	if h == types.InvalidHandle {
		return types.InvalidHandle
	}

	w.entities[h] = 0
	arch := w.archetypes.GetOrCreate(0)
	index := arch.AddEntity(h)
	w.locations[h] = EntityLocation{archetype: arch, index: index}
	return h
}

// Despawn removes an entity and all its components
// Single-threaded - no lock needed
func (w *World) Despawn(h types.Handle) bool {
	mask, ok := w.entities[h]
	if !ok {
		return false
	}

	if loc, ok := w.locations[h]; ok {
		if swappedHandle := loc.archetype.RemoveEntityAt(loc.index); swappedHandle != types.InvalidHandle {
			// Update location of swapped entity
			w.locations[swappedHandle] = EntityLocation{archetype: loc.archetype, index: loc.index}
		}
		delete(w.locations, h)
	}

	delete(w.entities, h)
	w.handles.Free(h)

	// Remove components
	for id, storage := range w.storages {
		if mask.Has(id) {
			if remover, ok := storage.(componentRemover); ok {
				remover.RemoveByHandle(h)
			}
		}
	}

	return true
}

type componentRemover interface {
	RemoveByHandle(h types.Handle)
}

// Alive checks if an entity exists and handle generation is valid
// Returns false for stale handles (generation mismatch)
// Single-threaded - no lock needed
func (w *World) Alive(h types.Handle) bool {
	if h == types.InvalidHandle {
		return false
	}
	// Single map lookup - if present in entities, it's alive and valid
	// World maintains invariant: only valid handles are in entities map
	_, ok := w.entities[h]
	return ok
}

// EntityCount returns the number of alive entities
// Single-threaded - no lock needed
func (w *World) EntityCount() int {
	return len(w.entities)
}

// GetMask returns the component mask for an entity
// Single-threaded - no lock needed
func (w *World) GetMask(h types.Handle) ComponentMask {
	return w.entities[h]
}

// GetExternalID returns the external EntityID for a handle
func (w *World) GetExternalID(h types.Handle) (types.EntityID, bool) {
	ext, ok := GetComponent[ExternalID](w, h)
	if !ok {
		return 0, false
	}
	return ext.ID, true
}

// updateEntityArchetype moves entity to new archetype after component change
func (w *World) updateEntityArchetype(h types.Handle, oldMask, newMask ComponentMask) {
	if oldMask == newMask {
		return
	}

	// Remove from old archetype using O(1) location lookup
	if loc, ok := w.locations[h]; ok {
		if swappedHandle := loc.archetype.RemoveEntityAt(loc.index); swappedHandle != types.InvalidHandle {
			// Update location of swapped entity
			w.locations[swappedHandle] = EntityLocation{archetype: loc.archetype, index: loc.index}
		}
	}

	// Add to new archetype
	newArch := w.archetypes.GetOrCreate(newMask)
	newIndex := newArch.AddEntity(h)
	w.locations[h] = EntityLocation{archetype: newArch, index: newIndex}
	w.entities[h] = newMask
}

// AddSystem registers a system to be run each tick
// Single-threaded - no lock needed
func (w *World) AddSystem(system System) {
	w.systems = append(w.systems, system)
	w.systemsSorted = false
}

// Update runs all systems in priority order
// Single-threaded - no lock needed
func (w *World) Update(dt float64) {
	if !w.systemsSorted {
		sort.Slice(w.systems, func(i, j int) bool {
			return w.systems[i].Priority() < w.systems[j].Priority()
		})
		w.systemsSorted = true
	}
	w.DeltaTime = dt

	for _, sys := range w.systems {
		sys.Update(w, dt)
	}
}

// Query creates a new query for this world
func (w *World) Query() *Query {
	return NewQuery(w)
}

// SetResource stores a singleton resource
// Single-threaded - no lock needed
func (w *World) SetResource(key, value any) {
	w.resources[key] = value
}

// GetResource retrieves a singleton resource
// Single-threaded - no lock needed
func (w *World) GetResource(key any) (any, bool) {
	v, ok := w.resources[key]
	return v, ok
}

// GetStorage returns the component storage for a given component ID
// Single-threaded - no lock needed
func (w *World) GetStorage(componentID ComponentID) any {
	storage, ok := w.storages[componentID]
	if ok {
		return storage
	}
	return nil
}

// SetStorage sets the component storage for a given component ID
// Single-threaded - no lock needed
func (w *World) SetStorage(componentID ComponentID, storage any) {
	w.storages[componentID] = storage
}

// GetOrCreateStorage returns existing storage or creates new one
// Single-threaded - no lock needed
func GetOrCreateStorage[T Component](w *World) *ComponentStorage[T] {
	componentID := GetComponentID[T]()

	if storage, ok := w.storages[componentID]; ok {
		return storage.(*ComponentStorage[T])
	}

	storage := NewComponentStorage[T](1024)
	w.storages[componentID] = storage
	return storage
}

// AddComponent adds a component to an entity
// Single-threaded - no lock needed
func AddComponent[T Component](w *World, h types.Handle, component T) {
	componentID := GetComponentID[T]()
	storage := GetOrCreateStorage[T](w)
	storage.Set(h, component)

	oldMask := w.entities[h]
	newMask := oldMask
	newMask.Set(componentID)
	w.updateEntityArchetype(h, oldMask, newMask)
}

// GetComponent retrieves a component from an entity
func GetComponent[T Component](w *World, h types.Handle) (T, bool) {
	storage := GetOrCreateStorage[T](w)
	return storage.Get(h)
}

// MutateComponent executes a callback with a pointer to the component for mutation
// The callback returns bool to indicate success/failure or conditional logic
// The pointer is only valid within the callback scope
func MutateComponent[T Component](w *World, h types.Handle, fn func(*T) bool) bool {
	storage := GetOrCreateStorage[T](w)
	return storage.Mutate(h, fn)
}

// WithComponent executes a callback with a pointer to the component for mutation
// The pointer is only valid within the callback scope
// Returns false if the component doesn't exist
func WithComponent[T Component](w *World, h types.Handle, fn func(*T)) bool {
	storage := GetOrCreateStorage[T](w)
	return storage.WithPtr(h, fn)
}

// RemoveComponent removes a component from an entity
// Single-threaded - no lock needed
func RemoveComponent[T Component](w *World, h types.Handle) bool {
	componentID := GetComponentID[T]()
	storage := GetOrCreateStorage[T](w)

	if !storage.Remove(h) {
		return false
	}

	oldMask := w.entities[h]
	newMask := oldMask
	newMask.Clear(componentID)
	w.updateEntityArchetype(h, oldMask, newMask)
	return true
}

// HasComponent checks if an entity has a component
func HasComponent[T Component](w *World, h types.Handle) bool {
	storage := GetOrCreateStorage[T](w)
	return storage.Has(h)
}
