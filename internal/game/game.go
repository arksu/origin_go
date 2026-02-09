package game

import (
	"context"
	"errors"
	"fmt"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence"
	"origin/internal/timeutil"
	"origin/internal/types"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"origin/internal/config"
	"origin/internal/game/inventory"
	"origin/internal/game/world"
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
	systemStats map[string]ecs.SystemTimingStat // per-system accumulated stats
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

	objectFactory   *world.ObjectFactory
	shardManager    *ShardManager
	entityIDManager *EntityIDManager
	networkServer   *network.Server
	inventoryLoader *inventory.InventoryLoader

	clock       timeutil.Clock
	startTime   time.Time
	tickRate    int
	tickPeriod  time.Duration
	currentTick uint64
	state       atomic.Int32 // GameState
	tickStats   tickStats

	serverTimeManager *timeutil.ServerTimeManager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewGame(cfg *config.Config, db *persistence.Postgres, objectFactory *world.ObjectFactory, inventoryLoader *inventory.InventoryLoader, inventorySnapshotSender *inventory.SnapshotSender, logger *zap.Logger) *Game {
	ctx, cancel := context.WithCancel(context.Background())

	serverTimeManager := timeutil.NewServerTimeManager(db, logger)
	startWall := serverTimeManager.LoadServerTime()

	clk := timeutil.NewMonotonicClockAt(startWall)

	g := &Game{
		cfg:               cfg,
		db:                db,
		objectFactory:     objectFactory,
		inventoryLoader:   inventoryLoader,
		logger:            logger,
		clock:             clk,
		startTime:         clk.GameNow(),
		tickRate:          cfg.Game.TickRate,
		tickPeriod:        time.Second / time.Duration(cfg.Game.TickRate),
		ctx:               ctx,
		cancel:            cancel,
		serverTimeManager: serverTimeManager,
		tickStats: tickStats{
			lastLog:     time.Now(),
			minDuration: time.Hour,
			systemStats: make(map[string]ecs.SystemTimingStat),
		},
	}
	g.state.Store(int32(GameStateStarting))

	g.entityIDManager = NewEntityIDManager(cfg, db, logger)
	g.shardManager = NewShardManager(cfg, db, g.entityIDManager, objectFactory, inventorySnapshotSender, logger)
	g.networkServer = network.NewServer(&cfg.Network, &cfg.Game, logger)

	g.setupNetworkHandlers()

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

func (g *Game) handlePacket(c *network.Client, data []byte) {
	msg := &netproto.ClientMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		g.logger.Warn("Failed to unmarshal packet", zap.Uint64("client_id", c.ID), zap.Error(err))
		return
	}

	switch payload := msg.Payload.(type) {
	case *netproto.ClientMessage_Ping:
		g.handlePing(c, msg.Sequence, payload.Ping)
	case *netproto.ClientMessage_Auth:
		g.handleAuth(c, msg.Sequence, payload.Auth)
	case *netproto.ClientMessage_PlayerAction:
		g.handlePlayerAction(c, msg.Sequence, payload.PlayerAction)
	case *netproto.ClientMessage_Chat:
		g.handleChatMessage(c, msg.Sequence, payload.Chat)
	case *netproto.ClientMessage_InventoryOp:
		g.handleInventoryOp(c, msg.Sequence, payload.InventoryOp)
	case *netproto.ClientMessage_OpenContainer:
		g.handleOpenContainer(c, msg.Sequence, payload.OpenContainer)
	case *netproto.ClientMessage_CloseContainer:
		g.handleCloseContainer(c, msg.Sequence, payload.CloseContainer)
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

func (g *Game) handlePlayerAction(c *network.Client, sequence uint32, action *netproto.C2S_PlayerAction) {
	if c.CharacterID == 0 {
		g.logger.Warn("Player action from unauthenticated client", zap.Uint64("client_id", c.ID))
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}

	// Get the shard for this player
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		g.logger.Error("Shard not found for player action",
			zap.Uint64("client_id", c.ID),
			zap.Int("layer", c.Layer))
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}

	// Determine command type and payload
	var cmdType network.CommandType
	var payload any

	switch act := action.Action.(type) {
	case *netproto.C2S_PlayerAction_MoveTo:
		cmdType = network.CmdMoveTo
		payload = act.MoveTo
	case *netproto.C2S_PlayerAction_MoveToEntity:
		cmdType = network.CmdMoveToEntity
		payload = act.MoveToEntity
	case *netproto.C2S_PlayerAction_Interact:
		cmdType = network.CmdInteract
		payload = act.Interact
	default:
		g.logger.Warn("Unknown player action type",
			zap.Uint64("client_id", c.ID),
			zap.Any("action_type", action.Action))
		return
	}

	// Create command and enqueue
	cmd := &network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence), // Use sequence from ClientMessage as CommandID
		CommandType: cmdType,
		Payload:     payload,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	}

	if err := shard.PlayerInbox().Enqueue(cmd); err != nil {
		var overflowError network.OverflowError
		var rateLimitError network.RateLimitError
		var duplicateCommandError network.DuplicateCommandError
		switch {
		case errors.As(err, &overflowError):
			g.logger.Warn("Command queue overflow",
				zap.Uint64("client_id", c.ID))
			c.SendWarning(netproto.WarningCode_WARN_INPUT_QUEUE_OVERFLOW, "Command queue overflow")
		case errors.As(err, &rateLimitError):
			g.logger.Warn("Rate limit exceeded",
				zap.Uint64("client_id", c.ID))
			c.SendError(netproto.ErrorCode_ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED, "Rate limit exceeded")
			c.Close()
		case errors.As(err, &duplicateCommandError):
			// Ignore silently - already processed
		default:
			g.logger.Error("Failed to enqueue command",
				zap.Uint64("client_id", c.ID),
				zap.Error(err))
		}
	}
}

