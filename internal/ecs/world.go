package ecs

import (
	"origin/internal/eventbus"
	"origin/internal/types"
	"reflect"
	"sort"
	"sync"
	"time"
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
	entities         map[types.Handle]ComponentMask
	locations        map[types.Handle]EntityLocation // O(1) archetype removal
	entityIDToHandle map[types.EntityID]types.Handle // O(1) EntityID -> Handle lookup
	entityIDMu       sync.RWMutex                    // Protects entityIDToHandle map for concurrent access
	archetypes       *ArchetypeGraph
	handles          *HandleAllocator

	// System scheduling
	systems       []System
	systemsSorted bool

	// Component storages (type-erased, accessed via typed helpers)
	storages map[ComponentID]any

	// Resources (singleton data shared across systems, keyed by reflect.Type)
	resources map[reflect.Type]any

	// Event bus for publishing events
	eventBus *eventbus.EventBus

	// Layer for this world (shard layer)
	Layer int

	// Delta time for current tick
	DeltaTime float64

	// Tick counter for system throttling (UpdateEveryNTicks)
	tickCount uint64

	// Per-system timing stats
	systemStats []SystemTimingStat
}

// SystemTimingStat holds accumulated timing for a single system
type SystemTimingStat struct {
	Name        string
	DurationSum time.Duration
	Count       uint64
}

// NewWorld creates a new ECS world with default capacity
func NewWorld(eventBus *eventbus.EventBus, layer int) *World {
	return NewWorldWithCapacity(DefaultMaxHandles, eventBus, layer)
}

// NewWorldForTesting creates a new ECS world for testing (without event bus)
func NewWorldForTesting() *World {
	return NewWorldWithCapacity(DefaultMaxHandles, nil, 0)
}

// NewWorldWithCapacity creates a new ECS world with specified max handles
func NewWorldWithCapacity(maxHandles uint32, eventBus *eventbus.EventBus, layer int) *World {
	w := &World{
		entities:         make(map[types.Handle]ComponentMask, 1024),
		locations:        make(map[types.Handle]EntityLocation, 1024),
		entityIDToHandle: make(map[types.EntityID]types.Handle, 1024),
		archetypes:       NewArchetypeGraph(),
		handles:          NewHandleAllocator(maxHandles),
		systems:          make([]System, 0, 16),
		storages:         make(map[ComponentID]any),
		resources:        make(map[reflect.Type]any),
		eventBus:         eventBus,
		Layer:            layer,
	}

	// Initialise built-in resources
	InitResource(w, MovedEntities{
		Handles: make([]types.Handle, 2048),
		IntentX: make([]float64, 2048),
		IntentY: make([]float64, 2048),
		Count:   0,
	})
	InitResource(w, VisibilityState{
		VisibleByObserver:        make(map[types.Handle]ObserverVisibility, 256),
		ObserversByVisibleTarget: make(map[types.Handle]map[types.Handle]struct{}, 256),
	})
	InitResource(w, DetachedEntities{
		Map: make(map[types.EntityID]DetachedEntity, 64),
	})
	InitResource(w, CharacterEntities{
		Map: make(map[types.EntityID]CharacterEntity, 64),
	})
	InitResource(w, InventoryRefIndex{
		index: make(map[InventoryRefKey]types.Handle, 64),
	})
	InitResource(w, LinkState{
		LinkedByPlayer:  make(map[types.EntityID]PlayerLink, 64),
		PlayersByTarget: make(map[types.EntityID]map[types.EntityID]struct{}, 64),
		IntentByPlayer:  make(map[types.EntityID]LinkIntent, 64),
	})
	InitResource(w, OpenContainerState{
		PendingAutoOpenByPlayer: make(map[types.EntityID]types.EntityID, 64),
		OpenRootByPlayer:        make(map[types.EntityID]types.EntityID, 64),
		PlayersByRoot:           make(map[types.EntityID]map[types.EntityID]struct{}, 64),
		OpenRefsByPlayer:        make(map[types.EntityID]map[InventoryRefKey]struct{}, 64),
		PlayersByRef:            make(map[InventoryRefKey]map[types.EntityID]struct{}, 64),
	})
	InitResource(w, ObjectBehaviorDirtyQueue{
		pending: make([]types.Handle, 0, 256),
		inQueue: make(map[types.Handle]struct{}, 256),
		head:    0,
	})
	InitResource(w, TimeState{})

	return w
}

// System interface for ECS systems
type System interface {
	Update(w *World, dt float64)
	Priority() int // Lower priority runs first
	Name() string
	UpdateEveryNTicks() uint64 // 0 = every tick, N = every N ticks
}

