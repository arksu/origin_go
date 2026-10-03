package events

import (
	"context"
	"origin/internal/charactervisual"
	constt "origin/internal/const"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type NetworkVisibilityDispatcher struct {
	shardManager *game.ShardManager
	logger       *zap.Logger
}

// Pool for per-call observer→entryIndices maps (avoids alloc per tick per shard)
var observerEntriesPool = sync.Pool{
	New: func() any {
		return make(map[types.Handle][]int, 64)
	},
}

func NewNetworkVisibilityDispatcher(shardManager *game.ShardManager, logger *zap.Logger) *NetworkVisibilityDispatcher {
	return &NetworkVisibilityDispatcher{
		shardManager: shardManager,
		logger:       logger,
	}
}

func (d *NetworkVisibilityDispatcher) Subscribe(eventBus *eventbus.EventBus) {
	eventBus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, d.handleObjectMoveBatch)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntitySpawn, eventbus.PriorityMedium, d.handleEntitySpawn)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntitySpawnBatch, eventbus.PriorityMedium, d.handleEntitySpawnBatch)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntityDespawn, eventbus.PriorityMedium, d.handleEntityDespawn)
	eventBus.SubscribeAsync(ecs.TopicGameplayEntityAppearance, eventbus.PriorityMedium, d.handleEntityAppearanceChanged)
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
	observerEntries := observerEntriesPool.Get().(map[types.Handle][]int)
	for k, v := range observerEntries {
		observerEntries[k] = v[:0]
	}

	visibilityState.Mu.RLock()
	for i := range batch.Entries {
		observers, has := visibilityState.ObserversByVisibleTarget[batch.Entries[i].Handle]
		if !has {
			continue
		}
		for observerHandle := range observers {
			observerEntries[observerHandle] = append(observerEntries[observerHandle], i)
		}
	}
	visibilityState.Mu.RUnlock()

	hasEntries := false
	for _, v := range observerEntries {
		if len(v) > 0 {
			hasEntries = true
			break
		}
	}
	if !hasEntries {
		observerEntriesPool.Put(observerEntries)
		return nil
	}

	// Build shared immutable entries once; each recipient gets its visible subset.
	encoder := newObjectMoveEncoder(batch.Entries)

	observerEntityIDs := make(map[types.Handle]types.EntityID, len(observerEntries))
	shard.WithWorldRead(func(w *ecs.World) {
		for observerHandle := range observerEntries {
			if observerEntityID, ok := w.GetExternalID(observerHandle); ok {
				observerEntityIDs[observerHandle] = observerEntityID
			}
		}
	})

	// Keep one client-map lock across fanout and marshal each visible subset once.
	shard.ClientsMu.RLock()
	for observerHandle, entryIndices := range observerEntries {
		if len(entryIndices) == 0 {
			continue
		}
		observerEntityID, ok := observerEntityIDs[observerHandle]
		if !ok {
			continue
		}

		client, exists := shard.Clients[observerEntityID]
		if !exists {
			continue
		}

		encoded, err := encoder.marshalVisible(entryIndices)
		if err != nil {
			d.logger.Error("Unable to encode object movement", zap.Error(err))
			continue
		}
		client.Send(encoded)
	}
	shard.ClientsMu.RUnlock()

	observerEntriesPool.Put(observerEntries)
	return nil
}

type objectMoveEncoder struct {
	moves        []*netproto.S2C_ObjectMove
	visibleMoves []*netproto.S2C_ObjectMove
	fullBatch    []byte
}

func newObjectMoveEncoder(entries []ecs.MoveBatchEntry) *objectMoveEncoder {
	moves := make([]*netproto.S2C_ObjectMove, len(entries))
	for index := range entries {
		moves[index] = buildObjectMove(&entries[index])
	}
	return &objectMoveEncoder{moves: moves}
}

func (encoder *objectMoveEncoder) marshalVisible(indices []int) ([]byte, error) {
	// Routing appends each index once in event order. A complete subset therefore
	// has identical bytes for every observer; share them to avoid dense fanout cost.
	if len(indices) == len(encoder.moves) {
		if encoder.fullBatch == nil {
			encoded, err := marshalObjectMoves(encoder.moves)
			if err != nil {
				return nil, err
			}
			encoder.fullBatch = encoded
		}
		return encoder.fullBatch, nil
	}
	encoder.visibleMoves = encoder.visibleMoves[:0]
	for _, index := range indices {
		encoder.visibleMoves = append(encoder.visibleMoves, encoder.moves[index])
	}
	return marshalObjectMoves(encoder.visibleMoves)
}

func buildObjectMove(entry *ecs.MoveBatchEntry) *netproto.S2C_ObjectMove {
	movement := &netproto.EntityMovement{
		Position: &netproto.Position{
			X:       int32(entry.X),
			Y:       int32(entry.Y),
			Heading: float32(entry.Heading),
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

	return &netproto.S2C_ObjectMove{
		EntityId: uint64(entry.EntityID), Movement: movement,
		ServerTimeMs: entry.ServerTimeMs, MoveSeq: entry.MoveSeq,
		IsTeleport: entry.IsTeleport, CarriedByEntityId: uint64(entry.CarriedByEntityID),
	}
}

func marshalObjectMoves(moves []*netproto.S2C_ObjectMove) ([]byte, error) {
	switch len(moves) {
	case 0:
		return nil, nil
	case 1:
		return proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMove{ObjectMove: moves[0]}})
	default:
		return proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectMoveBatch{
			ObjectMoveBatch: &netproto.S2C_ObjectMoveBatch{Moves: moves},
		}})
	}
}

