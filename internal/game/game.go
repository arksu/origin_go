package game

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"origin/internal/config"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence"
)

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
	}

	g.entityIDManager = NewEntityIDManager(cfg, db, logger)
	g.shardManager = NewShardManager(cfg, db, g.entityIDManager, objectFactory, logger)
	g.networkServer = network.NewServer(&cfg.Network, logger)

	g.setupNetworkHandlers()

	return g
}

func (g *Game) setupNetworkHandlers() {
	g.networkServer.SetOnConnect(func(c *network.Client) {
		g.logger.Info("Client connected", zap.Uint64("client_id", c.ID))
	})

	g.networkServer.SetOnDisconnect(func(c *network.Client) {
		g.handleDisconnect(c)
	})

	g.networkServer.SetOnMessage(func(c *network.Client, data []byte) {
		g.handlePacket(c, data)
	})
}

func (g *Game) handlePacket(c *network.Client, data []byte) {
	msg := &netproto.ClientMessage{}
	if err := proto.Unmarshal(data, msg); err != nil {
		g.logger.Warn("Failed to unmarshal packet", zap.Uint64("client_id", c.ID), zap.Error(err))
		return
	}
	g.logger.Debug("Received packet", zap.Uint64("client_id", c.ID), zap.Any("payload", msg.Payload))

	switch payload := msg.Payload.(type) {
	case *netproto.ClientMessage_Ping:
		g.handlePing(c, msg.Sequence, payload.Ping)
	case *netproto.ClientMessage_Auth:
		g.handleAuth(c, msg.Sequence, payload.Auth)
	default:
		g.logger.Warn("Unknown packet type", zap.Uint64("client_id", c.ID))
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

func (g *Game) handleAuth(c *network.Client, sequence uint32, auth *netproto.C2S_Auth) {
	g.logger.Debug("Auth request", zap.Uint64("client_id", c.ID), zap.String("token", auth.Token))

	if auth.Token == "" {
		g.sendAuthResult(c, sequence, false, "Empty token")
		return
	}
	// Validate token against auth service, select from character where auth_token=token and token_expires_at is valid
	character, err := g.db.Queries().GetCharacterByToken(g.ctx, sql.NullString{String: auth.Token, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			g.sendAuthResult(c, sequence, false, "Invalid token")
			return
		}
		g.logger.Error("Failed to get character by token", zap.Uint64("client_id", c.ID), zap.Error(err))
		g.sendAuthResult(c, sequence, false, "Database error")
		return
	}

	// Check if token is expired
	if character.TokenExpiresAt.Valid && character.TokenExpiresAt.Time.Before(time.Now()) {
		g.sendAuthResult(c, sequence, false, "Token expired")
		return
	}

	// Check if character is already online
	if character.IsOnline.Valid && character.IsOnline.Bool {
		g.sendAuthResult(c, sequence, false, "Character already online")
		return
	}

	// Update character: set is_online=true where is_online=false
	if err := g.db.Queries().SetCharacterOnline(g.ctx, character.ID); err != nil {
		g.logger.Error("Failed to set character online", zap.Uint64("client_id", c.ID), zap.Int64("character_id", character.ID), zap.Error(err))
		g.sendAuthResult(c, sequence, false, "Failed to set character online")
		return
	}

	// Set character as online and update client association
	c.CharacterID = character.ID
	g.logger.Info("Character authenticated", zap.Uint64("client_id", c.ID), zap.Int64("character_id", character.ID), zap.String("character_name", character.Name))

	g.sendAuthResult(c, sequence, true, "")
}

func (g *Game) sendAuthResult(c *network.Client, sequence uint32, success bool, errorMsg string) {
	result := &netproto.S2C_AuthResult{
		Success:      success,
		ErrorMessage: errorMsg,
	}

	response := &netproto.ServerMessage{
		Sequence: sequence,
		Payload: &netproto.ServerMessage_AuthResult{
			AuthResult: result,
		},
	}

	data, err := proto.Marshal(response)
	if err != nil {
		g.logger.Error("Failed to marshal auth result", zap.Uint64("client_id", c.ID), zap.Error(err))
		return
	}

	c.Send(data)
}

func (g *Game) handleDisconnect(c *network.Client) {
	g.logger.Info("Client disconnected", zap.Uint64("client_id", c.ID))

	// If IsOnline - update is_online=false in character
	if c.CharacterID != 0 {
		if err := g.db.Queries().SetCharacterOffline(g.ctx, c.CharacterID); err != nil {
			g.logger.Error("Failed to set character offline", zap.Uint64("client_id", c.ID), zap.Int64("character_id", c.CharacterID), zap.Error(err))
		} else {
			g.logger.Info("Character set offline", zap.Uint64("client_id", c.ID), zap.Int64("character_id", c.CharacterID))
		}
	}
}

func (g *Game) StartGameLoop() {
	g.wg.Add(1)
	go g.gameLoop()

	g.logger.Info("Game loop started", zap.Int("tick_rate_hz", g.tickRate))
}

func (g *Game) gameLoop() {
	defer g.wg.Done()

	ticker := time.NewTicker(g.tickPeriod)
	defer ticker.Stop()

	lastTime := time.Now()

	for {
		select {
		case <-g.ctx.Done():
			return
		case now := <-ticker.C:
			dt := now.Sub(lastTime).Seconds()
			lastTime = now

			g.currentTick++
			g.update(float32(dt))
		}
	}
}

func (g *Game) update(dt float32) {
	g.shardManager.Update(dt)
}

func (g *Game) Stop() {
	g.logger.Info("Stopping game...")
	g.cancel()

	g.networkServer.Stop()
	g.shardManager.Stop()
	g.entityIDManager.Stop()

	g.wg.Wait()
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

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
