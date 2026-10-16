package game

import (
	"context"
	"fmt"
	"time"

	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/lifecycle"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/playerstate"
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
	g.transferInFlight[req.PlayerID] = make(chan struct{})
	g.transferMu.Unlock()

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		s.executeTransfer(req)
	}()
	return nil
}

func (s *PlayerTransferService) executeTransfer(req PlayerTransferRequest) {
	g := s.game
	defer func() {
		g.transferMu.Lock()
		done := g.transferInFlight[req.PlayerID]
		delete(g.transferInFlight, req.PlayerID)
		if done != nil {
			close(done)
		}
		g.transferMu.Unlock()
	}()

	sourceShard := g.shardManager.GetShard(req.SourceLayer)
	resultErr := fmt.Errorf("source layer unavailable")
	resultDelivered := false
	defer func() {
		if !resultDelivered {
			s.completeResult(req, sourceShard, types.InvalidHandle, resultErr)
		}
	}()
	if sourceShard == nil {
		return
	}
	targetShard := g.shardManager.GetShard(req.TargetLayer)
	if targetShard == nil {
		resultErr = fmt.Errorf("target layer unavailable")
		s.sendFailureToLayer(req, fmt.Sprintf("Teleport failed: target layer %d not found.", req.TargetLayer))
		return
	}

	characterTemplate, err := g.db.Queries().GetCharacter(g.ctx, int64(req.PlayerID))
	if err != nil {
		resultErr = fmt.Errorf("load transfer character: %w", err)
		s.sendFailureToLayer(req, "Teleport failed: character not found.")
		return
	}

	snapshot, detachErr := s.detachTransferSource(req, sourceShard, characterTemplate)
	defer releaseTransferResources(snapshot.ParticipantStates)
	if detachErr != nil {
		resultErr = detachErr
		s.sendFailureToLayer(req, "Teleport failed: could not detach current entity.")
		return
	}
	if snapshot.Client != nil {
		snapshot.Client.Layer = req.TargetLayer
	}

	characterTemplate = snapshot.Character
	characterSpawn := snapshot.Character
	characterSpawn.Layer = req.TargetLayer
	characterSpawn.X = req.TargetX
	characterSpawn.Y = req.TargetY

	// Spawn calls this before releasing its owning shard. No world update may
	// observe the restored body between participant restoration and attachment.
	finish := func(shard *Shard, handle types.Handle, actual repository.Character, restoreErr error) {
		if restoreErr == nil {
			req.TargetLayer, req.TargetX, req.TargetY = actual.Layer, actual.X, actual.Y
			sourceShard.offlineHealth.Delete(req.PlayerID)
			s.restoreParticipantsOnTargetLocked(req, shard, handle, snapshot.ParticipantStates)
		} else {
			s.restoreParticipantsOnRollbackLocked(req, shard, handle, snapshot.ParticipantStates)
		}
		if req.OnResult != nil {
			req.OnResult(shard, handle, restoreErr)
		}
		resultDelivered = true
		if g.attachClientToWorldLocked(shard, snapshot.Client, req.PlayerID, actual, handle) {
			g.ensureObserverVisibilityImmediate(shard.world, handle)
		} else {
			delay := time.Duration(shard.cfg.Game.DisconnectDelay) * time.Second
			shard.queuePlayerLogoutLocked(req.PlayerID, handle, ecs.GetResource[ecs.TimeState](shard.world).Now, delay)
		}
	}
	finishTarget := func(handle types.Handle, actual repository.Character) {
		finish(targetShard, handle, actual, nil)
	}
	var spawnErr error
	if req.SpawnTarget != nil {
		_, characterSpawn, spawnErr = req.SpawnTarget(targetShard, characterSpawn, snapshot.RuntimeState, finishTarget)
	} else {
		_, spawnErr = g.spawnTeleportedPlayerWithRuntime(snapshot.Client, targetShard, characterSpawn, req.TargetX, req.TargetY, req.IgnoreObjectCollision, &snapshot.RuntimeState, finishTarget)
	}
	if spawnErr != nil {
		resultErr = spawnErr
		rollbackChar := characterTemplate
		rollbackChar.Layer = snapshot.SourceLayer
		rollbackChar.X = snapshot.SourceX
		rollbackChar.Y = snapshot.SourceY
		if snapshot.Client != nil {
			snapshot.Client.Layer = snapshot.SourceLayer
		}
		_, rollbackErr := g.spawnTeleportedPlayerWithRuntime(snapshot.Client, sourceShard, rollbackChar, snapshot.SourceX, snapshot.SourceY, req.IgnoreObjectCollision, &snapshot.RuntimeState,
			func(handle types.Handle, actual repository.Character) {
				finish(sourceShard, handle, actual, spawnErr)
			})
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
		g.sendTeleportSystemMessageToClient(snapshot.Client, "Teleport failed, restored previous position.")
		return
	}

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

func releaseTransferResources(states map[string]any) {
	for _, state := range states {
		if resource, ok := state.(interface{ releaseTransferResources() }); ok {
			resource.releaseTransferResources()
		}
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

	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.ClientsMu.RLock()
	client, exists := shard.Clients[req.PlayerID]
	shard.ClientsMu.RUnlock()
	if !exists || client == nil {
		return snapshot, fmt.Errorf("client not bound")
	}
	snapshot.Client = client

	playerHandle := shard.world.GetHandleByEntityID(req.PlayerID)
	if playerHandle == types.InvalidHandle || !shard.world.Alive(playerHandle) {
		return snapshot, fmt.Errorf("entity not alive")
	}
	if ecs.InventoryHandleReserved(shard.world, playerHandle) {
		return snapshot, fmt.Errorf("inventory operation is still being committed")
	}
	// Most transfers prohibit any incapacity. A feature may supply a narrower
	// authoritative policy without changing stairs or administrator teleports.
	if req.SourcePolicy != nil {
		if err := req.SourcePolicy(shard, playerHandle, client); err != nil {
			return snapshot, err
		}
	} else if playerstate.IsIncapacitated(shard.world, playerHandle) {
		return snapshot, fmt.Errorf("cannot teleport during KO or while lying")
	}

	transform, hasTransform := ecs.GetComponent[components.Transform](shard.world, playerHandle)
	if !hasTransform {
		return snapshot, fmt.Errorf("missing transform")
	}
	snapshot.RuntimeState.Health, _ = ecs.GetComponent[components.EntityHealth](shard.world, playerHandle)
	combat, prepared := ecs.GetResource[ecs.CombatActivityState](shard.world).Capture(playerHandle)
	if shard.creatureDamage != nil && !prepared {
		return snapshot, ecs.ErrCombatActivityUnprepared
	}
	if err := ecs.ValidateCombatState(combat); err != nil {
		return snapshot, err
	}
	snapshot.RuntimeState.CombatState = combat
	snapshot.Character.IsLying = snapshot.RuntimeState.Health.IsLying
	snapshot.SourceX = int(transform.X)
	snapshot.SourceY = int(transform.Y)
	cooldowns, _ := ecs.GetComponent[components.ActionCooldowns](shard.world, playerHandle)
	encodedCooldowns, err := cooldowns.MarshalActive(ecs.GetResource[ecs.TimeState](shard.world).UnixMs)
	if err != nil {
		return snapshot, fmt.Errorf("capture cooldowns before transfer: %w", err)
	}
	snapshot.Character.ActionCooldowns = []byte(encodedCooldowns)

	if shard.characterSaver != nil {
		if err := shard.characterSaver.SaveSync(shard.world, req.PlayerID, playerHandle); err != nil {
			s.logger.Warn("Transfer: SaveSync failed",
				zap.Uint64("player_id", uint64(req.PlayerID)),
				zap.Error(err))
			// A retained source snapshot must not retry after the player moves to another shard.
			return snapshot, fmt.Errorf("save character before transfer: %w", err)
		}
		// Both destination and rollback need the profile and stats just saved,
		// not the character row loaded before acquiring the source lock.
		readCtx, cancel := context.WithTimeout(s.game.ctx, 5*time.Second)
		savedCharacter, err := s.game.db.Queries().GetCharacter(readCtx, int64(req.PlayerID))
		cancel()
		if err != nil {
			return snapshot, fmt.Errorf("reload saved transfer character: %w", err)
		}
		snapshot.Character = savedCharacter
	}

	// Saving or reloading can block while the connection closes. Recheck immediately
	// before destructive participant capture under the same owning lock.
	if req.SourcePolicy != nil {
		if err := req.SourcePolicy(shard, playerHandle, client); err != nil {
			return snapshot, err
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
	if shard.actionService != nil {
		shard.actionService.Cancel(shard.world, req.PlayerID, playerHandle)
	}

	s.game.sendPlayerLeaveWorld(client, req.PlayerID)
	client.InWorld.Store(false)
	shard.soundEvents.Detach(playerHandle, client.ID)
	invalidateEntityVisibility(shard.world, shard.layer, playerHandle, req.PlayerID, shard.EventBus())

	// Reset transient per-player state; carry is preserved only via transfer participants.
	linkState := ecs.GetResource[ecs.LinkState](shard.world)
	linkState.ClearIntent(req.PlayerID)
	linkState.RemoveLink(req.PlayerID)
	openState := ecs.GetResource[ecs.OpenContainerState](shard.world)
	openState.CloseAllForPlayer(req.PlayerID)
	ecs.GetResource[ecs.OpenedWindowsState](shard.world).ClearPlayer(req.PlayerID)
	ecs.ClearPendingAdminClicks(shard.world, req.PlayerID)
	ecs.RemoveComponent[components.PendingInteraction](shard.world, playerHandle)
	ecs.RemoveComponent[components.PendingContextAction](shard.world, playerHandle)
	ecs.RemoveComponent[components.PendingBuildPlacement](shard.world, playerHandle)
	ecs.RemoveComponent[components.PendingLiftTransition](shard.world, playerHandle)
	cyclicaction.Clear(shard.world, playerHandle)
	ecs.RemoveComponent[components.ActiveCraft](shard.world, playerHandle)
	ecs.WithComponent(shard.world, playerHandle, func(col *components.Collider) {
		col.Phantom = nil
	})

	if chunkRef, hasChunkRef := ecs.GetComponent[components.ChunkRef](shard.world, playerHandle); hasChunkRef {
		if chunk := shard.chunkManager.GetChunk(types.ChunkCoord{X: chunkRef.CurrentChunkX, Y: chunkRef.CurrentChunkY}); chunk != nil {
			chunk.Spatial().RemoveDynamic(playerHandle, int(transform.X), int(transform.Y))
		}
	}

	shard.offlineHealth.Store(req.PlayerID, snapshot.RuntimeState)
	lifecycle.DeleteOwnedInventoryContainers(shard.world, req.PlayerID)
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

func (s *PlayerTransferService) restoreParticipantsOnTargetLocked(
	req PlayerTransferRequest,
	targetShard *Shard,
	playerHandle types.Handle,
	states map[string]any,
) {
	if targetShard == nil || playerHandle == types.InvalidHandle {
		return
	}
	if !targetShard.world.Alive(playerHandle) {
		return
	}
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

func (s *PlayerTransferService) restoreParticipantsOnRollbackLocked(
	req PlayerTransferRequest,
	sourceShard *Shard,
	playerHandle types.Handle,
	states map[string]any,
) {
	if sourceShard == nil || playerHandle == types.InvalidHandle {
		return
	}
	if !sourceShard.world.Alive(playerHandle) {
		return
	}
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
	// A feature completion owns its guarded failure notification. Looking up
	// the current layer client here could report stale work to a newer session.
	if s == nil || s.game == nil || text == "" || req.OnResult != nil {
		return
	}
	s.game.sendTeleportSystemMessage(req.PlayerID, req.SourceLayer, text)
}

func (s *PlayerTransferService) completeResult(req PlayerTransferRequest, shard *Shard, handle types.Handle, err error) {
	if req.OnResult == nil || shard == nil {
		return
	}
	shard.mu.Lock()
	defer shard.mu.Unlock()
	req.OnResult(shard, handle, err)
}
