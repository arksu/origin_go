package world

import (
	"context"
	"errors"

	_const "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs/components"
	"origin/internal/types"
	"sync"
	"sync/atomic"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"
)

// Ensure context is used (for eventbus handler signature)
var _ context.Context

type loadRequest struct {
	coord types.ChunkCoord
}

type saveRequest struct {
	coord   types.ChunkCoord
	chunk   *core.Chunk
	version uint64 // Версия чанка на момент eviction
}

type ChunkStats struct {
	ActiveCount    int64
	PreloadedCount int64
	InactiveCount  int64
	LoadRequests   int64
	SaveRequests   int64
	CacheHits      int64
	CacheMisses    int64
}

// EntityAOI represents Area of Interest for a single entity
type EntityAOI struct {
	EntityID            types.EntityID
	CenterChunk         types.ChunkCoord
	ActiveChunks        map[types.ChunkCoord]struct{}
	PreloadChunks       map[types.ChunkCoord]struct{}
	SendChunkLoadEvents bool
	StreamEpoch         uint32
}

func newEntityAOI(entityID types.EntityID, center types.ChunkCoord, sendChunkLoadEvents bool) *EntityAOI {
	return &EntityAOI{
		EntityID:            entityID,
		CenterChunk:         center,
		ActiveChunks:        make(map[types.ChunkCoord]struct{}),
		PreloadChunks:       make(map[types.ChunkCoord]struct{}),
		SendChunkLoadEvents: sendChunkLoadEvents,
	}
}

// ChunkInterest tracks which entities are interested in a chunk
type ChunkInterest struct {
	activeEntities  map[types.EntityID]struct{} // entities with this chunk in active zone
	preloadEntities map[types.EntityID]struct{} // entities with this chunk in preload zone (not active)
}

func newChunkInterest() *ChunkInterest {
	return &ChunkInterest{
		activeEntities:  make(map[types.EntityID]struct{}),
		preloadEntities: make(map[types.EntityID]struct{}),
	}
}

func (ci *ChunkInterest) isEmpty() bool {
	return len(ci.activeEntities) == 0 && len(ci.preloadEntities) == 0
}

func (ci *ChunkInterest) hasActive() bool {
	return len(ci.activeEntities) > 0
}

func (ci *ChunkInterest) hasPreload() bool {
	return len(ci.preloadEntities) > 0
}

var (
	ErrChunkNotLoaded = errors.New("chunk not loaded")
)

type ChunkManager struct {
	cfg              *config.Config
	db               *persistence.Postgres
	world            *ecs.World
	shard            interface{}
	layer            int
	region           int
	objectFactory    *ObjectFactory
	behaviorRegistry contracts.BehaviorRegistry
	logger           *zap.Logger

	chunks   map[types.ChunkCoord]*core.Chunk
	chunksMu sync.RWMutex

	lruCache *lru.LRU[types.ChunkCoord, *core.Chunk]

	loadQueue chan loadRequest
	saveQueue chan saveRequest
	stopCh    chan struct{}
	wg        sync.WaitGroup
	stopped   int32 // atomic flag to prevent saves after shutdown

	// Per-entity AOI tracking
	entityAOIs map[types.EntityID]*EntityAOI
	aoiMu      sync.RWMutex

	// Global chunk interest tracking
	chunkInterests map[types.ChunkCoord]*ChunkInterest
	interestMu     sync.RWMutex

	// Cached active chunks for fast access (hot path optimization)
	activeChunks   map[types.ChunkCoord]struct{}
	activeChunksMu sync.RWMutex

	loadFutures   map[types.ChunkCoord]*loadFuture
	loadFuturesMu sync.Mutex

	stats ChunkStats

	eventBus *eventbus.EventBus
}

type loadFuture struct {
	done    chan struct{}
	waiters int
}

func NewChunkManager(
	cfg *config.Config,
	db *persistence.Postgres,
	world *ecs.World,
	shard interface{},
	layer int,
	region int,
	objectFactory *ObjectFactory,
	behaviorRegistry contracts.BehaviorRegistry,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
) *ChunkManager {
	ttl := time.Duration(cfg.Game.ChunkLRUTTL) * time.Second

	cm := &ChunkManager{
		cfg:              cfg,
		db:               db,
		world:            world,
		shard:            shard,
		layer:            layer,
		region:           region,
		objectFactory:    objectFactory,
		behaviorRegistry: behaviorRegistry,
		logger:           logger.Named("chunk_manager"),
		chunks:           make(map[types.ChunkCoord]*core.Chunk),
		loadQueue:        make(chan loadRequest, 512),
		saveQueue:        make(chan saveRequest, 512),
		stopCh:           make(chan struct{}),
		entityAOIs:       make(map[types.EntityID]*EntityAOI),
		chunkInterests:   make(map[types.ChunkCoord]*ChunkInterest),
		activeChunks:     make(map[types.ChunkCoord]struct{}),
		loadFutures:      make(map[types.ChunkCoord]*loadFuture),
		eventBus:         eventBus,
	}

	cm.lruCache = lru.NewLRU(
		cfg.Game.ChunkLRUCapacity,
		cm.onEvict,
		ttl,
	)

	for i := 0; i < cfg.Game.LoadWorkers; i++ {
		cm.wg.Add(1)
		go cm.loadWorker()
	}

	for i := 0; i < cfg.Game.SaveWorkers; i++ {
		cm.wg.Add(1)
		go cm.saveWorker()
	}

	return cm
}

