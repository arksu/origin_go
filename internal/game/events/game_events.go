package events

import (
	"context"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type NetworkVisibilityDispatcher struct {
	shardManager *game.ShardManager
	logger       *zap.Logger

	// Reusable buffers for handleObjectMoveBatch (safe: single async worker per subscription)
	observerEntries map[types.Handle][]int // observerHandle → entry indices
	serializedMoves [][]byte               // indexed by batch entry index
}

func NewNetworkVisibilityDispatcher(shardManager *game.ShardManager, logger *zap.Logger) *NetworkVisibilityDispatcher {
	return &NetworkVisibilityDispatcher{
		shardManager:    shardManager,
		logger:          logger,
		observerEntries: make(map[types.Handle][]int, 64),
	}
}

func (d *NetworkVisibilityDispatcher) Subscribe(eventBus *eventbus.EventBus) {
	eventBus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, d.handleObjectMoveBatch)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntitySpawn, eventbus.PriorityMedium, d.handleEntitySpawn)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntityDespawn, eventbus.PriorityMedium, d.handleEntityDespawn)
	eventBus.SubscribeAsync(ecs.TopicGameplayChunkUnload, eventbus.PriorityMedium, d.handleChunkUnload)
	eventBus.SubscribeAsync(ecs.TopicGameplayChunkLoad, eventbus.PriorityMedium, d.handleChunkLoad)
}