func (d *NetworkVisibilityDispatcher) handleEntitySpawnBatch(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.EntitySpawnBatchEvent)
	if !ok {
		return nil
	}
	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}
	shard.WithWorldRead(func(w *ecs.World) {
		spawns := make([]*netproto.S2C_ObjectSpawn, 0, len(event.Entries))
		for _, entry := range event.Entries {
			if !targetVisibleToObserver(w, event.ObserverID, entry.EntityID) {
				continue
			}
			if spawn := d.buildObjectSpawn(w, entry.EntityID, entry.Handle); spawn != nil {
				spawns = append(spawns, spawn)
			}
		}
		d.sendObjectSpawns(shard, event.ObserverID, spawns)
	})
	return nil
}

func (d *NetworkVisibilityDispatcher) handleEntitySpawn(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.EntitySpawnEvent)
	if !ok {
		return nil
	}
	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}
	// Capture and enqueue under the same world read lock. Visual updates are
	// authored on the ECS thread; a later spawn always includes their latest state.
	shard.WithWorldRead(func(w *ecs.World) {
		spawn := d.buildObjectSpawn(w, event.TargetID, event.TargetHandle)
		if spawn != nil {
			d.sendObjectSpawn(w, shard, event.ObserverID, event.TargetHandle, spawn)
		}
	})
	return nil
}

func (d *NetworkVisibilityDispatcher) buildObjectSpawn(w *ecs.World, entityID types.EntityID, handle types.Handle) *netproto.S2C_ObjectSpawn {
	if !w.Alive(handle) {
		return nil
	}
	actualID, ok := w.GetExternalID(handle)
	if !ok || actualID != entityID {
		return nil
	}
	transform, hasTransform := ecs.GetComponent[components.Transform](w, handle)
	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	if !hasTransform || !hasInfo {
		return nil
	}
	resource := "unknown"
	var displayName string
	nameColor := components.NameColorDefault
	if appearance, ok := ecs.GetComponent[components.Appearance](w, handle); ok {
		if appearance.Resource != "" {
			resource = appearance.Resource
		}
		if appearance.Name != nil {
			displayName = *appearance.Name
			nameColor = appearance.NameColor
		}
	}
	size := &netproto.Vector2{}
	if collider, ok := ecs.GetComponent[components.Collider](w, handle); ok {
		size.X, size.Y = int32(collider.HalfWidth*2), int32(collider.HalfHeight*2)
	}
	visual, err := charactervisual.Snapshot(w, handle)
	if err != nil {
		d.logger.Error("Unable to build spawn visual", zap.Uint64("entity_id", uint64(entityID)), zap.Error(err))
		return nil
	}
	animation, err := cyclicaction.Snapshot(w, handle)
	if err != nil {
		d.logger.Error("Unable to build spawn action animation", zap.Uint64("entity_id", uint64(entityID)), zap.Error(err))
		return nil
	}
	return &netproto.S2C_ObjectSpawn{
		EntityId: uint64(entityID), TypeId: info.TypeID, ResourcePath: resource,
		Name: displayName, NameColor: nicknameColorToProto(nameColor),
		CarriedByEntityId: carryVisualCarrierIDForHandle(w, handle),
		CharacterVisual:   visual,
		ActionAnimation:   animation,
		CombatExecution:   game.CombatExecutionSnapshot(w, handle),
		CombatTarget:      game.CombatTargetSnapshot(w, handle),
		Position: &netproto.EntityPosition{
			Position: &netproto.Position{X: int32(transform.X), Y: int32(transform.Y)},
			Size:     size,
		},
	}
}

// nicknameColorToProto maps the ECS role to the wire enum; unknown future
// roles degrade to default until the client palette knows them.
func nicknameColorToProto(c components.NameColor) netproto.NicknameColor {
	switch c {
	case components.NameColorDefault:
		return netproto.NicknameColor_NICKNAME_COLOR_DEFAULT
	default:
		return netproto.NicknameColor_NICKNAME_COLOR_DEFAULT
	}
}

func (d *NetworkVisibilityDispatcher) sendObjectSpawn(w *ecs.World, shard *game.Shard, observerID types.EntityID, target types.Handle, spawn *netproto.S2C_ObjectSpawn) {
	observer := w.GetHandleByEntityID(observerID)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.Mu.RLock()
	_, visible := visibility.ObserversByVisibleTarget[target][observer]
	visibility.Mu.RUnlock()
	if !visible {
		return
	}
	d.sendObjectSpawns(shard, observerID, []*netproto.S2C_ObjectSpawn{spawn})
}

