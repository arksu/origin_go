package game

import (
	"context"
	"fmt"
	"origin/internal/characterattrs"
	_const "origin/internal/const"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/entitystats"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"strings"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/inventory"
	"origin/internal/game/world"
	"origin/internal/persistence"
	"time"
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
	playerInbox     *network.PlayerCommandInbox
	serverInbox     *network.ServerJobInbox
	snapshotSender  *inventory.SnapshotSender
	adminHandler    *ChatAdminCommandHandler
	craftingService *CraftingService
	buildService    *BuildService

	Clients   map[types.EntityID]*network.Client
	ClientsMu sync.RWMutex

	state ShardState
	mu    sync.RWMutex
}

func NewShard(layer int, cfg *config.Config, db *persistence.Postgres, entityIDManager *EntityIDManager, objectFactory *world.ObjectFactory, snapshotSender *inventory.SnapshotSender, eb *eventbus.EventBus, enableVisionStats bool, logger *zap.Logger) *Shard {
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
		snapshotSender:  snapshotSender,
		Clients:         make(map[types.EntityID]*network.Client),
		state:           ShardStateRunning,
	}
	ecs.SetResource(s.world, ecs.EntityStatsRuntimeConfig{
		PlayerStatsTTLms:          uint32(cfg.Game.PlayerStatsTTLms),
		StaminaRegenIntervalTicks: uint64(cfg.Game.StaminaRegenIntervalTicks),
	})
	ecs.SetResource(s.world, ecs.BehaviorTickPolicy{
		GlobalBudgetPerTick: cfg.Game.BehaviorTickGlobalBudget,
		CatchUpLimitTicks:   uint64(cfg.Game.BehaviorTickCatchupLimit),
	})

	behaviorRegistry := behaviors.MustDefaultRegistry()
	s.chunkManager = world.NewChunkManager(cfg, db, s.world, s, layer, cfg.Game.Region, objectFactory, behaviorRegistry, eb, logger)

	chunkSize := _const.ChunkSize * _const.CoordPerTile
	worldMinX := float64(cfg.Game.WorldMinXChunks * chunkSize)
	worldMaxX := float64((cfg.Game.WorldMinXChunks + cfg.Game.WorldWidthChunks) * chunkSize)
	worldMinY := float64(cfg.Game.WorldMinYChunks * chunkSize)
	worldMaxY := float64((cfg.Game.WorldMinYChunks + cfg.Game.WorldHeightChunks) * chunkSize)

	droppedItemPersister := world.NewDroppedItemPersisterDB(db, logger)
	// Create vision system first so it can be passed to other systems
	visionSystem := systems.NewVisionSystem(s.world, s.chunkManager, s.eventBus, enableVisionStats, logger)
	inventoryExecutor := inventory.NewInventoryExecutor(logger, entityIDManager, droppedItemPersister, s.chunkManager, visionSystem)

	networkCmdSystem := systems.NewNetworkCommandSystem(s.playerInbox, s.serverInbox, s, inventoryExecutor, s, visionSystem, cfg.Game.ChatLocalRadius, logger)
	openContainerService := NewOpenContainerService(s.world, s.eventBus, s, logger)
	craftingService := NewCraftingService(s.world, s.eventBus, inventoryExecutor, s, logger)
	contextActionService := NewContextActionService(
		s.world,
		s.eventBus,
		openContainerService,
		func(
			w *ecs.World,
			playerID types.EntityID,
			playerHandle types.Handle,
			itemKey string,
			count uint32,
			quality uint32,
		) contracts.GiveItemOutcome {
			if inventoryExecutor == nil {
				return contracts.GiveItemOutcome{Success: false, Message: "inventory executor unavailable"}
			}
			result := inventoryExecutor.GiveItem(w, playerID, playerHandle, itemKey, count, quality)
			if result == nil {
				return contracts.GiveItemOutcome{Success: false, Message: "nil give result"}
			}
			if result.Success && len(result.UpdatedContainers) > 0 {
				states := inventoryExecutor.ConvertContainersToStates(w, result.UpdatedContainers)
				updated := make([]*netproto.InventoryState, 0, len(states))
				for _, state := range states {
					updated = append(updated, systems.BuildInventoryStateProto(state))
				}
				if len(updated) > 0 {
					s.SendInventoryOpResult(playerID, &netproto.S2C_InventoryOpResult{
						OpId:    0,
						Success: true,
						Updated: updated,
					})
				}
			}
			if result.Success && result.DiscoveryLPGained > 0 {
				lp := result.DiscoveryLPGained
				s.SendExpGained(playerID, &netproto.S2C_ExpGained{
					EntityId: uint64(playerID),
					Lp:       &lp,
				})

				// Send Fx and Sound for LP gain
				fxKey := "exp_gain"

				var posX, posY float64
				ecs.WithComponent(w, playerHandle, func(t *components.Transform) {
					posX = t.X
					posY = t.Y
				})

				s.SendFx(playerID, &netproto.S2C_Fx{
					FxKey: fxKey,
					Position: &netproto.Vector2{
						X: int32(posX),
						Y: int32(posY),
					},
				})

				s.SendSound(playerID, &netproto.S2C_Sound{
					SoundKey:        fxKey,
					X:               posX,
					Y:               posY,
					MaxHearDistance: 80.0,
				})
			}
			return contracts.GiveItemOutcome{
				Success:      result.Success,
				AnyDropped:   false,
				PlacedInHand: result.PlacedInHand,
				GrantedCount: result.GrantedCount,
				Message:      result.Message,
			}
		},
		s,
		s,
		visionSystem,
		s.chunkManager,
		s.entityIDManager,
		behaviorRegistry,
		logger,
	)
	contextActionService.SetCraftingService(craftingService)
	contextActionService.SetSoundEventSender(s)
	s.craftingService = craftingService
	buildService := NewBuildService(
		s.world,
		s.eventBus,
		s.chunkManager,
		s.entityIDManager,
		behaviorRegistry,
		visionSystem,
		s,
		inventoryExecutor,
		networkCmdSystem,
		logger,
	)
	s.buildService = buildService
	networkCmdSystem.SetOpenContainerService(openContainerService)
	networkCmdSystem.SetContextActionService(contextActionService)
	networkCmdSystem.SetContextMenuSender(s)
	networkCmdSystem.SetCraftCommandService(craftingService)
	networkCmdSystem.SetBuildCommandService(buildService)
	networkCmdSystem.SetContextPendingTTL(cfg.Game.InteractionPendingTimeout)

	adminHandler := NewChatAdminCommandHandler(inventoryExecutor, s, s, s, entityIDManager, s.chunkManager, visionSystem, behaviorRegistry, s.eventBus, logger)
	s.adminHandler = adminHandler
	networkCmdSystem.SetAdminHandler(adminHandler)
	networkCmdSystem.SetInventorySnapshotSender(s)

	s.world.AddSystem(networkCmdSystem)
	s.world.AddSystem(systems.NewResetSystem(logger))
	s.world.AddSystem(systems.NewMovementSystem(s.world, s.chunkManager, logger))
	s.world.AddSystem(systems.NewCollisionSystem(s.world, s.chunkManager, logger, worldMinX, worldMaxX, worldMinY, worldMaxY, cfg.Game.WorldMarginTiles))
	s.world.AddSystem(systems.NewBuildPlacementSystem(s.world, buildService, logger))
	s.world.AddSystem(systems.NewTransformUpdateSystem(s.world, s.chunkManager, s.eventBus, logger))
	s.world.AddSystem(systems.NewLinkSystem(s.eventBus, logger))
	s.world.AddSystem(NewCyclicActionSystem(contextActionService, s, logger))
	s.world.AddSystem(visionSystem)
	s.world.AddSystem(systems.NewAutoInteractSystem(inventoryExecutor, s, visionSystem, logger))
	s.world.AddSystem(systems.NewBehaviorTickSystem(logger, systems.BehaviorTickSystemConfig{
		BudgetPerTick:    cfg.Game.BehaviorTickGlobalBudget,
		BehaviorRegistry: behaviorRegistry,
	}))
	s.world.AddSystem(systems.NewObjectBehaviorSystem(s.eventBus, logger, systems.ObjectBehaviorConfig{
		BudgetPerTick:       cfg.Game.ObjectBehaviorBudgetPerTick,
		EnableDebugFallback: strings.EqualFold(cfg.Game.Env, "dev"),
		BehaviorRegistry:    behaviorRegistry,
	}))
	s.world.AddSystem(systems.NewChunkSystem(s.chunkManager, logger))

	inventorySaver := inventory.NewInventorySaver(logger)
	s.characterSaver = systems.NewCharacterSaver(db, cfg.Game.SaveWorkers, inventorySaver, logger)
	s.world.AddSystem(systems.NewEntityStatsRegenSystem())
	s.world.AddSystem(systems.NewPlayerStatsPushSystem(s))
	s.world.AddSystem(systems.NewCharacterSaveSystem(s.characterSaver, cfg.Game.PlayerSaveInterval, logger))
	s.world.AddSystem(systems.NewExpireDetachedSystem(logger, s.characterSaver, s.onDetachedEntityExpired, s.onDetachedEntitiesExpired))
	s.world.AddSystem(systems.NewDropDecaySystem(droppedItemPersister, s.chunkManager, logger))

	return s
}

