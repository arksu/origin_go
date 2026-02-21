package game

import (
	"context"
	"fmt"
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type adminTeleportRequest struct {
	PlayerID     types.EntityID
	SourceLayer  int
	TargetLayer  int
	TargetX      int
	TargetY      int
	RequestedAt  time.Time
	OriginalText string
}

type adminTeleportSnapshot struct {
	client      *network.Client
	sourceLayer int
	sourceX     int
	sourceY     int
}

// RequestAdminTeleport schedules full leave+respawn teleport flow for an admin command.
func (g *Game) RequestAdminTeleport(playerID types.EntityID, sourceLayer int, targetX, targetY int, targetLayer *int) error {
	resolvedLayer := sourceLayer
	if targetLayer != nil {
		resolvedLayer = *targetLayer
	}

	if resolvedLayer < 0 || resolvedLayer >= g.cfg.Game.MaxLayers {
		return fmt.Errorf("invalid layer: %d", resolvedLayer)
	}
	if !g.isValidSpawnPos(targetX, targetY) {
		return fmt.Errorf("target (%d,%d) outside world bounds", targetX, targetY)
	}

	g.teleportMu.Lock()
	if _, exists := g.teleportInFlight[playerID]; exists {
		g.teleportMu.Unlock()
		return fmt.Errorf("teleport already in progress")
	}
	g.teleportInFlight[playerID] = struct{}{}
	g.teleportMu.Unlock()

	req := adminTeleportRequest{
		PlayerID:    playerID,
		SourceLayer: sourceLayer,
		TargetLayer: resolvedLayer,
		TargetX:     targetX,
		TargetY:     targetY,
		RequestedAt: time.Now(),
	}

	go g.executeAdminTeleport(req)
	return nil
}

func (g *Game) executeAdminTeleport(req adminTeleportRequest) {
	defer func() {
		g.teleportMu.Lock()
		delete(g.teleportInFlight, req.PlayerID)
		g.teleportMu.Unlock()
	}()

	sourceShard := g.shardManager.GetShard(req.SourceLayer)
	if sourceShard == nil {
		return
	}
	targetShard := g.shardManager.GetShard(req.TargetLayer)
	if targetShard == nil {
		g.sendTeleportSystemMessage(req.PlayerID, req.SourceLayer, fmt.Sprintf("Teleport failed: target layer %d not found.", req.TargetLayer))
		return
	}

	characterBefore, beforeErr := g.db.Queries().GetCharacter(g.ctx, int64(req.PlayerID))
	if beforeErr != nil {
		g.sendTeleportSystemMessage(req.PlayerID, req.SourceLayer, "Teleport failed: character not found.")
		return
	}

	snapshot, detachErr := g.detachTeleportSource(req, sourceShard)
	if detachErr != nil {
		g.sendTeleportSystemMessage(req.PlayerID, req.SourceLayer, "Teleport failed: could not detach current entity.")
		return
	}
	snapshot.client.Layer = req.TargetLayer

	characterAfter, afterErr := g.db.Queries().GetCharacter(g.ctx, int64(req.PlayerID))
	if afterErr != nil {
		g.logger.Warn("Teleport: failed to reload character after SaveSync, fallback to prefetch",
			zap.Uint64("player_id", uint64(req.PlayerID)),
			zap.Error(afterErr))
		characterAfter = characterBefore
	}
	characterAfter.Layer = req.TargetLayer
	characterAfter.X = req.TargetX
	characterAfter.Y = req.TargetY

	if _, spawnErr := g.spawnTeleportedPlayer(snapshot.client, targetShard, characterAfter, req.TargetX, req.TargetY, true); spawnErr != nil {
		rollbackChar := characterBefore
		rollbackChar.Layer = snapshot.sourceLayer
		rollbackChar.X = snapshot.sourceX
		rollbackChar.Y = snapshot.sourceY
		snapshot.client.Layer = snapshot.sourceLayer
		if _, rollbackErr := g.spawnTeleportedPlayer(snapshot.client, sourceShard, rollbackChar, snapshot.sourceX, snapshot.sourceY, true); rollbackErr != nil {
			snapshot.client.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Teleport failed and rollback failed. Reconnect required.")
			g.logger.Error("Teleport rollback failed",
				zap.Uint64("player_id", uint64(req.PlayerID)),
				zap.Int("source_layer", snapshot.sourceLayer),
				zap.Int("source_x", snapshot.sourceX),
				zap.Int("source_y", snapshot.sourceY),
				zap.Error(rollbackErr))
			return
		}
		g.sendTeleportSystemMessage(req.PlayerID, snapshot.sourceLayer, "Teleport failed, restored previous position.")
		return
	}

	if _, err := g.db.Pool().Exec(
		g.ctx,
		"UPDATE character SET x=$2, y=$3, layer=$4 WHERE id=$1 AND deleted_at IS NULL",
		int64(req.PlayerID), req.TargetX, req.TargetY, req.TargetLayer,
	); err != nil {
		g.logger.Warn("Teleport: failed to persist final position/layer",
			zap.Uint64("player_id", uint64(req.PlayerID)),
			zap.Int("target_layer", req.TargetLayer),
			zap.Int("target_x", req.TargetX),
			zap.Int("target_y", req.TargetY),
			zap.Error(err))
	}

	g.sendTeleportSystemMessage(req.PlayerID, req.TargetLayer, fmt.Sprintf("Teleported to (%d, %d) layer %d.", req.TargetX, req.TargetY, req.TargetLayer))
}

func (g *Game) detachTeleportSource(req adminTeleportRequest, shard *Shard) (adminTeleportSnapshot, error) {
	snapshot := adminTeleportSnapshot{sourceLayer: req.SourceLayer}

	shard.mu.Lock()
	defer shard.mu.Unlock()

	shard.ClientsMu.RLock()
	client, exists := shard.Clients[req.PlayerID]
	shard.ClientsMu.RUnlock()
	if !exists || client == nil {
		return snapshot, fmt.Errorf("client not bound")
	}

	playerHandle := shard.world.GetHandleByEntityID(req.PlayerID)
	if playerHandle == types.InvalidHandle || !shard.world.Alive(playerHandle) {
		return snapshot, fmt.Errorf("entity not alive")
	}

	transform, hasTransform := ecs.GetComponent[components.Transform](shard.world, playerHandle)
	if !hasTransform {
		return snapshot, fmt.Errorf("missing transform")
	}
	snapshot.sourceX = int(transform.X)
	snapshot.sourceY = int(transform.Y)
	snapshot.client = client

	if shard.characterSaver != nil {
		if err := shard.characterSaver.SaveSync(shard.world, req.PlayerID, playerHandle); err != nil {
			g.logger.Warn("Teleport: SaveSync failed",
				zap.Uint64("player_id", uint64(req.PlayerID)),
				zap.Error(err))
		}
	}

	g.sendPlayerLeaveWorld(client, req.PlayerID)
	client.InWorld.Store(false)

	invalidateVisibilityForTeleport(shard.world, shard.layer, playerHandle, req.PlayerID, shard.EventBus())

	// Clear stateful per-player resources before despawn.
	linkState := ecs.GetResource[ecs.LinkState](shard.world)
	linkState.ClearIntent(req.PlayerID)
	linkState.RemoveLink(req.PlayerID)
	openState := ecs.GetResource[ecs.OpenContainerState](shard.world)
	openState.CloseAllForPlayer(req.PlayerID)
	ecs.GetResource[ecs.PendingAdminSpawn](shard.world).Clear(req.PlayerID)
	ecs.GetResource[ecs.PendingAdminTeleport](shard.world).Clear(req.PlayerID)

	if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](shard.world, playerHandle); hasChunkRef {
		if chunk := shard.chunkManager.GetChunk(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}); chunk != nil {
			chunk.Spatial().RemoveDynamic(playerHandle, int(transform.X), int(transform.Y))
		}
	}

	shard.world.Despawn(playerHandle)
	ecs.GetResource[ecs.CharacterEntities](shard.world).Remove(req.PlayerID)
	ecs.GetResource[ecs.DetachedEntities](shard.world).RemoveDetachedEntity(req.PlayerID)
	shard.UnregisterEntityAOI(req.PlayerID)

	shard.PlayerInbox().RemoveClient(client.ID)
	shard.ClientsMu.Lock()
	delete(shard.Clients, req.PlayerID)
	shard.ClientsMu.Unlock()

	return snapshot, nil
}

