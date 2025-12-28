package ecs

import (
	"sync"
)

// Archetype represents a unique combination of component types
// Entities with the same component set share an archetype for cache-efficient iteration
// Uses Handle (uint32) for compact storage
type Archetype struct {
	mask    ComponentMask
	handles []Handle
	mu      sync.RWMutex
}

// NewArchetype creates a new archetype with the given component mask
func NewArchetype(mask ComponentMask) *Archetype {
	return &Archetype{
		mask:    mask,
		handles: make([]Handle, 0, 64),
	}
}

// Mask returns the component mask for this archetype
func (a *Archetype) Mask() ComponentMask {
	return a.mask
}

// AddEntity adds an entity to this archetype
func (a *Archetype) AddEntity(h Handle) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handles = append(a.handles, h)
}

// RemoveEntity removes an entity from this archetype using swap-remove
func (a *Archetype) RemoveEntity(h Handle) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i, e := range a.handles {
		if e == h {
			// Swap with last and truncate
			last := len(a.handles) - 1
			a.handles[i] = a.handles[last]
			a.handles = a.handles[:last]
			return true
		}
	}
	return false
}

// Handles returns a copy of all handles in this archetype
func (a *Archetype) Handles() []Handle {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]Handle, len(a.handles))
	copy(result, a.handles)
	return result
}

// Len returns the number of entities in this archetype
func (a *Archetype) Len() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.handles)
}

// ArchetypeGraph manages archetype transitions and lookups
// Inspired by Bevy's archetype graph for efficient component add/remove
type ArchetypeGraph struct {
	archetypes map[ComponentMask]*Archetype
	mu         sync.RWMutex
}

// NewArchetypeGraph creates a new archetype graph
func NewArchetypeGraph() *ArchetypeGraph {
	return &ArchetypeGraph{
		archetypes: make(map[ComponentMask]*Archetype),
	}
}

// GetOrCreate returns the archetype for the given mask, creating if needed
func (g *ArchetypeGraph) GetOrCreate(mask ComponentMask) *Archetype {
	g.mu.RLock()
	if arch, ok := g.archetypes[mask]; ok {
		g.mu.RUnlock()
		return arch
	}
	g.mu.RUnlock()

	g.mu.Lock()
	defer g.mu.Unlock()

	// Double-check
	if arch, ok := g.archetypes[mask]; ok {
		return arch
	}

	arch := NewArchetype(mask)
	g.archetypes[mask] = arch
	return arch
}

// Get returns the archetype for the given mask, or nil if not found
func (g *ArchetypeGraph) Get(mask ComponentMask) *Archetype {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.archetypes[mask]
}

// QueryArchetypes returns all archetypes that match the given component mask
// An archetype matches if it has ALL components in the query mask
func (g *ArchetypeGraph) QueryArchetypes(queryMask ComponentMask) []*Archetype {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Archetype, 0, len(g.archetypes))
	for mask, arch := range g.archetypes {
		if mask.HasAll(queryMask) {
			result = append(result, arch)
		}
	}
	return result
}

// All returns all archetypes
func (g *ArchetypeGraph) All() []*Archetype {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Archetype, 0, len(g.archetypes))
	for _, arch := range g.archetypes {
		result = append(result, arch)
	}
	return result
}
