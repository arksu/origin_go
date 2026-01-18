package game

import (
	"context"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence"
	"origin/internal/types"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"origin/internal/config"
)

type GameState int32

const (
	GameStateStarting GameState = iota
	GameStateRunning
	GameStateStopping
	GameStateStopped
)

func (gs GameState) String() string {
	switch gs {
	case GameStateStarting:
		return "Starting"
	case GameStateRunning:
		return "Running"
	case GameStateStopping:
		return "Stopping"
	case GameStateStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}

type tickStats struct {
	durationSum time.Duration
	count       uint64
	minDuration time.Duration
	maxDuration time.Duration
	lastLog     time.Time
}

type GameStats struct {
	ConnectedClients int
	TotalPlayers     int
	CurrentTick      uint64
	TickRate         int
	AvgTickDuration  time.Duration
}

type Game struct {
	cfg    *config.Config
	db     *persistence.Postgres
	logger *zap.Logger

	objectFactory   *ObjectFactory
	shardManager    *ShardManager
	entityIDManager *EntityIDManager
	networkServer   *network.Server

	tickRate    int
	tickPeriod  time.Duration
	currentTick uint64
	state       atomic.Int32 // GameState
	tickStats   tickStats

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewGame(cfg *config.Config, db *persistence.Postgres, objectFactory *ObjectFactory, logger *zap.Logger) *Game {
	ctx, cancel := context.WithCancel(context.Background())

	g := &Game{
		cfg:           cfg,
		db:            db,
		objectFactory: objectFactory,
		logger:        logger,
		tickRate:      cfg.Game.TickRate,
		tickPeriod:    time.Second / time.Duration(cfg.Game.TickRate),
		ctx:           ctx,
		cancel:        cancel,
		tickStats: tickStats{
			lastLog:     time.Now(),
			minDuration: time.Hour, // Initialize with large value
		},
	}
	g.state.Store(int32(GameStateStarting))

	g.entityIDManager = NewEntityIDManager(cfg, db, logger)
	g.shardManager = NewShardManager(cfg, db, g.entityIDManager, objectFactory, logger)
	g.networkServer = network.NewServer(&cfg.Network, &cfg.Game, logger)

	g.setupNetworkHandlers()
	g.setupEventHandlers()

	g.resetOnlinePlayers()

	return g
}

func (g *Game) setState(state GameState) {
	oldState := g.state.Load()
	g.state.Store(int32(state))
	g.logger.Info("Game state changed", zap.String("old_state", GameState(oldState).String()), zap.String("new_state", state.String()))
}

func (g *Game) getState() GameState {
	return GameState(g.state.Load())
}

func (g *Game) setupNetworkHandlers() {
	g.networkServer.SetOnConnect(func(c *network.Client) {
		if g.getState() == GameStateStopping {
			c.Close()
			return
		}
		g.logger.Info("Client connected", zap.Uint64("client_id", c.ID))
	})

	g.networkServer.SetOnDisconnect(func(c *network.Client) {
		g.handleDisconnect(c)
	})

	g.networkServer.SetOnMessage(func(c *network.Client, data []byte) {
		if g.getState() == GameStateRunning {
			g.handlePacket(c, data)
		}
	})
}

func (g *Game) setupEventHandlers() {
	// Subscribe to object move events from systems
	eventBus := g.shardManager.EventBus()
	eventBus.SubscribeAsync(ecs.TopicGameplayMovementMove, eventbus.PriorityMedium, func(ctx context.Context, e eventbus.Event) error {
		if objectMove, ok := e.(*ecs.ObjectMoveEvent); ok {
			g.handleObjectMove(objectMove)
		}
		return nil
	})
}

func (g *Game) handleObjectMove(event *ecs.ObjectMoveEvent) {
	// Type assert the movement data back to the expected proto type
	movement, ok := event.Movement.(*netproto.EntityMovement)
	if !ok {
		return
	}

	// Create S2C_ObjectMove packet
	objectMoveMsg := &netproto.S2C_ObjectMove{
		EntityId: uint64(event.EntityID),
		Movement: movement,
	}

	// Create server message
	serverMsg := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ObjectMove{
			ObjectMove: objectMoveMsg,
		},
	}

	// BroadcastToAllClients to all connected clients
	data, err := proto.Marshal(serverMsg)
	if err != nil {
		g.logger.Error("Failed to marshal S2C_ObjectMove", zap.Error(err))
		return
	}

	if event.EntityID == 3519523 {
		g.networkServer.BroadcastToAllClients(data)
	}
	//g.logger.Debug("Broadcasted S2C_ObjectMove", zap.Uint64("entity_id", uint64(event.EntityID)))
}

func (g *Game) handlePacket(c *network.Client, data []byte) {
	msg := &netproto.ClientMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		g.logger.Warn("Failed to unmarshal packet", zap.Uint64("client_id", c.ID), zap.Error(err))
		return
	}
	//g.logger.Debug("Received packet", zap.Uint64("client_id", c.ID), zap.Any("payload", msg.Payload))

	switch payload := msg.Payload.(type) {
	case *netproto.ClientMessage_Ping:
		g.handlePing(c, msg.Sequence, payload.Ping)
	case *netproto.ClientMessage_Auth:
		g.handleAuth(c, msg.Sequence, payload.Auth)
	case *netproto.ClientMessage_PlayerAction:
		g.handlePlayerAction(c, msg.Sequence, payload.PlayerAction)
	default:
		g.logger.Warn("Unknown packet type", zap.Uint64("client_id", c.ID), zap.Any("payload", msg.Payload))
	}
}

