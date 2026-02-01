package game

import (
	"context"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type NetworkVisibilityDispatcher struct {
	shardManager *ShardManager
	logger       *zap.Logger
}

func NewNetworkVisibilityDispatcher(shardManager *ShardManager, logger *zap.Logger) *NetworkVisibilityDispatcher {
	return &NetworkVisibilityDispatcher{
		shardManager: shardManager,
		logger:       logger,
	}
}

func (d *NetworkVisibilityDispatcher) Subscribe(eventBus *eventbus.EventBus) {
	eventBus.SubscribeAsync(ecs.TopicGameplayMovementMove, eventbus.PriorityMedium, d.handleObjectMove)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntitySpawn, eventbus.PriorityMedium, d.handleEntitySpawn)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntityDespawn, eventbus.PriorityMedium, d.handleEntityDespawn)
	eventBus.SubscribeAsync(ecs.TopicGameplayChunkUnload, eventbus.PriorityMedium, d.handleChunkUnload)
	eventBus.SubscribeAsync(ecs.TopicGameplayChunkLoad, eventbus.PriorityMedium, d.handleChunkLoad)
}

func (d *NetworkVisibilityDispatcher) handleObjectMove(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.ObjectMoveEvent)
	if !ok {
		return nil
	}

	// Get the specific shard by layer
	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}

	shard.clientsMu.RLock()
	defer shard.clientsMu.RUnlock()

	visibilityState := shard.World().VisibilityState()
	if visibilityState == nil {
		return nil
	}

	// Get the target entity's handle
	targetHandle := shard.World().GetHandleByEntityID(event.EntityID)
	if targetHandle == types.InvalidHandle {
		return nil
	}

	// Get observers who can see this target entity directly from ObserversByVisibleTarget
	visibilityState.Mu.RLock()
	observers, hasObservers := visibilityState.ObserversByVisibleTarget[targetHandle]
	visibilityState.Mu.RUnlock()

	if !hasObservers || len(observers) == 0 {
		// No observers can see this entity, nothing to send
		return nil
	}

	// Create the movement message once (reuse for all observers)
	movement := &netproto.EntityMovement{
		Position: &netproto.Position{
			X:       int32(event.X),
			Y:       int32(event.Y),
			Heading: uint32(event.Heading),
		},
		Velocity: &netproto.Vector2{
			X: int32(event.VelocityX),
			Y: int32(event.VelocityY),
		},
		MoveMode: convertMoveMode(event.MoveMode),
		IsMoving: event.IsMoving,
	}

	if event.TargetX != nil && event.TargetY != nil {
		movement.TargetPosition = &netproto.Vector2{
			X: int32(*event.TargetX),
			Y: int32(*event.TargetY),
		}
	}

	// Send only to observers who can see the target entity
	for observerHandle := range observers {
		observerEntityID, ok := shard.World().GetExternalID(observerHandle)
		if !ok {
			continue
		}

		// Check if this observer is a connected client
		if client, exists := shard.clients[observerEntityID]; exists {
			// Create message with client-specific StreamEpoch
			msg := &netproto.ServerMessage{
				Payload: &netproto.ServerMessage_ObjectMove{
					ObjectMove: &netproto.S2C_ObjectMove{
						EntityId:     uint64(event.EntityID),
						Movement:     movement,
						ServerTimeMs: event.ServerTimeMs,
						MoveSeq:      event.MoveSeq,
						IsTeleport:   event.IsTeleport,
						StreamEpoch:  client.StreamEpoch.Load(),
					},
				},
			}

			// Marshal the message for this specific client
			data, err := proto.Marshal(msg)
			if err != nil {
				d.logger.Error("failed to marshal ObjectMove message",
					zap.Error(err),
					zap.Int64("entity_id", int64(event.EntityID)),
					zap.Uint64("client_id", client.ID),
				)
				continue
			}

			client.Send(data)
		}
	}

	return nil
}

func (d *NetworkVisibilityDispatcher) handleEntitySpawn(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.EntitySpawnEvent)
	if !ok {
		return nil
	}

	// Get the specific shard by layer
	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}

	shard.clientsMu.RLock()
	// Find the client that is the observer
	if client, exists := shard.clients[event.ObserverID]; exists {
		// Get the target entity's components
		transform, hasTransform := ecs.GetComponent[components.Transform](shard.World(), event.TargetHandle)
		if !hasTransform {
			shard.clientsMu.RUnlock()
			return nil
		}

		entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](shard.World(), event.TargetHandle)
		if !hasEntityInfo {
			shard.clientsMu.RUnlock()
			return nil
		}

		msg := &netproto.ServerMessage{
			Payload: &netproto.ServerMessage_ObjectSpawn{
				ObjectSpawn: &netproto.S2C_ObjectSpawn{
					EntityId:     uint64(event.TargetID),
					ObjectType:   int32(entityInfo.ObjectType),
					ResourcePath: "",
					Position: &netproto.EntityPosition{
						Position: &netproto.Position{
							X: int32(transform.X),
							Y: int32(transform.Y),
						},
					},
				},
			},
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			d.logger.Error("failed to marshal ObjectSpawn message",
				zap.Error(err),
				zap.Int64("observer_id", int64(event.ObserverID)),
				zap.Int64("target_id", int64(event.TargetID)),
			)
			shard.clientsMu.RUnlock()
			return nil
		}

		client.Send(data)
	}
	shard.clientsMu.RUnlock()

	return nil
}