// RegisterEntity registers an entity for AOI tracking
func (cm *ChunkManager) RegisterEntity(entityID types.EntityID, worldX, worldY int, sendChunkLoadEvents bool) {
	center := types.WorldToChunkCoord(worldX, worldY, _const.ChunkSize, _const.CoordPerTile)

	cm.aoiMu.Lock()
	if _, exists := cm.entityAOIs[entityID]; exists {
		cm.aoiMu.Unlock()
		return
	}
	aoi := newEntityAOI(entityID, center, sendChunkLoadEvents)
	cm.entityAOIs[entityID] = aoi
	cm.aoiMu.Unlock()

	cm.updateEntityAOI(entityID, center)
}

// GetEntityEpoch returns the current stream epoch for an entity
func (cm *ChunkManager) GetEntityEpoch(entityID types.EntityID) uint32 {
	cm.aoiMu.RLock()
	defer cm.aoiMu.RUnlock()

	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		return 0
	}
	return aoi.StreamEpoch
}

// EnableChunkLoadEvents enables chunk load events for an entity
func (cm *ChunkManager) EnableChunkLoadEvents(entityID types.EntityID, epoch uint32) {
	cm.aoiMu.Lock()
	defer cm.aoiMu.Unlock()

	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		return
	}

	// Enable chunk events and trigger initial load events
	aoi.SendChunkLoadEvents = true
	aoi.StreamEpoch = epoch

	// Get current active chunks and send load events for them
	center := aoi.CenterChunk
	activeRadius := cm.cfg.Game.PlayerActiveChunkRadius

	// Send chunk load events for all currently active chunks
	for dy := -activeRadius; dy <= activeRadius; dy++ {
		for dx := -activeRadius; dx <= activeRadius; dx++ {
			coord := types.ChunkCoord{X: center.X + dx, Y: center.Y + dy}
			if cm.isWithinWorldBounds(coord) {
				if _, isActive := aoi.ActiveChunks[coord]; isActive {
					// Get chunk data to include tiles in the event
					chunk := cm.GetChunkFast(coord)
					var tiles []byte
					var version uint32
					if chunk != nil {
						tiles = append([]byte(nil), chunk.Tiles...) // Create copy of tiles
						version = chunk.Version
					}
					cm.eventBus.PublishAsync(ecs.NewChunkLoadEvent(entityID, coord.X, coord.Y, cm.layer, tiles, epoch, version), eventbus.PriorityMedium)
				}
			}
		}
	}
}

// UnregisterEntity removes an entity from AOI tracking
func (cm *ChunkManager) UnregisterEntity(entityID types.EntityID) {
	cm.UnregisterEntities([]types.EntityID{entityID})
}

// UnregisterEntities removes multiple entities from AOI tracking and
// recalculates chunk states once for the whole batch.
func (cm *ChunkManager) UnregisterEntities(entityIDs []types.EntityID) {
	if len(entityIDs) == 0 {
		return
	}

	aois := make([]*EntityAOI, 0, len(entityIDs))

	cm.aoiMu.Lock()
	for _, entityID := range entityIDs {
		aoi, exists := cm.entityAOIs[entityID]
		if !exists {
			continue
		}
		delete(cm.entityAOIs, entityID)
		aois = append(aois, aoi)
	}
	cm.aoiMu.Unlock()

	if len(aois) == 0 {
		return
	}

	cm.interestMu.Lock()
	for _, aoi := range aois {
		entityID := aoi.EntityID
		for coord := range aoi.ActiveChunks {
			if interest, exists := cm.chunkInterests[coord]; exists {
				delete(interest.activeEntities, entityID)
			}
		}
		for coord := range aoi.PreloadChunks {
			if interest, exists := cm.chunkInterests[coord]; exists {
				delete(interest.preloadEntities, entityID)
			}
		}
	}
	cm.interestMu.Unlock()

	cm.recalculateChunkStates()
}

// UpdateEntityPosition updates AOI for an entity when it moves to a new chunk
func (cm *ChunkManager) UpdateEntityPosition(entityID types.EntityID, newCenter types.ChunkCoord) {
	cm.aoiMu.Lock()
	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		cm.aoiMu.Unlock()
		return
	}
	oldCenter := aoi.CenterChunk
	cm.aoiMu.Unlock()

	if oldCenter == newCenter {
		return
	}

	cm.updateEntityAOI(entityID, newCenter)
}

