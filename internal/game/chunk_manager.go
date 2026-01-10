package game

import (
	"context"
	"database/sql"
	"origin/internal/ecs/components"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
)

// Ensure context is used (for eventbus handler signature)
var _ context.Context

type ChunkState uint8

/*
Unloaded → Loading (requestLoad)
Loading → Preloaded (loadChunkFromDB завершён)
Preloaded → Active (ActivateChunk)
Active → Preloaded (DeactivateChunk)
Preloaded → Inactive (выход из preload-зоны в updatePreloadZone)
Inactive → Unloaded (LRU eviction)
Preloaded → Inactive (выход из preload-зоны)
Inactive → Preloaded (возврат в preload-зону)
*/

const (
	ChunkStateUnloaded ChunkState = iota
	ChunkStateLoading
	ChunkStatePreloaded
	ChunkStateActive
	ChunkStateInactive
)

func (s ChunkState) String() string {
	switch s {
	case ChunkStateUnloaded:
		return "unloaded"
	case ChunkStateLoading:
		return "loading"
	case ChunkStatePreloaded:
		return "preloaded"
	case ChunkStateActive:
		return "active"
	case ChunkStateInactive:
		return "inactive"
	default:
		return "unknown"
	}
}

type ChunkCoord struct {
	X, Y int
}

type Chunk struct {
	Coord    ChunkCoord
	Layer    int32
	State    ChunkState
	Tiles    []byte
	LastTick uint64

	rawObjects []*repository.Object
	spatial    *SpatialHashGrid

	mu sync.RWMutex
}

func NewChunk(coord ChunkCoord, layer int32, chunkSize int) *Chunk {
	cellSize := 16.0

	return &Chunk{
		Coord:   coord,
		Layer:   layer,
		State:   ChunkStateUnloaded,
		Tiles:   make([]byte, chunkSize*chunkSize),
		spatial: NewSpatialHashGrid(cellSize),
	}
}

func (c *Chunk) SetState(state ChunkState) {
	c.mu.Lock()
	c.State = state
	c.mu.Unlock()
}

func (c *Chunk) GetState() ChunkState {
	c.mu.RLock()
	state := c.State
	c.mu.RUnlock()
	return state
}

func (c *Chunk) SetRawObjects(objects []*repository.Object) {
	c.mu.Lock()
	c.rawObjects = objects
	c.mu.Unlock()
}

func (c *Chunk) GetRawObjects() []*repository.Object {
	c.mu.RLock()
	objects := c.rawObjects
	c.mu.RUnlock()
	return objects
}

func (c *Chunk) ClearRawObjects() {
	c.mu.Lock()
	c.rawObjects = nil
	c.mu.Unlock()
}

func (c *Chunk) GetHandles() []ecs.Handle {
	return c.spatial.GetAllHandles()
}

func (c *Chunk) GetDynamicHandles() []ecs.Handle {
	return c.spatial.GetDynamicHandles()
}

func (c *Chunk) ClearHandles() {
	c.mu.Lock()
	c.spatial.ClearDynamic()
	c.spatial.ClearStatic()
	c.mu.Unlock()
}

func (c *Chunk) Spatial() *SpatialHashGrid {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.spatial
}

type loadRequest struct {
	coord ChunkCoord
}

type saveRequest struct {
	chunk *Chunk
}

type ChunkStats struct {
	ActiveCount    int64
	PreloadedCount int64
	InactiveCount  int64
	LoadRequests   int64
	CacheHits      int64
	CacheMisses    int64
}

// EntityAOI represents Area of Interest for a single entity
type EntityAOI struct {
	EntityID      ecs.EntityID
	Handle        ecs.Handle
	CenterChunk   ChunkCoord
	ActiveChunks  map[ChunkCoord]struct{}
	PreloadChunks map[ChunkCoord]struct{}
}

func newEntityAOI(entityID ecs.EntityID, handle ecs.Handle, center ChunkCoord) *EntityAOI {
	return &EntityAOI{
		EntityID:      entityID,
		Handle:        handle,
		CenterChunk:   center,
		ActiveChunks:  make(map[ChunkCoord]struct{}),
		PreloadChunks: make(map[ChunkCoord]struct{}),
	}
}

