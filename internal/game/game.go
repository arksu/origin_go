package game

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	"origin/internal/config"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
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
	g.state.Store(int32(GameStateStarting))

	g.entityIDManager = NewEntityIDManager(cfg, db, logger)
	g.shardManager = NewShardManager(cfg, db, g.entityIDManager, objectFactory, logger)
	g.networkServer = network.NewServer(&cfg.Network, logger)

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

	var character repository.Character

	// Use transaction with FOR UPDATE lock to prevent race conditions
	err := g.db.WithTx(g.ctx, func(q *repository.Queries) error {
		var err error
		character, err = q.GetCharacterByTokenForUpdate(g.ctx, sql.NullString{String: auth.Token, Valid: true})
		if err != nil {
			return err
		}

		// Check if token is expired
		if character.TokenExpiresAt.Valid && character.TokenExpiresAt.Time.Before(time.Now()) {
			return fmt.Errorf("token expired")
		}

		// Check if character is already online
		if character.IsOnline.Valid && character.IsOnline.Bool {
			return fmt.Errorf("character already online")
		}

		// Update character: set is_online=true where is_online=false
		if err := q.SetCharacterOnline(g.ctx, character.ID); err != nil {
			return fmt.Errorf("set character online: %w", err)
		}

		return nil
	})

	if err != nil {
		if err == sql.ErrNoRows {
			g.sendAuthResult(c, sequence, false, "Invalid token")
			return
		}
		errMsg := err.Error()
		if errMsg == "token expired" {
			g.sendAuthResult(c, sequence, false, "Token expired")
			return
		}
		if errMsg == "character already online" {
			g.sendAuthResult(c, sequence, false, "Character already online")
			return
		}
		g.logger.Error("Failed to authenticate character", zap.Uint64("client_id", c.ID), zap.Error(err))
		g.sendAuthResult(c, sequence, false, "Database error")
		return
	}

	// Set character as online and update client association
	c.CharacterID = ecs.EntityID(character.ID)
	c.Layer = character.Layer
	g.logger.Info("Character authenticated", zap.Uint64("client_id", c.ID), zap.Int64("character_id", character.ID), zap.String("character_name", character.Name))

	g.sendAuthResult(c, sequence, true, "")

	go g.spawnAndLogin(c, character)
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