// updateEntityAOI recalculates AOI zones for a single entity and updates global interests
func (cm *ChunkManager) updateEntityAOI(entityID types.EntityID, newCenter types.ChunkCoord) {
	activeRadius := cm.cfg.Game.PlayerActiveChunkRadius
	preloadRadius := cm.cfg.Game.PlayerPreloadChunkRadius

	newActive := make(map[types.ChunkCoord]struct{})
	newPreload := make(map[types.ChunkCoord]struct{})

	// Calculate new active zone (with bounds checking)
	for dy := -activeRadius; dy <= activeRadius; dy++ {
		for dx := -activeRadius; dx <= activeRadius; dx++ {
			coord := types.ChunkCoord{X: newCenter.X + dx, Y: newCenter.Y + dy}
			if cm.isWithinWorldBounds(coord) {
				newActive[coord] = struct{}{}
			}
		}
	}

	// Calculate new preload zone (excluding active, with bounds checking)
	for dy := -preloadRadius; dy <= preloadRadius; dy++ {
		for dx := -preloadRadius; dx <= preloadRadius; dx++ {
			coord := types.ChunkCoord{X: newCenter.X + dx, Y: newCenter.Y + dy}
			if cm.isWithinWorldBounds(coord) {
				if _, isActive := newActive[coord]; !isActive {
					newPreload[coord] = struct{}{}
				}
			}
		}
	}

	cm.aoiMu.Lock()
	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		cm.aoiMu.Unlock()
		return
	}
	oldActive := aoi.ActiveChunks
	oldPreload := aoi.PreloadChunks
	aoi.CenterChunk = newCenter
	aoi.ActiveChunks = newActive
	aoi.PreloadChunks = newPreload
	sendChunkLoadEvents := aoi.SendChunkLoadEvents // Copy flag while holding lock
	cm.aoiMu.Unlock()

	// Calculate delta for network events
	// toDeactivate = oldActive - newActive (chunks that were active but no longer are)
	// toActivate = newActive - oldActive (chunks that are now active but weren't before)
	toDeactivate := make([]types.ChunkCoord, 0)
	toActivate := make([]types.ChunkCoord, 0)

	for coord := range oldActive {
		if _, stillActive := newActive[coord]; !stillActive {
			toDeactivate = append(toDeactivate, coord)
		}
	}

	for coord := range newActive {
		if _, wasActive := oldActive[coord]; !wasActive {
			toActivate = append(toActivate, coord)
		}
	}

	// Update global chunk interests
	cm.interestMu.Lock()

	// Remove old active interests
	for coord := range oldActive {
		if interest, exists := cm.chunkInterests[coord]; exists {
			delete(interest.activeEntities, entityID)
		}
	}

	// Remove old preload interests
	for coord := range oldPreload {
		if interest, exists := cm.chunkInterests[coord]; exists {
			delete(interest.preloadEntities, entityID)
		}
	}

	// Add new active interests
	for coord := range newActive {
		interest, exists := cm.chunkInterests[coord]
		if !exists {
			interest = newChunkInterest()
			cm.chunkInterests[coord] = interest
		}
		interest.activeEntities[entityID] = struct{}{}
	}

	// Add new preload interests
	for coord := range newPreload {
		interest, exists := cm.chunkInterests[coord]
		if !exists {
			interest = newChunkInterest()
			cm.chunkInterests[coord] = interest
		}
		interest.preloadEntities[entityID] = struct{}{}
	}

	cm.interestMu.Unlock()

	cm.recalculateChunkStates()

	// Get client's stream epoch for event validation
	epoch := cm.GetEntityEpoch(entityID)

	// Publish chunk events for network layer only if enabled
	// Send deactivate first to free client memory, then activate
	if sendChunkLoadEvents {
		for _, coord := range toDeactivate {
			cm.eventBus.PublishAsync(ecs.NewChunkUnloadEvent(entityID, coord.X, coord.Y, cm.layer, epoch), eventbus.PriorityMedium)
		}

		for _, coord := range toActivate {
			// Get chunk data to include tiles in the event
			chunk := cm.GetChunkFast(coord)
			var tiles []byte
			var version uint32
			if chunk != nil {
				tiles = append([]byte(nil), chunk.Tiles...) // Create copy of tiles
				version = chunk.Version
			}
			cm.eventBus.PublishAsync(ecs.NewChunkLoadEvent(entityID, coord.X, coord.Y, cm.layer, tiles, epoch, version), eventbus.PriorityMedium)
		}
	}
}

// removeEntityInterests removes all interests for an entity
func (cm *ChunkManager) removeEntityInterests(entityID types.EntityID, aoi *EntityAOI) {
	cm.interestMu.Lock()
	defer cm.interestMu.Unlock()

	for coord := range aoi.ActiveChunks {
		if interest, exists := cm.chunkInterests[coord]; exists {
			delete(interest.activeEntities, entityID)
		}
	}

	for coord := range aoi.PreloadChunks {
		if interest, exists := cm.chunkInterests[coord]; exists {
			delete(interest.preloadEntities, entityID)
		}
	}
}

// chunkInterestSnapshot holds a thread-safe snapshot of interest counts
type chunkInterestSnapshot struct {
	activeCount  int
	preloadCount int
}

