package ecs

import "origin/internal/types"

// HandleAllocator manages allocation and recycling of Handle values
// Handles are allocated from a pool to ensure compact indices for sparse arrays
// Tracks generation per index to prevent stale handle bugs
// Uses generation array for O(1) validation without map overhead
// Single-threaded per shard - no locks needed
type HandleAllocator struct {
	nextIndex   uint32   // Next index to allocate (if freeList is empty)
	freeList    []uint32 // Recycled indices available for reuse
	generations []uint32 // Next generation to allocate per index (incremented on Alloc)
	maxHandles  uint32   // Maximum number of handles (capacity limit)
}

// NewHandleAllocator creates a new handle allocator with the given capacity
func NewHandleAllocator(maxHandles uint32) *HandleAllocator {
	return &HandleAllocator{
		nextIndex:   1, // Start from 1, 0 is InvalidHandle
		freeList:    make([]uint32, 0, 256),
		generations: make([]uint32, maxHandles+1),
		maxHandles:  maxHandles,
	}
}

// Alloc allocates a new handle from the pool
// Returns InvalidHandle if capacity is exceeded
// Single-threaded - no lock needed
func (a *HandleAllocator) Alloc() types.Handle {

	var index uint32

	// Prefer recycled indices
	if len(a.freeList) > 0 {
		index = a.freeList[len(a.freeList)-1]
		a.freeList = a.freeList[:len(a.freeList)-1]
	} else {
		// Allocate new index
		if a.nextIndex > a.maxHandles {
			return types.InvalidHandle
		}
		index = a.nextIndex
		a.nextIndex++
	}

	// Use current generation (Free increments it when handle is freed)
	gen := a.generations[index]

	return types.MakeHandle(index, gen)
}

// Free returns a handle to the pool for reuse
// Increments generation to invalidate the freed handle
// Single-threaded - no lock needed
func (a *HandleAllocator) Free(h types.Handle) {
	if h == types.InvalidHandle {
		return
	}
	index := h.Index()
	// Increment generation to invalidate this handle
	a.generations[index]++
	a.freeList = append(a.freeList, index)
}

// ActiveCount returns the number of currently allocated handles
// Single-threaded - no lock needed
func (a *HandleAllocator) ActiveCount() int {
	return int(a.nextIndex-1) - len(a.freeList)
}

// IsValid checks if a handle is valid (currently allocated)
// O(1) array lookup - no map overhead
// Single-threaded - no lock needed
func (a *HandleAllocator) IsValid(h types.Handle) bool {
	if h == types.InvalidHandle {
		return false
	}
	index := h.Index()
	if index >= a.nextIndex {
		return false
	}
	// Valid if handle's generation matches current generation
	// Free increments generation, so freed handles have old generation
	return h.Generation() == a.generations[index]
}
