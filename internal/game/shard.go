package game

import (
	"origin/internal/ecs"
	"origin/internal/ecs/systems"
	"origin/internal/persistence"
	"sync"
)

// Shard is part of layer world
type Shard struct {
	layer            int
	chunkManager     *ChunkManager                    // manage chunks (load, unload)
	players          map[ecs.Handle]bool              // connected players in this shard (by handle)
	world            *ecs.World                       // ECS world to operate with entities
	visibilitySystem *systems.VisibilitySystem        // manages what entities can see
	broadcastSystem  *systems.MovementBroadcastSystem // generates movement events
	networkFlush     *systems.NetworkFlushSystem      // sends packets to clients
	mu               sync.RWMutex
	// TODO chunks spatial hash grid
}

// NewShard creates a new shard for the given layer
func NewShard(layer int, db *persistence.Postgres) *Shard {
	world := ecs.NewWorld()

	// Create systems
	visSystem := systems.NewVisibilitySystem()
	broadcastSystem := systems.NewMovementBroadcastSystem(visSystem)
	networkFlush := systems.NewNetworkFlushSystem(visSystem, broadcastSystem)
	collisionSystem := systems.NewCollisionSystem()

	// Link collision system to broadcast system for dirty position tracking
	collisionSystem.SetBroadcastSystem(broadcastSystem)

	// Add systems in priority order
	world.AddSystem(systems.NewMovementSystem()) // Priority 100
	world.AddSystem(collisionSystem)             // Priority 200
	world.AddSystem(visSystem)                   // Priority 300
	world.AddSystem(broadcastSystem)             // Priority 400
	world.AddSystem(networkFlush)                // Priority 500

	return &Shard{
		layer:            layer,
		chunkManager:     NewChunkManager(db),
		players:          make(map[ecs.Handle]bool),
		world:            world,
		visibilitySystem: visSystem,
		broadcastSystem:  broadcastSystem,
		networkFlush:     networkFlush,
	}
}

// AddPlayer adds a player to the shard
func (s *Shard) AddPlayer(h ecs.Handle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.players[h] = true
}

// RemovePlayer removes a player from the shard
func (s *Shard) RemovePlayer(h ecs.Handle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.players, h)
}

// HasPlayer checks if a player is in the shard
func (s *Shard) HasPlayer(h ecs.Handle) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.players[h]
}

// PlayerCount returns the number of players in the shard
func (s *Shard) PlayerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.players)
}

// ECSWorld returns the ECS world for this shard
func (s *Shard) ECSWorld() *ecs.World {
	return s.world
}

// NetworkFlush returns the network flush system
func (s *Shard) NetworkFlush() *systems.NetworkFlushSystem {
	return s.networkFlush
}

// BroadcastSystem returns the movement broadcast system
func (s *Shard) BroadcastSystem() *systems.MovementBroadcastSystem {
	return s.broadcastSystem
}

// ChunkManager returns the chunk manager for this shard
func (s *Shard) ChunkManager() *ChunkManager {
	return s.chunkManager
}

// Layer returns the layer number of this shard
func (s *Shard) Layer() int {
	return s.layer
}

// Manage layers of current region world
type ShardManager struct {
	db     *persistence.Postgres
	shards map[int]*Shard // layer -> shard
	mu     sync.RWMutex
}

// NewShardManager creates a new shard manager
func NewShardManager(db *persistence.Postgres) *ShardManager {
	return &ShardManager{
		db:     db,
		shards: make(map[int]*Shard),
	}
}

// GetOrCreateShard returns the shard for the given layer, creating it if needed
func (sm *ShardManager) GetOrCreateShard(layer int) *Shard {
	sm.mu.RLock()
	shard, ok := sm.shards[layer]
	sm.mu.RUnlock()
	if ok {
		return shard
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	// Double-check after acquiring write lock
	if shard, ok = sm.shards[layer]; ok {
		return shard
	}
	shard = NewShard(layer, sm.db)
	sm.shards[layer] = shard
	return shard
}

// GetShard returns the shard for the given layer, or nil if not found
func (sm *ShardManager) GetShard(layer int) *Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.shards[layer]
}