func (g *Game) handlePing(c *network.Client, sequence uint32, ping *netproto.C2S_Ping) {
	pong := &netproto.S2C_Pong{
		ClientTimeMs: ping.ClientTimeMs,
		ServerTimeMs: time.Now().UnixMilli(),
	}

	response := &netproto.ServerMessage{
		Sequence: sequence,
		Payload: &netproto.ServerMessage_Pong{
			Pong: pong,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		g.logger.Error("Failed to marshal pong", zap.Uint64("client_id", c.ID), zap.Error(err))
		return
	}

	c.Send(data)
}

func (g *Game) sendError(c *network.Client, errorMsg string) {
	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_Error{
			Error: &netproto.S2C_Error{
				Code:    netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST,
				Message: errorMsg,
			},
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		g.logger.Error("Failed to marshal error", zap.Uint64("client_id", c.ID), zap.Error(err))
		return
	}

	c.Send(data)
}

func (g *Game) handlePlayerAction(c *network.Client, sequence uint32, action *netproto.C2S_PlayerAction) {
	if c.CharacterID == 0 {
		g.logger.Warn("Player action from unauthenticated client", zap.Uint64("client_id", c.ID))
		g.sendError(c, "Not authenticated")
		return
	}

	// Get the shard for this player
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		g.logger.Error("Shard not found for player action",
			zap.Uint64("client_id", c.ID),
			zap.Int("layer", c.Layer))
		g.sendError(c, "Invalid shard")
		return
	}

	// Handle different action types
	switch act := action.Action.(type) {
	case *netproto.C2S_PlayerAction_MoveTo:
		g.handleMoveToAction(c, shard, act.MoveTo)
	case *netproto.C2S_PlayerAction_MoveToEntity:
		g.handleMoveToEntityAction(c, shard, act.MoveToEntity)
	case *netproto.C2S_PlayerAction_Interact:
		g.handleInteractAction(c, shard, act.Interact)
	default:
		g.logger.Warn("Unknown player action type",
			zap.Uint64("client_id", c.ID),
			zap.Any("action_type", action.Action))
	}
}

