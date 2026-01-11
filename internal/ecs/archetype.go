package ecs

import "origin/internal/types"

// Archetype represents a unique combination of component types
// Entities with the same component set share an archetype for cache-efficient iteration
// Single-threaded per shard - no locks needed
type Archetype struct {
	mask    ComponentMask
	handles []types.Handle
}

// NewArchetype creates a new archetype with the given component mask
func NewArchetype(mask ComponentMask) *Archetype {
	return &Archetype{
		mask:    mask,
		handles: make([]types.Handle, 0, 64),
	}
}

// Mask returns the component mask for this archetype
func (a *Archetype) Mask() ComponentMask {
	return a.mask
}

// AddHandle adds an entity to this archetype
// Returns the index where the entity was added
// Single-threaded - no lock needed
func (a *Archetype) AddHandle(h types.Handle) int {
	index := len(a.handles)
	a.handles = append(a.handles, h)
	return index
}

// RemoveHandle removes an entity from this archetype using swap-remove
// O(n) - deprecated, use RemoveHandleAt with location tracking instead
// Single-threaded - no lock needed
func (a *Archetype) RemoveHandle(h types.Handle) bool {
	for i, handle := range a.handles {
		if handle == h {
			// Swap with last and truncate
			last := len(a.handles) - 1
			a.handles[i] = a.handles[last]
			a.handles = a.handles[:last]
			return true
		}
	}
	return false
}

// RemoveHandleAt removes an entity at the given index using swap-remove
// O(1) operation - requires location tracking
// Returns the handle that was swapped into this position (or InvalidHandle if removed last)
// Single-threaded - no lock needed
func (a *Archetype) RemoveHandleAt(index int) types.Handle {
	if index < 0 || index >= len(a.handles) {
		return types.InvalidHandle
	}

	last := len(a.handles) - 1
	if index == last {
		// Removing last element, no swap needed
		a.handles = a.handles[:last]
		return types.InvalidHandle
	}

	// Swap with last and truncate
	swappedHandle := a.handles[last]
	a.handles[index] = swappedHandle
	a.handles = a.handles[:last]
	return swappedHandle
}

// GetHandles returns a copy of all handles in this archetype
// Single-threaded - no lock needed
func (a *Archetype) GetHandles() []types.Handle {
	result := make([]types.Handle, len(a.handles))
	copy(result, a.handles)
	return result
}

// Len returns the number of entities in this archetype
// Single-threaded - no lock needed
func (a *Archetype) Len() int {
	return len(a.handles)
}

// AddEntity is an alias for AddHandle for backward compatibility
func (a *Archetype) AddEntity(h types.Handle) int {
	return a.AddHandle(h)
}

// RemoveEntity is an alias for RemoveHandle for backward compatibility
func (a *Archetype) RemoveEntity(h types.Handle) bool {
	return a.RemoveHandle(h)
}

// RemoveEntityAt is an alias for RemoveHandleAt for backward compatibility
func (a *Archetype) RemoveEntityAt(index int) types.Handle {
	return a.RemoveHandleAt(index)
}

// ForEachHandle iterates over handles without copying
// Single-threaded - no lock needed, safe to modify during iteration if needed
func (a *Archetype) ForEachHandle(fn func(types.Handle)) {
	for _, h := range a.handles {
		fn(h)
	}
}

// ArchetypeGraph manages archetype transitions and lookups
// Inspired by Bevy's archetype graph for efficient component add/remove
// Single-threaded per shard - no locks needed
// Tracks version to invalidate PreparedQuery caches when new archetypes are created
type ArchetypeGraph struct {
	archetypes map[ComponentMask]*Archetype
	version    int64 // Incremented when new archetype is created
}

// NewArchetypeGraph creates a new archetype graph
func NewArchetypeGraph() *ArchetypeGraph {
	return &ArchetypeGraph{
		archetypes: make(map[ComponentMask]*Archetype),
		version:    0,
	}
}

// GetOrCreate returns the archetype for the given mask, creating if needed
// Increments version when new archetype is created to invalidate PreparedQuery caches
// Single-threaded - no lock needed
func (g *ArchetypeGraph) GetOrCreate(mask ComponentMask) *Archetype {
	if arch, ok := g.archetypes[mask]; ok {
		return arch
	}

	// New archetype created - increment version
	g.version++
	arch := NewArchetype(mask)
	g.archetypes[mask] = arch
	return arch
}

// Get returns the archetype for the given mask, or nil if not found
// Single-threaded - no lock needed
func (g *ArchetypeGraph) Get(mask ComponentMask) *Archetype {
	return g.archetypes[mask]
}

// QueryArchetypes returns all archetypes that match the given component mask
// An archetype matches if it has ALL components in the query mask
// Single-threaded - no lock needed
func (g *ArchetypeGraph) QueryArchetypes(queryMask ComponentMask) []*Archetype {
	result := make([]*Archetype, 0, len(g.archetypes))
	for mask, arch := range g.archetypes {
		if mask.HasAll(queryMask) {
			result = append(result, arch)
		}
	}
	return result
}

// All returns all archetypes
// Single-threaded - no lock needed
func (g *ArchetypeGraph) All() []*Archetype {
	result := make([]*Archetype, 0, len(g.archetypes))
	for _, arch := range g.archetypes {
		result = append(result, arch)
	}
	return result
}

// Version returns the current archetype version
// Increments when new archetypes are created
func (g *ArchetypeGraph) Version() int64 {
	return g.version
}
