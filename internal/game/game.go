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

	objectFactory       *world.ObjectFactory
	shardManager        *ShardManager
	entityIDManager     *EntityIDManager
	networkServer       *network.Server
	inventoryLoader     *inventory.InventoryLoader
	serverTimeManager   *timeutil.ServerTimeManager
	timePersistStopCh   chan struct{}
	timePersistDoneCh   chan struct{}
	timeStateMu         sync.RWMutex
	runtimeSecondsTotal int64
	runtimeRemainder    time.Duration

	clock             timeutil.Clock
	startTime         time.Time
	tickRate          int
	tickPeriod        time.Duration
	currentTick       uint64
	state             atomic.Int32 // GameState
	tickStats         tickStats
	enableStats       bool
	enableVisionStats bool
	teleportMu        sync.Mutex
	teleportInFlight  map[types.EntityID]struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewGame(cfg *config.Config, db *persistence.Postgres, objectFactory *world.ObjectFactory, inventoryLoader *inventory.InventoryLoader, inventorySnapshotSender *inventory.SnapshotSender, enableStats bool, enableVisionStats bool, logger *zap.Logger) *Game {
	ctx, cancel := context.WithCancel(context.Background())

	serverTimeManager := timeutil.NewServerTimeManager(db, logger)
	bootstrap, bootstrapErr := serverTimeManager.LoadOrInitBootstrap()
	if bootstrapErr != nil {
		logger.Fatal("Failed to initialize server time bootstrap", zap.Error(bootstrapErr))
	}
	tickPeriod := time.Second / time.Duration(cfg.Game.TickRate)
	clockStart := time.Unix(0, 0)
	clk := timeutil.NewMonotonicClockAt(clockStart)
	if bootstrap.RuntimeSecondsTotal > 0 {
		clk.Advance(time.Duration(bootstrap.RuntimeSecondsTotal) * time.Second)
	}

	g := &Game{
		cfg:                 cfg,
		db:                  db,
		objectFactory:       objectFactory,
		inventoryLoader:     inventoryLoader,
		logger:              logger,
		serverTimeManager:   serverTimeManager,
		clock:               clk,
		startTime:           clk.GameNow(),
		tickRate:            cfg.Game.TickRate,
		tickPeriod:          tickPeriod,
		currentTick:         bootstrap.InitialTick,
		runtimeSecondsTotal: bootstrap.RuntimeSecondsTotal,
		ctx:                 ctx,
		cancel:              cancel,
		enableStats:         enableStats,
		enableVisionStats:   enableVisionStats,
		tickStats: tickStats{
			lastLog:     time.Now(),
			minDuration: time.Hour,
			systemStats: make(map[string]ecs.SystemTimingStat),
		},
		teleportInFlight: make(map[types.EntityID]struct{}),
	}
	g.state.Store(int32(GameStateStarting))

	g.entityIDManager = NewEntityIDManager(cfg, db, logger)
	g.shardManager = NewShardManager(cfg, db, g.entityIDManager, objectFactory, inventorySnapshotSender, enableVisionStats, logger)
	g.networkServer = network.NewServer(&cfg.Network, &cfg.Game, logger)

	g.setupNetworkHandlers()
	for _, shard := range g.shardManager.GetShards() {
		shard.SetAdminTeleportExecutor(g)
	}

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
	case *netproto.ClientMessage_MovementMode:
		g.handleMovementMode(c, msg.Sequence, payload.MovementMode)
	case *netproto.ClientMessage_Chat:
		g.handleChatMessage(c, msg.Sequence, payload.Chat)
	case *netproto.ClientMessage_InventoryOp:
		g.handleInventoryOp(c, msg.Sequence, payload.InventoryOp)
	case *netproto.ClientMessage_OpenContainer:
		g.handleOpenContainer(c, msg.Sequence, payload.OpenContainer)
	case *netproto.ClientMessage_CloseContainer:
		g.handleCloseContainer(c, msg.Sequence, payload.CloseContainer)
	case *netproto.ClientMessage_StartCraftOne:
		g.handleStartCraftOne(c, msg.Sequence, payload.StartCraftOne)
	case *netproto.ClientMessage_StartCraftMany:
		g.handleStartCraftMany(c, msg.Sequence, payload.StartCraftMany)
	case *netproto.ClientMessage_BuildStart:
		g.handleStartBuild(c, msg.Sequence, payload.BuildStart)
	case *netproto.ClientMessage_BuildProgress:
		g.handleBuildProgress(c, msg.Sequence, payload.BuildProgress)
	case *netproto.ClientMessage_BuildTakeBack:
		g.handleBuildTakeBack(c, msg.Sequence, payload.BuildTakeBack)
	case *netproto.ClientMessage_OpenWindow:
		g.handleOpenWindow(c, msg.Sequence, payload.OpenWindow)
	case *netproto.ClientMessage_CloseWindow:
		g.handleCloseWindow(c, msg.Sequence, payload.CloseWindow)
	default:
		g.logger.Warn("Unknown packet type", zap.Uint64("client_id", c.ID), zap.Any("payload", msg.Payload))
	}
}