// recalculateChunkStates updates chunk states based on global interests
func (cm *ChunkManager) recalculateChunkStates() {
	cm.interestMu.RLock()
	interestsSnapshot := make(map[types.ChunkCoord]chunkInterestSnapshot, len(cm.chunkInterests))
	for coord, interest := range cm.chunkInterests {
		interestsSnapshot[coord] = chunkInterestSnapshot{
			activeCount:  len(interest.activeEntities),
			preloadCount: len(interest.preloadEntities),
		}
	}
	cm.interestMu.RUnlock()

	// Track changes to active chunks
	activatedChunks := make([]types.ChunkCoord, 0)
	deactivatedChunks := make([]types.ChunkCoord, 0)

	for coord, snapshot := range interestsSnapshot {
		chunk := cm.GetChunkFast(coord)

		if snapshot.activeCount > 0 {
			// Should be Active
			if chunk == nil {
				_ = cm.requestLoad(coord)
			} else {
				state := chunk.GetState()
				switch state {
				case types.ChunkStatePreloaded, types.ChunkStateInactive:
					if err := cm.activateChunkInternal(coord, chunk); err != nil {
						cm.logger.Warn("failed to activate chunk (activateChunkInternal)",
							zap.Int("chunk_x", coord.X),
							zap.Int("chunk_y", coord.Y),
							zap.Error(err),
						)
					} else {
						activatedChunks = append(activatedChunks, coord)
					}
				case types.ChunkStateActive:
					// Already active, ensure it's in cache
					activatedChunks = append(activatedChunks, coord)
				}
			}
		} else if snapshot.preloadCount > 0 {
			// Should be Preloaded
			if chunk == nil {
				_ = cm.requestLoad(coord)
			} else {
				state := chunk.GetState()
				switch state {
				case types.ChunkStateActive:
					if err := cm.deactivateChunkInternal(chunk); err != nil {
						cm.logger.Warn("failed to deactivate chunk (deactivateChunkInternal)",
							zap.Int("chunk_x", coord.X),
							zap.Int("chunk_y", coord.Y),
							zap.Error(err),
						)
					} else {
						deactivatedChunks = append(deactivatedChunks, coord)
					}
				case types.ChunkStateInactive:
					chunk.SetState(types.ChunkStatePreloaded)
					cm.lruCache.Remove(coord)
				}
			}
		} else {
			// No interest -> Inactive
			if chunk != nil {
				state := chunk.GetState()
				switch state {
				case types.ChunkStateActive:
					if err := cm.deactivateChunkInternal(chunk); err == nil {
						chunk.SetState(types.ChunkStateInactive)
						cm.lruCache.Add(coord, chunk)
						deactivatedChunks = append(deactivatedChunks, coord)
					}
				case types.ChunkStatePreloaded:
					chunk.SetState(types.ChunkStateInactive)
					cm.lruCache.Add(coord, chunk)
				}
			}

			// Clean up empty interest
			cm.interestMu.Lock()
			if interest, exists := cm.chunkInterests[coord]; exists && interest.isEmpty() {
				delete(cm.chunkInterests, coord)
			}
			cm.interestMu.Unlock()
		}
	}

	// Update cached active chunks
	cm.activeChunksMu.Lock()
	for _, coord := range activatedChunks {
		cm.activeChunks[coord] = struct{}{}
	}
	for _, coord := range deactivatedChunks {
		delete(cm.activeChunks, coord)
	}
	cm.activeChunksMu.Unlock()
}

func (cm *ChunkManager) getChunkUnsafe(coord types.ChunkCoord) *core.Chunk {
	cm.chunksMu.RLock()
	chunk := cm.chunks[coord]
	cm.chunksMu.RUnlock()
	return chunk
}

func (cm *ChunkManager) loadWorker() {
	defer cm.wg.Done()

	for {
		select {
		case <-cm.stopCh:
			return
		case req := <-cm.loadQueue:
			cm.loadChunkFromDB(req.coord)
		}
	}
}

func (cm *ChunkManager) saveWorker() {
	defer cm.wg.Done()

	for {
		select {
		case <-cm.stopCh:
			return
		case req := <-cm.saveQueue:
			cm.safeSaveAndRemove(req.coord, req.chunk)
		}
	}
}

func (cm *ChunkManager) loadChunkFromDB(coord types.ChunkCoord) {
	cm.chunksMu.Lock()
	chunk, exists := cm.chunks[coord]
	if !exists {
		chunk = core.NewChunk(coord, cm.region, cm.layer, _const.ChunkSize)
		cm.chunks[coord] = chunk
	}
	cm.chunksMu.Unlock()

	state := chunk.GetState()

	if state != types.ChunkStateUnloaded {
		cm.completeFuture(coord)
		return
	}

	if err := chunk.LoadFromDB(cm.db, cm.region, cm.layer, cm.logger); err != nil {
		cm.logger.Error("failed to load chunk from database",
			zap.Int("chunk_x", coord.X),
			zap.Int("chunk_y", coord.Y),
			zap.Error(err),
		)
		chunk.SetState(types.ChunkStateUnloaded)
		cm.completeFuture(coord)
		return
	}

	cm.interestMu.RLock()
	interest, hasInterest := cm.chunkInterests[coord]
	hasAnyInterest := hasInterest && !interest.isEmpty()
	cm.interestMu.RUnlock()

	// Reconcile loaded chunk state with the latest interests.
	// Do not activate here: load workers run off the shard tick thread and must not mutate ECS world state.
	// If a load request outlives AOI changes, move stale chunk to Inactive so it can be evicted.
	if !hasAnyInterest {
		chunk.SetState(types.ChunkStateInactive)
		cm.lruCache.Add(coord, chunk)
	}

	cm.completeFuture(coord)
}

func (cm *ChunkManager) onEvict(coord types.ChunkCoord, chunk *core.Chunk) {
	if chunk == nil {
		return
	}

	// Don't save if chunk manager is stopped
	if atomic.LoadInt32(&cm.stopped) != 0 {
		return
	}

	// Check if any entity is still interested in this chunk
	cm.interestMu.RLock()
	interest, hasInterest := cm.chunkInterests[coord]
	isInterested := hasInterest && !interest.isEmpty()
	cm.interestMu.RUnlock()

	if isInterested {
		return
	}

	atomic.AddInt64(&cm.stats.SaveRequests, 1)

	select {
	case cm.saveQueue <- saveRequest{coord: coord, chunk: chunk}:
	default:
		cm.logger.Warn("save queue full, saving synchronously",
			zap.Int("chunk_x", coord.X),
			zap.Int("chunk_y", coord.Y),
		)
		// Синхронное сохранение с проверкой актуальности
		cm.safeSaveAndRemove(coord, chunk)
	}
}

