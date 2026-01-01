package ecs

import "sync"

// EntityID is a global unique identifier for an entity (for persistence/replication)
// Uses generational index pattern: lower 32 bits = index, upper 32 bits = generation
type EntityID uint64

// Handle is a compact runtime identifier (uint32) used internally by ECS
// Assigned from a pool and used by all systems and ComponentStorage
// Sparse arrays are bounded by max active entities, not by astronomical EntityID values
type Handle uint32

// InvalidHandle represents an invalid/unassigned handle
const InvalidHandle Handle = 0

// IsValid checks if the handle is valid (non-zero)
func (h Handle) IsValid() bool {
	return h != InvalidHandle
}

// HandleAllocator manages allocation and recycling of Handle values
// Handles are allocated from a pool to ensure compact indices for sparse arrays
type HandleAllocator struct {
	nextHandle Handle   // Next handle to allocate (if freeList is empty)
	freeList   []Handle // Recycled handles available for reuse
	maxHandles uint32   // Maximum number of handles (capacity limit)
	mu         sync.Mutex
}

// NewHandleAllocator creates a new handle allocator with the given capacity
func NewHandleAllocator(maxHandles uint32) *HandleAllocator {
	return &HandleAllocator{
		nextHandle: 1, // Start from 1, 0 is InvalidHandle
		freeList:   make([]Handle, 0, 256),
		maxHandles: maxHandles,
	}
}

// Alloc allocates a new handle from the pool
// Returns InvalidHandle if capacity is exceeded
func (a *HandleAllocator) Alloc() Handle {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Prefer recycled handles
	if len(a.freeList) > 0 {
		h := a.freeList[len(a.freeList)-1]
		a.freeList = a.freeList[:len(a.freeList)-1]
		return h
	}

	// Allocate new handle
	if uint32(a.nextHandle) > a.maxHandles {
		return InvalidHandle
	}
	h := a.nextHandle
	a.nextHandle++
	return h
}

// Free returns a handle to the pool for reuse
func (a *HandleAllocator) Free(h Handle) {
	if h == InvalidHandle {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.freeList = append(a.freeList, h)
}

// ActiveCount returns the number of currently allocated handles
func (a *HandleAllocator) ActiveCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return int(a.nextHandle-1) - len(a.freeList)
}