func (g *Game) spawnTeleportedPlayer(
	client *network.Client,
	shard *Shard,
	character repository.Character,
	x,
	y int,
	ignoreObjectCollision bool,
) (types.Handle, error) {
	if client == nil {
		return types.InvalidHandle, fmt.Errorf("nil client")
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.cfg.Game.SpawnTimeout)
	defer cancel()

	playerEntityID := types.EntityID(character.ID)
	if err := shard.PrepareEntityAOI(ctx, playerEntityID, x, y); err != nil {
		return types.InvalidHandle, err
	}

	normalizedAttributes, _ := characterattrs.FromRaw(character.Attributes)
	profileExperience, profileSkills, profileDiscovery := loadCharacterProfileData(character, g.logger)
	pos := spawnPos{X: x, Y: y}
	setupFn := g.buildPlayerSetupFunc(ctx, character, pos, normalizedAttributes, profileExperience, profileSkills, profileDiscovery)
	ok, handle := shard.TrySpawnPlayerWithPolicy(x, y, character, setupFn, SpawnCollisionPolicy{
		IgnoreObjectCollision: ignoreObjectCollision,
	})
	if !ok || handle == types.InvalidHandle {
		shard.mu.Lock()
		shard.UnregisterEntityAOI(playerEntityID)
		shard.mu.Unlock()
		return types.InvalidHandle, fmt.Errorf("spawn blocked")
	}

	charEntities := ecs.GetResource[ecs.CharacterEntities](shard.world)
	nextSaveAt := g.clock.GameNow().Add(g.cfg.Game.PlayerSaveInterval)
	charEntities.Add(playerEntityID, handle, nextSaveAt)

	client.Layer = character.Layer
	shard.PlayerInbox().RemoveClient(client.ID)
	shard.ClientsMu.Lock()
	shard.Clients[playerEntityID] = client
	shard.ClientsMu.Unlock()

	client.StreamEpoch.Add(1)
	client.InWorld.Store(true)
	g.sendPlayerEnterWorld(client, playerEntityID, shard, character)
	shard.ChunkManager().EnableChunkLoadEvents(playerEntityID, client.StreamEpoch.Load())

	_ = shard.ServerInbox().Enqueue(&network.ServerJob{
		JobType:  network.JobSendInventorySnapshot,
		TargetID: playerEntityID,
		Payload:  &network.InventorySnapshotJobPayload{Handle: handle},
	})
	_ = shard.ServerInbox().Enqueue(&network.ServerJob{
		JobType:  network.JobSendCharacterProfileSnapshot,
		TargetID: playerEntityID,
		Payload:  &network.CharacterProfileSnapshotJobPayload{Handle: handle},
	})
	_ = shard.ServerInbox().Enqueue(&network.ServerJob{
		JobType:  network.JobSendPlayerStatsSnapshot,
		TargetID: playerEntityID,
		Payload:  &network.PlayerStatsSnapshotJobPayload{Handle: handle},
	})
	_ = shard.ServerInbox().Enqueue(&network.ServerJob{
		JobType:  network.JobSendMovementModeSnapshot,
		TargetID: playerEntityID,
		Payload:  &network.MovementModeSnapshotJobPayload{Handle: handle},
	})

	return handle, nil
}