func (cm *ChunkManager) safeSaveAndRemove(coord types.ChunkCoord, chunk *core.Chunk) {
	// Повторная проверка interest
	cm.interestMu.RLock()
	interest, hasInterest := cm.chunkInterests[coord]
	isInterested := hasInterest && !interest.isEmpty()
	cm.interestMu.RUnlock()

	if isInterested {
		cm.logger.Debug("chunk became interested during save, keeping in memory",
			zap.Int("chunk_x", coord.X),
			zap.Int("chunk_y", coord.Y),
		)
		return
	}

	if chunk.State != types.ChunkStateInactive {
		return
	}

	// Save to DB only if chunk has dirty state
	if chunk.IsDirty(cm.world) {
		chunk.SaveToDB(cm.db, cm.world, cm.objectFactory, cm.logger)
	}

	// Удаляем из памяти с финальными проверками
	cm.chunksMu.Lock()
	currentChunk := cm.chunks[coord]

	if currentChunk == chunk {

		cm.interestMu.RLock()
		interest, hasInterest := cm.chunkInterests[coord]
		stillNotInterested := !hasInterest || interest.isEmpty()
		cm.interestMu.RUnlock()

		if stillNotInterested && chunk.State == types.ChunkStateInactive {
			delete(cm.chunks, coord)
		}
	}
	cm.chunksMu.Unlock()
}

// isWithinWorldBounds checks if a chunk coordinate is within world boundaries
func (cm *ChunkManager) isWithinWorldBounds(coord types.ChunkCoord) bool {
	minX := cm.cfg.Game.WorldMinXChunks
	minY := cm.cfg.Game.WorldMinYChunks
	maxX := minX + cm.cfg.Game.WorldWidthChunks
	maxY := minY + cm.cfg.Game.WorldHeightChunks

	return coord.X >= minX && coord.X < maxX && coord.Y >= minY && coord.Y < maxY
}

func (cm *ChunkManager) GetChunk(coord types.ChunkCoord) *core.Chunk {
	if !cm.isWithinWorldBounds(coord) {
		return nil
	}

	cm.chunksMu.RLock()
	chunk := cm.chunks[coord]
	cm.chunksMu.RUnlock()

	if chunk != nil {
		atomic.AddInt64(&cm.stats.CacheHits, 1)
	} else {
		atomic.AddInt64(&cm.stats.CacheMisses, 1)
	}

	return chunk
}

// GetChunkFast returns chunk pointer without cache hit/miss stats updates.
// Intended for hot paths where atomics would dominate profile noise.
func (cm *ChunkManager) GetChunkFast(coord types.ChunkCoord) *core.Chunk {
	if !cm.isWithinWorldBounds(coord) {
		return nil
	}

	cm.chunksMu.RLock()
	chunk := cm.chunks[coord]
	cm.chunksMu.RUnlock()
	return chunk
}

func (cm *ChunkManager) requestLoad(coord types.ChunkCoord) bool {
	atomic.AddInt64(&cm.stats.LoadRequests, 1)

	select {
	case cm.loadQueue <- loadRequest{coord: coord}:
		return true
	default:
		cm.logger.Warn("load queue full, dropping load request",
			zap.Int("chunk_x", coord.X),
			zap.Int("chunk_y", coord.Y),
		)
		return false
	}
}

func (cm *ChunkManager) getOrCreateFuture(coord types.ChunkCoord) chan struct{} {
	cm.loadFuturesMu.Lock()
	defer cm.loadFuturesMu.Unlock()

	if fut, exists := cm.loadFutures[coord]; exists {
		fut.waiters++
		return fut.done
	}

	fut := &loadFuture{done: make(chan struct{}), waiters: 1}
	cm.loadFutures[coord] = fut
	return fut.done
}

func (cm *ChunkManager) completeFuture(coord types.ChunkCoord) {
	cm.loadFuturesMu.Lock()
	defer cm.loadFuturesMu.Unlock()

	if fut, exists := cm.loadFutures[coord]; exists {
		close(fut.done)
		delete(cm.loadFutures, coord)
	}
}

func (cm *ChunkManager) cleanupFuture(coord types.ChunkCoord) {
	cm.loadFuturesMu.Lock()
	fut, exists := cm.loadFutures[coord]
	if !exists {
		cm.loadFuturesMu.Unlock()
		return
	}

	fut.waiters--
	noWaiters := fut.waiters <= 0
	cm.loadFuturesMu.Unlock()

	if !noWaiters {
		return
	}

	// Avoid lock inversion between chunksMu and loadFuturesMu:
	// check chunk state outside the futures lock, then confirm-and-delete.
	chunk := cm.GetChunkFast(coord)
	if chunk != nil && chunk.GetState() != types.ChunkStateUnloaded {
		return
	}

	cm.loadFuturesMu.Lock()
	if fut2, exists2 := cm.loadFutures[coord]; exists2 && fut2.waiters <= 0 {
		delete(cm.loadFutures, coord)
	}
	cm.loadFuturesMu.Unlock()
}

