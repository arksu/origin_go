package game

import (
	"origin/internal/config"
	"origin/internal/persistence"
	"sync"
	"time"
)

// World represents the game world
type World struct {
	cfg          *config.Config
	idManager    *EntityIdManager
	shardManager *ShardManager
	db           *persistence.Postgres
	tickCount    uint64
	lastSaveTime time.Time
	running      bool
	stopCh       chan struct{}
	mu           sync.RWMutex
}

// NewWorld creates a new game world
func NewWorld(cfg *config.Config, db *persistence.Postgres) *World {
	idManager := NewEntityIdManager(cfg, db)

	return &World{
		cfg:          cfg,
		idManager:    idManager,
		shardManager: NewShardManager(db),
		db:           db,
		tickCount:    0,
		lastSaveTime: time.Now(),
		stopCh:       make(chan struct{}),
	}
}

// Start begins the game loop
func (w *World) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	go w.gameLoop()
}

// Stop stops the game loop
func (w *World) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	w.mu.Unlock()

	close(w.stopCh)

	// Persist entity ID state
	w.idManager.Stop()
}

// gameLoop runs the main game tick loop
func (w *World) gameLoop() {
	ticker := time.NewTicker(w.cfg.TickRate)
	defer ticker.Stop()

	lastTick := time.Now()

	for {
		select {
		case <-w.stopCh:
			return
		case now := <-ticker.C:
			dt := now.Sub(lastTick).Seconds()
			lastTick = now

			w.tick(dt)
		}
	}
}

// tick processes a single game tick
func (w *World) tick(dt float64) {
	w.mu.Lock()
	w.tickCount++
	w.mu.Unlock()

	// Update all ECS systems in all shards
	w.shardManager.mu.RLock()
	for _, shard := range w.shardManager.shards {
		shard.world.Update(dt)
	}
	w.shardManager.mu.RUnlock()
}

// DB returns the database connection
func (w *World) DB() *persistence.Postgres {
	return w.db
}

// ShardManager returns the shard manager
func (w *World) ShardManager() *ShardManager {
	return w.shardManager
}

// Config returns the world configuration
func (w *World) Config() *config.Config {
	return w.cfg
}