func (d *NetworkVisibilityDispatcher) handleEntityDespawn(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.EntityDespawnEvent)
	if !ok {
		return nil
	}

	// Get the specific shard by layer
	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}

	shard.clientsMu.RLock()
	// Find the client that is the observer
	if client, exists := shard.clients[event.ObserverID]; exists {
		msg := &netproto.ServerMessage{
			Payload: &netproto.ServerMessage_ObjectDespawn{
				ObjectDespawn: &netproto.S2C_ObjectDespawn{
					EntityId: uint64(event.TargetID),
				},
			},
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			d.logger.Error("failed to marshal ObjectDespawn message",
				zap.Error(err),
				zap.Int64("observer_id", int64(event.ObserverID)),
				zap.Int64("target_id", int64(event.TargetID)),
			)
			shard.clientsMu.RUnlock()
			return nil
		}

		client.Send(data)
	}
	shard.clientsMu.RUnlock()

	return nil
}

func (d *NetworkVisibilityDispatcher) handleChunkUnload(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.ChunkUnloadEvent)
	if !ok {
		return nil
	}

	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}

	shard.clientsMu.RLock()
	client, exists := shard.clients[event.EntityID]
	if !exists {
		shard.clientsMu.RUnlock()
		return nil
	}

	// Check if client is in world and epoch matches
	if !client.InWorld.Load() || event.Epoch != client.StreamEpoch.Load() {
		shard.clientsMu.RUnlock()
		return nil
	}

	msg := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ChunkUnload{
			ChunkUnload: &netproto.S2C_ChunkUnload{
				Coord: &netproto.ChunkCoord{
					X: int32(event.X),
					Y: int32(event.Y),
				},
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		d.logger.Error("failed to marshal ChunkUnload message",
			zap.Error(err),
			zap.Int64("entity_id", int64(event.EntityID)),
			zap.Int("x", event.X),
			zap.Int("y", event.Y),
		)
		shard.clientsMu.RUnlock()
		return nil
	}

	client.Send(data)
	shard.clientsMu.RUnlock()

	return nil
}

func (d *NetworkVisibilityDispatcher) handleChunkLoad(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.ChunkLoadEvent)
	if !ok {
		return nil
	}

	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}

	shard.clientsMu.RLock()
	client, exists := shard.clients[event.EntityID]
	if !exists {
		shard.clientsMu.RUnlock()
		return nil
	}
	//d.logger.Debug("handleChunkLoad",
	//	zap.Int64("entity_id", int64(event.EntityID)),
	//	zap.Uint32("epoch", event.Epoch),
	//	zap.Any("coord", types.ChunkCoord{X: event.X, Y: event.Y}),
	//	zap.Int("tiles_len", len(event.Tiles)))
	//d.logger.Debug("handleChunkLoad", zap.Any("client", client))

	// Check if client is in world and epoch matches
	if !client.InWorld.Load() || event.Epoch != client.StreamEpoch.Load() || client.StreamEpoch.Load() == 0 {
		shard.clientsMu.RUnlock()
		return nil
	}
	shard.clientsMu.RUnlock()

	// Use tiles from the event instead of fetching chunk data again
	msg := &netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ChunkLoad{
			ChunkLoad: &netproto.S2C_ChunkLoad{
				Chunk: &netproto.ChunkData{
					Coord: &netproto.ChunkCoord{
						X: int32(event.X),
						Y: int32(event.Y),
					},
					Tiles:   event.Tiles,
					Version: event.Version,
				},
			},
		},
	}
	//d.logger.Debug("ChunkLoad send", zap.Int64("entity_id", int64(event.EntityID)), zap.Int("tiles_len", len(event.Tiles)))

	data, err := proto.Marshal(msg)
	if err != nil {
		d.logger.Error("failed to marshal ChunkLoad message",
			zap.Error(err),
			zap.Int64("entity_id", int64(event.EntityID)),
			zap.Int("x", event.X),
			zap.Int("y", event.Y),
		)
		return nil
	}

	shard.clientsMu.RLock()
	if client, exists := shard.clients[event.EntityID]; exists {
		client.Send(data)
	}
	shard.clientsMu.RUnlock()

	return nil
}

func convertMoveMode(mode constt.MoveMode) netproto.MovementMode {
	switch mode {
	case constt.Walk: // Walk
		return netproto.MovementMode_MOVE_MODE_WALK
	case constt.Run: // Run
		return netproto.MovementMode_MOVE_MODE_RUN
	case constt.FastRun: // FastRun
		return netproto.MovementMode_MOVE_MODE_FAST_RUN
	case constt.Swim: // Swim
		return netproto.MovementMode_MOVE_MODE_SWIM
	default:
		return netproto.MovementMode_MOVE_MODE_WALK
	}
}