func (cm *ChunkManager) WaitPreloaded(ctx context.Context, coord types.ChunkCoord) error {
	chunk := cm.GetChunkFast(coord)
	if chunk != nil {
		state := chunk.GetState()
		if state == types.ChunkStatePreloaded || state == types.ChunkStateActive {
			return nil
		}
	}

	done := cm.getOrCreateFuture(coord)
	_ = cm.requestLoad(coord)

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		cm.cleanupFuture(coord)
		return ctx.Err()
	case <-cm.stopCh:
		cm.cleanupFuture(coord)
		return ErrChunkNotLoaded
	}
}

// activateChunkInternal activates a chunk by building entities from raw objects
func (cm *ChunkManager) activateChunkInternal(coord types.ChunkCoord, chunk *core.Chunk) error {
	state := chunk.GetState()
	if state == types.ChunkStateActive {
		return nil
	}
	if state == types.ChunkStateUnloaded || state == types.ChunkStateLoading {
		return ErrChunkNotLoaded
	}

	rawObjects := chunk.GetRawObjects()
	rawInventoriesByOwner := chunk.GetRawInventoriesByOwner()
	spatial := chunk.Spatial()
	behaviorHandles := make([]types.Handle, 0, len(rawObjects))
	preRecomputeDirty := make(map[types.Handle]bool, len(rawObjects))

	for _, raw := range rawObjects {
		h, err := cm.objectFactory.Build(cm.world, raw, rawInventoriesByOwner[types.EntityID(raw.ID)])
		if err != nil {
			cm.logger.Error("failed to build object",
				zap.Int64("object_id", raw.ID),
				zap.Int("type_id", raw.TypeID),
				zap.Error(err),
			)
			continue
		}

		ecs.AddComponent(cm.world, h, components.ChunkRef{
			CurrentChunkX: coord.X,
			CurrentChunkY: coord.Y,
		})

		restoredState, stateErr := cm.objectFactory.DeserializeObjectState(raw)
		if stateErr != nil {
			cm.logger.Warn("failed to deserialize object state, using nil",
				zap.Int64("object_id", raw.ID),
				zap.Error(stateErr),
			)
		}

		ecs.AddComponent(cm.world, h, components.ObjectInternalState{
			State:   restoredState,
			IsDirty: false,
		})

		isStatic := cm.objectFactory.IsStatic(raw)
		if info, hasInfo := ecs.GetComponent[components.EntityInfo](cm.world, h); hasInfo && len(info.Behaviors) > 0 {
			if cm.behaviorRegistry != nil {
				if initErr := cm.behaviorRegistry.InitObjectBehaviors(&contracts.BehaviorObjectInitContext{
					World:      cm.world,
					Handle:     h,
					EntityID:   types.EntityID(raw.ID),
					EntityType: info.TypeID,
					Reason:     contracts.ObjectBehaviorInitReasonRestore,
				}, info.Behaviors); initErr != nil {
					cm.logger.Warn("failed to init restored object behaviors",
						zap.Int64("object_id", raw.ID),
						zap.Error(initErr),
					)
				}
			}
			if state, hasState := ecs.GetComponent[components.ObjectInternalState](cm.world, h); hasState {
				preRecomputeDirty[h] = state.IsDirty
			}
			behaviorHandles = append(behaviorHandles, h)
		}

		if isStatic {
			spatial.AddStatic(h, raw.X, raw.Y)
		} else {
			spatial.AddDynamic(h, raw.X, raw.Y)
		}
	}

	// Force initial behavior computation on activation (no lazy defer).
	ecssystems.RecomputeObjectBehaviorsNow(cm.world, cm.eventBus, cm.logger, cm.behaviorRegistry, behaviorHandles)
	// Recompute updates runtime flags/appearance and must not implicitly make restored objects persistent-dirty.
	for _, h := range behaviorHandles {
		dirtyBeforeRecompute, tracked := preRecomputeDirty[h]
		if !tracked {
			continue
		}
		ecs.WithComponent(cm.world, h, func(state *components.ObjectInternalState) {
			state.IsDirty = dirtyBeforeRecompute
		})
	}

	chunk.ClearRawObjects()
	chunk.ClearRawInventoriesByOwner()
	chunk.SetState(types.ChunkStateActive)
	cm.lruCache.Remove(coord)

	cm.activeChunksMu.Lock()
	cm.activeChunks[coord] = struct{}{}
	cm.activeChunksMu.Unlock()

	return nil
}

