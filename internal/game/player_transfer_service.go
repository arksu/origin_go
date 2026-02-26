package game

import (
	"fmt"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

type PlayerTransferService struct {
	game         *Game
	logger       *zap.Logger
	participants []PlayerTransferParticipant
}

func NewPlayerTransferService(g *Game, logger *zap.Logger) *PlayerTransferService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &PlayerTransferService{
		game:         g,
		logger:       logger,
		participants: make([]PlayerTransferParticipant, 0, 2),
	}
}

func (s *PlayerTransferService) RegisterParticipant(participant PlayerTransferParticipant) {
	if s == nil || participant == nil {
		return
	}
	key := participant.Key()
	if key == "" {
		return
	}
	for _, existing := range s.participants {
		if existing != nil && existing.Key() == key {
			return
		}
	}
	s.participants = append(s.participants, participant)
}

func (s *PlayerTransferService) RequestTransfer(req PlayerTransferRequest) error {
	if s == nil || s.game == nil {
		return fmt.Errorf("transfer service unavailable")
	}
	g := s.game
	if req.PlayerID == 0 {
		return fmt.Errorf("invalid player")
	}
	if req.TargetLayer < 0 || req.TargetLayer >= g.cfg.Game.MaxLayers {
		return fmt.Errorf("invalid layer: %d", req.TargetLayer)
	}
	if !g.isValidSpawnPos(req.TargetX, req.TargetY) {
		return fmt.Errorf("target (%d,%d) outside world bounds", req.TargetX, req.TargetY)
	}

	g.transferMu.Lock()
	if _, exists := g.transferInFlight[req.PlayerID]; exists {
		g.transferMu.Unlock()
		return fmt.Errorf("transfer already in progress")
	}
	g.transferInFlight[req.PlayerID] = struct{}{}
	g.transferMu.Unlock()

	go s.executeTransfer(req)
	return nil
}

func (s *PlayerTransferService) executeTransfer(req PlayerTransferRequest) {
	g := s.game
	defer func() {
		g.transferMu.Lock()
		delete(g.transferInFlight, req.PlayerID)
		g.transferMu.Unlock()
	}()

	sourceShard := g.shardManager.GetShard(req.SourceLayer)
	if sourceShard == nil {
		return
	}
	targetShard := g.shardManager.GetShard(req.TargetLayer)
	if targetShard == nil {
		s.sendFailureToLayer(req, fmt.Sprintf("Teleport failed: target layer %d not found.", req.TargetLayer))
		return
	}

	characterTemplate, err := g.db.Queries().GetCharacter(g.ctx, int64(req.PlayerID))
	if err != nil {
		s.sendFailureToLayer(req, "Teleport failed: character not found.")
		return
	}

	snapshot, detachErr := s.detachTransferSource(req, sourceShard, characterTemplate)
	if detachErr != nil {
		s.sendFailureToLayer(req, "Teleport failed: could not detach current entity.")
		return
	}
	if snapshot.Client != nil {
		snapshot.Client.Layer = req.TargetLayer
	}

	characterSpawn := characterTemplate
	characterSpawn.Layer = req.TargetLayer
	characterSpawn.X = req.TargetX
	characterSpawn.Y = req.TargetY

	targetHandle, spawnErr := g.spawnTeleportedPlayer(snapshot.Client, targetShard, characterSpawn, req.TargetX, req.TargetY, req.IgnoreObjectCollision)
	if spawnErr != nil {
		rollbackChar := characterTemplate
		rollbackChar.Layer = snapshot.SourceLayer
		rollbackChar.X = snapshot.SourceX
		rollbackChar.Y = snapshot.SourceY
		if snapshot.Client != nil {
			snapshot.Client.Layer = snapshot.SourceLayer
		}
		rollbackHandle, rollbackErr := g.spawnTeleportedPlayer(snapshot.Client, sourceShard, rollbackChar, snapshot.SourceX, snapshot.SourceY, req.IgnoreObjectCollision)
		if rollbackErr != nil {
			if snapshot.Client != nil {
				snapshot.Client.SendError(netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, "Teleport failed and rollback failed. Reconnect required.")
			}
			s.logger.Error("Transfer rollback failed",
				zap.Uint64("player_id", uint64(req.PlayerID)),
				zap.Int("source_layer", snapshot.SourceLayer),
				zap.Int("source_x", snapshot.SourceX),
				zap.Int("source_y", snapshot.SourceY),
				zap.Error(rollbackErr),
			)
			return
		}
		s.restoreParticipantsOnRollback(req, sourceShard, rollbackHandle, snapshot.ParticipantStates)
		g.sendTeleportSystemMessageToClient(snapshot.Client, "Teleport failed, restored previous position.")
		return
	}

	s.restoreParticipantsOnTarget(req, targetShard, targetHandle, snapshot.ParticipantStates)

	if err := g.db.Queries().UpdateCharacterPositionAndLayer(g.ctx, repository.UpdateCharacterPositionAndLayerParams{
		ID:    int64(req.PlayerID),
		X:     req.TargetX,
		Y:     req.TargetY,
		Layer: req.TargetLayer,
	}); err != nil {
		s.logger.Warn("Transfer: failed to persist final position/layer",
			zap.Uint64("player_id", uint64(req.PlayerID)),
			zap.Int("target_layer", req.TargetLayer),
			zap.Int("target_x", req.TargetX),
			zap.Int("target_y", req.TargetY),
			zap.Error(err),
		)
	}

	switch req.Cause {
	case PlayerTransferCauseAdminTeleport:
		g.sendTeleportSystemMessageToClient(snapshot.Client, fmt.Sprintf("Teleported to (%d, %d) layer %d.", req.TargetX, req.TargetY, req.TargetLayer))
	default:
		g.sendTeleportSystemMessageToClient(snapshot.Client, "Transfer complete.")
	}
}