func (g *Game) handleChatMessage(c *network.Client, sequence uint32, chat *netproto.C2S_ChatMessage) {
	if c.CharacterID == 0 {
		g.logger.Warn("Chat message from unauthenticated client", zap.Uint64("client_id", c.ID))
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}

	// Validate channel (only LOCAL supported for now)
	if chat.Channel != netproto.ChatChannel_CHAT_CHANNEL_LOCAL {
		g.logger.Debug("Unsupported chat channel",
			zap.Uint64("client_id", c.ID),
			zap.String("channel", chat.Channel.String()))
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Only local chat is supported")
		return
	}

	// Validate text
	text := strings.TrimSpace(chat.Text)
	if text == "" {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Empty message")
		return
	}

	if len(text) > g.cfg.Game.ChatMaxLen {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Message too long")
		return
	}

	// Normalize text: strip CR, collapse multiple newlines
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimSpace(text)

	// Get shard
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		g.logger.Error("Shard not found for chat message",
			zap.Uint64("client_id", c.ID),
			zap.Int("layer", c.Layer))
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}

	// Enqueue chat command to player inbox
	cmd := &network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdChat,
		Payload: &network.ChatCommandPayload{
			Channel: netproto.ChatChannel_CHAT_CHANNEL_LOCAL,
			Text:    text,
		},
		ReceivedAt: time.Now(),
		Layer:      c.Layer,
	}

	if err := shard.PlayerInbox().Enqueue(cmd); err != nil {
		switch err.(type) {
		case network.OverflowError:
			c.SendWarning(netproto.WarningCode_WARN_INPUT_QUEUE_OVERFLOW, "Command queue overflow")
		case network.RateLimitError:
			c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Rate limit exceeded")
		default:
			g.logger.Error("Failed to enqueue chat command",
				zap.Uint64("client_id", c.ID),
				zap.Error(err))
			c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Failed to process chat message")
		}
		return
	}

	g.logger.Debug("Chat command enqueued",
		zap.Uint64("client_id", c.ID),
		zap.Int64("character_id", int64(c.CharacterID)),
		zap.Int("text_len", len(text)))
}

func (g *Game) handleInventoryOp(c *network.Client, sequence uint32, inventoryOp *netproto.C2S_InventoryOp) {
	if c.CharacterID == 0 {
		g.logger.Warn("Inventory op from unauthenticated client", zap.Uint64("client_id", c.ID))
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}

	if inventoryOp == nil || inventoryOp.Op == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid inventory operation")
		return
	}

	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		g.logger.Error("Shard not found for inventory op",
			zap.Uint64("client_id", c.ID),
			zap.Int("layer", c.Layer))
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}

	g.logger.Debug("handleInventoryOp", zap.Uint64("client_id", c.ID), zap.Any("inventory_op", inventoryOp))

	cmd := &network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdInventoryOp,
		Payload:     inventoryOp.Op,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	}

	if err := shard.PlayerInbox().Enqueue(cmd); err != nil {
		var overflowError network.OverflowError
		var rateLimitError network.RateLimitError
		var duplicateCommandError network.DuplicateCommandError
		switch {
		case errors.As(err, &overflowError):
			g.logger.Warn("Command queue overflow for inventory op",
				zap.Uint64("client_id", c.ID))
			c.SendWarning(netproto.WarningCode_WARN_INPUT_QUEUE_OVERFLOW, "Command queue overflow")
		case errors.As(err, &rateLimitError):
			g.logger.Warn("Rate limit exceeded for inventory op",
				zap.Uint64("client_id", c.ID))
			c.SendError(netproto.ErrorCode_ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED, "Rate limit exceeded")
			c.Close()
		case errors.As(err, &duplicateCommandError):
			// Ignore silently - already processed
		default:
			g.logger.Error("Failed to enqueue inventory op command",
				zap.Uint64("client_id", c.ID),
				zap.Error(err))
		}
	}
}

