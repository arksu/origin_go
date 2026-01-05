package game

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/persistence"
)

type ShardManager struct {
	cfg             *config.Config
	db              *persistence.Postgres
	entityIDManager *EntityIDManager
	objectFactory   *ObjectFactory
	logger          *zap.Logger

	shards   map[int32]*Shard
	shardsMu sync.RWMutex

	workerPool *WorkerPool
	eventBus   *eventbus.EventBus
}

func NewShardManager(cfg *config.Config, db *persistence.Postgres, entityIDManager *EntityIDManager, objectFactory *ObjectFactory, logger *zap.Logger) *ShardManager {
	ebCfg := &eventbus.Config{
		MinWorkers: cfg.Game.WorkerPoolSize,
		MaxWorkers: cfg.Game.WorkerPoolSize * 4,
		Logger:     logger.Named("eventbus"),
		OnError: func(event eventbus.Event, handlerID string, err error) {
			logger.Error("event handler error",
				zap.String("topic", event.Topic()),
				zap.String("handler", handlerID),
				zap.Error(err),
			)
		},
	}

	sm := &ShardManager{
		cfg:             cfg,
		db:              db,
		entityIDManager: entityIDManager,
		objectFactory:   objectFactory,
		logger:          logger,
		shards:          make(map[int32]*Shard),
		workerPool:      NewWorkerPool(cfg.Game.WorkerPoolSize),
		eventBus:        eventbus.New(ebCfg),
	}

	var layer int32
	for layer = 0; layer < cfg.Game.MaxLayers; layer++ {
		sm.shards[layer] = NewShard(layer, cfg, db, entityIDManager, objectFactory, sm.eventBus, logger.Named("shard"))
	}

	return sm
}

func (sm *ShardManager) GetShard(layer int32) *Shard {
	sm.shardsMu.RLock()
	defer sm.shardsMu.RUnlock()
	return sm.shards[layer]
}

func (sm *ShardManager) Update(dt float32) {
	sm.shardsMu.RLock()
	shards := make([]*Shard, 0, len(sm.shards))
	for _, s := range sm.shards {
		shards = append(shards, s)
	}
	sm.shardsMu.RUnlock()

	var wg sync.WaitGroup
	for _, shard := range shards {
		wg.Add(1)
		s := shard
		sm.workerPool.Submit(func() {
			defer wg.Done()
			s.Update(dt)
		})
	}
	wg.Wait()
}

func (sm *ShardManager) Stop() {
	sm.shardsMu.Lock()
	defer sm.shardsMu.Unlock()

	for _, s := range sm.shards {
		s.Stop()
	}

	sm.workerPool.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*1e9)
	defer cancel()
	_ = sm.eventBus.Shutdown(ctx)
}

func (sm *ShardManager) EventBus() *eventbus.EventBus {
	return sm.eventBus
}

type Shard struct {
	layer           int32
	cfg             *config.Config
	db              *persistence.Postgres
	entityIDManager *EntityIDManager

	world        *ecs.World
	chunkManager *ChunkManager
	eventBus     *eventbus.EventBus

	mu sync.RWMutex
}

func NewShard(layer int32, cfg *config.Config, db *persistence.Postgres, entityIDManager *EntityIDManager, objectFactory *ObjectFactory, eb *eventbus.EventBus, logger *zap.Logger) *Shard {
	s := &Shard{
		layer:           layer,
		cfg:             cfg,
		db:              db,
		entityIDManager: entityIDManager,
		world:           ecs.NewWorldWithCapacity(uint32(cfg.Game.MaxEntities)),
		eventBus:        eb,
	}

	s.chunkManager = NewChunkManager(cfg, db, s.world, layer, cfg.Game.Region, objectFactory, eb, logger)

	return s
}

func (s *Shard) Layer() int32 {
	return s.layer
}

func (s *Shard) World() *ecs.World {
	return s.world
}

func (s *Shard) ChunkManager() *ChunkManager {
	return s.chunkManager
}

func (s *Shard) Update(dt float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.chunkManager.Update(dt)
	s.world.Update(dt)
}

func (s *Shard) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.chunkManager.Stop()
}

func (s *Shard) SpawnEntity() ecs.Handle {
	id := s.entityIDManager.GetFreeID()
	handle := s.world.Spawn(id)

	s.eventBus.PublishAsync(
		eventbus.NewEntitySpawnEvent(uint64(id), "entity", 0, 0, 0, s.layer),
		eventbus.PriorityMedium,
	)

	return handle
}

func (s *Shard) EventBus() *eventbus.EventBus {
	return s.eventBus
}

func (s *Shard) PublishEvent(event eventbus.Event, priority eventbus.Priority) {
	s.eventBus.PublishAsync(event, priority)
}

func (s *Shard) PublishEventSync(event eventbus.Event) error {
	return s.eventBus.PublishSync(event)
}

func (s *Shard) PrepareAOI(ctx context.Context, centerWorldX, centerWorldY int) ([]ChunkCoord, []*Chunk, error) {
	centerChunk := WorldToChunkCoord(centerWorldX, centerWorldY, s.cfg.Game.ChunkSize, s.cfg.Game.CoordPerTile)
	radius := s.cfg.Game.AOIRadius

	coords := make([]ChunkCoord, 0, (2*radius+1)*(2*radius+1))
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			coords = append(coords, ChunkCoord{X: centerChunk.X + dx, Y: centerChunk.Y + dy})
		}
	}

	for _, coord := range coords {
		if err := s.chunkManager.WaitPreloaded(ctx, coord); err != nil {
			return nil, nil, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	chunks := make([]*Chunk, 0, len(coords))
	for _, coord := range coords {
		if err := s.chunkManager.ActivateChunk(coord); err != nil {
			continue
		}
		if chunk := s.chunkManager.GetChunk(coord); chunk != nil {
			chunks = append(chunks, chunk)
		}
	}

	return coords, chunks, nil
}

func (s *Shard) TrySpawnPlayer(worldX, worldY int) (bool, uint64) {
	if !s.CanSpawnAt(worldX, worldY) {
		return false, 0
	}

	handle := s.SpawnEntity()
	return true, uint64(handle)
}

func (s *Shard) CanSpawnAt(worldX, worldY int) bool {
	// TODO check spawn logic
	return true
}

func (s *Shard) ReleaseAOI(coords []ChunkCoord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, coord := range coords {
		_ = s.chunkManager.DeactivateChunk(coord)
	}
}

type WorkerPool struct {
	tasks chan func()
	wg    sync.WaitGroup
}

func NewWorkerPool(size int) *WorkerPool {
	wp := &WorkerPool{
		tasks: make(chan func(), 256),
	}

	for i := 0; i < size; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}

	return wp
}

func (wp *WorkerPool) worker() {
	defer wp.wg.Done()
	for task := range wp.tasks {
		task()
	}
}

func (wp *WorkerPool) Submit(task func()) {
	wp.tasks <- task
}

func (wp *WorkerPool) Stop() {
	close(wp.tasks)
	wp.wg.Wait()
}