func (s *PlayerTransferService) detachTransferSource(
	req PlayerTransferRequest,
	shard *Shard,
	character repository.Character,
) (PlayerTransferSnapshot, error) {
	snapshot := PlayerTransferSnapshot{
		SourceLayer:       req.SourceLayer,
		Character:         character,
		ParticipantStates: make(map[string]any, len(s.participants)),
	}

	shard.ClientsMu.RLock()
	client, exists := shard.Clients[req.PlayerID]
	shard.ClientsMu.RUnlock()
	if !exists || client == nil {
		return snapshot, fmt.Errorf("client not bound")
	}
	snapshot.Client = client

	shard.mu.Lock()
	defer shard.mu.Unlock()

	playerHandle := shard.world.GetHandleByEntityID(req.PlayerID)
	if playerHandle == types.InvalidHandle || !shard.world.Alive(playerHandle) {
		return snapshot, fmt.Errorf("entity not alive")
	}

	transform, hasTransform := ecs.GetComponent[components.Transform](shard.world, playerHandle)
	if !hasTransform {
		return snapshot, fmt.Errorf("missing transform")
	}
	snapshot.SourceX = int(transform.X)
	snapshot.SourceY = int(transform.Y)

	if shard.characterSaver != nil {
		if err := shard.characterSaver.SaveSync(shard.world, req.PlayerID, playerHandle); err != nil {
			s.logger.Warn("Transfer: SaveSync failed",
				zap.Uint64("player_id", uint64(req.PlayerID)),
				zap.Error(err))
		}
	}

	for _, participant := range s.participants {
		if participant == nil {
			continue
		}
		state, err := participant.CaptureSource(s.game, shard, req, playerHandle)
		if err != nil {
			return snapshot, fmt.Errorf("participant %s capture failed: %w", participant.Key(), err)
		}
		snapshot.ParticipantStates[participant.Key()] = state
	}

	s.game.sendPlayerLeaveWorld(client, req.PlayerID)
	client.InWorld.Store(false)
	invalidateVisibilityForTeleport(shard.world, shard.layer, playerHandle, req.PlayerID, shard.EventBus())

	// Reset transient per-player state; carry is preserved only via transfer participants.
	linkState := ecs.GetResource[ecs.LinkState](shard.world)
	linkState.ClearIntent(req.PlayerID)
	linkState.RemoveLink(req.PlayerID)
	openState := ecs.GetResource[ecs.OpenContainerState](shard.world)
	openState.CloseAllForPlayer(req.PlayerID)
	ecs.GetResource[ecs.OpenedWindowsState](shard.world).ClearPlayer(req.PlayerID)
	ecs.GetResource[ecs.PendingAdminSpawn](shard.world).Clear(req.PlayerID)
	ecs.GetResource[ecs.PendingAdminTeleport](shard.world).Clear(req.PlayerID)
	ecs.RemoveComponent[components.PendingInteraction](shard.world, playerHandle)
	ecs.RemoveComponent[components.PendingContextAction](shard.world, playerHandle)
	ecs.RemoveComponent[components.PendingBuildPlacement](shard.world, playerHandle)
	ecs.RemoveComponent[components.PendingLiftTransition](shard.world, playerHandle)
	ecs.RemoveComponent[components.ActiveCyclicAction](shard.world, playerHandle)
	ecs.RemoveComponent[components.ActiveCraft](shard.world, playerHandle)
	ecs.WithComponent(shard.world, playerHandle, func(col *components.Collider) {
		col.Phantom = nil
	})

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

func (s *PlayerTransferService) restoreParticipantsOnTarget(
	req PlayerTransferRequest,
	targetShard *Shard,
	playerHandle types.Handle,
	states map[string]any,
) {
	if targetShard == nil || playerHandle == types.InvalidHandle || !targetShard.world.Alive(playerHandle) {
		return
	}
	targetShard.mu.Lock()
	defer targetShard.mu.Unlock()
	for _, participant := range s.participants {
		if participant == nil {
			continue
		}
		state := states[participant.Key()]
		if err := participant.RestoreTarget(s.game, targetShard, req, playerHandle, state); err != nil {
			participant.OnTargetRestoreFailure(s.game, targetShard, req, playerHandle, state, err)
		}
	}
}

func (s *PlayerTransferService) restoreParticipantsOnRollback(
	req PlayerTransferRequest,
	sourceShard *Shard,
	playerHandle types.Handle,
	states map[string]any,
) {
	if sourceShard == nil || playerHandle == types.InvalidHandle || !sourceShard.world.Alive(playerHandle) {
		return
	}
	sourceShard.mu.Lock()
	defer sourceShard.mu.Unlock()
	for i := len(s.participants) - 1; i >= 0; i-- {
		participant := s.participants[i]
		if participant == nil {
			continue
		}
		state := states[participant.Key()]
		if err := participant.RestoreSourceRollback(s.game, sourceShard, req, playerHandle, state); err != nil {
			s.logger.Warn("Transfer participant rollback restore failed",
				zap.String("participant", participant.Key()),
				zap.Uint64("player_id", uint64(req.PlayerID)),
				zap.Error(err),
			)
		}
	}
}

func (s *PlayerTransferService) sendFailureToLayer(req PlayerTransferRequest, text string) {
	if s == nil || s.game == nil || text == "" {
		return
	}
	switch req.Cause {
	case PlayerTransferCauseAdminTeleport:
		s.game.sendTeleportSystemMessage(req.PlayerID, req.SourceLayer, text)
	default:
		s.game.sendTeleportSystemMessage(req.PlayerID, req.SourceLayer, text)
	}
}
