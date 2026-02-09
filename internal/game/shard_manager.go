package game

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	"origin/internal/game/world"
	"origin/internal/persistence"
)

type ShardUpdateResult struct {
	TotalDuration  time.Duration
	ShardDurations []ShardDuration
}

type ShardDuration struct {
	Layer    int
	Duration time.Duration
}

type ShardManager struct {
	cfg             *config.Config
	db              *persistence.Postgres
	entityIDManager *EntityIDManager
	objectFactory   *world.ObjectFactory
	snapshotSender  *inventory.SnapshotSender
	logger          *zap.Logger

	shards map[int]*Shard

	workerPool *WorkerPool
	eventBus   *eventbus.EventBus

	openContainerService *OpenContainerService
}

func NewShardManager(cfg *config.Config, db *persistence.Postgres, entityIDManager *EntityIDManager, objectFactory *world.ObjectFactory, snapshotSender *inventory.SnapshotSender, logger *zap.Logger) *ShardManager {
	ebCfg := &eventbus.Config{
		MinWorkers: cfg.Game.EventBusMinWorkers,
		MaxWorkers: cfg.Game.EventBusMaxWorkers,
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
		snapshotSender:  snapshotSender,
		logger:          logger,
		shards:          make(map[int]*Shard),
		workerPool:      NewWorkerPool(cfg.Game.WorkerPoolSize),
		eventBus:        eventbus.New(ebCfg),
	}

	for layer := 0; layer < cfg.Game.MaxLayers; layer++ {
		sm.shards[layer] = NewShard(layer, cfg, db, entityIDManager, objectFactory, snapshotSender, sm.eventBus, logger.Named("shard"))
	}

	sm.openContainerService = NewOpenContainerService(sm)
	sm.openContainerService.Subscribe(sm.eventBus)

	return sm
}

func (sm *ShardManager) GetShard(layer int) *Shard {
	return sm.shards[layer]
}

func (sm *ShardManager) GetShards() map[int]*Shard {
	return sm.shards
}

func (sm *ShardManager) Update(ts ecs.TimeState) ShardUpdateResult {
	shards := make([]*Shard, 0, len(sm.shards))
	for _, s := range sm.shards {
		shards = append(shards, s)
	}

	var wg sync.WaitGroup
	schedStart := time.Now()

	shardDurations := make([]ShardDuration, len(shards))

	for i, shard := range shards {
		s := shard
		idx := i
		wg.Add(1)
		if !sm.workerPool.Submit(func() {
			defer wg.Done()
			execStart := time.Now()
			s.Update(ts)
			shardDurations[idx] = ShardDuration{
				Layer:    s.layer,
				Duration: time.Since(execStart),
			}
		}) {
			wg.Done()
		}
	}
	wg.Wait()

	return ShardUpdateResult{
		TotalDuration:  time.Since(schedStart),
		ShardDurations: shardDurations,
	}
}

func (sm *ShardManager) Stop() {
	sm.workerPool.Stop()

	for _, s := range sm.shards {
		s.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*1e9)
	defer cancel()
	_ = sm.eventBus.Shutdown(ctx)
}

func (sm *ShardManager) EventBus() *eventbus.EventBus {
	return sm.eventBus
}

type WorkerPool struct {
	tasks   chan func()
	wg      sync.WaitGroup
	mu      sync.RWMutex
	stopped bool
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

func (wp *WorkerPool) Submit(task func()) bool {
	wp.mu.RLock()
	if wp.stopped {
		wp.mu.RUnlock()
		return false
	}
	wp.tasks <- task
	wp.mu.RUnlock()
	return true
}

func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	wp.stopped = true
	wp.mu.Unlock()
	close(wp.tasks)
	wp.wg.Wait()
}
