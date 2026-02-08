package game

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
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
	// Client is not yet in world during spawn attempts
	c.InWorld.Store(false)

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
		c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Spawn failed: invalid layer")
		return
	}

	playerEntityID := types.EntityID(character.ID)

	// Check if entity is detached (reconnect scenario)
	if g.tryReattachPlayer(c, shard, playerEntityID, character) {
		return
	}

	// Normal spawn flow
	candidates := g.generateSpawnCandidates(character.X, character.Y)
	spawned := false
	var playerHandle *types.Handle

	for _, pos := range candidates {
		select {
		case <-ctx.Done():
			c.SendError(netproto.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED, "Spawn timeout")
			return
		default:
		}

		if err := shard.PrepareEntityAOI(ctx, playerEntityID, pos.X, pos.Y); err != nil {
			g.logger.Error("Failed to prepare AOI for player",
				zap.Uint64("client_id", c.ID),
				zap.Int64("character_id", character.ID),
				zap.Error(err),
			)
			c.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Spawn failed: AOI preparation error")
			return
		}

		ok, handle := shard.TrySpawnPlayer(pos.X, pos.Y, character, func(w *ecs.World, h types.Handle) {
			playerDef, _ := objectdefs.Global().GetByKey("player")
			var playerTypeID uint32
			var playerBehaviors []string
			if playerDef != nil {
				playerTypeID = uint32(playerDef.DefID)
				playerBehaviors = playerDef.Behavior
			}
			ecs.AddComponent(w, h, components.EntityInfo{
				TypeID:    playerTypeID,
				Behaviors: playerBehaviors,
				IsStatic:  false,
				Region:    character.Region,
				Layer:     character.Layer,
			})
			ecs.AddComponent(w, h, components.CreateTransform(pos.X, pos.Y, int(character.Heading)))
			ecs.AddComponent(w, h, components.ChunkRef{
				CurrentChunkX: pos.X / _const.ChunkWorldSize,
				CurrentChunkY: pos.Y / _const.ChunkWorldSize,
				PrevChunkX:    pos.X / _const.ChunkWorldSize,
				PrevChunkY:    pos.Y / _const.ChunkWorldSize,
			})
			ecs.AddComponent(w, h, components.Movement{
				VelocityX:        0,
				VelocityY:        0,
				Mode:             _const.Walk,
				State:            _const.StateIdle,
				Speed:            _const.PlayerSpeed,
				TargetType:       _const.TargetNone,
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
			ecs.AddComponent(w, h, components.Appearance{
				Name:     &character.Name,
				Resource: "player",
			})
			ecs.AddComponent(w, h, components.Vision{
				Radius: _const.PlayerVisionRadius,
				Power:  _const.PlayerVisionPower,
			})

			// If entity has Vision component - add it to VisibilityState.VisibleByObserver with immediate update
			visState := ecs.GetResource[ecs.VisibilityState](w)
			visState.VisibleByObserver[h] = ecs.ObserverVisibility{
				Known:          make(map[types.Handle]types.EntityID, 32),
				NextUpdateTime: time.Time{}, // Zero time for immediate update
			}

			// Load player inventories from database
			dbInventories, err := g.db.Queries().GetInventoriesByOwner(ctx, character.ID)
			if err != nil {
				g.logger.Error("Failed to load inventories from database",
					zap.Int64("character_id", character.ID),
					zap.Error(err))
			} else {
				// Parse inventories from database format
				inventoryDataList, parseWarnings := g.inventoryLoader.ParseInventoriesFromDB(dbInventories)
				if len(parseWarnings) > 0 {
					g.logger.Warn("Inventory parse warnings",
						zap.Int64("character_id", character.ID),
						zap.Strings("warnings", parseWarnings))
				}

				// Always take inventories from database and enrich with missing defaults by kind+key
				inventoryDataList = g.enrichWithMissingDefaults(inventoryDataList)

				// Load inventories into ECS
				loadResult, err := g.inventoryLoader.LoadPlayerInventories(w, playerEntityID, inventoryDataList)
				if err != nil {
					g.logger.Error("Failed to load player inventories",
						zap.Int64("character_id", character.ID),
						zap.Error(err))
				} else {
					if len(loadResult.Warnings) > 0 {
						g.logger.Warn("Inventory load warnings",
							zap.Int64("character_id", character.ID),
							zap.Strings("warnings", loadResult.Warnings))
					}

					// Create InventoryOwner component with all inventory links (including nested)
					inventoryLinks := make([]components.InventoryLink, 0, len(loadResult.ContainerHandles))
					refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
					for _, containerHandle := range loadResult.ContainerHandles {
						if !w.Alive(containerHandle) {
							continue
						}
						container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
						if hasContainer {
							inventoryLinks = append(inventoryLinks, components.InventoryLink{
								Kind:    container.Kind,
								Key:     container.Key,
								OwnerID: container.OwnerID,
								Handle:  containerHandle,
							})
							refIndex.Add(container.Kind, container.OwnerID, container.Key, containerHandle)
						}
					}
					ecs.AddComponent(w, h, components.InventoryOwner{
						Inventories: inventoryLinks,
					})

					g.logger.Debug("Player inventories loaded",
						zap.Int64("character_id", character.ID),
						zap.Int("containers", len(loadResult.ContainerHandles)),
						zap.Bool("lost_and_found_used", loadResult.LostAndFoundUsed))
				}
			}
		})
		if ok {
			// Register character entity for periodic saving
			charEntities := ecs.GetResource[ecs.CharacterEntities](shard.world)
			nextSaveAt := g.clock.GameNow().Add(g.cfg.Game.PlayerSaveInterval)
			charEntities.Add(playerEntityID, handle, nextSaveAt)

			character.X = pos.X
			character.Y = pos.Y

			spawned = true
			playerHandle = &handle
			break
		}
		shard.mu.Lock()
		shard.UnregisterEntityAOI(playerEntityID)
		shard.mu.Unlock()

		g.logger.Debug("failed to spawn player", zap.Int64("character_id", character.ID), zap.Any("coord", pos))
	}

	if !spawned {
		g.logger.Debug("Player NOT spawned", zap.Int64("character_id", character.ID))
		c.SendError(netproto.ErrorCode_ERROR_CODE_PATH_BLOCKED, "Spawn failed: no valid position")
		return
	}

	// Add client to shard's client map
	shard.ClientsMu.Lock()
	shard.Clients[playerEntityID] = c
	shard.ClientsMu.Unlock()

	// Final check: ensure spawn context hasn't timed out before sending packets
	select {
	case <-ctx.Done():
		g.logger.Info("Spawn context timed out before sending packets", zap.Uint64("client_id", c.ID), zap.Error(ctx.Err()))
		shard.ClientsMu.Lock()
		delete(shard.Clients, playerEntityID)
		shard.ClientsMu.Unlock()
		shard.mu.Lock()
		shard.UnregisterEntityAOI(playerEntityID)
		shard.mu.Unlock()
		return
	default:
	}

	// After successful spawn: increment epoch, enable chunk events, send PlayerEnterWorld, then set InWorld = true
	c.StreamEpoch.Add(1)
	c.InWorld.Store(true)
	g.sendPlayerEnterWorld(c, playerEntityID, shard, character)
	shard.ChunkManager().EnableChunkLoadEvents(playerEntityID, c.StreamEpoch.Load())

	// Enqueue inventory snapshot job to be processed on the ECS tick thread (avoids concurrent map access)
	_ = shard.ServerInbox().Enqueue(&network.ServerJob{
		JobType:  network.JobSendInventorySnapshot,
		TargetID: playerEntityID,
		Payload:  &network.InventorySnapshotJobPayload{Handle: *playerHandle},
	})

	g.logger.Info("Player spawned",
		zap.Uint64("client_id", c.ID),
		zap.Int64("character_id", character.ID),
		zap.Uint64("entity_id", uint64(playerEntityID)),
		zap.Any("posX", character.X),
		zap.Any("posY", character.Y),
	)
}

// tryReattachPlayer attempts to reattach a client to an existing detached entity
// Returns true if reattach was successful, false if normal spawn should proceed
func (g *Game) tryReattachPlayer(c *network.Client, shard *Shard, playerEntityID types.EntityID, character repository.Character) bool {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	detachedEntities := ecs.GetResource[ecs.DetachedEntities](shard.world)

	// Check if entity is detached
	detachedEntity, isDetached := detachedEntities.GetDetachedEntity(playerEntityID)
	if !isDetached {
		return false
	}

	handle := detachedEntity.Handle

	// Verify entity is still alive
	if !shard.world.Alive(handle) {
		// Entity was despawned (e.g., killed), remove from detached map and proceed with normal spawn
		detachedEntities.RemoveDetachedEntity(playerEntityID)
		g.logger.Info("Detached entity no longer alive, proceeding with normal spawn",
			zap.Uint64("client_id", c.ID),
			zap.Int64("character_id", int64(playerEntityID)),
		)
		return false
	}

	// Remove from detached map (cancel expiration timer)
	detachedEntities.RemoveDetachedEntity(playerEntityID)

	// Re-register character entity for periodic saving
	charEntities := ecs.GetResource[ecs.CharacterEntities](shard.world)
	nextSaveAt := g.clock.GameNow().Add(g.cfg.Game.PlayerSaveInterval)
	charEntities.Add(playerEntityID, handle, nextSaveAt)

	detachedDuration := time.Since(detachedEntity.DetachedAt)

	// Add client to shard's client map
	shard.ClientsMu.Lock()
	shard.Clients[playerEntityID] = c
	shard.ClientsMu.Unlock()

	// Get current position from entity
	var posX, posY int
	if transform, hasTransform := ecs.GetComponent[components.Transform](shard.world, handle); hasTransform {
		posX = int(transform.X)
		posY = int(transform.Y)
	}

	// After successful reattach: increment epoch, enable chunk events, send PlayerEnterWorld, then set InWorld = true
	c.StreamEpoch.Add(1)
	c.InWorld.Store(true)
	g.sendPlayerEnterWorld(c, playerEntityID, shard, character)

	// Force immediate visibility update for the reattached observer
	visState := ecs.GetResource[ecs.VisibilityState](shard.world)
	if observerVis, ok := visState.VisibleByObserver[handle]; ok {
		observerVis.NextUpdateTime = time.Time{} // Zero time for immediate update
		visState.VisibleByObserver[handle] = observerVis

		// Send spawn events for all currently visible entities
		g.logger.Debug("Sending spawn events for reattached player's visible entities",
			zap.Uint64("client_id", c.ID),
			zap.Int64("character_id", int64(playerEntityID)),
			zap.Int("visible_count", len(observerVis.Known)),
		)

		for targetHandle, targetEntityID := range observerVis.Known {
			if shard.world.Alive(targetHandle) {
				// Send spawn event for each visible entity
				shard.PublishEventAsync(
					ecs.NewEntitySpawnEvent(playerEntityID, targetEntityID, targetHandle, character.Layer),
					eventbus.PriorityMedium,
				)
			}
		}
	}

	shard.ChunkManager().EnableChunkLoadEvents(playerEntityID, c.StreamEpoch.Load())

	// Enqueue inventory snapshot job to be processed on the ECS tick thread (avoids concurrent map access)
	if handle != types.InvalidHandle {
		_ = shard.ServerInbox().Enqueue(&network.ServerJob{
			JobType:  network.JobSendInventorySnapshot,
			TargetID: playerEntityID,
			Payload:  &network.InventorySnapshotJobPayload{Handle: handle},
		})
	}

	g.logger.Info("Player reattached to existing entity",
		zap.Uint64("client_id", c.ID),
		zap.Int64("character_id", int64(playerEntityID)),
		zap.Int("layer", character.Layer),
		zap.Duration("detached_duration", detachedDuration),
		zap.Int("pos_x", posX),
		zap.Int("pos_y", posY),
	)

	return true
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

	select {
	case <-c.Done():
		return
	default:
	}

	// Send enter world after chunks are sent (signals "ready to render")
	enterWorld := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_PlayerEnterWorld{
			PlayerEnterWorld: &netproto.S2C_PlayerEnterWorld{
				EntityId:     uint64(entityID),
				Name:         character.Name,
				CoordPerTile: _const.CoordPerTile,
				ChunkSize:    _const.ChunkSize,
				TickRate:     uint32(g.cfg.Game.TickRate),
				StreamEpoch:  c.StreamEpoch.Load(),
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

// enrichWithMissingDefaults enriches database inventories with missing default inventories by kind+key
func (g *Game) enrichWithMissingDefaults(dbInventories []inventory.InventoryDataV1) []inventory.InventoryDataV1 {
	// Create map of existing inventories by kind+key for quick lookup
	existing := make(map[string]bool)
	for _, inv := range dbInventories {
		key := fmt.Sprintf("%d_%d", inv.Kind, inv.Key)
		existing[key] = true
	}

	result := make([]inventory.InventoryDataV1, len(dbInventories))
	copy(result, dbInventories)

	// Define required default inventories by kind+key
	defaultInventories := []inventory.InventoryDataV1{
		{
			Kind:    uint8(_const.InventoryGrid),
			Key:     0, // Default key for Grid inventory
			Width:   inventory.DefaultBackpackWidth,
			Height:  inventory.DefaultBackpackHeight,
			Version: 1,
			Items:   []inventory.InventoryItemV1{}, // Empty inventory
		},
		{
			Kind:    uint8(_const.InventoryEquipment),
			Key:     0, // Default key for Equipment inventory
			Version: 1,
			Items:   []inventory.InventoryItemV1{}, // Empty inventory
		},
		{
			Kind:    uint8(_const.InventoryHand),
			Key:     0,
			Version: 1,
			Items:   []inventory.InventoryItemV1{},
		},
	}

	// Add missing default inventories
	for _, defaultInv := range defaultInventories {
		key := fmt.Sprintf("%d_%d", defaultInv.Kind, defaultInv.Key)
		if !existing[key] {
			g.logger.Debug("Adding missing default inventory",
				zap.Uint8("kind", defaultInv.Kind),
				zap.Uint32("key", defaultInv.Key))
			result = append(result, defaultInv)
		}
	}

	return result
}
