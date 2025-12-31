package ecs

import "sync"

// ChunkIndex provides O(1) lookup of entities by chunk
// Uses swap-back removal for efficient deletions without holes
type ChunkIndex struct {
	// chunkEntities maps chunk key -> dense slice of handles
	chunkEntities map[uint64][]Handle

	// entityChunk maps handle -> chunk key (for O(1) lookup of entity's chunk)
	entityChunk map[Handle]uint64

	// entityIndex maps handle -> index in chunkEntities slice (for O(1) swap-back removal)
	entityIndex map[Handle]int

	mu sync.RWMutex
}

// NewChunkIndex creates a new chunk index
func NewChunkIndex() *ChunkIndex {
	return &ChunkIndex{
		chunkEntities: make(map[uint64][]Handle, 256),
		entityChunk:   make(map[Handle]uint64, 1024),
		entityIndex:   make(map[Handle]int, 1024),
	}
}

// Add adds an entity to a chunk
// O(1) amortized (append to slice)
func (ci *ChunkIndex) Add(h Handle, chunkKey uint64) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	// Remove from old chunk if exists
	if oldKey, exists := ci.entityChunk[h]; exists {
		if oldKey == chunkKey {
			return // Already in this chunk
		}
		ci.removeFromChunkLocked(h, oldKey)
	}

	// Add to new chunk
	entities := ci.chunkEntities[chunkKey]
	idx := len(entities)
	ci.chunkEntities[chunkKey] = append(entities, h)
	ci.entityChunk[h] = chunkKey
	ci.entityIndex[h] = idx
}

// Remove removes an entity from its chunk
// O(1) using swap-back removal
func (ci *ChunkIndex) Remove(h Handle) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	chunkKey, exists := ci.entityChunk[h]
	if !exists {
		return
	}
	ci.removeFromChunkLocked(h, chunkKey)
}

// removeFromChunkLocked removes entity from chunk (must hold lock)
func (ci *ChunkIndex) removeFromChunkLocked(h Handle, chunkKey uint64) {
	entities := ci.chunkEntities[chunkKey]
	idx, exists := ci.entityIndex[h]
	if !exists || idx >= len(entities) {
		return
	}

	// Swap-back removal
	lastIdx := len(entities) - 1
	if idx != lastIdx {
		lastHandle := entities[lastIdx]
		entities[idx] = lastHandle
		ci.entityIndex[lastHandle] = idx
	}

	// Shrink slice
	ci.chunkEntities[chunkKey] = entities[:lastIdx]

	// Clean up maps
	delete(ci.entityChunk, h)
	delete(ci.entityIndex, h)

	// Remove empty chunk entry
	if len(ci.chunkEntities[chunkKey]) == 0 {
		delete(ci.chunkEntities, chunkKey)
	}
}

// UpdateChunk moves an entity to a new chunk if needed
// Returns true if chunk changed
func (ci *ChunkIndex) UpdateChunk(h Handle, newChunkKey uint64) bool {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	oldKey, exists := ci.entityChunk[h]
	if exists && oldKey == newChunkKey {
		return false // No change
	}

	// Remove from old chunk
	if exists {
		ci.removeFromChunkLocked(h, oldKey)
	}

	// Add to new chunk
	entities := ci.chunkEntities[newChunkKey]
	idx := len(entities)
	ci.chunkEntities[newChunkKey] = append(entities, h)
	ci.entityChunk[h] = newChunkKey
	ci.entityIndex[h] = idx

	return true
}

// GetChunkKey returns the chunk key for an entity
func (ci *ChunkIndex) GetChunkKey(h Handle) (uint64, bool) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	key, exists := ci.entityChunk[h]
	return key, exists
}

// GetEntitiesInChunk returns all entities in a chunk
// Returns a copy to avoid race conditions
func (ci *ChunkIndex) GetEntitiesInChunk(chunkKey uint64) []Handle {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	entities := ci.chunkEntities[chunkKey]
	if len(entities) == 0 {
		return nil
	}

	result := make([]Handle, len(entities))
	copy(result, entities)
	return result
}

// GetEntitiesInChunks returns all entities in the given chunks
// Appends to the provided buffer to minimize allocations
func (ci *ChunkIndex) GetEntitiesInChunks(chunkKeys []uint64, buffer []Handle) []Handle {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	for _, key := range chunkKeys {
		buffer = append(buffer, ci.chunkEntities[key]...)
	}
	return buffer
}

// GetEntitiesInChunksSet returns all entities in the given chunks (set version)
// Appends to the provided buffer to minimize allocations
func (ci *ChunkIndex) GetEntitiesInChunksSet(chunkKeys map[uint64]struct{}, buffer []Handle) []Handle {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	for key := range chunkKeys {
		buffer = append(buffer, ci.chunkEntities[key]...)
	}
	return buffer
}

// ChunkCount returns the number of non-empty chunks
func (ci *ChunkIndex) ChunkCount() int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return len(ci.chunkEntities)
}

// EntityCount returns the total number of indexed entities
func (ci *ChunkIndex) EntityCount() int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return len(ci.entityChunk)
}

// GetAllChunkKeys returns all chunk keys that have entities
func (ci *ChunkIndex) GetAllChunkKeys() []uint64 {
	ci.mu.RLock()
	defer ci.mu.RUnlock()

	keys := make([]uint64, 0, len(ci.chunkEntities))
	for k := range ci.chunkEntities {
		keys = append(keys, k)
	}
	return keys
}
