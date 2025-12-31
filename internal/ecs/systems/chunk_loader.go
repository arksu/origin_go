package systems

import (
	"context"
	"log"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"time"
)

// ChunkLoaderConfig holds configuration for chunk loading
type ChunkLoaderConfig struct {
	// UnloadTimeout is how long a chunk stays loaded after becoming inactive
	UnloadTimeout time.Duration
}

// DefaultChunkLoaderConfig returns default configuration
func DefaultChunkLoaderConfig() ChunkLoaderConfig {
	return ChunkLoaderConfig{
		UnloadTimeout: 30 * time.Second,
	}
}

// ChunkLoaderSystem manages loading and unloading of chunk objects
// Runs at priority 15 (after ActiveChunksSystem, before ActiveListsBuilder)
type ChunkLoaderSystem struct {
	ecs.BaseSystem
	config ChunkLoaderConfig

	// loadedChunks tracks when each chunk was last active
	loadedChunks map[uint64]time.Time

	// pendingLoads are chunks that need to be loaded
	pendingLoads []chunkCoords

	// pendingUnloads are chunks that need to be unloaded
	pendingUnloads []chunkCoords

	// objectLoader is the loader interface (set via SetObjectLoader)
	objectLoader ObjectLoaderInterface

	// context for database operations
	ctx context.Context
}

// chunkCoords holds chunk coordinates for load/unload operations
type chunkCoords struct {
	Region int32
	ChunkX int32
	ChunkY int32
	Layer  int32
}

// ObjectLoaderInterface defines the interface for loading/unloading objects
type ObjectLoaderInterface interface {
	LoadChunkObjects(ctx context.Context, w *ecs.World, region, chunkX, chunkY, layer int32) error
	UnloadChunkObjects(w *ecs.World, region, chunkX, chunkY, layer int32)
	IsChunkLoaded(region, chunkX, chunkY, layer int32) bool
}

// NewChunkLoaderSystem creates a new chunk loader system
func NewChunkLoaderSystem(ctx context.Context, config ChunkLoaderConfig) *ChunkLoaderSystem {
	return &ChunkLoaderSystem{
		BaseSystem:     ecs.NewBaseSystem("ChunkLoaderSystem", 15),
		config:         config,
		loadedChunks:   make(map[uint64]time.Time, 256),
		pendingLoads:   make([]chunkCoords, 0, 64),
		pendingUnloads: make([]chunkCoords, 0, 64),
		ctx:            ctx,
	}
}

// SetObjectLoader sets the object loader implementation
func (s *ChunkLoaderSystem) SetObjectLoader(loader ObjectLoaderInterface) {
	s.objectLoader = loader
}

// Update processes chunk loading and unloading
func (s *ChunkLoaderSystem) Update(w *ecs.World, dt float64) {
	if s.objectLoader == nil {
		return
	}

	activeLists := ecs.GetActiveLists(w)
	if activeLists == nil {
		return
	}

	now := time.Now()

	// Reset pending lists
	s.pendingLoads = s.pendingLoads[:0]
	s.pendingUnloads = s.pendingUnloads[:0]

	// Check which active chunks need loading
	for chunkKey := range activeLists.ActiveChunks {
		s.loadedChunks[chunkKey] = now // Update last active time

		region, layer, chunkX, chunkY := components.UnpackChunkKey(chunkKey)
		if !s.objectLoader.IsChunkLoaded(region, chunkX, chunkY, layer) {
			s.pendingLoads = append(s.pendingLoads, chunkCoords{
				Region: region,
				ChunkX: chunkX,
				ChunkY: chunkY,
				Layer:  layer,
			})
		}
	}

	// Check which loaded chunks should be unloaded
	for chunkKey, lastActive := range s.loadedChunks {
		if _, isActive := activeLists.ActiveChunks[chunkKey]; isActive {
			continue // Still active
		}

		if now.Sub(lastActive) > s.config.UnloadTimeout {
			region, layer, chunkX, chunkY := components.UnpackChunkKey(chunkKey)
			s.pendingUnloads = append(s.pendingUnloads, chunkCoords{
				Region: region,
				ChunkX: chunkX,
				ChunkY: chunkY,
				Layer:  layer,
			})
		}
	}

	// Process loads
	for _, coords := range s.pendingLoads {
		if err := s.objectLoader.LoadChunkObjects(s.ctx, w, coords.Region, coords.ChunkX, coords.ChunkY, coords.Layer); err != nil {
			log.Printf("ChunkLoaderSystem: failed to load chunk (%d, %d, %d, %d): %v",
				coords.Region, coords.ChunkX, coords.ChunkY, coords.Layer, err)
		}
	}

	// Process unloads
	for _, coords := range s.pendingUnloads {
		s.objectLoader.UnloadChunkObjects(w, coords.Region, coords.ChunkX, coords.ChunkY, coords.Layer)
		chunkKey := components.ChunkKeyFromCoords(coords.Region, coords.Layer, coords.ChunkX, coords.ChunkY)
		delete(s.loadedChunks, chunkKey)
	}
}

// ForceUnloadAll unloads all loaded chunks
func (s *ChunkLoaderSystem) ForceUnloadAll(w *ecs.World) {
	if s.objectLoader == nil {
		return
	}

	for chunkKey := range s.loadedChunks {
		region, layer, chunkX, chunkY := components.UnpackChunkKey(chunkKey)
		s.objectLoader.UnloadChunkObjects(w, region, chunkX, chunkY, layer)
	}
	s.loadedChunks = make(map[uint64]time.Time, 256)
}

// LoadedChunkCount returns the number of currently loaded chunks
func (s *ChunkLoaderSystem) LoadedChunkCount() int {
	return len(s.loadedChunks)
}