func (g *Game) spawnAndLogin(c *network.Client, character repository.Character) {
	ctx, cancel := context.WithTimeout(context.Background(), g.cfg.Game.SpawnTimeout)
	defer cancel()

	go func() {
		select {
		case <-c.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	select {
	case <-c.Done():
		g.logger.Debug("Client disconnected before spawn", zap.Uint64("client_id", c.ID))
		return
	default:
	}

	shard := g.shardManager.GetShard(character.Layer)
	if shard == nil {
		g.logger.Error("Shard not found for layer", zap.Int32("layer", character.Layer), zap.Int64("character_id", character.ID))
		g.sendError(c, "Spawn failed: invalid layer")
		return
	}

	candidates := g.generateSpawnCandidates(int(character.X), int(character.Y))

	playerEntityID := ecs.EntityID(character.ID)
	spawned := false

	for _, pos := range candidates {
		select {
		case <-ctx.Done():
			g.sendError(c, "Spawn timeout")
			return
		default:
		}

		if err := shard.PrepareEntityAOI(ctx, playerEntityID, pos.X, pos.Y); err != nil {
			g.logger.Error("Failed to prepare entity AOI",
				zap.Uint64("client_id", c.ID),
				zap.Int64("character_id", character.ID),
				zap.Error(err),
			)
			g.sendError(c, "Spawn failed: AOI preparation error")
			return
		}

		ok, handle := shard.TrySpawnPlayer(pos.X, pos.Y, character)
		if ok {

			// add player components
			ecs.AddComponent(shard.world, handle, components.EntityInfo{
				ObjectType: components.ObjectTypePlayer,
				IsStatic:   false,
				Region:     character.Region,
				Layer:      character.Layer,
			})
			ecs.AddComponent(shard.world, handle, components.Transform{X: pos.X, Y: pos.Y, Direction: float32(character.Heading) * 45})
			ecs.AddComponent(shard.world, handle, components.Movement{
				VelocityX:        0,
				VelocityY:        0,
				Mode:             components.Walk,
				State:            components.StateIdle,
				Speed:            12.0, // TODO player speed
				TargetType:       components.TargetNone,
				TargetX:          0,
				TargetY:          0,
				TargetHandle:     ecs.InvalidHandle,
				InteractionRange: 5.0,
			})
			ecs.AddComponent(shard.world, handle, components.Collider{
				HalfWidth:  PlayerAABBSize / 2,
				HalfHeight: PlayerAABBSize / 2,
			})

			spawned = true
			break
		}
		g.logger.Debug("failed to spawn player", zap.Int64("character_id", character.ID), zap.Any("coord", pos))
	}

	if !spawned {
		g.sendError(c, "Spawn failed: no valid position")
		return
	}

	// Final check: ensure spawn context hasn't timed out before sending packets
	select {
	case <-ctx.Done():
		g.logger.Info("Spawn context timed out before sending packets", zap.Uint64("client_id", c.ID), zap.Error(ctx.Err()))
		shard.UnregisterEntityAOI(playerEntityID)
		return
	default:
	}

	g.sendPlayerEnterWorld(c, playerEntityID, shard, character)

	g.logger.Info("Player spawned",
		zap.Uint64("client_id", c.ID),
		zap.Int64("character_id", character.ID),
		zap.Uint64("entity_id", uint64(playerEntityID)),
	)
}

type spawnPos struct {
	X, Y int
}

func (g *Game) generateSpawnCandidates(dbX, dbY int) []spawnPos {
	candidates := make([]spawnPos, 0, 1+g.cfg.Game.NearSpawnTries+g.cfg.Game.RandomSpawnTries)

	candidates = append(candidates, spawnPos{X: dbX, Y: dbY})

	radius := g.cfg.Game.NearSpawnRadius
	visited := make(map[spawnPos]struct{})
	visited[spawnPos{X: dbX, Y: dbY}] = struct{}{}

	for i := 0; i < g.cfg.Game.NearSpawnTries; i++ {
		dx := rand.Intn(radius*2+1) - radius
		dy := rand.Intn(radius*2+1) - radius
		pos := spawnPos{X: dbX + dx, Y: dbY + dy}
		if _, exists := visited[pos]; !exists {
			visited[pos] = struct{}{}
			candidates = append(candidates, pos)
		}
	}

	chunkSize := g.cfg.Game.ChunkSize * g.cfg.Game.CoordPerTile
	worldWidth := chunkSize * g.cfg.Game.WorldWidthChunks
	worldHeight := chunkSize * g.cfg.Game.WorldHeightChunks
	for i := 0; i < g.cfg.Game.RandomSpawnTries; i++ {
		pos := spawnPos{
			X: g.cfg.Game.WorldMinXChunks*chunkSize + rand.Intn(worldWidth),
			Y: g.cfg.Game.WorldMinYChunks*chunkSize + rand.Intn(worldHeight),
		}
		candidates = append(candidates, pos)
	}

	return candidates
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

func (g *Game) sendPlayerEnterWorld(c *network.Client, entityID ecs.EntityID, shard *Shard, character repository.Character) {
	chunks := shard.ChunkManager().GetEntityActiveChunks(entityID)

	// Send chunks first so client can start rendering
	for _, chunk := range chunks {
		select {
		case <-c.Done():
			return
		default:
		}

		loadChunk := &netproto.ServerMessage{
			Payload: &netproto.ServerMessage_LoadChunk{
				LoadChunk: &netproto.S2C_LoadChunk{
					Chunk: &netproto.ChunkData{
						Coord: &netproto.ChunkCoord{
							X: int32(chunk.Coord.X),
							Y: int32(chunk.Coord.Y),
						},
						Tiles: chunk.Tiles,
					},
				},
			},
		}

		chunkData, err := proto.Marshal(loadChunk)
		if err != nil {
			g.logger.Error("Failed to marshal load chunk", zap.Uint64("client_id", c.ID), zap.Error(err))
			continue
		}
		c.Send(chunkData)
	}

	select {
	case <-c.Done():
		return
	default:
	}

	// Send enter world after chunks are sent (signals "ready to render")
	enterWorld := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_PlayerEnterWorld{
			PlayerEnterWorld: &netproto.S2C_PlayerEnterWorld{
				EntityId: uint64(entityID),
				Movement: &netproto.EntityMovement{
					Position: &netproto.Position{
						X:       character.X,
						Y:       character.Y,
						Heading: uint32(character.Heading),
					},
				},
			},
		},
	}

	data, err := proto.Marshal(enterWorld)
	if err != nil {
		g.logger.Error("Failed to marshal player enter world", zap.Uint64("client_id", c.ID), zap.Error(err))
		return
	}
	c.Send(data)
}

func (g *Game) handleDisconnect(c *network.Client) {
	g.logger.Info("Client disconnected", zap.Uint64("client_id", c.ID))

	if c.CharacterID != 0 {
		if g.getState() == GameStateRunning {
			if shard := g.shardManager.GetShard(c.Layer); shard != nil {
				playerEntityID := c.CharacterID
				shard.UnregisterEntityAOI(playerEntityID)
				g.logger.Debug("Unregistered entity AOI",
					zap.Uint64("client_id", c.ID),
					zap.Int64("character_id", int64(c.CharacterID)),
					zap.Int32("layer", c.Layer),
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
		g.logger.Error("Failed to reset online players", zap.Int32("region", g.cfg.Game.Region), zap.Error(err))
	} else {
		g.logger.Info("Reset online players", zap.Int32("region", g.cfg.Game.Region))
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

	for {
		select {
		case <-g.ctx.Done():
			return
		case now := <-ticker.C:
			if g.getState() != GameStateRunning {
				return
			}
			dt := now.Sub(lastTime).Seconds()
			lastTime = now

			g.currentTick++
			g.update(dt)
		}
	}
}

func (g *Game) update(dt float64) {

	g.shardManager.Update(dt)
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