func (d *NetworkVisibilityDispatcher) handleObjectMoveBatch(ctx context.Context, e eventbus.Event) error {
	batch, ok := e.(*ecs.ObjectMoveBatchEvent)
	if !ok {
		return nil
	}

	shard := d.shardManager.GetShard(batch.Layer)
	if shard == nil {
		return nil
	}

	visibilityState := ecs.GetResource[ecs.VisibilityState](shard.World())

	// Phase 1: Single lock acquisition — build per-observer → []entryIndex mapping
	// Reuse map: clear entries but keep allocated buckets
	for k, v := range d.observerEntries {
		d.observerEntries[k] = v[:0]
	}

	visibilityState.Mu.RLock()
	for i := range batch.Entries {
		observers, has := visibilityState.ObserversByVisibleTarget[batch.Entries[i].Handle]
		if !has {
			continue
		}
		for observerHandle := range observers {
			d.observerEntries[observerHandle] = append(d.observerEntries[observerHandle], i)
		}
	}
	visibilityState.Mu.RUnlock()

	hasEntries := false
	for _, v := range d.observerEntries {
		if len(v) > 0 {
			hasEntries = true
			break
		}
	}
	if !hasEntries {
		return nil
	}

	// Phase 2: Pre-serialize each unique entity movement once (shared across observers)
	// Grow slice to match batch size, reuse capacity
	if cap(d.serializedMoves) < len(batch.Entries) {
		d.serializedMoves = make([][]byte, len(batch.Entries))
	} else {
		d.serializedMoves = d.serializedMoves[:len(batch.Entries)]
		for i := range d.serializedMoves {
			d.serializedMoves[i] = nil
		}
	}
	for i := range batch.Entries {
		entry := &batch.Entries[i]

		movement := &netproto.EntityMovement{
			Position: &netproto.Position{
				X:       int32(entry.X),
				Y:       int32(entry.Y),
				Heading: uint32(entry.Heading),
			},
			Velocity: &netproto.Vector2{
				X: int32(entry.VelocityX),
				Y: int32(entry.VelocityY),
			},
			MoveMode: convertMoveMode(entry.MoveMode),
			IsMoving: entry.IsMoving,
		}

		if entry.TargetX != nil && entry.TargetY != nil {
			movement.TargetPosition = &netproto.Vector2{
				X: int32(*entry.TargetX),
				Y: int32(*entry.TargetY),
			}
		}

		msg := &netproto.ServerMessage{
			Payload: &netproto.ServerMessage_ObjectMove{
				ObjectMove: &netproto.S2C_ObjectMove{
					EntityId:     uint64(entry.EntityID),
					Movement:     movement,
					ServerTimeMs: entry.ServerTimeMs,
					MoveSeq:      entry.MoveSeq,
					IsTeleport:   entry.IsTeleport,
				},
			},
		}

		data, err := proto.Marshal(msg)
		if err != nil {
			d.logger.Error("failed to marshal ObjectMove message",
				zap.Error(err),
				zap.Int64("entity_id", int64(entry.EntityID)),
			)
			continue
		}
		d.serializedMoves[i] = data
	}

	// Phase 3: Single ClientsMu lock — send pre-serialized bytes to each observer
	shard.ClientsMu.RLock()
	for observerHandle, entryIndices := range d.observerEntries {
		if len(entryIndices) == 0 {
			continue
		}
		observerEntityID, ok := shard.World().GetExternalID(observerHandle)
		if !ok {
			continue
		}

		client, exists := shard.Clients[observerEntityID]
		if !exists {
			continue
		}

		for _, idx := range entryIndices {
			if data := d.serializedMoves[idx]; data != nil {
				client.Send(data)
			}
		}
	}
	shard.ClientsMu.RUnlock()

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

	shard.ClientsMu.RLock()
	// Find the client that is the observer
	if client, exists := shard.Clients[event.ObserverID]; exists {
		// Get the target entity's components
		transform, hasTransform := ecs.GetComponent[components.Transform](shard.World(), event.TargetHandle)
		if !hasTransform {
			shard.ClientsMu.RUnlock()
			return nil
		}

		entityInfo, hasEntityInfo := ecs.GetComponent[components.EntityInfo](shard.World(), event.TargetHandle)
		if !hasEntityInfo {
			shard.ClientsMu.RUnlock()
			return nil
		}

		// Get collider component for size information
		collider, hasCollider := ecs.GetComponent[components.Collider](shard.World(), event.TargetHandle)

		// Calculate size - use collider if available, otherwise zero size
		var sizeX, sizeY int32
		if hasCollider {
			sizeX = int32(collider.HalfWidth * 2)  // Convert from half-width to full width
			sizeY = int32(collider.HalfHeight * 2) // Convert from half-height to full height
		}

		// Get appearance component for resource path
		var resourcePath string
		appearance, hasAppearance := ecs.GetComponent[components.Appearance](shard.World(), event.TargetHandle)
		if hasAppearance && appearance.Resource != "" {
			resourcePath = appearance.Resource
		} else {
			resourcePath = "unknown" // Fallback to unknown resource
		}

		msg := &netproto.ServerMessage{
			Payload: &netproto.ServerMessage_ObjectSpawn{
				ObjectSpawn: &netproto.S2C_ObjectSpawn{
					EntityId:     uint64(event.TargetID),
					TypeId:       entityInfo.TypeID,
					ResourcePath: resourcePath,
					Position: &netproto.EntityPosition{
						Position: &netproto.Position{
							X: int32(transform.X),
							Y: int32(transform.Y),
						},
						Size: &netproto.Vector2{
							X: sizeX,
							Y: sizeY,
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
			shard.ClientsMu.RUnlock()
			return nil
		}

		client.Send(data)
	}
	shard.ClientsMu.RUnlock()

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

	shard.ClientsMu.RLock()
	// Find the client that is the observer
	if client, exists := shard.Clients[event.ObserverID]; exists {
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
			shard.ClientsMu.RUnlock()
			return nil
		}

		client.Send(data)
	}
	shard.ClientsMu.RUnlock()

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

	shard.ClientsMu.RLock()
	client, exists := shard.Clients[event.EntityID]
	if !exists {
		shard.ClientsMu.RUnlock()
		return nil
	}

	// Check if client is in world and epoch matches
	if !client.InWorld.Load() || event.Epoch != client.StreamEpoch.Load() {
		shard.ClientsMu.RUnlock()
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
		shard.ClientsMu.RUnlock()
		return nil
	}

	client.Send(data)
	shard.ClientsMu.RUnlock()

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

	shard.ClientsMu.RLock()
	client, exists := shard.Clients[event.EntityID]
	if !exists {
		shard.ClientsMu.RUnlock()
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
		shard.ClientsMu.RUnlock()
		return nil
	}
	shard.ClientsMu.RUnlock()

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

	shard.ClientsMu.RLock()
	if client, exists := shard.Clients[event.EntityID]; exists {
		client.Send(data)
	}
	shard.ClientsMu.RUnlock()

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