// deactivateChunkInternal deactivates a chunk by serializing entities to raw objects
func (cm *ChunkManager) deactivateChunkInternal(chunk *core.Chunk) error {
	if chunk.GetState() != types.ChunkStateActive {
		return nil
	}

	// Preserve dirty state from active runtime objects/tiles before despawn.
	wasDirty := chunk.IsDirty(cm.world)

	handles := chunk.GetHandles()
	rawObjects := make([]*repository.Object, 0, len(handles))
	rawInventoriesByOwner := make(map[types.EntityID][]repository.Inventory, len(handles))
	rawDirtyObjectIDs := make(map[types.EntityID]struct{}, len(handles))
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](cm.world)

	for _, h := range handles {
		if !cm.world.Alive(h) {
			continue
		}

		entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](cm.world, h)
		internalState, hasInternalState := ecs.GetComponent[components.ObjectInternalState](cm.world, h)
		objectDirty := hasInternalState && internalState.IsDirty
		hasPersistentInventories := false
		if hasEntityInfo {
			hasPersistentInventories = cm.objectFactory.HasPersistentInventories(entityInfo.TypeID, entityInfo.Behaviors)
		}

		obj, err := cm.objectFactory.Serialize(cm.world, h)
		if err != nil {
			cm.logger.Error("failed to serialize object for deactivation",
				zap.Error(err),
			)
			continue
		}
		if obj != nil {
			rawObjects = append(rawObjects, obj)
			if objectDirty {
				rawDirtyObjectIDs[types.EntityID(obj.ID)] = struct{}{}
			}
			if hasPersistentInventories {
				objectInventories, invErr := cm.objectFactory.SerializeObjectInventories(cm.world, h)
				if invErr != nil {
					cm.logger.Error("failed to serialize object inventories for deactivation",
						zap.Int64("object_id", obj.ID),
						zap.Error(invErr),
					)
				} else if len(objectInventories) > 0 {
					rawInventoriesByOwner[types.EntityID(obj.ID)] = append(rawInventoriesByOwner[types.EntityID(obj.ID)], objectInventories...)
				}
			}
		}

		// Despawn all inventory container entities owned by this object.
		// Container entities are spawned without external IDs and are not tracked by chunk handles.
		if hasPersistentInventories {
			extID, hasExtID := ecs.GetComponent[ecs.ExternalID](cm.world, h)
			if !hasExtID {
				cm.world.Despawn(h)
				continue
			}
			containerHandles := refIndex.RemoveAllByOwner(extID.ID)
			for _, containerHandle := range containerHandles {
				// Depth=1 nested containers are keyed by item_id; remove them before root despawn
				// to avoid orphaned runtime entities across chunk lifecycle.
				if container, ok := ecs.GetComponent[components.InventoryContainer](cm.world, containerHandle); ok {
					for _, item := range container.Items {
						if nestedHandle, found := refIndex.Lookup(_const.InventoryGrid, item.ItemID, 0); found {
							refIndex.Remove(_const.InventoryGrid, item.ItemID, 0)
							if cm.world.Alive(nestedHandle) {
								cm.world.Despawn(nestedHandle)
							}
						}
					}
				}
				if cm.world.Alive(containerHandle) {
					cm.world.Despawn(containerHandle)
				}
			}
		}
		cm.world.Despawn(h)
	}

	chunk.SetRawObjects(rawObjects)
	chunk.SetRawInventoriesByOwner(rawInventoriesByOwner)
	chunk.SetRawDirtyObjectIDs(rawDirtyObjectIDs)
	if wasDirty {
		chunk.MarkRawDataDirty()
	} else {
		chunk.ClearRawDataDirty()
		chunk.ClearRawDirtyObjectIDs()
	}
	chunk.ClearHandles()
	chunk.SetState(types.ChunkStatePreloaded)

	return nil
}

func (cm *ChunkManager) PreloadChunksAround(center types.ChunkCoord) {
	radius := cm.cfg.Game.PlayerPreloadChunkRadius

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			coord := types.ChunkCoord{X: center.X + dx, Y: center.Y + dy}

			// Skip chunks outside world bounds
			if !cm.isWithinWorldBounds(coord) {
				continue
			}

			chunk := cm.GetChunkFast(coord)
			if chunk == nil || chunk.GetState() == types.ChunkStateUnloaded {
				_ = cm.requestLoad(coord)
			}
		}
	}
}

