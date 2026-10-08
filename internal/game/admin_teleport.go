package game

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"origin/internal/characterattrs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"
	"time"
)

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

	if g.transferService == nil {
		return fmt.Errorf("transfer service unavailable")
	}
	return g.transferService.RequestTransfer(PlayerTransferRequest{
		PlayerID:              playerID,
		SourceLayer:           sourceLayer,
		TargetLayer:           resolvedLayer,
		TargetX:               targetX,
		TargetY:               targetY,
		IgnoreObjectCollision: true,
		Cause:                 PlayerTransferCauseAdminTeleport,
	})
}

func (g *Game) spawnTeleportedPlayer(
	client *network.Client,
	shard *Shard,
	character repository.Character,
	x,
	y int,
	ignoreObjectCollision bool,
	runtimeHealth ...components.EntityHealth,
) (types.Handle, error) {
	var runtime *playerRuntimeState
	if len(runtimeHealth) > 0 {
		runtime = &playerRuntimeState{Health: runtimeHealth[0]}
	}
	return g.spawnTeleportedPlayerWithRuntime(client, shard, character, x, y, ignoreObjectCollision, runtime, false)
}

func (g *Game) spawnTeleportedPlayerWithRuntime(
	client *network.Client,
	shard *Shard,
	character repository.Character,
	x, y int,
	ignoreObjectCollision bool,
	runtime *playerRuntimeState,
	deferAttachment bool,
) (types.Handle, error) {
	if client == nil {
		return types.InvalidHandle, fmt.Errorf("nil client")
	}
	if !deferAttachment {
		select {
		case <-client.Done():
			return types.InvalidHandle, fmt.Errorf("client disconnected")
		default:
		}
	}
	// After an accepted transfer capture, restoring a vulnerable body and carry
	// state is independent of whether its connection is still open.
	ctx, cancel := context.WithTimeout(context.Background(), g.cfg.Game.SpawnTimeout)
	defer cancel()

	playerEntityID := types.EntityID(character.ID)
	if err := shard.PrepareEntityAOI(ctx, playerEntityID, x, y); err != nil {
		return types.InvalidHandle, err
	}

	normalizedAttributes, _ := characterattrs.FromRaw(character.Attributes)
	profileExperience, profileSkills, profileDiscovery := loadCharacterProfileData(character, g.logger)
	pos := spawnPos{X: x, Y: y}
	setupFn := g.buildPlayerSetupWithRuntimeFunc(ctx, character, pos, normalizedAttributes, profileExperience, profileSkills, profileDiscovery, runtime)
	handle, spawnErr := shard.trySpawnPlayerWithPolicy(x, y, character, setupFn, SpawnCollisionPolicy{
		IgnoreObjectCollision: ignoreObjectCollision,
	})
	if handle == types.InvalidHandle {
		shard.mu.Lock()
		shard.UnregisterEntityAOI(playerEntityID)
		shard.mu.Unlock()
		if spawnErr != nil {
			return types.InvalidHandle, spawnErr
		}
		return types.InvalidHandle, fmt.Errorf("spawn blocked")
	}

	shard.mu.Lock()
	defer shard.mu.Unlock()
	charEntities := ecs.GetResource[ecs.CharacterEntities](shard.world)
	nextSaveAt := g.clock.GameNow().Add(g.cfg.Game.PlayerSaveInterval)
	charEntities.Add(playerEntityID, handle, nextSaveAt)

	client.Layer = character.Layer
	if deferAttachment {
		// Transfer participants restore their state before the session is exposed
		// to disconnect/expiry; the caller completes attachment afterwards.
		return handle, nil
	}
	if g.attachClientToWorldLocked(shard, client, playerEntityID, character, handle) {
		g.ensureObserverVisibilityImmediate(shard.world, handle)
	} else {
		// The restored body remains authoritative even if its connection closes
		// during setup. Queue logout rather than creating a duplicate rollback.
		delay := time.Duration(shard.cfg.Game.DisconnectDelay) * time.Second
		shard.queuePlayerLogoutLocked(playerEntityID, handle, ecs.GetResource[ecs.TimeState](shard.world).Now, delay)
	}
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

func (g *Game) sendTeleportSystemMessageToClient(c *network.Client, text string) {
	if c == nil {
		return
	}
	response := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_Chat{
			Chat: &netproto.S2C_ChatMessage{
				Channel:      netproto.ChatChannel_CHAT_CHANNEL_LOCAL,
				FromEntityId: 0,
				FromName:     "[Server]",
				Text:         text,
			},
		},
	}
	data, err := proto.Marshal(response)
	if err != nil {
		return
	}
	c.Send(data)
}

// invalidateEntityVisibility notifies existing observers without waiting for the
// periodic vision refresh. Call while holding the world's mutation lock.
func invalidateEntityVisibility(
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