func (g *Game) handleOpenContainer(c *network.Client, sequence uint32, msg *netproto.C2S_OpenContainer) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || msg.Ref == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid open container request")
		return
	}

	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}

	cmd := &network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdOpenContainer,
		Payload:     msg.Ref,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	}
	_ = shard.PlayerInbox().Enqueue(cmd)
}

func (g *Game) handleCloseContainer(c *network.Client, sequence uint32, msg *netproto.C2S_CloseContainer) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || msg.Ref == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid close container request")
		return
	}

	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}

	cmd := &network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdCloseContainer,
		Payload:     msg.Ref,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	}
	_ = shard.PlayerInbox().Enqueue(cmd)
}

func (g *Game) handleDisconnect(c *network.Client) {
	g.logger.Info("Client disconnected", zap.Uint64("client_id", c.ID))

	if c.CharacterID != 0 {
		if g.getState() == GameStateRunning {
			if shard := g.shardManager.GetShard(c.Layer); shard != nil {
				playerEntityID := c.CharacterID

				// Remove client from command queue to clean up rate limiting state
				shard.PlayerInbox().RemoveClient(c.ID)

				// Reset client state and remove from shard's client map
				c.InWorld.Store(false)
				c.StreamEpoch.Store(0)
				shard.ClientsMu.Lock()
				delete(shard.Clients, playerEntityID)
				shard.ClientsMu.Unlock()

				disconnectDelay := g.cfg.Game.DisconnectDelay

				shard.mu.Lock()
				playerHandle := shard.world.GetHandleByEntityID(playerEntityID)

				if disconnectDelay > 0 && playerHandle != types.InvalidHandle {
					// Detached mode: keep entity in world for DisconnectDelay seconds
					gameNow := g.clock.GameNow()
					expirationTime := gameNow.Add(time.Duration(disconnectDelay) * time.Second)
					ecs.GetResource[ecs.DetachedEntities](shard.world).AddDetachedEntity(playerEntityID, playerHandle, expirationTime, gameNow)

					// Stop movement for detached entity
					ecs.MutateComponent[components.Movement](shard.world, playerHandle, func(m *components.Movement) bool {
						m.ClearTarget()
						return true
					})

					shard.mu.Unlock()

					g.logger.Info("Player detached, entity remains in world",
						zap.Uint64("client_id", c.ID),
						zap.Int64("character_id", int64(c.CharacterID)),
						zap.Int("layer", c.Layer),
						zap.Int("disconnect_delay_sec", disconnectDelay),
						zap.Time("expiration_time", expirationTime),
					)
				} else {
					// Immediate despawn (DisconnectDelay=0 or entity not found)
					if playerHandle != types.InvalidHandle {
						// Remove from chunk spatial index before despawning
						if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](shard.world, playerHandle); hasChunkRef {
							if transform, hasTransform := ecs.GetComponent[components.Transform](shard.world, playerHandle); hasTransform {
								if chunk := shard.chunkManager.GetChunk(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}); chunk != nil {
									if entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](shard.world, playerHandle); hasEntityInfo && entityInfo.IsStatic {
										chunk.Spatial().RemoveStatic(playerHandle, int(transform.X), int(transform.Y))
									} else {
										chunk.Spatial().RemoveDynamic(playerHandle, int(transform.X), int(transform.Y))
									}
								}
							}
						}
						shard.world.Despawn(playerHandle)

						// Save character data before despawn
						if shard.characterSaver != nil {
							shard.characterSaver.Save(shard.world, playerEntityID, playerHandle)
						}

						// Remove from CharacterEntities
						ecs.GetResource[ecs.CharacterEntities](shard.world).Remove(playerEntityID)
					}
					shard.UnregisterEntityAOI(playerEntityID)
					shard.mu.Unlock()

					g.logger.Debug("Unregistered entity AOI",
						zap.Uint64("client_id", c.ID),
						zap.Int64("character_id", int64(c.CharacterID)),
						zap.Int("layer", c.Layer),
					)
				}
			}

			// Set character offline in DB immediately (per requirement)
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