func (g *Game) sendPlayerLeaveWorld(c *network.Client, entityID types.EntityID) {
	if c == nil {
		return
	}
	leaveWorld := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_PlayerLeaveWorld{
			PlayerLeaveWorld: &netproto.S2C_PlayerLeaveWorld{EntityId: uint64(entityID)},
		},
	}
	data, err := proto.Marshal(leaveWorld)
	if err != nil {
		return
	}
	c.Send(data)
}

func (g *Game) sendTeleportSystemMessage(playerID types.EntityID, layer int, text string) {
	shard := g.shardManager.GetShard(layer)
	if shard == nil {
		return
	}
	shard.SendChatMessage(playerID, netproto.ChatChannel_CHAT_CHANNEL_LOCAL, 0, "[Server]", text)
}

func invalidateVisibilityForTeleport(
	w *ecs.World,
	layer int,
	targetHandle types.Handle,
	targetEntityID types.EntityID,
	eb *eventbus.EventBus,
) {
	if w == nil || targetHandle == types.InvalidHandle {
		return
	}
	visState := ecs.GetResource[ecs.VisibilityState](w)
	despawns := make([]*ecs.EntityDespawnEvent, 0, 16)

	visState.Mu.Lock()
	if observers := visState.ObserversByVisibleTarget[targetHandle]; observers != nil {
		for observerHandle := range observers {
			observerID, ok := w.GetExternalID(observerHandle)
			if !ok {
				continue
			}
			if observerVis, exists := visState.VisibleByObserver[observerHandle]; exists {
				delete(observerVis.Known, targetHandle)
				visState.VisibleByObserver[observerHandle] = observerVis
			}
			despawns = append(despawns, ecs.NewEntityDespawnEvent(observerID, targetEntityID, layer))
		}
		delete(visState.ObserversByVisibleTarget, targetHandle)
	}

	if ownVis, exists := visState.VisibleByObserver[targetHandle]; exists {
		for knownHandle := range ownVis.Known {
			if observers := visState.ObserversByVisibleTarget[knownHandle]; observers != nil {
				delete(observers, targetHandle)
				if len(observers) == 0 {
					delete(visState.ObserversByVisibleTarget, knownHandle)
				}
			}
		}
		delete(visState.VisibleByObserver, targetHandle)
	}
	visState.Mu.Unlock()

	if eb == nil {
		return
	}
	for _, evt := range despawns {
		eb.PublishAsync(evt, eventbus.PriorityMedium)
	}
}