func (g *Game) handlePing(c *network.Client, sequence uint32, ping *netproto.C2S_Ping) {
	pong := &netproto.S2C_Pong{
		ClientTimeMs: ping.ClientTimeMs,
		ServerTimeMs: g.clock.WallNow().UnixMilli(),
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
	case *netproto.C2S_PlayerAction_SelectContextAction:
		cmdType = network.CmdSelectContextAction
		payload = act.SelectContextAction
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

func (g *Game) handleMovementMode(c *network.Client, sequence uint32, movementMode *netproto.C2S_MovementMode) {
	if c.CharacterID == 0 {
		g.logger.Warn("Movement mode update from unauthenticated client", zap.Uint64("client_id", c.ID))
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if movementMode == nil {
		return
	}

	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		g.logger.Error("Shard not found for movement mode update",
			zap.Uint64("client_id", c.ID),
			zap.Int("layer", c.Layer))
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}

	cmd := &network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdSetMovementMode,
		Payload:     movementMode,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	}

	if err := shard.PlayerInbox().Enqueue(cmd); err != nil {
		var overflowError network.OverflowError
		var rateLimitError network.RateLimitError
		var duplicateCommandError network.DuplicateCommandError
		switch {
		case errors.As(err, &overflowError):
			g.logger.Warn("Movement mode queue overflow",
				zap.Uint64("client_id", c.ID))
			c.SendWarning(netproto.WarningCode_WARN_INPUT_QUEUE_OVERFLOW, "Command queue overflow")
		case errors.As(err, &rateLimitError):
			g.logger.Warn("Movement mode rate limit exceeded",
				zap.Uint64("client_id", c.ID))
			c.SendError(netproto.ErrorCode_ERROR_PACKET_PER_SECOND_LIMIT_THRESHOLDED, "Rate limit exceeded")
			c.Close()
		case errors.As(err, &duplicateCommandError):
			// Ignore silently - already processed
		default:
			g.logger.Error("Failed to enqueue movement mode command",
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

func (g *Game) handleStartCraftOne(c *network.Client, sequence uint32, msg *netproto.C2S_StartCraftOne) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || strings.TrimSpace(msg.CraftKey) == "" {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid craft request")
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
		CommandType: network.CmdStartCraftOne,
		Payload:     msg,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	}
	_ = shard.PlayerInbox().Enqueue(cmd)
}

func (g *Game) handleStartCraftMany(c *network.Client, sequence uint32, msg *netproto.C2S_StartCraftMany) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || strings.TrimSpace(msg.CraftKey) == "" || msg.Cycles == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid craft request")
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
		CommandType: network.CmdStartCraftMany,
		Payload:     msg,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	}
	_ = shard.PlayerInbox().Enqueue(cmd)
}

func (g *Game) handleStartBuild(c *network.Client, sequence uint32, msg *netproto.C2S_BuildStart) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || msg.Pos == nil || strings.TrimSpace(msg.BuildKey) == "" {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid build request")
		return
	}
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}
	_ = shard.PlayerInbox().Enqueue(&network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdStartBuild,
		Payload:     msg,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	})
}

func (g *Game) handleBuildProgress(c *network.Client, sequence uint32, msg *netproto.C2S_BuildProgress) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || msg.EntityId == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid build progress request")
		return
	}
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}
	_ = shard.PlayerInbox().Enqueue(&network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdBuildProgress,
		Payload:     msg,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	})
}

func (g *Game) handleBuildTakeBack(c *network.Client, sequence uint32, msg *netproto.C2S_BuildTakeBack) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || msg.EntityId == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid build take-back request")
		return
	}
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}
	_ = shard.PlayerInbox().Enqueue(&network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdBuildTakeBack,
		Payload:     msg,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	})
}

func (g *Game) handleOpenWindow(c *network.Client, sequence uint32, msg *netproto.C2S_OpenWindow) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || strings.TrimSpace(msg.Name) == "" {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid window name")
		return
	}
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}
	_ = shard.PlayerInbox().Enqueue(&network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdOpenWindow,
		Payload:     msg,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	})
}