const maxCatchUpTicks = 4

func (g *Game) gameLoop() {
	defer g.wg.Done()

	lastTime := g.clock.GameNow()
	var accum time.Duration
	maxFrameTime := g.tickPeriod * time.Duration(maxCatchUpTicks)

	for {
		if g.getState() != GameStateRunning {
			return
		}

		now := g.clock.GameNow()
		frameTime := now.Sub(lastTime)
		lastTime = now

		if frameTime > maxFrameTime {
			frameTime = maxFrameTime
		}
		accum += frameTime

		catchUp := 0
		for accum >= g.tickPeriod && catchUp < maxCatchUpTicks {
			g.currentTick++

			tickNow := g.clock.GameNow()
			ts := ecs.TimeState{
				Tick:       g.currentTick,
				TickRate:   g.tickRate,
				TickPeriod: g.tickPeriod,
				Delta:      g.tickPeriod.Seconds(),
				Now:        tickNow,
				UnixMs:     tickNow.UnixMilli(),
				Uptime:     tickNow.Sub(g.startTime),
			}

			g.update(ts)

			accum -= g.tickPeriod
			catchUp++
		}

		// Drop excess accumulator to prevent death spiral
		if accum >= g.tickPeriod {
			accum = 0
		}

		// Sleep until next tick
		sleepTime := g.tickPeriod - accum
		select {
		case <-g.ctx.Done():
			return
		case <-time.After(sleepTime):
		}
	}
}

func (g *Game) accumulateStat(name string, d time.Duration) {
	acc := g.tickStats.systemStats[name]
	acc.Name = name
	acc.DurationSum += d
	acc.Count++
	g.tickStats.systemStats[name] = acc
}

func (g *Game) update(ts ecs.TimeState) {
	start := time.Now()

	schedResult := g.shardManager.Update(ts)
	schedDuration := schedResult.TotalDuration

	persistStart := time.Now()
	g.serverTimeManager.MaybePersistServerTime(ts.Now)
	persistDuration := time.Since(persistStart)

	// Collect per-system stats from all shards (prefixed by shard layer)
	collectStart := time.Now()
	for layer, shard := range g.shardManager.GetShards() {
		for _, st := range shard.World().DrainSystemStats() {
			if st.Count == 0 {
				continue
			}
			name := fmt.Sprintf("S%d:%s", layer, st.Name)
			acc := g.tickStats.systemStats[name]
			acc.Name = name
			acc.DurationSum += st.DurationSum
			acc.Count += st.Count
			g.tickStats.systemStats[name] = acc
		}
	}
	collectDuration := time.Since(collectStart)

	duration := time.Since(start)

	// Accumulate game-level timing (not through shards to avoid duplication)
	g.accumulateStat("ShardScheduling", schedDuration)
	g.accumulateStat("ServerTimePersist", persistDuration)
	g.accumulateStat("StatsCollect", collectDuration)

	unaccounted := duration - schedDuration - persistDuration - collectDuration
	if unaccounted < 0 {
		unaccounted = 0
	}
	g.accumulateStat("Unaccounted", unaccounted)

	// Accumulate per-shard execution times
	for _, sd := range schedResult.ShardDurations {
		g.accumulateStat(fmt.Sprintf("Shard%d", sd.Layer), sd.Duration)
	}

	// Collect statistics
	g.tickStats.durationSum += duration
	g.tickStats.count++

	// Log every 5 seconds
	if time.Since(g.tickStats.lastLog) >= 5*time.Second {
		if g.tickStats.count > 0 {
			avgDuration := g.tickStats.durationSum / time.Duration(g.tickStats.count)

			// Build per-system average fields
			sysFields := make([]zap.Field, 0, len(g.tickStats.systemStats))
			for _, st := range g.tickStats.systemStats {
				if st.Count > 0 {
					sysFields = append(sysFields, zap.Duration(st.Name, st.DurationSum/time.Duration(st.Count)))
				}
			}

			fields := []zap.Field{
				zap.Uint64("ticks", g.tickStats.count),
				zap.Duration("avg", avgDuration),
				zap.Duration("min", g.tickStats.minDuration),
				zap.Duration("max", g.tickStats.maxDuration),
			}
			fields = append(fields, sysFields...)

			//g.logger.Info("Game tick statistics (5s)", fields...)
		}

		// Reset statistics
		g.tickStats = tickStats{
			lastLog:     time.Now(),
			minDuration: time.Hour, // Initialize with large value
			systemStats: make(map[string]ecs.SystemTimingStat),
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

	// Save current server time before shutdown
	g.serverTimeManager.SaveCurrentTime(g.clock)

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
