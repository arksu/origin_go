package game

import (
	"context"
	"fmt"
	_const "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	"origin/internal/game/world"
	"origin/internal/persistence"
)

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

	world          *ecs.World
	chunkManager   *world.ChunkManager
	eventBus       *eventbus.EventBus
	characterSaver *systems.CharacterSaver

	// Command queues for network/ECS separation
	playerInbox *network.PlayerCommandInbox
	serverInbox *network.ServerJobInbox

	Clients   map[types.EntityID]*network.Client
	ClientsMu sync.RWMutex

	state ShardState
	mu    sync.RWMutex
}

func NewShard(layer int, cfg *config.Config, db *persistence.Postgres, entityIDManager *EntityIDManager, objectFactory *world.ObjectFactory, eb *eventbus.EventBus, logger *zap.Logger) *Shard {
	// Initialize command queue config from game config
	queueConfig := network.CommandQueueConfig{
		MaxQueueSize:                cfg.Game.CommandQueueSize,
		MaxPacketsPerSecond:         cfg.Game.MaxPacketsPerSecond,
		MaxCommandsPerTickPerClient: cfg.Game.MaxCommandsPerTickPerClient,
	}

	s := &Shard{
		layer:           layer,
		cfg:             cfg,
		db:              db,
		entityIDManager: entityIDManager,
		logger:          logger,
		world:           ecs.NewWorldWithCapacity(uint32(cfg.Game.MaxEntities), eb, layer),
		eventBus:        eb,
		playerInbox:     network.NewPlayerCommandInbox(queueConfig),
		serverInbox:     network.NewServerJobInbox(queueConfig),
		Clients:         make(map[types.EntityID]*network.Client),
		state:           ShardStateRunning,
	}

	s.chunkManager = world.NewChunkManager(cfg, db, s.world, s, layer, cfg.Game.Region, objectFactory, eb, logger)

	chunkSize := _const.ChunkSize * _const.CoordPerTile
	worldMinX := float64(cfg.Game.WorldMinXChunks * chunkSize)
	worldMaxX := float64((cfg.Game.WorldMinXChunks + cfg.Game.WorldWidthChunks) * chunkSize)
	worldMinY := float64(cfg.Game.WorldMinYChunks * chunkSize)
	worldMaxY := float64((cfg.Game.WorldMinYChunks + cfg.Game.WorldHeightChunks) * chunkSize)

	droppedItemPersister := world.NewDroppedItemPersisterDB(db)
	inventoryExecutor := inventory.NewInventoryExecutor(logger, entityIDManager, droppedItemPersister, s.chunkManager)

	// Create vision system first so it can be passed to other systems
	visionSystem := systems.NewVisionSystem(s.world, s.chunkManager, s.eventBus, logger)

	networkCmdSystem := systems.NewNetworkCommandSystem(s.playerInbox, s.serverInbox, s, inventoryExecutor, s, visionSystem, cfg.Game.ChatLocalRadius, logger)

	adminHandler := NewChatAdminCommandHandler(inventoryExecutor, s, s, logger)
	networkCmdSystem.SetAdminHandler(adminHandler)

	s.world.AddSystem(networkCmdSystem)
	s.world.AddSystem(systems.NewResetSystem(logger))
	s.world.AddSystem(systems.NewMovementSystem(s.world, s.chunkManager, logger))
	s.world.AddSystem(systems.NewCollisionSystem(s.world, s.chunkManager, logger, worldMinX, worldMaxX, worldMinY, worldMaxY, cfg.Game.WorldMarginTiles))
	s.world.AddSystem(systems.NewTransformUpdateSystem(s.world, s.chunkManager, s.eventBus, logger))
	s.world.AddSystem(visionSystem)
	s.world.AddSystem(systems.NewAutoInteractSystem(inventoryExecutor, s, visionSystem, logger))
	s.world.AddSystem(systems.NewChunkSystem(s.chunkManager, logger))

	inventorySaver := inventory.NewInventorySaver(logger)
	s.characterSaver = systems.NewCharacterSaver(db, cfg.Game.SaveWorkers, inventorySaver, logger)
	s.world.AddSystem(systems.NewCharacterSaveSystem(s.characterSaver, cfg.Game.PlayerSaveInterval, logger))
	s.world.AddSystem(systems.NewExpireDetachedSystem(logger, s.characterSaver, s.onDetachedEntityExpired))
	s.world.AddSystem(systems.NewDropDecaySystem(droppedItemPersister, logger))

	return s
}

func (s *Shard) Layer() int {
	return s.layer
}

func (s *Shard) World() *ecs.World {
	return s.world
}

func (s *Shard) ChunkManager() *world.ChunkManager {
	return s.chunkManager
}

func (s *Shard) Update(ts ecs.TimeState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != ShardStateRunning {
		return
	}

	// Set TimeState resource before systems run (zero-alloc: mutate in place)
	*ecs.GetResource[ecs.TimeState](s.world) = ts

	s.chunkManager.Update(ts.Delta)
	s.world.Update(ts.Delta)
}

func (s *Shard) Stop() {
	s.mu.Lock()
	s.state = ShardStateStopping
	s.mu.Unlock()

	if s.characterSaver != nil {
		s.characterSaver.SaveAll(s.world)
		s.characterSaver.Stop()
	}
	s.chunkManager.Stop()
}

