package ecs

// ActiveLists contains precomputed entity lists for hot systems
// Built once per tick from active chunks, reused by all systems
// Buffers are reused between ticks to minimize allocations
type ActiveLists struct {
	// ActiveChunks is the set of chunk keys that are active this tick
	ActiveChunks map[uint64]struct{}

	// Dynamic entities: have Velocity, not Static (for movement/collision)
	Dynamic []Handle

	// Static entities: have Static + Collider (for collision spatial hash)
	Static []Handle

	// Vision entities: have Perception (observers for visibility system)
	Vision []Handle

	// Visible entities: have EntityMeta (targets for visibility system)
	Visible []Handle

	// All active entities (union of all above, deduplicated)
	All []Handle
}

// NewActiveLists creates a new ActiveLists with preallocated buffers
func NewActiveLists() *ActiveLists {
	return &ActiveLists{
		ActiveChunks: make(map[uint64]struct{}, 64),
		Dynamic:      make([]Handle, 0, 256),
		Static:       make([]Handle, 0, 256),
		Vision:       make([]Handle, 0, 64),
		Visible:      make([]Handle, 0, 256),
		All:          make([]Handle, 0, 512),
	}
}

// Clear resets all lists while preserving capacity
func (al *ActiveLists) Clear() {
	for k := range al.ActiveChunks {
		delete(al.ActiveChunks, k)
	}
	al.Dynamic = al.Dynamic[:0]
	al.Static = al.Static[:0]
	al.Vision = al.Vision[:0]
	al.Visible = al.Visible[:0]
	al.All = al.All[:0]
}

// AddActiveChunk marks a chunk as active
func (al *ActiveLists) AddActiveChunk(chunkKey uint64) {
	al.ActiveChunks[chunkKey] = struct{}{}
}

// IsChunkActive checks if a chunk is in the active set
func (al *ActiveLists) IsChunkActive(chunkKey uint64) bool {
	_, ok := al.ActiveChunks[chunkKey]
	return ok
}

// ActiveChunkCount returns the number of active chunks
func (al *ActiveLists) ActiveChunkCount() int {
	return len(al.ActiveChunks)
}

// ActiveListsKey is the resource key for ActiveLists in World
type ActiveListsKey struct{}

// GetActiveLists retrieves ActiveLists from world resources
func GetActiveLists(w *World) *ActiveLists {
	if res, ok := w.GetResource(ActiveListsKey{}); ok {
		return res.(*ActiveLists)
	}
	return nil
}

// SetActiveLists stores ActiveLists in world resources
func SetActiveLists(w *World, al *ActiveLists) {
	w.SetResource(ActiveListsKey{}, al)
}

// ChunkIndexKey is the resource key for ChunkIndex in World
type ChunkIndexKey struct{}

// GetChunkIndex retrieves ChunkIndex from world resources
func GetChunkIndex(w *World) *ChunkIndex {
	if res, ok := w.GetResource(ChunkIndexKey{}); ok {
		return res.(*ChunkIndex)
	}
	return nil
}

// SetChunkIndex stores ChunkIndex in world resources
func SetChunkIndex(w *World, ci *ChunkIndex) {
	w.SetResource(ChunkIndexKey{}, ci)
}
