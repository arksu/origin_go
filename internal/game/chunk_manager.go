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

const (
	ChunkStateUnloaded ChunkState = iota
	ChunkStateLoading
	ChunkStatePreloaded
	ChunkStateActive
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
	coord   ChunkCoord
	preload bool
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

	activeChunks    map[ChunkCoord]struct{}
	preloadedChunks map[ChunkCoord]struct{}
	stateMu         sync.RWMutex

	stats ChunkStats

	eventBus *eventbus.EventBus
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
		cfg:             cfg,
		db:              db,
		world:           world,
		layer:           layer,
		region:          region,
		objectFactory:   objectFactory,
		logger:          logger.Named("chunk_manager"),
		chunks:          make(map[ChunkCoord]*Chunk),
		loadQueue:       make(chan loadRequest, 256),
		saveQueue:       make(chan saveRequest, 256),
		stopCh:          make(chan struct{}),
		activeChunks:    make(map[ChunkCoord]struct{}),
		preloadedChunks: make(map[ChunkCoord]struct{}),
		eventBus:        eventBus,
	}

	cm.lruCache = lru.NewLRU[ChunkCoord, *Chunk](
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
		cm.updatePreloadZone(newChunk)
	}
}

func (cm *ChunkManager) updatePreloadZone(center ChunkCoord) {
	radius := cm.cfg.Game.PreloadRadius
	newPreload := make(map[ChunkCoord]struct{})

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			coord := ChunkCoord{X: center.X + dx, Y: center.Y + dy}
			newPreload[coord] = struct{}{}
		}
	}

	cm.stateMu.Lock()
	oldPreload := cm.preloadedChunks

	for coord := range oldPreload {
		if _, inNew := newPreload[coord]; !inNew {
			if _, isActive := cm.activeChunks[coord]; !isActive {
				if chunk := cm.getChunkUnsafe(coord); chunk != nil {
					cm.lruCache.Add(coord, chunk)
					atomic.AddInt64(&cm.stats.InactiveCount, 1)
				}
			}
		}
	}

	for coord := range newPreload {
		if _, isActive := cm.activeChunks[coord]; !isActive {
			cm.lruCache.Remove(coord)
			cm.requestLoad(coord, true)
		}
	}

	cm.preloadedChunks = newPreload
	cm.stateMu.Unlock()
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
			cm.loadChunkFromDB(req.coord, req.preload)
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

func (cm *ChunkManager) loadChunkFromDB(coord ChunkCoord, preload bool) {
	atomic.AddInt64(&cm.stats.LoadRequests, 1)

	cm.chunksMu.Lock()
	chunk, exists := cm.chunks[coord]
	if !exists {
		chunk = NewChunk(coord, cm.layer, cm.cfg.Game.ChunkSize)
		cm.chunks[coord] = chunk
	}
	cm.chunksMu.Unlock()

	state := chunk.GetState()
	if state != ChunkStateUnloaded && state != ChunkStateLoading {
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

		rawObjects := make([]*repository.Object, len(objects))
		for i := range objects {
			rawObjects[i] = &objects[i]
		}
		chunk.SetRawObjects(rawObjects)
	}

	if preload {
		chunk.SetState(ChunkStatePreloaded)
		atomic.AddInt64(&cm.stats.PreloadedCount, 1)
	} else {
		chunk.SetState(ChunkStatePreloaded)
	}

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
}