// BaseSystem provides default implementations for System interface
type BaseSystem struct {
	priority          int
	name              string
	updateEveryNTicks uint64
}

func NewBaseSystem(name string, priority int) BaseSystem {
	return BaseSystem{name: name, priority: priority}
}

func NewBaseSystemWithInterval(name string, priority int, everyNTicks uint64) BaseSystem {
	return BaseSystem{name: name, priority: priority, updateEveryNTicks: everyNTicks}
}

func (s BaseSystem) Priority() int             { return s.priority }
func (s BaseSystem) Name() string              { return s.name }
func (s BaseSystem) UpdateEveryNTicks() uint64 { return s.updateEveryNTicks }

// Spawn creates a new entity with the given external EntityID
// Returns the Handle for internal ECS operations
// The ExternalID component is automatically added to map Handle -> EntityID
// The setupFunc callback is called to add all other components
// Single-threaded - no lock needed
func (w *World) Spawn(externalID types.EntityID, setupFunc func(*World, types.Handle)) types.Handle {
	h := w.handles.Alloc()
	if h == types.InvalidHandle {
		return types.InvalidHandle
	}

	w.entities[h] = 0 // Empty component mask
	arch := w.archetypes.GetOrCreate(0)
	index := arch.AddEntity(h)
	w.locations[h] = EntityLocation{archetype: arch, index: index}

	// Maintain reverse lookup map
	w.entityIDMu.Lock()
	w.entityIDToHandle[externalID] = h
	w.entityIDMu.Unlock()

	// Add ExternalID component
	AddComponent(w, h, ExternalID{ID: externalID})

	// Call setup function to add all other components
	if setupFunc != nil {
		setupFunc(w, h)
	}

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

	// Get EntityID before removing components
	var targetID types.EntityID
	if extID, ok := GetComponent[ExternalID](w, h); ok {
		targetID = extID.ID
	}

	// Remove from entityIDToHandle map if entity has ExternalID
	if targetID != 0 {
		w.entityIDMu.Lock()
		delete(w.entityIDToHandle, targetID)
		w.entityIDMu.Unlock()
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

// GetHandleByEntityID returns the handle for a given EntityID
// O(1) lookup using reverse map - optimized for frequent packet handling
// Returns InvalidHandle if entity not found
func (w *World) GetHandleByEntityID(entityID types.EntityID) types.Handle {
	w.entityIDMu.RLock()
	defer w.entityIDMu.RUnlock()

	if h, ok := w.entityIDToHandle[entityID]; ok {
		return h
	}
	return types.InvalidHandle
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
		// (Re)initialize per-system stats slice
		w.systemStats = make([]SystemTimingStat, len(w.systems))
		for i, sys := range w.systems {
			w.systemStats[i].Name = sys.Name()
		}
	}
	w.DeltaTime = dt
	w.tickCount++

	for i, sys := range w.systems {
		if n := sys.UpdateEveryNTicks(); n > 0 && w.tickCount%n != 0 {
			continue
		}
		start := time.Now()
		sys.Update(w, dt)
		w.systemStats[i].DurationSum += time.Since(start)
		w.systemStats[i].Count++
	}
}

// DrainSystemStats returns accumulated per-system timing stats and resets them.
// Single-threaded - no lock needed (called from shard under lock).
func (w *World) DrainSystemStats() []SystemTimingStat {
	out := make([]SystemTimingStat, len(w.systemStats))
	copy(out, w.systemStats)
	for i := range w.systemStats {
		w.systemStats[i].DurationSum = 0
		w.systemStats[i].Count = 0
	}
	return out
}

// AddExternalTiming allows external components (like ChunkManager) to add timing data.
// This creates a virtual "system" entry for non-ECS work done during the tick.
// Single-threaded - no lock needed (called from shard under lock).
func (w *World) AddExternalTiming(name string, duration time.Duration) {
	for i := range w.systemStats {
		if w.systemStats[i].Name == name {
			w.systemStats[i].DurationSum += duration
			w.systemStats[i].Count++
			return
		}
	}
	// If not found, append new entry
	w.systemStats = append(w.systemStats, SystemTimingStat{
		Name:        name,
		DurationSum: duration,
		Count:       1,
	})
}

// Query creates a new query for this world
func (w *World) Query() *Query {
	return NewQuery(w)
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

// HasComponent checks if an entity has a component
func HasComponent[T Component](w *World, h types.Handle) bool {
	storage := GetOrCreateStorage[T](w)
	return storage.Has(h)
}