// ChunkInterest tracks which entities are interested in a chunk
type ChunkInterest struct {
	activeEntities  map[ecs.EntityID]struct{} // entities with this chunk in active zone
	preloadEntities map[ecs.EntityID]struct{} // entities with this chunk in preload zone (not active)
}

func newChunkInterest() *ChunkInterest {
	return &ChunkInterest{
		activeEntities:  make(map[ecs.EntityID]struct{}),
		preloadEntities: make(map[ecs.EntityID]struct{}),
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

type ChunkManager struct {
	cfg           *config.Config
	db            *persistence.Postgres
	world         *ecs.World
	layer         int32
	region        int32
	objectFactory *ObjectFactory
	logger        *zap.Logger

	chunks   map[ChunkCoord]*Chunk
	chunksMu sync.RWMutex

	lruCache *lru.LRU[ChunkCoord, *Chunk]

	loadQueue chan loadRequest
	saveQueue chan saveRequest
	stopCh    chan struct{}
	wg        sync.WaitGroup

	// Per-entity AOI tracking
	entityAOIs map[ecs.EntityID]*EntityAOI
	aoiMu      sync.RWMutex

	// Global chunk interest tracking
	chunkInterests map[ChunkCoord]*ChunkInterest
	interestMu     sync.RWMutex

	// Cached active chunks for fast access (hot path optimization)
	activeChunks   map[ChunkCoord]struct{}
	activeChunksMu sync.RWMutex

	loadFutures   map[ChunkCoord]*loadFuture
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
	layer int32,
	region int32,
	objectFactory *ObjectFactory,
	eventBus *eventbus.EventBus,
	logger *zap.Logger,
) *ChunkManager {
	ttl := time.Duration(cfg.Game.ChunkLRUTTL) * time.Second

	cm := &ChunkManager{
		cfg:            cfg,
		db:             db,
		world:          world,
		layer:          layer,
		region:         region,
		objectFactory:  objectFactory,
		logger:         logger.Named("chunk_manager"),
		chunks:         make(map[ChunkCoord]*Chunk),
		loadQueue:      make(chan loadRequest, 256),
		saveQueue:      make(chan saveRequest, 256),
		stopCh:         make(chan struct{}),
		entityAOIs:     make(map[ecs.EntityID]*EntityAOI),
		chunkInterests: make(map[ChunkCoord]*ChunkInterest),
		activeChunks:   make(map[ChunkCoord]struct{}),
		loadFutures:    make(map[ChunkCoord]*loadFuture),
		eventBus:       eventBus,
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

	cm.subscribeToEvents()

	return cm
}

func (cm *ChunkManager) subscribeToEvents() {
	cm.eventBus.SubscribeAsync(eventbus.TopicGameplayMovementMove, eventbus.PriorityMedium, func(ctx context.Context, e eventbus.Event) error {
		if move, ok := e.(*eventbus.MovementEvent); ok {
			cm.handleMovement(move)
		}
		return nil
	})
}

func (cm *ChunkManager) handleMovement(move *eventbus.MovementEvent) {
	oldChunk := WorldToChunkCoord(int(move.FromX), int(move.FromY), cm.cfg.Game.ChunkSize, cm.cfg.Game.CoordPerTile)
	newChunk := WorldToChunkCoord(int(move.ToX), int(move.ToY), cm.cfg.Game.ChunkSize, cm.cfg.Game.CoordPerTile)

	if oldChunk != newChunk {
		cm.UpdateEntityPosition(move.EntityID, newChunk)
	}
}

// RegisterEntity registers an entity for AOI tracking
func (cm *ChunkManager) RegisterEntity(entityID ecs.EntityID, handle ecs.Handle, worldX, worldY int) {
	center := WorldToChunkCoord(worldX, worldY, cm.cfg.Game.ChunkSize, cm.cfg.Game.CoordPerTile)

	cm.aoiMu.Lock()
	if _, exists := cm.entityAOIs[entityID]; exists {
		cm.aoiMu.Unlock()
		return
	}
	aoi := newEntityAOI(entityID, handle, center)
	cm.entityAOIs[entityID] = aoi
	cm.aoiMu.Unlock()

	cm.updateEntityAOI(entityID, center)
}

// UnregisterEntity removes an entity from AOI tracking
func (cm *ChunkManager) UnregisterEntity(entityID ecs.EntityID) {
	cm.aoiMu.Lock()
	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		cm.aoiMu.Unlock()
		return
	}
	delete(cm.entityAOIs, entityID)
	cm.aoiMu.Unlock()

	cm.removeEntityInterests(entityID, aoi)
	cm.recalculateChunkStates()
}

// UpdateEntityPosition updates AOI for an entity when it moves to a new chunk
func (cm *ChunkManager) UpdateEntityPosition(entityID ecs.EntityID, newCenter ChunkCoord) {
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
func (cm *ChunkManager) updateEntityAOI(entityID ecs.EntityID, newCenter ChunkCoord) {
	activeRadius := cm.cfg.Game.PlayerActiveChunkRadius
	preloadRadius := cm.cfg.Game.PlayerPreloadChunkRadius

	newActive := make(map[ChunkCoord]struct{})
	newPreload := make(map[ChunkCoord]struct{})

	// Calculate new active zone (with bounds checking)
	for dy := -activeRadius; dy <= activeRadius; dy++ {
		for dx := -activeRadius; dx <= activeRadius; dx++ {
			coord := ChunkCoord{X: newCenter.X + dx, Y: newCenter.Y + dy}
			if cm.isWithinWorldBounds(coord) {
				newActive[coord] = struct{}{}
			}
		}
	}

	// Calculate new preload zone (excluding active, with bounds checking)
	for dy := -preloadRadius; dy <= preloadRadius; dy++ {
		for dx := -preloadRadius; dx <= preloadRadius; dx++ {
			coord := ChunkCoord{X: newCenter.X + dx, Y: newCenter.Y + dy}
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
	cm.aoiMu.Unlock()

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
}

// removeEntityInterests removes all interests for an entity
func (cm *ChunkManager) removeEntityInterests(entityID ecs.EntityID, aoi *EntityAOI) {
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
	interestsSnapshot := make(map[ChunkCoord]chunkInterestSnapshot, len(cm.chunkInterests))
	for coord, interest := range cm.chunkInterests {
		interestsSnapshot[coord] = chunkInterestSnapshot{
			activeCount:  len(interest.activeEntities),
			preloadCount: len(interest.preloadEntities),
		}
	}
	cm.interestMu.RUnlock()

	// Track changes to active chunks
	activatedChunks := make([]ChunkCoord, 0)
	deactivatedChunks := make([]ChunkCoord, 0)

	for coord, snapshot := range interestsSnapshot {
		chunk := cm.GetChunk(coord)

		if snapshot.activeCount > 0 {
			// Should be Active
			if chunk == nil {
				_ = cm.requestLoad(coord)
			} else {
				state := chunk.GetState()
				switch state {
				case ChunkStatePreloaded, ChunkStateInactive:
					if err := cm.activateChunkInternal(coord, chunk); err != nil {
						cm.logger.Debug("failed to activate chunk",
							zap.Int("x", coord.X),
							zap.Int("y", coord.Y),
							zap.Error(err),
						)
					} else {
						activatedChunks = append(activatedChunks, coord)
					}
				case ChunkStateActive:
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
				case ChunkStateActive:
					if err := cm.deactivateChunkInternal(chunk); err != nil {
						cm.logger.Debug("failed to deactivate chunk",
							zap.Int("x", coord.X),
							zap.Int("y", coord.Y),
							zap.Error(err),
						)
					} else {
						deactivatedChunks = append(deactivatedChunks, coord)
					}
				case ChunkStateInactive:
					chunk.SetState(ChunkStatePreloaded)
					cm.lruCache.Remove(coord)
					atomic.AddInt64(&cm.stats.InactiveCount, -1)
					atomic.AddInt64(&cm.stats.PreloadedCount, 1)
				}
			}
		} else {
			// No interest -> Inactive
			if chunk != nil {
				state := chunk.GetState()
				switch state {
				case ChunkStateActive:
					if err := cm.deactivateChunkInternal(chunk); err == nil {
						chunk.SetState(ChunkStateInactive)
						cm.lruCache.Add(coord, chunk)
						atomic.AddInt64(&cm.stats.PreloadedCount, -1)
						atomic.AddInt64(&cm.stats.InactiveCount, 1)
						deactivatedChunks = append(deactivatedChunks, coord)
					}
				case ChunkStatePreloaded:
					chunk.SetState(ChunkStateInactive)
					cm.lruCache.Add(coord, chunk)
					atomic.AddInt64(&cm.stats.PreloadedCount, -1)
					atomic.AddInt64(&cm.stats.InactiveCount, 1)
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

func (cm *ChunkManager) getChunkUnsafe(coord ChunkCoord) *Chunk {
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
			cm.saveChunkToDB(req.chunk)
		}
	}
}

func (cm *ChunkManager) loadChunkFromDB(coord ChunkCoord) {
	atomic.AddInt64(&cm.stats.LoadRequests, 1)

	cm.chunksMu.Lock()
	chunk, exists := cm.chunks[coord]
	if !exists {
		chunk = NewChunk(coord, cm.layer, cm.cfg.Game.ChunkSize)
		cm.chunks[coord] = chunk
	}
	cm.chunksMu.Unlock()

	state := chunk.GetState()
	if state != ChunkStateUnloaded {
		return
	}

	chunk.SetState(ChunkStateLoading)

	if cm.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		tilesData, err := cm.db.Queries().GetChunk(ctx, repository.GetChunkParams{
			Region: cm.region,
			X:      int32(coord.X),
			Y:      int32(coord.Y),
			Layer:  cm.layer,
		})
		if err == nil {
			chunk.mu.Lock()
			chunk.Tiles = tilesData.TilesData
			chunk.LastTick = uint64(tilesData.LastTick)
			chunk.mu.Unlock()
		}

		objects, err := cm.db.Queries().GetObjectsByChunk(ctx, repository.GetObjectsByChunkParams{
			Region: cm.region,
			ChunkX: int32(coord.X),
			ChunkY: int32(coord.Y),
			Layer:  cm.layer,
		})
		if err != nil {
			cm.logger.Error("failed to load objects",
				zap.Int("chunk_x", coord.X),
				zap.Int("chunk_y", coord.Y),
				zap.Error(err),
			)
			objects = nil
		}
		cm.logger.Debug("loaded objects", zap.Any("coord", coord), zap.Int("count", len(objects)))

		rawObjects := make([]*repository.Object, len(objects))
		for i := range objects {
			rawObjects[i] = &objects[i]
		}
		chunk.SetRawObjects(rawObjects)
	}

	chunk.SetState(ChunkStatePreloaded)
	atomic.AddInt64(&cm.stats.PreloadedCount, 1)

	cm.completeFuture(coord)

	cm.eventBus.PublishAsync(
		eventbus.NewChunkLoadEvent(coord.X, coord.Y, cm.layer),
		eventbus.PriorityLow,
	)
}

func (cm *ChunkManager) saveChunkToDB(chunk *Chunk) {
	if cm.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coord := chunk.Coord

	chunk.mu.RLock()
	tiles := make([]byte, len(chunk.Tiles))
	copy(tiles, chunk.Tiles)
	lastTick := chunk.LastTick
	totalHandles := chunk.GetHandles()
	entityCount := len(totalHandles)
	chunk.mu.RUnlock()

	err := cm.db.Queries().UpsertChunk(ctx, repository.UpsertChunkParams{
		Region:      cm.region,
		X:           int32(coord.X),
		Y:           int32(coord.Y),
		Layer:       cm.layer,
		TilesData:   tiles,
		LastTick:    int64(lastTick),
		EntityCount: sql.NullInt32{Int32: int32(entityCount), Valid: true},
	})
	if err != nil {
		cm.logger.Error("failed to save chunk tiles",
			zap.Int("chunk_x", coord.X),
			zap.Int("chunk_y", coord.Y),
			zap.Error(err),
		)
	}

	handles := totalHandles
	for _, h := range handles {
		if !cm.world.Alive(h) {
			continue
		}

		info, ok := ecs.GetComponent[components.EntityInfo](cm.world, h)
		if !ok {
			continue
		}

		obj, err := cm.objectFactory.Serialize(cm.world, h, info.ObjectType)
		if err != nil {
			cm.logger.Error("failed to serialize object",
				zap.Error(err),
			)
			continue
		}
		//cm.logger.Info("save object", zap.Int64("object_id", obj.ID), zap.Any("info", info))

		obj.LastTick = int64(lastTick)
		err = cm.db.Queries().UpsertObject(ctx, repository.UpsertObjectParams{
			ID:         obj.ID,
			ObjectType: obj.ObjectType,
			Region:     obj.Region,
			X:          obj.X,
			Y:          obj.Y,
			Layer:      obj.Layer,
			ChunkX:     obj.ChunkX,
			ChunkY:     obj.ChunkY,
			Heading:    obj.Heading,
			Quality:    obj.Quality,
			HpCurrent:  obj.HpCurrent,
			HpMax:      obj.HpMax,
			IsStatic:   obj.IsStatic,
			OwnerID:    obj.OwnerID,
			DataJsonb:  obj.DataJsonb,
			CreateTick: obj.CreateTick,
			LastTick:   obj.LastTick,
		})
		if err != nil {
			cm.logger.Error("failed to save object",
				zap.Int64("object_id", obj.ID),
				zap.Error(err),
			)
		}
	}
	cm.logger.Debug("saved objects", zap.Int("count", len(handles)))
}

func (cm *ChunkManager) onEvict(coord ChunkCoord, chunk *Chunk) {
	if chunk == nil {
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

	select {
	case cm.saveQueue <- saveRequest{chunk: chunk}:
	default:
		cm.logger.Warn("save queue full, saving synchronously",
			zap.Int("chunk_x", coord.X),
			zap.Int("chunk_y", coord.Y),
		)
		cm.saveChunkToDB(chunk)
	}

	cm.chunksMu.Lock()
	delete(cm.chunks, coord)
	cm.chunksMu.Unlock()

	atomic.AddInt64(&cm.stats.InactiveCount, -1)

	cm.eventBus.PublishAsync(
		eventbus.NewChunkUnloadEvent(coord.X, coord.Y, cm.layer),
		eventbus.PriorityLow,
	)
}

// isWithinWorldBounds checks if a chunk coordinate is within world boundaries
func (cm *ChunkManager) isWithinWorldBounds(coord ChunkCoord) bool {
	minX := cm.cfg.Game.WorldMinXChunks
	minY := cm.cfg.Game.WorldMinYChunks
	maxX := minX + cm.cfg.Game.WorldWidthChunks
	maxY := minY + cm.cfg.Game.WorldHeightChunks

	return coord.X >= minX && coord.X < maxX && coord.Y >= minY && coord.Y < maxY
}

func (cm *ChunkManager) GetChunk(coord ChunkCoord) *Chunk {
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

func (cm *ChunkManager) GetOrCreateChunk(coord ChunkCoord) *Chunk {
	cm.chunksMu.Lock()
	defer cm.chunksMu.Unlock()

	if chunk, exists := cm.chunks[coord]; exists {
		return chunk
	}

	chunk := NewChunk(coord, cm.layer, cm.cfg.Game.ChunkSize)
	cm.chunks[coord] = chunk
	return chunk
}

func (cm *ChunkManager) requestLoad(coord ChunkCoord) bool {
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

func (cm *ChunkManager) getOrCreateFuture(coord ChunkCoord) chan struct{} {
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

func (cm *ChunkManager) completeFuture(coord ChunkCoord) {
	cm.loadFuturesMu.Lock()
	defer cm.loadFuturesMu.Unlock()

	if fut, exists := cm.loadFutures[coord]; exists {
		close(fut.done)
		delete(cm.loadFutures, coord)
	}
}

func (cm *ChunkManager) cleanupFuture(coord ChunkCoord) {
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
	chunk := cm.GetChunk(coord)
	if chunk != nil && chunk.GetState() != ChunkStateUnloaded {
		return
	}

	cm.loadFuturesMu.Lock()
	if fut2, exists2 := cm.loadFutures[coord]; exists2 && fut2.waiters <= 0 {
		delete(cm.loadFutures, coord)
	}
	cm.loadFuturesMu.Unlock()
}

func (cm *ChunkManager) WaitPreloaded(ctx context.Context, coord ChunkCoord) error {
	chunk := cm.GetChunk(coord)
	if chunk != nil {
		state := chunk.GetState()
		if state == ChunkStatePreloaded || state == ChunkStateActive {
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

// ActivateChunk transitions a chunk to an active state based on its current state.
// Returns an error if the chunk cannot be activated or is in an invalid state.
func (cm *ChunkManager) ActivateChunk(coord ChunkCoord) error {
	chunk := cm.GetOrCreateChunk(coord)
	if chunk == nil {
		return ErrChunkNotLoaded
	}
	state := chunk.GetState()

	switch state {
	case ChunkStateUnloaded:
		return ErrChunkNotLoaded

	case ChunkStateLoading:
		return ErrChunkNotLoaded

	case ChunkStatePreloaded:
		return cm.activatePreloadedChunk(coord, chunk)

	case ChunkStateActive:
		return nil

	case ChunkStateInactive:
		return cm.activatePreloadedChunk(coord, chunk)

	default:
		return ErrInvalidState
	}
}

func (cm *ChunkManager) activatePreloadedChunk(coord ChunkCoord, chunk *Chunk) error {
	return cm.activateChunkInternal(coord, chunk)
}

// activateChunkInternal activates a chunk by building entities from raw objects
func (cm *ChunkManager) activateChunkInternal(coord ChunkCoord, chunk *Chunk) error {
	state := chunk.GetState()
	if state == ChunkStateActive {
		return nil
	}
	if state == ChunkStateUnloaded || state == ChunkStateLoading {
		return ErrChunkNotLoaded
	}

	rawObjects := chunk.GetRawObjects()
	spatial := chunk.Spatial()

	for _, raw := range rawObjects {
		h, err := cm.objectFactory.Build(cm.world, raw)
		if err != nil {
			cm.logger.Error("failed to build object",
				zap.Int64("object_id", raw.ID),
				zap.Int32("object_type", raw.ObjectType),
				zap.Error(err),
			)
			continue
		}

		ecs.AddComponent(cm.world, h, components.ChunkRef{
			CurrentChunkX: coord.X,
			CurrentChunkY: coord.Y,
		})

		isStatic := cm.objectFactory.IsStatic(raw)
		x := float64(raw.X)
		y := float64(raw.Y)

		if isStatic {
			spatial.AddStatic(h, x, y)
		} else {
			spatial.AddDynamic(h, x, y)
		}
	}

	chunk.ClearRawObjects()
	chunk.SetState(ChunkStateActive)
	cm.lruCache.Remove(coord)

	atomic.AddInt64(&cm.stats.ActiveCount, 1)
	atomic.AddInt64(&cm.stats.PreloadedCount, -1)

	cm.activeChunksMu.Lock()
	cm.activeChunks[coord] = struct{}{}
	cm.activeChunksMu.Unlock()

	return nil
}

func (cm *ChunkManager) DeactivateChunk(coord ChunkCoord) error {
	chunk := cm.GetChunk(coord)
	if chunk == nil {
		return nil
	}

	if chunk.GetState() != ChunkStateActive {
		return nil
	}

	err := cm.deactivateChunkInternal(chunk)
	if err == nil {
		cm.activeChunksMu.Lock()
		delete(cm.activeChunks, coord)
		cm.activeChunksMu.Unlock()
	}
	return err
}

// deactivateChunkInternal deactivates a chunk by serializing entities to raw objects
func (cm *ChunkManager) deactivateChunkInternal(chunk *Chunk) error {
	if chunk.GetState() != ChunkStateActive {
		return nil
	}

	handles := chunk.GetHandles()
	rawObjects := make([]*repository.Object, 0, len(handles))

	for _, h := range handles {
		if !cm.world.Alive(h) {
			continue
		}

		info, ok := ecs.GetComponent[components.EntityInfo](cm.world, h)
		if !ok {
			continue
		}

		obj, err := cm.objectFactory.Serialize(cm.world, h, info.ObjectType)
		if err != nil {
			cm.logger.Error("failed to serialize object for deactivation",
				zap.Error(err),
			)
			continue
		}

		rawObjects = append(rawObjects, obj)
		cm.world.Despawn(h)
	}

	chunk.SetRawObjects(rawObjects)
	chunk.ClearHandles()
	chunk.SetState(ChunkStatePreloaded)

	atomic.AddInt64(&cm.stats.ActiveCount, -1)
	atomic.AddInt64(&cm.stats.PreloadedCount, 1)

	return nil
}

// MigrateObject moves an entity identified by the given handle from one chunk to another, updating all relevant spatial and reference data. Returns an error if the operation cannot be completed.
// переход только из активного в другой активный или preloaded чанк
func (cm *ChunkManager) MigrateObject(h ecs.Handle, fromCoord, toCoord ChunkCoord, toX, toY float64) error {
	if fromCoord == toCoord {
		return nil
	}

	fromChunk := cm.GetChunk(fromCoord)
	toChunk := cm.GetChunk(toCoord)

	if fromChunk == nil || toChunk == nil {
		return ErrChunkNotFound
	}

	if fromChunk.GetState() != ChunkStateActive {
		return ErrChunkNotActive
	}

	info, ok := ecs.GetComponent[components.EntityInfo](cm.world, h)
	if !ok {
		return ErrEntityNotFound
	}
	chunkRef, ok := ecs.GetComponent[components.ChunkRef](cm.world, h)
	if !ok {
		return ErrEntityNotFound
	}
	if chunkRef.CurrentChunkX != fromCoord.X || chunkRef.CurrentChunkY != fromCoord.Y {
		return ErrEntityNotInChunk
	}

	isStatic := info.IsStatic

	fromSpatial := fromChunk.Spatial()
	if isStatic {
		fromSpatial.RemoveStatic(h, float64(chunkRef.CurrentChunkX), float64(chunkRef.CurrentChunkY))
	} else {
		fromSpatial.RemoveDynamic(h, float64(chunkRef.CurrentChunkX), float64(chunkRef.CurrentChunkY))
	}

	ecs.WithComponent(cm.world, h, func(ref *components.ChunkRef) {
		ref.PrevChunkX = chunkRef.CurrentChunkX
		ref.PrevChunkY = chunkRef.CurrentChunkY
		ref.CurrentChunkX = toCoord.X
		ref.CurrentChunkY = toCoord.Y
	})

	if toChunk.GetState() == ChunkStateActive {
		toSpatial := toChunk.Spatial()
		if isStatic {
			toSpatial.AddStatic(h, toX, toY)
		} else {
			toSpatial.AddDynamic(h, toX, toY)
		}
	} else {
		obj, err := cm.objectFactory.Serialize(cm.world, h, info.ObjectType)
		if err != nil {
			return err
		}

		toChunk.mu.Lock()
		toChunk.rawObjects = append(toChunk.rawObjects, obj)
		toChunk.mu.Unlock()

		cm.world.Despawn(h)
	}

	return nil
}

func (cm *ChunkManager) PreloadChunksAround(center ChunkCoord) {
	radius := cm.cfg.Game.PlayerPreloadChunkRadius

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			coord := ChunkCoord{X: center.X + dx, Y: center.Y + dy}

			// Skip chunks outside world bounds
			if !cm.isWithinWorldBounds(coord) {
				continue
			}

			chunk := cm.GetChunk(coord)
			if chunk == nil || chunk.GetState() == ChunkStateUnloaded {
				_ = cm.requestLoad(coord)
			}
		}
	}
}

func (cm *ChunkManager) ActiveChunks() []*Chunk {
	// Fast path: read from cached active chunks
	cm.activeChunksMu.RLock()
	coords := make([]ChunkCoord, 0, len(cm.activeChunks))
	for coord := range cm.activeChunks {
		coords = append(coords, coord)
	}
	cm.activeChunksMu.RUnlock()

	chunks := make([]*Chunk, 0, len(coords))
	for _, coord := range coords {
		if chunk := cm.GetChunk(coord); chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func (cm *ChunkManager) ActiveChunkCoords() []ChunkCoord {
	// Fast path: read from cached active chunks
	cm.activeChunksMu.RLock()
	defer cm.activeChunksMu.RUnlock()

	coords := make([]ChunkCoord, 0, len(cm.activeChunks))
	for coord := range cm.activeChunks {
		coords = append(coords, coord)
	}
	return coords
}

func (cm *ChunkManager) GetEntityActiveChunks(entityID ecs.EntityID) []*Chunk {
	cm.aoiMu.RLock()
	aoi, exists := cm.entityAOIs[entityID]
	if !exists {
		cm.aoiMu.RUnlock()
		return nil
	}
	activeCoords := make([]ChunkCoord, 0, len(aoi.ActiveChunks))
	for coord := range aoi.ActiveChunks {
		activeCoords = append(activeCoords, coord)
	}
	cm.aoiMu.RUnlock()

	chunks := make([]*Chunk, 0, len(activeCoords))
	for _, coord := range activeCoords {
		if chunk := cm.GetChunk(coord); chunk != nil && chunk.GetState() == ChunkStateActive {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func (cm *ChunkManager) Stats() ChunkStats {
	return ChunkStats{
		ActiveCount:    atomic.LoadInt64(&cm.stats.ActiveCount),
		PreloadedCount: atomic.LoadInt64(&cm.stats.PreloadedCount),
		InactiveCount:  atomic.LoadInt64(&cm.stats.InactiveCount),
		LoadRequests:   atomic.LoadInt64(&cm.stats.LoadRequests),
		CacheHits:      atomic.LoadInt64(&cm.stats.CacheHits),
		CacheMisses:    atomic.LoadInt64(&cm.stats.CacheMisses),
	}
}

func (cm *ChunkManager) Update(dt float32) {
}

func (cm *ChunkManager) Stop() {
	close(cm.stopCh)
	cm.wg.Wait()

	// Collect all chunks that need saving
	cm.interestMu.RLock()
	interestedCoords := make([]ChunkCoord, 0, len(cm.chunkInterests))
	for coord := range cm.chunkInterests {
		interestedCoords = append(interestedCoords, coord)
	}
	cm.interestMu.RUnlock()

	// Also save all loaded chunks
	cm.chunksMu.RLock()
	allCoords := make([]ChunkCoord, 0, len(cm.chunks))
	for coord := range cm.chunks {
		allCoords = append(allCoords, coord)
	}
	cm.chunksMu.RUnlock()

	savedCount := 0
	for _, coord := range allCoords {
		if chunk := cm.GetChunk(coord); chunk != nil {
			state := chunk.GetState()
			if state == ChunkStateActive || state == ChunkStatePreloaded {
				cm.saveChunkToDB(chunk)
				savedCount++
			}
		}
	}

	cm.lruCache.Purge()

	cm.logger.Info("chunk manager stopped",
		zap.Int32("layer", cm.layer),
		zap.Int("chunks_saved", savedCount),
	)
}

func (cm *ChunkManager) ObjectFactory() *ObjectFactory {
	return cm.objectFactory
}

func WorldToChunkCoord(worldX, worldY int, chunkSize, coordPerTile int) ChunkCoord {
	tileX := worldX / coordPerTile
	tileY := worldY / coordPerTile
	return ChunkCoord{
		X: tileX / chunkSize,
		Y: tileY / chunkSize,
	}
}