func (s *Shard) Layer() int {
	return s.layer
}

func (s *Shard) SetAdminTeleportExecutor(executor AdminTeleportExecutor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.adminHandler != nil {
		s.adminHandler.SetTeleportExecutor(executor)
	}
}

func (s *Shard) World() *ecs.World {
	return s.world
}

// WithWorldRead executes fn while holding shard read lock.
// Use this for safe ECS world reads from async goroutines.
func (s *Shard) WithWorldRead(fn func(w *ecs.World)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn(s.world)
}

func (s *Shard) ChunkManager() *world.ChunkManager {
	return s.chunkManager
}

func (s *Shard) Update(ts ecs.TimeState) {
	shardStart := time.Now()
	s.mu.Lock()
	lockWait := time.Since(shardStart)
	defer s.mu.Unlock()
	if s.state != ShardStateRunning {
		return
	}

	// Set TimeState resource before systems run (zero-alloc: mutate in place)
	*ecs.GetResource[ecs.TimeState](s.world) = ts

	// Add lock wait timing
	s.world.AddExternalTiming("ShardLockWait", lockWait)

	// ChunkManager does work via systems, not in Update()
	// Measure actual chunk work through ChunkSystem timing

	s.world.Update(ts.Delta)

	// Add full shard timing (including lock overhead) after world.Update
	shardDuration := time.Since(shardStart)
	s.world.AddExternalTiming("ShardTotal", shardDuration)
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
	return s.TrySpawnPlayerWithPolicy(worldX, worldY, character, setupFunc, SpawnCollisionPolicy{})
}