func (s *Shard) spawnPlayerLocked(id types.EntityID, x int, y int, setupFunc func(*ecs.World, types.Handle)) types.Handle {
	handle := s.world.Spawn(id, setupFunc)

	// Publish event when player enters the world
	s.PublishEventAsync(
		ecs.NewPlayerEnteredWorldEvent(id, s.layer, x, y),
		eventbus.PriorityMedium,
	)

	return handle
}

func (s *Shard) EventBus() *eventbus.EventBus {
	return s.eventBus
}

// PlayerInbox returns the player command inbox for this shard
func (s *Shard) PlayerInbox() *network.PlayerCommandInbox {
	return s.playerInbox
}

// ServerInbox returns the server job inbox for this shard
func (s *Shard) ServerInbox() *network.ServerJobInbox {
	return s.serverInbox
}

func (s *Shard) PublishEventAsync(event eventbus.Event, priority eventbus.Priority) {
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

	centerChunk := types.WorldToChunkCoord(centerWorldX, centerWorldY, _const.ChunkSize, _const.CoordPerTile)
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
	s.chunkManager.RegisterEntity(entityID, centerWorldX, centerWorldY, false) // Don't send chunk load events during preparation
	s.mu.Unlock()

	s.logger.Debug("Entity registered with chunk manager",
		zap.Int64("entity_id", int64(entityID)),
		zap.Int("world_x", centerWorldX),
		zap.Int("world_y", centerWorldY),
	)

	return nil
}

func (s *Shard) TrySpawnPlayer(worldX, worldY int, character repository.Character, setupFunc func(*ecs.World, types.Handle)) (bool, types.Handle) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entityID := types.EntityID(character.ID)

	halfSize := _const.PlayerColliderSize / 2
	minX := worldX - halfSize
	minY := worldY - halfSize
	maxX := worldX + halfSize
	maxY := worldY + halfSize

	coordPerTile := _const.CoordPerTile
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
		if _const.PlayerLayer&collider.Mask == 0 && collider.Layer&_const.PlayerMask == 0 {
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

	handle := s.spawnPlayerLocked(entityID, worldX, worldY, setupFunc)
	if chunk, ok := s.chunkManager.GetEntityChunk(entityID); ok {
		chunk.Spatial().AddDynamic(handle, worldX, worldY)
	}
	return true, handle
}

func (s *Shard) UnregisterEntityAOI(entityID types.EntityID) {
	s.chunkManager.UnregisterEntity(entityID)
}

// onDetachedEntityExpired is called when a detached entity's TTL expires
// It handles cleanup of spatial index and AOI before the entity is despawned
func (s *Shard) onDetachedEntityExpired(entityID types.EntityID, handle types.Handle) {
	// Remove from chunk spatial index
	if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](s.world, handle); hasChunkRef {
		if transform, hasTransform := ecs.GetComponent[components.Transform](s.world, handle); hasTransform {
			if chunk := s.chunkManager.GetChunk(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}); chunk != nil {
				if entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](s.world, handle); hasEntityInfo && entityInfo.IsStatic {
					chunk.Spatial().RemoveStatic(handle, int(transform.X), int(transform.Y))
				} else {
					chunk.Spatial().RemoveDynamic(handle, int(transform.X), int(transform.Y))
				}
			}
		}
	}

	// Unregister from AOI
	s.chunkManager.UnregisterEntity(entityID)
}

// SendChatMessage sends a chat message to a single entity
func (s *Shard) SendChatMessage(entityID types.EntityID, channel netproto.ChatChannel, fromEntityID types.EntityID, fromName, text string) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	msg := &netproto.S2C_ChatMessage{
		Channel:      channel,
		FromEntityId: uint64(fromEntityID),
		FromName:     fromName,
		Text:         text,
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_Chat{
			Chat: msg,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal chat message",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendInventoryOpResult sends an inventory operation result to a client
func (s *Shard) SendInventoryOpResult(entityID types.EntityID, result *netproto.S2C_InventoryOpResult) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_InventoryOpResult{
			InventoryOpResult: result,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal inventory op result",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendContainerOpened sends a container opened state to a client
func (s *Shard) SendContainerOpened(entityID types.EntityID, state *netproto.InventoryState) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ContainerOpened{
			ContainerOpened: &netproto.S2C_ContainerOpened{
				State: state,
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal container opened",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// BroadcastChatMessage sends a chat message to multiple entities
func (s *Shard) BroadcastChatMessage(entityIDs []types.EntityID, channel netproto.ChatChannel, fromEntityID types.EntityID, fromName, text string) {
	if len(entityIDs) == 0 {
		return
	}

	msg := &netproto.S2C_ChatMessage{
		Channel:      channel,
		FromEntityId: uint64(fromEntityID),
		FromName:     fromName,
		Text:         text,
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_Chat{
			Chat: msg,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal chat message for broadcast",
			zap.Int("recipient_count", len(entityIDs)),
			zap.Error(err))
		return
	}

	s.ClientsMu.RLock()
	defer s.ClientsMu.RUnlock()

	for _, entityID := range entityIDs {
		if client, ok := s.Clients[entityID]; ok && client != nil {
			client.Send(data)
		}
	}
}