func (cm *ChunkManager) ActiveChunks() []*core.Chunk {
	// Fast path: read from cached active chunks
	cm.activeChunksMu.RLock()
	coords := make([]types.ChunkCoord, 0, len(cm.activeChunks))
	for coord := range cm.activeChunks {
		coords = append(coords, coord)
	}
	cm.activeChunksMu.RUnlock()

	chunks := make([]*core.Chunk, 0, len(coords))
	for _, coord := range coords {
		if chunk := cm.GetChunkFast(coord); chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func (cm *ChunkManager) ActiveChunkCoords() []types.ChunkCoord {
	// Fast path: read from cached active chunks
	cm.activeChunksMu.RLock()
	defer cm.activeChunksMu.RUnlock()

	coords := make([]types.ChunkCoord, 0, len(cm.activeChunks))
	for coord := range cm.activeChunks {
		coords = append(coords, coord)
	}
	return coords
}

func (cm *ChunkManager) GetEntityActiveChunks(entityID types.EntityID) []*core.Chunk {
	cm.aoiMu.RLock()
	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		cm.aoiMu.RUnlock()
		return nil
	}
	activeCoords := make([]types.ChunkCoord, 0, len(aoi.ActiveChunks))
	for coord := range aoi.ActiveChunks {
		activeCoords = append(activeCoords, coord)
	}
	cm.aoiMu.RUnlock()

	chunks := make([]*core.Chunk, 0, len(activeCoords))
	for _, coord := range activeCoords {
		if chunk := cm.GetChunkFast(coord); chunk != nil && chunk.GetState() == types.ChunkStateActive {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

// GetEntityChunk returns the chunk coordinate where the entity is currently located
func (cm *ChunkManager) GetEntityChunk(entityID types.EntityID) (*core.Chunk, bool) {
	cm.aoiMu.RLock()
	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		cm.aoiMu.RUnlock()
		return nil, false
	}
	centerChunk := aoi.CenterChunk
	cm.aoiMu.RUnlock()

	// Verify the entity actually exists in an active chunk
	chunk := cm.GetChunkFast(centerChunk)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		return nil, false
	}

	return chunk, true
}

func (cm *ChunkManager) Stats() ChunkStats {
	var activeCount int64
	var preloadedCount int64
	var inactiveCount int64

	cm.chunksMu.RLock()
	for _, chunk := range cm.chunks {
		if chunk == nil {
			continue
		}
		switch chunk.GetState() {
		case types.ChunkStateActive:
			activeCount++
		case types.ChunkStatePreloaded:
			preloadedCount++
		case types.ChunkStateInactive:
			inactiveCount++
		}
	}
	cm.chunksMu.RUnlock()

	return ChunkStats{
		ActiveCount:    activeCount,
		PreloadedCount: preloadedCount,
		InactiveCount:  inactiveCount,
		LoadRequests:   atomic.LoadInt64(&cm.stats.LoadRequests),
		SaveRequests:   atomic.LoadInt64(&cm.stats.SaveRequests),
		CacheHits:      atomic.LoadInt64(&cm.stats.CacheHits),
		CacheMisses:    atomic.LoadInt64(&cm.stats.CacheMisses),
	}
}

func (cm *ChunkManager) Update(dt float64) {
}

func (cm *ChunkManager) Stop() {
	// Set stopped flag to prevent LRU evictions from saving
	atomic.StoreInt32(&cm.stopped, 1)

	close(cm.stopCh)
	cm.wg.Wait()

	// Now purge the LRU cache - evictions won't save because stopped flag is set
	cm.lruCache.Purge()

	// Collect all chunks that need saving
	cm.chunksMu.RLock()
	allCoords := make([]types.ChunkCoord, 0, len(cm.chunks))
	for coord := range cm.chunks {
		allCoords = append(allCoords, coord)
	}
	cm.chunksMu.RUnlock()

	// Save chunks in parallel using worker pool
	numWorkers := cm.cfg.Game.SaveWorkers
	if numWorkers <= 0 {
		numWorkers = 1
	}

	workCh := make(chan types.ChunkCoord, numWorkers)
	var wg sync.WaitGroup

	// Start save workers
	var savedCount int64
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for coord := range workCh {
				if chunk := cm.GetChunkFast(coord); chunk != nil {
					state := chunk.GetState()
					if state == types.ChunkStateActive || state == types.ChunkStatePreloaded || state == types.ChunkStateInactive {
						if chunk.IsDirty(cm.world) {
							chunk.SaveToDB(cm.db, cm.world, cm.objectFactory, cm.logger)
							atomic.AddInt64(&savedCount, 1)
						}
					}
				}
			}
		}()
	}

	// Send work to workers
	for _, coord := range allCoords {
		workCh <- coord
	}
	close(workCh)

	// Wait for all saves to finish
	wg.Wait()

	cm.logger.Info("chunk manager stopped",
		zap.Int("layer", cm.layer),
		zap.Int("chunks_total", len(allCoords)),
		zap.Int64("chunks_saved", savedCount),
	)
}

func (cm *ChunkManager) ObjectFactory() *ObjectFactory {
	return cm.objectFactory
}

// AddStaticToChunkSpatial adds a static entity to the chunk's spatial hash.
// Used by inventory executor to register runtime-spawned dropped items.
func (cm *ChunkManager) AddStaticToChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int) {
	coord := types.ChunkCoord{X: chunkX, Y: chunkY}
	chunk := cm.GetChunkFast(coord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		return
	}
	chunk.Spatial().AddStatic(handle, x, y)
}

// RemoveStaticFromChunkSpatial removes a static entity from the chunk's spatial hash.
// Used by inventory executor to unregister dropped items on pickup.
func (cm *ChunkManager) RemoveStaticFromChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int) {
	coord := types.ChunkCoord{X: chunkX, Y: chunkY}
	chunk := cm.GetChunkFast(coord)
	if chunk == nil || chunk.GetState() != types.ChunkStateActive {
		return
	}
	chunk.Spatial().RemoveStatic(handle, x, y)
}

func (cm *ChunkManager) IsTilePassable(tileX, tileY int) bool {
	chunkSize := _const.ChunkSize
	chunkCoord := types.ChunkCoord{
		X: tileX / chunkSize,
		Y: tileY / chunkSize,
	}

	chunk := cm.GetChunk(chunkCoord)
	if chunk == nil {
		return false
	}

	localTileX := tileX % chunkSize
	localTileY := tileY % chunkSize
	if localTileX < 0 {
		localTileX += chunkSize
	}
	if localTileY < 0 {
		localTileY += chunkSize
	}

	return chunk.IsTilePassable(localTileX, localTileY, chunkSize)
}

func (cm *ChunkManager) IsTileSwimmable(tileX, tileY int) bool {
	chunkSize := _const.ChunkSize
	chunkCoord := types.ChunkCoord{
		X: tileX / chunkSize,
		Y: tileY / chunkSize,
	}

	chunk := cm.GetChunk(chunkCoord)
	if chunk == nil {
		return false
	}

	localTileX := tileX % chunkSize
	localTileY := tileY % chunkSize
	if localTileX < 0 {
		localTileX += chunkSize
	}
	if localTileY < 0 {
		localTileY += chunkSize
	}

	return chunk.IsTileSwimmable(localTileX, localTileY, chunkSize)
}