type SpawnCollisionPolicy struct {
	IgnoreObjectCollision bool
}

func (s *Shard) TrySpawnPlayerWithPolicy(
	worldX,
	worldY int,
	character repository.Character,
	setupFunc func(*ecs.World, types.Handle),
	policy SpawnCollisionPolicy,
) (bool, types.Handle) {
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

	if !policy.IgnoreObjectCollision {
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

// onDetachedEntityExpired is called when a detached entity's TTL expires.
// It handles per-entity spatial cleanup before despawn.
func (s *Shard) onDetachedEntityExpired(entityID types.EntityID, handle types.Handle) {
	_ = entityID

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
}

// onDetachedEntitiesExpired handles AOI cleanup in one batch after detached despawns.
func (s *Shard) onDetachedEntitiesExpired(entityIDs []types.EntityID) {
	s.chunkManager.UnregisterEntities(entityIDs)
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

// SendContainerClosed sends a container closed notification to a client
func (s *Shard) SendContainerClosed(entityID types.EntityID, ref *netproto.InventoryRef) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ContainerClosed{
			ContainerClosed: &netproto.S2C_ContainerClosed{
				Ref: ref,
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal container closed",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendContextMenu sends context actions for a target entity to a client.
func (s *Shard) SendContextMenu(entityID types.EntityID, menu *netproto.S2C_ContextMenu) {
	if menu == nil {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ContextMenu{
			ContextMenu: menu,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal context menu",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendMiniAlert sends a short center-screen alert to a client.
func (s *Shard) SendMiniAlert(entityID types.EntityID, alert *netproto.S2C_MiniAlert) {
	if alert == nil {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_MiniAlert{
			MiniAlert: alert,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal mini alert",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

func (s *Shard) SendCyclicActionProgress(entityID types.EntityID, progress *netproto.S2C_CyclicActionProgress) {
	if progress == nil {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_CyclicActionProgress{
			CyclicActionProgress: progress,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal cyclic action progress",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

func (s *Shard) SendCyclicActionFinished(entityID types.EntityID, finished *netproto.S2C_CyclicActionFinished) {
	if finished == nil {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_CyclicActionFinished{
			CyclicActionFinished: finished,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal cyclic action finished",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

func (s *Shard) SendSound(entityID types.EntityID, sound *netproto.S2C_Sound) {
	if sound == nil {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_Sound{
			Sound: sound,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal sound event",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendInventoryUpdate sends inventory updates to a client.
func (s *Shard) SendInventoryUpdate(entityID types.EntityID, states []*netproto.InventoryState) {
	if len(states) == 0 {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_InventoryUpdate{
			InventoryUpdate: &netproto.S2C_InventoryUpdate{
				Updated: states,
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal inventory update",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendCraftList sends visible craft recipes and live craftability flags to a client.
func (s *Shard) SendCraftList(entityID types.EntityID, list *netproto.S2C_CraftList) {
	if list == nil {
		return
	}
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_CraftList{
			CraftList: list,
		},
	}
	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal craft list",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}
	client.Send(data)
}

// SendBuildList sends visible build recipes to a client.
func (s *Shard) SendBuildList(entityID types.EntityID, list *netproto.S2C_BuildList) {
	if list == nil {
		return
	}
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_BuildList{
			BuildList: list,
		},
	}
	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal build list",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}
	client.Send(data)
}

func (s *Shard) SendBuildState(entityID types.EntityID, state *netproto.S2C_BuildState) {
	if state == nil {
		return
	}
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_BuildState{
			BuildState: state,
		},
	}
	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal build state",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}
	client.Send(data)
}

func (s *Shard) SendBuildStateClosed(entityID types.EntityID, msg *netproto.S2C_BuildStateClosed) {
	if msg == nil {
		return
	}
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_BuildStateClosed{
			BuildStateClosed: msg,
		},
	}
	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal build state closed",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}
	client.Send(data)
}

// SendFx sends a visual effect trigger to a client.
func (s *Shard) SendFx(entityID types.EntityID, fx *netproto.S2C_Fx) {
	if fx == nil {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_Fx{
			Fx: fx,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal fx",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendExpGained sends experience gain delta to a client.
func (s *Shard) SendExpGained(entityID types.EntityID, gained *netproto.S2C_ExpGained) {
	if gained == nil {
		return
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ExpGained{
			ExpGained: gained,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal exp gained",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendInventorySnapshots sends full inventory snapshots to a client (called from ECS tick thread)
func (s *Shard) SendInventorySnapshots(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	if s.snapshotSender != nil {
		s.snapshotSender.SendInventorySnapshots(w, client, entityID, handle)
	}
}

// SendCharacterProfileSnapshot sends player-profile data after player enter world.
// Payload includes full profile snapshot (`S2C_CharacterProfile`).
func (s *Shard) SendCharacterProfileSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()

	if !ok || client == nil {
		return
	}

	values := characterattrs.Default()
	exp := components.CharacterExperience{}
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
		values = characterattrs.Normalize(profile.Attributes)
		exp = profile.Experience
	} else {
		s.logger.Warn("Character has no CharacterProfile component",
			zap.Int64("entity_id", int64(entityID)))
	}

	entries := make([]*netproto.CharacterAttributeEntry, 0, len(characterattrs.RequiredNames()))
	for _, name := range characterattrs.RequiredNames() {
		entries = append(entries, &netproto.CharacterAttributeEntry{
			Key:   characterAttributeNameToProtoKey(name),
			Value: int32(characterattrs.Get(values, name)),
		})
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_CharacterProfile{
			CharacterProfile: &netproto.S2C_CharacterProfile{
				Attributes: entries,
				Exp: &netproto.CharacterExperience{
					Lp:       exp.LP,
					Nature:   exp.Nature,
					Industry: exp.Industry,
					Combat:   exp.Combat,
				},
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal character profile snapshot",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return
	}

	client.Send(data)
}

// SendPlayerStatsSnapshot sends the initial player stats snapshot after enter-world/reattach.
func (s *Shard) SendPlayerStatsSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	s.sendPlayerStats(w, entityID, handle, true)
}

// SendPlayerStatsDeltaIfChanged sends player stats only when rounded network values changed.
func (s *Shard) SendPlayerStatsDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool {
	return s.sendPlayerStats(w, entityID, handle, false)
}

// SendMovementModeSnapshot sends the initial movement mode snapshot after enter-world/reattach.
func (s *Shard) SendMovementModeSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	s.sendMovementMode(w, entityID, handle, true)
}

// SendCraftListSnapshot sends a fresh craft list snapshot for the player.
func (s *Shard) SendCraftListSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	if s.craftingService == nil {
		return
	}
	s.craftingService.SendCraftListSnapshot(w, entityID, handle)
}

// SendBuildListSnapshot sends a fresh build list snapshot for the player.
func (s *Shard) SendBuildListSnapshot(w *ecs.World, entityID types.EntityID, handle types.Handle) {
	if s == nil {
		return
	}
	s.sendBuildListSnapshot(w, entityID, handle)
}

// SendMovementModeDeltaIfChanged sends movement mode only when it differs from last sent.
func (s *Shard) SendMovementModeDeltaIfChanged(w *ecs.World, entityID types.EntityID, handle types.Handle) bool {
	return s.sendMovementMode(w, entityID, handle, false)
}

func (s *Shard) sendPlayerStats(w *ecs.World, entityID types.EntityID, handle types.Handle, force bool) bool {
	if w == nil || entityID == 0 || handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}

	stats, hasStats := ecs.GetComponent[components.EntityStats](w, handle)
	if !hasStats {
		return false
	}

	snapshot := ecs.PlayerStatsNetSnapshot{
		Stamina: entitystats.RoundToUint32(stats.Stamina),
		Energy:  entitystats.RoundToUint32(stats.Energy),
	}
	attributes := characterattrs.Default()
	if profile, hasProfile := ecs.GetComponent[components.CharacterProfile](w, handle); hasProfile {
		attributes = characterattrs.Normalize(profile.Attributes)
	}
	snapshot.StaminaMax = entitystats.RoundToUint32(entitystats.MaxStaminaFromAttributes(attributes))
	snapshot.EnergyMax = entitystats.RoundToUint32(_const.EnergyMax)

	updateState := ecs.GetResource[ecs.EntityStatsUpdateState](w)
	if !updateState.ShouldSendPlayerStats(entityID, snapshot, force) {
		return false
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		ecs.ForgetPlayerStatsState(w, entityID)
		return false
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_PlayerStats{
			PlayerStats: &netproto.S2C_PlayerStats{
				Stamina:    snapshot.Stamina,
				Energy:     snapshot.Energy,
				StaminaMax: snapshot.StaminaMax,
				EnergyMax:  snapshot.EnergyMax,
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal player stats snapshot",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return false
	}

	client.Send(data)
	updateState.MarkPlayerStatsSent(entityID, snapshot, ecs.GetResource[ecs.TimeState](w).UnixMs)
	return true
}

func (s *Shard) sendMovementMode(w *ecs.World, entityID types.EntityID, handle types.Handle, force bool) bool {
	if w == nil || entityID == 0 || handle == types.InvalidHandle || !w.Alive(handle) {
		return false
	}

	movement, hasMovement := ecs.GetComponent[components.Movement](w, handle)
	if !hasMovement {
		return false
	}

	updateState := ecs.GetResource[ecs.EntityStatsUpdateState](w)
	if !updateState.ShouldSendMovementMode(entityID, movement.Mode, force) {
		return false
	}

	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		ecs.ForgetPlayerStatsState(w, entityID)
		return false
	}

	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_MovementMode{
			MovementMode: &netproto.S2C_MovementMode{
				EntityId:     uint64(entityID),
				MovementMode: moveModeToProto(movement.Mode),
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		s.logger.Error("Failed to marshal movement mode snapshot",
			zap.Int64("entity_id", int64(entityID)),
			zap.Error(err))
		return false
	}

	client.Send(data)
	updateState.MarkMovementModeSent(entityID, movement.Mode)
	return true
}

func moveModeToProto(mode _const.MoveMode) netproto.MovementMode {
	switch mode {
	case _const.Crawl:
		return netproto.MovementMode_MOVE_MODE_CRAWL
	case _const.Walk:
		return netproto.MovementMode_MOVE_MODE_WALK
	case _const.Run:
		return netproto.MovementMode_MOVE_MODE_RUN
	case _const.FastRun:
		return netproto.MovementMode_MOVE_MODE_FAST_RUN
	case _const.Swim:
		return netproto.MovementMode_MOVE_MODE_SWIM
	default:
		return netproto.MovementMode_MOVE_MODE_WALK
	}
}

func characterAttributeNameToProtoKey(name characterattrs.Name) netproto.CharacterAttributeKey {
	switch name {
	case characterattrs.INT:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_INT
	case characterattrs.STR:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_STR
	case characterattrs.PER:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_PER
	case characterattrs.PSY:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_PSY
	case characterattrs.AGI:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_AGI
	case characterattrs.CON:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_CON
	case characterattrs.CHA:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_CHA
	case characterattrs.DEX:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_DEX
	case characterattrs.WIL:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_WIL
	default:
		return netproto.CharacterAttributeKey_CHARACTER_ATTRIBUTE_KEY_UNSPECIFIED
	}
}

// SendError sends S2C_Error to a single entity.
func (s *Shard) SendError(entityID types.EntityID, errorCode netproto.ErrorCode, message string) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}
	client.SendError(errorCode, message)
}

// SendWarning sends S2C_Warning to a single entity.
func (s *Shard) SendWarning(entityID types.EntityID, warningCode netproto.WarningCode, message string) {
	s.ClientsMu.RLock()
	client, ok := s.Clients[entityID]
	s.ClientsMu.RUnlock()
	if !ok || client == nil {
		return
	}
	client.SendWarning(warningCode, message)
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