// The caller holds the world read lock until the snapshot is enqueued.
func (d *NetworkVisibilityDispatcher) sendObjectSpawns(shard *game.Shard, observerID types.EntityID, spawns []*netproto.S2C_ObjectSpawn) {
	if len(spawns) == 0 {
		return
	}
	shard.ClientsMu.RLock()
	defer shard.ClientsMu.RUnlock()
	client := shard.Clients[observerID]
	if client == nil || !client.InWorld.Load() {
		return
	}
	epoch := client.StreamEpoch.Load()
	critical := false
	for _, spawn := range spawns {
		spawn.StreamEpoch = epoch
		critical = critical || spawn.ActionAnimation != nil || spawn.CombatExecution != nil || spawn.CombatTarget != nil
	}
	message := &netproto.ServerMessage{}
	if len(spawns) == 1 {
		message.Payload = &netproto.ServerMessage_ObjectSpawn{ObjectSpawn: spawns[0]}
	} else {
		message.Payload = &netproto.ServerMessage_ObjectSpawnBatch{ObjectSpawnBatch: &netproto.S2C_ObjectSpawnBatch{Spawns: spawns}}
	}
	encoded, err := proto.Marshal(message)
	if err != nil {
		d.logger.Error("Unable to encode object spawns", zap.Error(err))
		return
	}
	if critical {
		client.SendCritical(encoded)
	} else {
		client.Send(encoded)
	}
}

func (d *NetworkVisibilityDispatcher) handleEntityDespawn(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.EntityDespawnEvent)
	if !ok {
		return nil
	}
	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}
	shard.WithWorldRead(func(w *ecs.World) {
		// Async visibility jobs can run after the target has re-entered AOI.
		// Never let an old despawn erase its newer spawn and equipment snapshot.
		if targetVisibleToObserver(w, event.ObserverID, event.TargetID) {
			return
		}
		shard.ClientsMu.RLock()
		defer shard.ClientsMu.RUnlock()
		client := shard.Clients[event.ObserverID]
		if client == nil || !client.InWorld.Load() {
			return
		}
		message := &netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectDespawn{
			ObjectDespawn: &netproto.S2C_ObjectDespawn{EntityId: uint64(event.TargetID), StreamEpoch: client.StreamEpoch.Load()},
		}}
		encoded, err := proto.Marshal(message)
		if err != nil {
			d.logger.Error("Unable to encode ObjectDespawn", zap.Error(err))
			return
		}
		client.Send(encoded)
	})
	return nil
}

func targetVisibleToObserver(w *ecs.World, observerID, targetID types.EntityID) bool {
	observer := w.GetHandleByEntityID(observerID)
	target := w.GetHandleByEntityID(targetID)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.Mu.RLock()
	defer visibility.Mu.RUnlock()
	_, visible := visibility.ObserversByVisibleTarget[target][observer]
	return visible
}

func (d *NetworkVisibilityDispatcher) handleEntityAppearanceChanged(ctx context.Context, e eventbus.Event) error {
	event, ok := e.(*ecs.EntityAppearanceChangedEvent)
	if !ok {
		return nil
	}
	shard := d.shardManager.GetShard(event.Layer)
	if shard == nil {
		return nil
	}
	shard.WithWorldRead(func(w *ecs.World) {
		spawn := d.buildObjectSpawn(w, event.TargetID, event.TargetHandle)
		if spawn == nil {
			return
		}
		visibility := ecs.GetResource[ecs.VisibilityState](w)
		visibility.Mu.RLock()
		observers := make([]types.EntityID, 0, len(visibility.ObserversByVisibleTarget[event.TargetHandle]))
		for handle := range visibility.ObserversByVisibleTarget[event.TargetHandle] {
			if id, ok := w.GetExternalID(handle); ok {
				observers = append(observers, id)
			}
		}
		visibility.Mu.RUnlock()
		for _, observerID := range observers {
			d.sendObjectSpawn(w, shard, observerID, event.TargetHandle, spawn)
		}
	})
	return nil
}

func carryVisualCarrierIDForHandle(w *ecs.World, handle types.Handle) uint64 {
	if w == nil || handle == types.InvalidHandle || !w.Alive(handle) {
		return 0
	}
	if lifted, ok := ecs.GetComponent[components.LiftedObjectState](w, handle); ok && lifted.CarrierPlayerID != 0 {
		return uint64(lifted.CarrierPlayerID)
	}
	return 0
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
				StreamEpoch: event.Epoch, EventSeq: event.EventSeq,
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

	if event.EventSeq != 0 && client.InWorld.Load() && event.Epoch == client.StreamEpoch.Load() {
		client.SendChunkVisibility(data)
	}
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
				StreamEpoch: event.Epoch, EventSeq: event.EventSeq,
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
	if client, exists := shard.Clients[event.EntityID]; exists && event.EventSeq != 0 && client.InWorld.Load() && event.Epoch == client.StreamEpoch.Load() {
		client.SendChunkVisibility(data)
	}
	shard.ClientsMu.RUnlock()

	return nil
}

func convertMoveMode(mode constt.MoveMode) netproto.MovementMode {
	switch mode {
	case constt.Crawl: // Crawl
		return netproto.MovementMode_MOVE_MODE_CRAWL
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
