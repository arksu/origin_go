package game

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"origin/internal/const"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type spawnPos struct {
	X, Y int
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
		if errors.Is(err, sql.ErrNoRows) {
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
	c.CharacterID = types.EntityID(character.ID)
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
		g.logger.Error("Shard not found for layer", zap.Int("layer", character.Layer), zap.Int64("character_id", character.ID))
		g.sendError(c, "Spawn failed: invalid layer")
		return
	}

	candidates := g.generateSpawnCandidates(character.X, character.Y)

	playerEntityID := types.EntityID(character.ID)
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

		ok, handle := shard.TrySpawnPlayer(pos.X, pos.Y, character, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.EntityInfo{
				ObjectType: types.ObjectTypePlayer,
				IsStatic:   false,
				Region:     character.Region,
				Layer:      character.Layer,
			})
			ecs.AddComponent(w, h, components.CreateTransform(pos.X, pos.Y, int(character.Heading)*45))
			ecs.AddComponent(w, h, components.ChunkRef{
				CurrentChunkX: pos.X / _const.ChunkWorldSize,
				CurrentChunkY: pos.Y / _const.ChunkWorldSize,
				PrevChunkX:    pos.X / _const.ChunkWorldSize,
				PrevChunkY:    pos.Y / _const.ChunkWorldSize,
			})
			ecs.AddComponent(w, h, components.Movement{
				VelocityX: 0,
				VelocityY: 0,
				Mode:      constt.Walk,
				State:     constt.StateIdle,
				// TODO player speed
				Speed:            32.0,
				TargetType:       constt.TargetNone,
				TargetX:          0,
				TargetY:          0,
				TargetHandle:     types.InvalidHandle,
				InteractionRange: 5.0,
			})
			ecs.AddComponent(w, h, components.Collider{
				HalfWidth:  _const.PlayerColliderSize / 2,
				HalfHeight: _const.PlayerColliderSize / 2,
				Layer:      _const.PlayerLayer,
				Mask:       _const.PlayerMask,
			})
			ecs.AddComponent(w, h, components.CollisionResult{
				HasCollision: false,
			})
			ecs.AddComponent(w, h, components.Vision{
				Radius: 240.0,
				Power:  100.0,
			})

			// If entity has Vision component - add it to VisibilityState.VisibleByObserver with immediate update
			visState := w.VisibilityState()
			visState.VisibleByObserver[h] = ecs.ObserverVisibility{
				Known:          make(map[types.Handle]struct{}, 32),
				NextUpdateTime: time.Time{}, // Zero time for immediate update
			}
		})
		if ok {
			_ = handle // Use the handle to avoid unused variable error

			character.X = pos.X
			character.Y = pos.Y

			spawned = true
			break
		}
		shard.mu.Lock()
		shard.UnregisterEntityAOI(playerEntityID)
		shard.mu.Unlock()

		g.logger.Debug("failed to spawn player", zap.Int64("character_id", character.ID), zap.Any("coord", pos))
	}

	if !spawned {
		g.logger.Debug("Player NOT spawned", zap.Int64("character_id", character.ID))
		g.sendError(c, "Spawn failed: no valid position")
		return
	}

	// Add client to shard's client map
	shard.clientsMu.Lock()
	shard.clients[playerEntityID] = c
	shard.clientsMu.Unlock()

	// Update client info
	c.CharacterID = playerEntityID
	c.Layer = character.Layer

	// Final check: ensure spawn context hasn't timed out before sending packets
	select {
	case <-ctx.Done():
		g.logger.Info("Spawn context timed out before sending packets", zap.Uint64("client_id", c.ID), zap.Error(ctx.Err()))
		shard.clientsMu.Lock()
		delete(shard.clients, playerEntityID)
		shard.clientsMu.Unlock()
		shard.mu.Lock()
		shard.UnregisterEntityAOI(playerEntityID)
		shard.mu.Unlock()
		return
	default:
	}

	g.sendPlayerEnterWorld(c, playerEntityID, shard, character)

	g.logger.Info("Player spawned",
		zap.Uint64("client_id", c.ID),
		zap.Int64("character_id", character.ID),
		zap.Uint64("entity_id", uint64(playerEntityID)),
		zap.Any("posX", character.X),
		zap.Any("posY", character.Y),
	)
}

func (g *Game) isValidSpawnPos(x, y int) bool {
	chunkSize := _const.ChunkSize * _const.CoordPerTile
	marginPixels := g.cfg.Game.WorldMarginTiles * _const.CoordPerTile
	minX := g.cfg.Game.WorldMinXChunks*chunkSize + marginPixels
	maxX := (g.cfg.Game.WorldMinXChunks+g.cfg.Game.WorldWidthChunks)*chunkSize - marginPixels
	minY := g.cfg.Game.WorldMinYChunks*chunkSize + marginPixels
	maxY := (g.cfg.Game.WorldMinYChunks+g.cfg.Game.WorldHeightChunks)*chunkSize - marginPixels

	return x >= minX && x < maxX && y >= minY && y < maxY
}

func (g *Game) generateSpawnCandidates(dbX, dbY int) []spawnPos {
	candidates := make([]spawnPos, 0, 1+g.cfg.Game.NearSpawnTries+g.cfg.Game.RandomSpawnTries)

	if g.isValidSpawnPos(dbX, dbY) {
		candidates = append(candidates, spawnPos{X: dbX, Y: dbY})
	}

	radius := g.cfg.Game.NearSpawnRadius
	visited := make(map[spawnPos]struct{})
	visited[spawnPos{X: dbX, Y: dbY}] = struct{}{}

	for i := 0; i < g.cfg.Game.NearSpawnTries; i++ {
		dx := rand.Intn(radius*2+1) - radius
		dy := rand.Intn(radius*2+1) - radius
		pos := spawnPos{X: dbX + dx, Y: dbY + dy}
		if _, exists := visited[pos]; !exists {
			visited[pos] = struct{}{}
			if g.isValidSpawnPos(pos.X, pos.Y) {
				candidates = append(candidates, pos)
			}
		}
	}

	chunkSize := _const.ChunkSize * _const.CoordPerTile
	marginPixels := g.cfg.Game.WorldMarginTiles * _const.CoordPerTile
	for i := 0; i < g.cfg.Game.RandomSpawnTries; i++ {
		minX := g.cfg.Game.WorldMinXChunks*chunkSize + marginPixels
		maxX := (g.cfg.Game.WorldMinXChunks+g.cfg.Game.WorldWidthChunks)*chunkSize - marginPixels
		minY := g.cfg.Game.WorldMinYChunks*chunkSize + marginPixels
		maxY := (g.cfg.Game.WorldMinYChunks+g.cfg.Game.WorldHeightChunks)*chunkSize - marginPixels

		if maxX <= minX || maxY <= minY {
			break
		}

		pos := spawnPos{
			X: minX + rand.Intn(maxX-minX),
			Y: minY + rand.Intn(maxY-minY),
		}
		if g.isValidSpawnPos(pos.X, pos.Y) {
			candidates = append(candidates, pos)
		}
	}

	return candidates
}

func (g *Game) sendPlayerEnterWorld(c *network.Client, entityID types.EntityID, shard *Shard, character repository.Character) {

	chunks := shard.ChunkManager().GetEntityActiveChunks(entityID)

	// Send chunks first so client can start rendering
	for _, chunk := range chunks {
		select {
		case <-c.Done():
			return
		default:
		}

		// TODO delete after events system
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
						X:       int32(character.X),
						Y:       int32(character.Y),
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
