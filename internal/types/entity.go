package types

// EntityID is a global unique identifier for an entity (for persistence/replication)
// Uses generational index pattern: lower 32 bits = index, upper 32 bits = generation
type EntityID uint64

// Handle is a compact runtime identifier (uint64) used internally by ECS
// Packs (index: uint32, generation: uint32) to prevent stale handle bugs
// Lower 32 bits = index, upper 32 bits = generation
// Assigned from a pool and used by all systems and ComponentStorage
type Handle uint64

// InvalidHandle represents an invalid/unassigned handle
const InvalidHandle Handle = 0

// MakeHandle packs index and generation into a Handle
func MakeHandle(index, generation uint32) Handle {
	return Handle(index) | (Handle(generation) << 32)
}

// Index extracts the index from a Handle
func (h Handle) Index() uint32 {
	return uint32(h & 0xFFFFFFFF)
}

// Generation extracts the generation from a Handle
func (h Handle) Generation() uint32 {
	return uint32(h >> 32)
}

// IsValid checks if the handle is valid (non-zero)
func (h Handle) IsValid() bool {
	return h != InvalidHandle
}