func (g *Game) handleCloseWindow(c *network.Client, sequence uint32, msg *netproto.C2S_CloseWindow) {
	if c.CharacterID == 0 {
		c.SendError(netproto.ErrorCode_ERROR_CODE_NOT_AUTHENTICATED, "Not authenticated")
		return
	}
	if msg == nil || strings.TrimSpace(msg.Name) == "" {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, "Invalid window name")
		return
	}
	shard := g.shardManager.GetShard(c.Layer)
	if shard == nil {
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Invalid shard")
		return
	}
	_ = shard.PlayerInbox().Enqueue(&network.PlayerCommand{
		ClientID:    c.ID,
		CharacterID: c.CharacterID,
		CommandID:   uint64(sequence),
		CommandType: network.CmdCloseWindow,
		Payload:     msg,
		ReceivedAt:  time.Now(),
		Layer:       c.Layer,
	})
}

func (g *Game) handleDisconnect(c *network.Client) {
	g.logger.Info("Client disconnected", zap.Uint64("client_id", c.ID))

	if c.CharacterID != 0 {
		if g.getState() == GameStateRunning {
			if shard := g.shardManager.GetShard(c.Layer); shard != nil {
				playerEntityID := c.CharacterID

				// Remove client from command queue to clean up rate limiting state
				shard.PlayerInbox().RemoveClient(c.ID)

				// If another client is already bound to this character, this is a stale disconnect
				// event from an old socket. Ignore it to avoid detaching/despawning an active player.
				shard.ClientsMu.RLock()
				activeClient, hasClient := shard.Clients[playerEntityID]
				shard.ClientsMu.RUnlock()
				if hasClient && activeClient != c {
					g.logger.Info("Ignoring stale disconnect for character with active replacement session",
						zap.Uint64("client_id", c.ID),
						zap.Int64("character_id", int64(c.CharacterID)),
						zap.Uint64("active_client_id", activeClient.ID),
						zap.Int("layer", c.Layer),
					)
					return
				}

				// Reset client state and remove from shard's client map
				c.InWorld.Store(false)
				c.StreamEpoch.Store(0)
				shard.ClientsMu.Lock()
				delete(shard.Clients, playerEntityID)
				shard.ClientsMu.Unlock()

				disconnectDelay := g.cfg.Game.DisconnectDelay

				shard.mu.Lock()
				playerHandle := shard.world.GetHandleByEntityID(playerEntityID)
				ecs.GetResource[ecs.OpenedWindowsState](shard.world).ClearPlayer(playerEntityID)

				if disconnectDelay > 0 && playerHandle != types.InvalidHandle {
					if _, _, err := ecs.BreakLinkForPlayer(shard.world, playerEntityID, ecs.LinkBreakClosed); err != nil {
						g.logger.Warn("Failed to publish LinkBroken on detach",
							zap.Error(err),
							zap.Int64("character_id", int64(playerEntityID)),
							zap.Int("layer", c.Layer),
						)
					}
					// Detached mode: keep entity in world for DisconnectDelay seconds
					gameNow := g.clock.GameNow()
					expirationTime := gameNow.Add(time.Duration(disconnectDelay) * time.Second)
					ecs.GetResource[ecs.DetachedEntities](shard.world).AddDetachedEntity(playerEntityID, playerHandle, expirationTime, gameNow)
					ecs.ForgetPlayerStatsState(shard.world, playerEntityID)

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
						if _, _, err := ecs.BreakLinkForPlayer(shard.world, playerEntityID, ecs.LinkBreakDespawn); err != nil {
							g.logger.Warn("Failed to publish LinkBroken on disconnect despawn",
								zap.Error(err),
								zap.Int64("character_id", int64(playerEntityID)),
								zap.Int("layer", c.Layer),
							)
						}
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
	g.startPeriodicServerTimePersist()
	g.wg.Add(1)
	go g.gameLoop()

	g.logger.Info("Game loop started", zap.Int("tick_rate_hz", g.tickRate))
}

const maxCatchUpTicks = 4
const serverTimePersistInterval = 20 * time.Second

func (g *Game) gameLoop() {
	defer g.wg.Done()

	lastWallTime := g.clock.WallNow()
	var accum time.Duration
	maxFrameTime := g.tickPeriod * time.Duration(maxCatchUpTicks)

	for {
		if g.getState() != GameStateRunning {
			return
		}

		nowWall := g.clock.WallNow()
		frameTime := nowWall.Sub(lastWallTime)
		lastWallTime = nowWall
		if frameTime < 0 {
			frameTime = 0
		}

		// Runtime time is based on real elapsed wall time, independent from tick catch-up limits.
		g.clock.Advance(frameTime)
		runtimeNow := g.clock.GameNow()
		g.timeStateMu.Lock()
		g.runtimeRemainder += frameTime
		if g.runtimeRemainder >= time.Second {
			addedRuntimeSeconds := int64(g.runtimeRemainder / time.Second)
			g.runtimeRemainder -= time.Duration(addedRuntimeSeconds) * time.Second
			g.runtimeSecondsTotal += addedRuntimeSeconds
		}
		g.timeStateMu.Unlock()

		frameTimeForTicks := frameTime
		if frameTimeForTicks > maxFrameTime {
			frameTimeForTicks = maxFrameTime
		}
		accum += frameTimeForTicks
		wallUnixMs := nowWall.UnixMilli()

		catchUp := 0
		for accum >= g.tickPeriod && catchUp < maxCatchUpTicks {
			g.timeStateMu.Lock()
			g.currentTick++
			currentTick := g.currentTick
			currentRuntimeSeconds := g.runtimeSecondsTotal
			g.timeStateMu.Unlock()

			ts := ecs.TimeState{
				Tick:                currentTick,
				TickRate:            g.tickRate,
				TickPeriod:          g.tickPeriod,
				Delta:               g.tickPeriod.Seconds(),
				Now:                 runtimeNow,
				UnixMs:              wallUnixMs,
				WallNow:             nowWall,
				RuntimeSecondsTotal: currentRuntimeSeconds,
				Uptime:              runtimeNow.Sub(g.startTime),
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
	g.accumulateStat("StatsCollect", collectDuration)

	unaccounted := duration - schedDuration - collectDuration
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

	// Log every 5 seconds if stats are enabled
	if time.Since(g.tickStats.lastLog) >= 5*time.Second {
		if g.tickStats.count > 0 && g.enableStats {
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

			g.logger.Info("Game tick statistics (5s)", fields...)
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

	g.resetOnlinePlayers()
	g.cancel()
	g.stopPeriodicServerTimePersist()

	g.logger.Info("Stopping networkServer")
	g.networkServer.Stop()
	g.logger.Info("Stopping shardManager")
	g.shardManager.Stop()
	g.logger.Info("Stopping entityIDManager")
	g.entityIDManager.Stop()

	// Wait for all goroutines to finish
	g.wg.Wait()
	g.persistServerTimeSnapshot("shutdown")

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
	g.timeStateMu.RLock()
	defer g.timeStateMu.RUnlock()
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
		charEntities := ecs.GetResource[ecs.CharacterEntities](shard.world)
		totalPlayers += len(charEntities.Map)
		shard.mu.RUnlock()
	}

	avgTickDuration := time.Duration(0)
	if g.tickStats.count > 0 {
		avgTickDuration = g.tickStats.durationSum / time.Duration(g.tickStats.count)
	}

	return GameStats{
		ConnectedClients: connectedClients,
		TotalPlayers:     totalPlayers,
		CurrentTick:      g.CurrentTick(),
		TickRate:         g.tickRate,
		AvgTickDuration:  avgTickDuration,
	}
}

func (g *Game) startPeriodicServerTimePersist() {
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	g.timePersistStopCh = stopCh
	g.timePersistDoneCh = doneCh

	go func() {
		defer close(doneCh)

		ticker := time.NewTicker(serverTimePersistInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				g.persistServerTimeSnapshot("periodic")
			case <-stopCh:
				return
			case <-g.ctx.Done():
				return
			}
		}
	}()
}

func (g *Game) stopPeriodicServerTimePersist() {
	stopCh := g.timePersistStopCh
	doneCh := g.timePersistDoneCh
	g.timePersistStopCh = nil
	g.timePersistDoneCh = nil

	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}
}

func (g *Game) persistServerTimeSnapshot(reason string) {
	if g.serverTimeManager == nil {
		return
	}

	snapshot := g.serverTimeSnapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := g.serverTimeManager.PersistState(ctx, snapshot); err != nil {
		g.logger.Error("Failed to persist server time state",
			zap.String("reason", reason),
			zap.Uint64("tick_total", snapshot.InitialTick),
			zap.Int64("runtime_seconds_total", snapshot.RuntimeSecondsTotal),
			zap.Error(err),
		)
		return
	}

	if reason == "shutdown" {
		g.logger.Info("Persisted server time state on shutdown",
			zap.Uint64("tick_total", snapshot.InitialTick),
			zap.Int64("runtime_seconds_total", snapshot.RuntimeSecondsTotal),
		)
	}
}

func (g *Game) serverTimeSnapshot() timeutil.ServerTimeBootstrap {
	g.timeStateMu.RLock()
	defer g.timeStateMu.RUnlock()
	return timeutil.ServerTimeBootstrap{
		InitialTick:         g.currentTick,
		RuntimeSecondsTotal: g.runtimeSecondsTotal,
	}
}