func (cm *ChunkManager) onEvict(coord ChunkCoord, chunk *Chunk) {
	if chunk == nil {
		return
	}

	cm.stateMu.RLock()
	_, isActive := cm.activeChunks[coord]
	_, isPreloaded := cm.preloadedChunks[coord]
	cm.stateMu.RUnlock()

	if isActive || isPreloaded {
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

func (cm *ChunkManager) GetChunk(coord ChunkCoord) *Chunk {
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

func (cm *ChunkManager) requestLoad(coord ChunkCoord, preload bool) {
	select {
	case cm.loadQueue <- loadRequest{coord: coord, preload: preload}:
	default:
	}
}

func (cm *ChunkManager) ActivateChunk(coord ChunkCoord) error {
	chunk := cm.GetOrCreateChunk(coord)
	state := chunk.GetState()

	switch state {
	case ChunkStateUnloaded:
		cm.requestLoad(coord, false)
		// TODO надо дождаться загрузки (ChunkStatePreloaded) чанка и обработать
		//  ActivateChunk
		return ErrChunkNotLoaded

	case ChunkStateLoading:
		// TODO надо дождаться загрузки чанка и обработать
		//  ActivateChunk
		return ErrChunkNotLoaded

	case ChunkStatePreloaded:
		return cm.activatePreloadedChunk(coord, chunk)

	case ChunkStateActive:
		return nil

	default:
		return ErrInvalidState
	}
}

func (cm *ChunkManager) activatePreloadedChunk(coord ChunkCoord, chunk *Chunk) error {
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

	cm.stateMu.Lock()
	cm.activeChunks[coord] = struct{}{}
	delete(cm.preloadedChunks, coord)
	cm.stateMu.Unlock()

	cm.lruCache.Remove(coord)

	atomic.AddInt64(&cm.stats.ActiveCount, 1)
	atomic.AddInt64(&cm.stats.PreloadedCount, -1)

	return nil
}

func (cm *ChunkManager) DeactivateChunk(coord ChunkCoord) error {
	chunk := cm.GetChunk(coord)
	if chunk == nil {
		return ErrChunkNotFound
	}

	if chunk.GetState() != ChunkStateActive {
		return ErrChunkNotActive
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

	cm.stateMu.Lock()
	delete(cm.activeChunks, coord)
	_, inPreloadZone := cm.preloadedChunks[coord]
	if !inPreloadZone {
		cm.lruCache.Add(coord, chunk)
	} else {
		cm.preloadedChunks[coord] = struct{}{}
	}
	cm.stateMu.Unlock()

	atomic.AddInt64(&cm.stats.ActiveCount, -1)
	if inPreloadZone {
		atomic.AddInt64(&cm.stats.PreloadedCount, 1)
	} else {
		atomic.AddInt64(&cm.stats.InactiveCount, 1)
	}

	return nil
}

func (cm *ChunkManager) MigrateObject(h ecs.Handle, fromCoord, toCoord ChunkCoord) error {
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
			toSpatial.AddStatic(h, float64(toCoord.X), float64(toCoord.Y))
		} else {
			toSpatial.AddDynamic(h, float64(toCoord.X), float64(toCoord.Y))
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
	radius := cm.cfg.Game.PreloadRadius

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			coord := ChunkCoord{X: center.X + dx, Y: center.Y + dy}

			cm.stateMu.Lock()
			cm.preloadedChunks[coord] = struct{}{}
			cm.stateMu.Unlock()

			chunk := cm.GetChunk(coord)
			if chunk == nil || chunk.GetState() == ChunkStateUnloaded {
				cm.requestLoad(coord, true)
			}
		}
	}
}

func (cm *ChunkManager) ActiveChunks() []*Chunk {
	cm.stateMu.RLock()
	coords := make([]ChunkCoord, 0, len(cm.activeChunks))
	for coord := range cm.activeChunks {
		coords = append(coords, coord)
	}
	cm.stateMu.RUnlock()

	chunks := make([]*Chunk, 0, len(coords))
	for _, coord := range coords {
		if chunk := cm.GetChunk(coord); chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func (cm *ChunkManager) ActiveChunkCoords() []ChunkCoord {
	cm.stateMu.RLock()
	defer cm.stateMu.RUnlock()

	coords := make([]ChunkCoord, 0, len(cm.activeChunks))
	for coord := range cm.activeChunks {
		coords = append(coords, coord)
	}
	return coords
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

	cm.stateMu.RLock()
	activeCoords := make([]ChunkCoord, 0, len(cm.activeChunks))
	for coord := range cm.activeChunks {
		activeCoords = append(activeCoords, coord)
	}
	preloadedCoords := make([]ChunkCoord, 0, len(cm.preloadedChunks))
	for coord := range cm.preloadedChunks {
		preloadedCoords = append(preloadedCoords, coord)
	}
	cm.stateMu.RUnlock()

	for _, coord := range activeCoords {
		if chunk := cm.GetChunk(coord); chunk != nil {
			cm.saveChunkToDB(chunk)
		}
	}

	for _, coord := range preloadedCoords {
		if chunk := cm.GetChunk(coord); chunk != nil {
			cm.saveChunkToDB(chunk)
		}
	}

	cm.lruCache.Purge()

	cm.logger.Info("chunk manager stopped",
		zap.Int32("layer", cm.layer),
		zap.Int64("active_saved", int64(len(activeCoords))),
		zap.Int64("preloaded_saved", int64(len(preloadedCoords))),
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