func (g *Game) handleMoveToAction(c *network.Client, shard *Shard, moveTo *netproto.MoveTo) {
	//g.logger.Debug("MoveTo action",
	//	zap.Uint64("client_id", c.ID),
	//	zap.Int32("target_x", moveTo.X),
	//	zap.Int32("target_y", moveTo.Y))

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// TODO: Validate target position (bounds, walkable, etc.)

	// Find player entity handle by EntityID (O(1) lookup)
	playerHandle := shard.world.GetHandleByEntityID(c.CharacterID)
	if playerHandle == types.InvalidHandle {
		g.logger.Error("Player entity not found",
			zap.Uint64("client_id", c.ID),
			zap.Uint64("entity_id", uint64(c.CharacterID)))
		return
	}

	// Get movement component
	mov, ok := ecs.GetComponent[components.Movement](shard.world, playerHandle)
	if !ok {
		g.logger.Error("Movement component not found",
			zap.Uint64("client_id", c.ID),
			zap.Uint64("entity_id", uint64(c.CharacterID)))
		return
	}

	// Only allow movement if not stunned
	if mov.State == components.StateStunned {
		g.logger.Debug("Cannot move while stunned",
			zap.Uint64("client_id", c.ID),
			zap.Uint64("entity_id", uint64(c.CharacterID)))
		return
	}

	// Set movement target using helper method
	ecs.WithComponent(shard.world, playerHandle, func(mov *components.Movement) {
		mov.SetTargetPoint(int(moveTo.X), int(moveTo.Y))
	})

	//g.logger.Debug("Set movement target",
	//	zap.Uint64("client_id", c.ID),
	//	zap.Int32("target_x", moveTo.X),
	//	zap.Int32("target_y", moveTo.Y))
}

func (g *Game) handleMoveToEntityAction(c *network.Client, shard *Shard, moveToEntity *netproto.MoveToEntity) {
	g.logger.Debug("MoveToEntity action",
		zap.Uint64("client_id", c.ID),
		zap.Uint64("target_entity_id", moveToEntity.EntityId),
		zap.Bool("auto_interact", moveToEntity.AutoInteract))

	// TODO: Implement entity targeting and pathfinding
	// 1. Validate target entity exists and is reachable
	// 2. Set movement target to entity position
	// 3. If auto_interact, queue interaction when reached
}

func (g *Game) handleInteractAction(c *network.Client, shard *Shard, interact *netproto.Interact) {
	g.logger.Debug("Interact action",
		zap.Uint64("client_id", c.ID),
		zap.Uint64("target_entity_id", interact.EntityId),
		zap.Int32("interaction_type", int32(interact.Type)))

	// TODO: Implement interaction system
	// 1. Validate target entity exists and is in range
	// 2. Check if interaction is valid for entity type
	// 3. Execute interaction (gather, open container, use, pickup, etc.)
}

func (g *Game) handleDisconnect(c *network.Client) {
	g.logger.Info("Client disconnected", zap.Uint64("client_id", c.ID))

	if c.CharacterID != 0 {
		if g.getState() == GameStateRunning {
			if shard := g.shardManager.GetShard(c.Layer); shard != nil {
				playerEntityID := c.CharacterID
				shard.mu.Lock()
				// Get the player's handle and despawn the entity
				playerHandle := shard.world.GetHandleByEntityID(playerEntityID)
				if playerHandle != types.InvalidHandle {
					// Remove from chunk spatial index before despawning
					if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](shard.world, playerHandle); hasChunkRef {
						if transform, hasTransform := ecs.GetComponent[components.Transform](shard.world, playerHandle); hasTransform {
							if chunk := shard.chunkManager.GetChunk(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}); chunk != nil {
								// Check if entity is static or dynamic and remove from appropriate spatial index
								if entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](shard.world, playerHandle); hasEntityInfo && entityInfo.IsStatic {
									chunk.Spatial().RemoveStatic(playerHandle, int(transform.X), int(transform.Y))
								} else {
									chunk.Spatial().RemoveDynamic(playerHandle, int(transform.X), int(transform.Y))
								}
							}
						}
					}
					shard.world.Despawn(playerHandle)
				}
				shard.UnregisterEntityAOI(playerEntityID)
				shard.mu.Unlock()
				g.logger.Debug("Unregistered entity AOI",
					zap.Uint64("client_id", c.ID),
					zap.Int64("character_id", int64(c.CharacterID)),
					zap.Int("layer", c.Layer),
				)
			}

			if err := g.db.Queries().SetCharacterOffline(g.ctx, int64(c.CharacterID)); err != nil {
				g.logger.Error("Failed to set character offline", zap.Uint64("client_id", c.ID), zap.Uint64("character_id", uint64(c.CharacterID)), zap.Error(err))
			} else {
				g.logger.Info("Character set offline", zap.Uint64("client_id", c.ID), zap.Uint64("character_id", uint64(c.CharacterID)))
			}
		}
	}
}

