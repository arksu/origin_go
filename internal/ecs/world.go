package ecs

import (
	"sort"
	"sync"
)

// DefaultMaxHandles is the default maximum number of active entities
const DefaultMaxHandles uint32 = 1 << 20 // 1M entities

// World manages all entities and systems for one shard
// Optimized for cache-friendly iteration using archetype-based storage
// Uses compact Handle (uint32) internally for all operations
type World struct {
	// Entity management
	entities   map[Handle]ComponentMask
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

	mu sync.RWMutex
}

// NewWorld creates a new ECS world with default capacity
func NewWorld() *World {
	return NewWorldWithCapacity(DefaultMaxHandles)
}

// NewWorldWithCapacity creates a new ECS world with specified max handles
func NewWorldWithCapacity(maxHandles uint32) *World {
	w := &World{
		entities:   make(map[Handle]ComponentMask, 1024),
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
func (w *World) Spawn(externalID EntityID) Handle {
	w.mu.Lock()

	h := w.handles.Alloc()
	if h == InvalidHandle {
		w.mu.Unlock()
		return InvalidHandle
	}

	w.entities[h] = 0 // Empty component mask
	w.archetypes.GetOrCreate(0).AddEntity(h)

	// Add ExternalID component (done inside lock, storage has its own lock)
	w.mu.Unlock()
	AddComponent(w, h, ExternalID{ID: externalID})

	return h
}

// SpawnWithoutExternalID creates a new entity without external ID
// Useful for temporary/runtime-only entities
func (w *World) SpawnWithoutExternalID() Handle {
	w.mu.Lock()
	defer w.mu.Unlock()

	h := w.handles.Alloc()
	if h == InvalidHandle {
		return InvalidHandle
	}

	w.entities[h] = 0
	w.archetypes.GetOrCreate(0).AddEntity(h)
	return h
}

// Despawn removes an entity and all its components
func (w *World) Despawn(h Handle) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	mask, ok := w.entities[h]
	if !ok {
		return false
	}

	// Remove from archetype
	if arch := w.archetypes.Get(mask); arch != nil {
		arch.RemoveEntity(h)
	}

	delete(w.entities, h)
	w.handles.Free(h)
	return true
}

// Alive checks if an entity exists
func (w *World) Alive(h Handle) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, ok := w.entities[h]
	return ok
}

// EntityCount returns the number of alive entities
func (w *World) EntityCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.entities)
}

// GetMask returns the component mask for an entity
func (w *World) GetMask(h Handle) ComponentMask {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.entities[h]
}

// GetExternalID returns the external EntityID for a handle
func (w *World) GetExternalID(h Handle) (EntityID, bool) {
	ext, ok := GetComponent[ExternalID](w, h)
	if !ok {
		return 0, false
	}
	return ext.ID, true
}

// updateEntityArchetype moves entity to new archetype after component change
func (w *World) updateEntityArchetype(h Handle, oldMask, newMask ComponentMask) {
	if oldMask == newMask {
		return
	}

	if arch := w.archetypes.Get(oldMask); arch != nil {
		arch.RemoveEntity(h)
	}
	w.archetypes.GetOrCreate(newMask).AddEntity(h)
	w.entities[h] = newMask
}

// AddSystem registers a system to be run each tick
func (w *World) AddSystem(system System) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.systems = append(w.systems, system)
	w.systemsSorted = false
}

// Update runs all systems in priority order
func (w *World) Update(dt float64) {
	w.mu.Lock()
	if !w.systemsSorted {
		sort.Slice(w.systems, func(i, j int) bool {
			return w.systems[i].Priority() < w.systems[j].Priority()
		})
		w.systemsSorted = true
	}
	systems := make([]System, len(w.systems))
	copy(systems, w.systems)
	w.DeltaTime = dt
	w.mu.Unlock()

	for _, sys := range systems {
		sys.Update(w, dt)
	}
}

// Query creates a new query for this world
func (w *World) Query() *Query {
	return NewQuery(w)
}

// SetResource stores a singleton resource
func (w *World) SetResource(key, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.resources[key] = value
}

// GetResource retrieves a singleton resource
func (w *World) GetResource(key any) (any, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	v, ok := w.resources[key]
	return v, ok
}

// GetStorage returns the component storage for a given component ID
// Creates storage if it doesn't exist
func (w *World) GetStorage(componentID ComponentID) any {
	w.mu.RLock()
	storage, ok := w.storages[componentID]
	w.mu.RUnlock()
	if ok {
		return storage
	}
	return nil
}

// SetStorage sets the component storage for a given component ID
func (w *World) SetStorage(componentID ComponentID, storage any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.storages[componentID] = storage
}

// GetOrCreateStorage returns existing storage or creates new one
func GetOrCreateStorage[T Component](w *World) *ComponentStorage[T] {
	componentID := GetComponentID[T]()

	w.mu.RLock()
	if storage, ok := w.storages[componentID]; ok {
		w.mu.RUnlock()
		return storage.(*ComponentStorage[T])
	}
	w.mu.RUnlock()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Double-check
	if storage, ok := w.storages[componentID]; ok {
		return storage.(*ComponentStorage[T])
	}

	storage := NewComponentStorage[T](1024)
	w.storages[componentID] = storage
	return storage
}

// AddComponent adds a component to an entity
func AddComponent[T Component](w *World, h Handle, component T) {
	componentID := GetComponentID[T]()
	storage := GetOrCreateStorage[T](w)
	storage.Set(h, component)

	w.mu.Lock()
	oldMask := w.entities[h]
	newMask := oldMask
	newMask.Set(componentID)
	w.updateEntityArchetype(h, oldMask, newMask)
	w.mu.Unlock()
}

// GetComponent retrieves a component from an entity
func GetComponent[T Component](w *World, h Handle) (T, bool) {
	storage := GetOrCreateStorage[T](w)
	return storage.Get(h)
}

// GetComponentPtr retrieves a pointer to a component for mutation
func GetComponentPtr[T Component](w *World, h Handle) *T {
	storage := GetOrCreateStorage[T](w)
	return storage.GetPtr(h)
}

// RemoveComponent removes a component from an entity
func RemoveComponent[T Component](w *World, h Handle) bool {
	componentID := GetComponentID[T]()
	storage := GetOrCreateStorage[T](w)

	if !storage.Remove(h) {
		return false
	}

	w.mu.Lock()
	oldMask := w.entities[h]
	newMask := oldMask
	newMask.Clear(componentID)
	w.updateEntityArchetype(h, oldMask, newMask)
	w.mu.Unlock()
	return true
}

// HasComponent checks if an entity has a component
func HasComponent[T Component](w *World, h Handle) bool {
	storage := GetOrCreateStorage[T](w)
	return storage.Has(h)
}
