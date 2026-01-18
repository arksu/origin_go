package game

import (
	"context"
	"fmt"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"origin/internal/utils"
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

	shards map[int]*Shard

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
		shards:          make(map[int]*Shard),
		workerPool:      NewWorkerPool(cfg.Game.WorkerPoolSize),
		eventBus:        eventbus.New(ebCfg),
	}

	for layer := 0; layer < cfg.Game.MaxLayers; layer++ {
		sm.shards[layer] = NewShard(layer, cfg, db, entityIDManager, objectFactory, sm.eventBus, logger.Named("shard"))
	}

	return sm
}

func (sm *ShardManager) GetShard(layer int) *Shard {
	return sm.shards[layer]
}

func (sm *ShardManager) GetShards() map[int]*Shard {
	return sm.shards
}

func (sm *ShardManager) Update(dt float64) {
	shards := make([]*Shard, 0, len(sm.shards))
	for _, s := range sm.shards {
		shards = append(shards, s)
	}

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

type ShardState int

const (
	ShardStateRunning ShardState = iota
	ShardStateStopping
)

type Shard struct {
	layer           int
	cfg             *config.Config
	db              *persistence.Postgres
	entityIDManager *EntityIDManager
	logger          *zap.Logger

	world        *ecs.World
	chunkManager *ChunkManager
	eventBus     *eventbus.EventBus

	state ShardState
	mu    sync.RWMutex
}

func NewShard(layer int, cfg *config.Config, db *persistence.Postgres, entityIDManager *EntityIDManager, objectFactory *ObjectFactory, eb *eventbus.EventBus, logger *zap.Logger) *Shard {
	s := &Shard{
		layer:           layer,
		cfg:             cfg,
		db:              db,
		entityIDManager: entityIDManager,
		logger:          logger,
		world:           ecs.NewWorldWithCapacity(uint32(cfg.Game.MaxEntities)),
		eventBus:        eb,
		state:           ShardStateRunning,
	}

	s.chunkManager = NewChunkManager(cfg, db, s.world, s, layer, cfg.Game.Region, objectFactory, eb, logger)

	chunkSize := utils.ChunkSize * utils.CoordPerTile
	worldMinX := float64(cfg.Game.WorldMinXChunks * chunkSize)
	worldMaxX := float64((cfg.Game.WorldMinXChunks + cfg.Game.WorldWidthChunks) * chunkSize)
	worldMinY := float64(cfg.Game.WorldMinYChunks * chunkSize)
	worldMaxY := float64((cfg.Game.WorldMinYChunks + cfg.Game.WorldHeightChunks) * chunkSize)

	s.world.AddSystem(systems.NewResetSystem(logger))
	s.world.AddSystem(systems.NewMovementSystem(s.world, s.chunkManager, logger))
	s.world.AddSystem(systems.NewCollisionSystem(s.world, s.chunkManager, logger, worldMinX, worldMaxX, worldMinY, worldMaxY, cfg.Game.WorldMarginTiles))
	s.world.AddSystem(systems.NewTransformUpdateSystem(s.world, s.chunkManager, s.eventBus, logger))
	s.world.AddSystem(systems.NewChunkSystem(s.chunkManager, logger))

	return s
}

func (s *Shard) Layer() int {
	return s.layer
}

func (s *Shard) World() *ecs.World {
	return s.world
}

func (s *Shard) ChunkManager() *ChunkManager {
	return s.chunkManager
}

func (s *Shard) Update(dt float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != ShardStateRunning {
		return
	}

	s.chunkManager.Update(dt)
	s.world.Update(dt)
}

func (s *Shard) Stop() {
	s.mu.Lock()
	s.state = ShardStateStopping
	s.mu.Unlock()

	s.chunkManager.Stop()
}

func (s *Shard) spawnEntityLocked(id types.EntityID, x int, y int) types.Handle {
	handle := s.world.Spawn(id)

	s.PublishEvent(
		eventbus.NewEntitySpawnEvent(id, "entity", x, y),
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

func (s *Shard) PrepareEntityAOI(ctx context.Context, entityID types.EntityID, centerWorldX, centerWorldY int) error {
	s.logger.Info("Preparing entity AOI",
		zap.Int64("entity_id", int64(entityID)),
		zap.Int("world_x", centerWorldX),
		zap.Int("world_y", centerWorldY),
		zap.Int("layer", s.layer),
	)

	centerChunk := types.WorldToChunkCoord(centerWorldX, centerWorldY, utils.ChunkSize, utils.CoordPerTile)
	radius := s.cfg.Game.PlayerActiveChunkRadius

	coords := make([]types.ChunkCoord, 0, (2*radius+1)*(2*radius+1))
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			chunkX := centerChunk.X + dx
			chunkY := centerChunk.Y + dy
			if chunkX < s.cfg.Game.WorldMinXChunks || chunkX >= s.cfg.Game.WorldMinXChunks+s.cfg.Game.WorldWidthChunks ||
				chunkY < s.cfg.Game.WorldMinYChunks || chunkY >= s.cfg.Game.WorldMinYChunks+s.cfg.Game.WorldHeightChunks {
				continue
			}
			coords = append(coords, types.ChunkCoord{X: chunkX, Y: chunkY})
		}
	}

	s.logger.Debug("Calculated chunk coordinates for AOI",
		zap.Int("center_chunk_x", centerChunk.X),
		zap.Int("center_chunk_y", centerChunk.Y),
		zap.Int("radius", radius),
		zap.Int("total_chunks", len(coords)),
	)

	for _, coord := range coords {
		if err := s.chunkManager.WaitPreloaded(ctx, coord); err != nil {
			s.logger.Error("Failed to preload chunk",
				zap.Int("chunk_x", coord.X),
				zap.Int("chunk_y", coord.Y),
				zap.Error(err),
			)
			return err
		}
	}
	// Verify all chunks are in correct state (preloaded or better, not unloaded)
	for _, coord := range coords {
		chunk := s.chunkManager.GetChunk(coord)
		if chunk == nil || chunk.GetState() == types.ChunkStateUnloaded {
			s.logger.Error("Chunk is not in expected preloaded state after WaitPreloaded",
				zap.Int("chunk_x", coord.X),
				zap.Int("chunk_y", coord.Y),
				zap.String("state", func() string {
					if chunk == nil {
						return "nil"
					}
					return chunk.GetState().String()
				}()),
			)
			return fmt.Errorf("chunk %v is not preloaded", coord)
		}
	}

	s.logger.Info("Successfully preloaded chunks for entity AOI",
		zap.Int64("entity_id", int64(entityID)),
		zap.Int("chunks_loaded", len(coords)),
	)
	s.mu.Lock()
	s.chunkManager.RegisterEntity(entityID, centerWorldX, centerWorldY)
	s.mu.Unlock()

	s.logger.Debug("Entity registered with chunk manager",
		zap.Int64("entity_id", int64(entityID)),
		zap.Int("world_x", centerWorldX),
		zap.Int("world_y", centerWorldY),
	)

	return nil
}

func (s *Shard) TrySpawnPlayer(worldX, worldY int, character repository.Character) (bool, types.Handle) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entityID := types.EntityID(character.ID)

	halfSize := utils.PlayerColliderSize / 2
	minX := worldX - halfSize
	minY := worldY - halfSize
	maxX := worldX + halfSize
	maxY := worldY + halfSize

	coordPerTile := utils.CoordPerTile
	minTileX := minX / coordPerTile
	minTileY := minY / coordPerTile
	maxTileX := (maxX - 1) / coordPerTile
	maxTileY := (maxY - 1) / coordPerTile

	chunks := s.chunkManager.GetEntityActiveChunks(entityID)
	if len(chunks) == 0 {
		return false, types.InvalidHandle
	}

	for tileY := minTileY; tileY <= maxTileY; tileY++ {
		for tileX := minTileX; tileX <= maxTileX; tileX++ {
			if !s.chunkManager.IsTilePassable(tileX, tileY) {
				return false, types.InvalidHandle
			}
		}
	}

	var collisionObjectsFromSpatial []types.Handle
	for _, chunk := range chunks {
		spatial := chunk.Spatial()
		spatial.QueryAABB(minX, minY, maxX, maxY, &collisionObjectsFromSpatial)
	}

	for _, h := range collisionObjectsFromSpatial {
		if !s.world.Alive(h) {
			continue
		}

		transform, hasTransform := ecs.GetComponent[components.Transform](s.world, h)
		if !hasTransform {
			continue
		}

		collider, hasCollider := ecs.GetComponent[components.Collider](s.world, h)
		if !hasCollider {
			continue
		}

		// Check if collision layers/masks overlap
		if utils.PlayerLayer&collider.Mask == 0 && collider.Layer&utils.PlayerMask == 0 {
			// No collision layer overlap, skip this object
			continue
		}

		objMinX := int(transform.X - collider.HalfWidth)
		objMinY := int(transform.Y - collider.HalfHeight)
		objMaxX := int(transform.X + collider.HalfWidth)
		objMaxY := int(transform.Y + collider.HalfHeight)

		if !(maxX <= objMinX || minX > objMaxX || maxY <= objMinY || minY > objMaxY) {
			return false, types.InvalidHandle
		}
	}

	handle := s.spawnEntityLocked(entityID, worldX, worldY)
	if chunk, ok := s.chunkManager.GetEntityChunk(entityID); ok {
		chunk.Spatial().AddDynamic(handle, worldX, worldY)
	}
	return true, handle
}

func (s *Shard) UnregisterEntityAOI(entityID types.EntityID) {
	s.chunkManager.UnregisterEntity(entityID)
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