func (g *Game) resetOnlinePlayers() {
	if err := g.db.Queries().ResetOnlinePlayers(g.ctx, g.cfg.Game.Region); err != nil {
		g.logger.Error("Failed to reset online players", zap.Int("region", g.cfg.Game.Region), zap.Error(err))
	} else {
		g.logger.Info("Reset online players", zap.Int("region", g.cfg.Game.Region))
	}
}

func (g *Game) StartGameLoop() {
	g.setState(GameStateRunning)
	g.wg.Add(1)
	go g.gameLoop()

	g.logger.Info("Game loop started", zap.Int("tick_rate_hz", g.tickRate))
}

func (g *Game) gameLoop() {
	defer g.wg.Done()

	ticker := time.NewTicker(g.tickPeriod)
	defer ticker.Stop()

	lastTime := time.Now()
	maxDt := g.tickPeriod.Seconds() * 2 // Cap dt to 2x the tick period

	for {
		select {
		case <-g.ctx.Done():
			return
		case now := <-ticker.C:
			if g.getState() != GameStateRunning {
				return
			}
			dt := now.Sub(lastTime).Seconds()
			if dt > maxDt {
				dt = maxDt
			}
			lastTime = now

			g.currentTick++
			g.update(dt)
		}
	}
}

func (g *Game) update(dt float64) {
	start := time.Now()

	g.shardManager.Update(dt)

	duration := time.Since(start)

	// Collect statistics
	g.tickStats.durationSum += duration
	g.tickStats.count++

	// Log every 5 seconds
	if time.Since(g.tickStats.lastLog) >= 5*time.Second {
		if g.tickStats.count > 0 {
			avgDuration := g.tickStats.durationSum / time.Duration(g.tickStats.count)
			g.logger.Info("Game tick statistics (5s)",
				zap.Uint64("ticks", g.tickStats.count),
				zap.Duration("avg", avgDuration),
				zap.Duration("min", g.tickStats.minDuration),
				zap.Duration("max", g.tickStats.maxDuration),
			)
		}

		// Reset statistics
		g.tickStats = tickStats{
			lastLog:     time.Now(),
			minDuration: time.Hour, // Initialize with large value
		}
	}

	// Update min/max for current period
	if duration < g.tickStats.minDuration {
		g.tickStats.minDuration = duration
	}
	if duration > g.tickStats.maxDuration {
		g.tickStats.maxDuration = duration
	}
}

func (g *Game) Stop() {
	g.logger.Info("Stopping game...")
	g.setState(GameStateStopping)

	g.resetOnlinePlayers()
	g.cancel()

	g.logger.Info("Stopping networkServer")
	g.networkServer.Stop()
	g.logger.Info("Stopping shardManager")
	g.shardManager.Stop()
	g.logger.Info("Stopping entityIDManager")
	g.entityIDManager.Stop()

	// Wait for all goroutines to finish
	g.wg.Wait()

	g.setState(GameStateStopped)
	g.logger.Info("Game stopped")
}

func (g *Game) ShardManager() *ShardManager {
	return g.shardManager
}

func (g *Game) EntityIDManager() *EntityIDManager {
	return g.entityIDManager
}

func (g *Game) NetworkServer() *network.Server {
	return g.networkServer
}

func (g *Game) CurrentTick() uint64 {
	return g.currentTick
}

func (g *Game) State() GameState {
	return g.getState()
}

func (g *Game) Stats() GameStats {
	connectedClients := g.networkServer.ClientCount()

	totalPlayers := 0
	for _, shard := range g.shardManager.shards {
		shard.mu.RLock()
		totalPlayers += shard.world.EntityCount()
		shard.mu.RUnlock()
	}

	avgTickDuration := time.Duration(0)
	if g.tickStats.count > 0 {
		avgTickDuration = g.tickStats.durationSum / time.Duration(g.tickStats.count)
	}

	return GameStats{
		ConnectedClients: connectedClients,
		TotalPlayers:     totalPlayers,
		CurrentTick:      g.currentTick,
		TickRate:         g.tickRate,
		AvgTickDuration:  avgTickDuration,
	}
}
