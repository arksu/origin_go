package game

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/protobuf/proto"
)

func newPlayerSpawnTestShard(t *testing.T, capacity uint32) (*Shard, <-chan eventbus.Event) {
	t.Helper()
	cfg := &config.Config{Game: config.GameConfig{
		ChunkLRUCapacity: 4, ChunkLRUTTL: 60, LoadWorkers: 1,
		WorldWidthChunks: 1, WorldHeightChunks: 1, SpawnTimeout: 3 * time.Second,
		NearSpawnRadius: 20, NearSpawnTries: 10, RandomSpawnTries: 10,
	}}
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	w := ecs.NewWorldWithCapacity(capacity, bus, 0)
	shard := &Shard{world: w, cfg: cfg, eventBus: bus, logger: zap.NewNop(), Clients: make(map[types.EntityID]*network.Client)}
	shard.chunkManager = gameworld.NewChunkManager(cfg, nil, w, shard, 0, 1, gameworld.NewObjectFactory(nil), nil, bus, shard.logger)
	t.Cleanup(func() {
		shard.chunkManager.Stop()
		require.NoError(t, bus.Shutdown(context.Background()))
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, shard.chunkManager.WaitPreloaded(ctx, types.ChunkCoord{}))
	chunk := shard.chunkManager.GetChunk(types.ChunkCoord{})
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for index := range tiles {
		tiles[index] = types.TileGrass
	}
	require.NoError(t, chunk.RestoreTiles(tiles, 0, 0))
	entered := make(chan eventbus.Event, 8)
	bus.SubscribeAsync(ecs.TopicGameplayPlayerEnterWorld, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		entered <- event
		return nil
	})
	return shard, entered
}

func flushPlayerSpawnEvents(t *testing.T, bus *eventbus.EventBus) {
	t.Helper()
	flushed := make(chan struct{})
	topic := "test.player_spawn.flush." + t.Name()
	bus.SubscribeAsync(topic, eventbus.PriorityMedium, func(_ context.Context, _ eventbus.Event) error {
		close(flushed)
		return nil
	})
	bus.PublishAsync(eventbus.NewEvent(topic, nil), eventbus.PriorityMedium)
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("player spawn events did not drain")
	}
}

func TestTrySpawnPlayerCapacityFailureAndRecovery(t *testing.T) {
	shard, entered := newPlayerSpawnTestShard(t, 1)
	character := repository.Character{ID: 10, X: 200, Y: 200}
	require.NoError(t, shard.PrepareEntityAOI(context.Background(), 10, 200, 200))
	blocker := shard.world.Spawn(99, nil)
	setupCalls := 0
	setup := func(_ *ecs.World, _ types.Handle) { setupCalls++ }
	ok, handle := shard.TrySpawnPlayer(200, 200, character, setup)
	require.False(t, ok)
	require.Equal(t, types.InvalidHandle, handle)
	require.Zero(t, setupCalls)
	handle, err := shard.trySpawnPlayerWithPolicy(200, 200, character, nil, SpawnCollisionPolicy{})
	require.Equal(t, types.InvalidHandle, handle)
	require.ErrorIs(t, err, ecs.ErrEntityCapacityExhausted)
	var capacityErr *playerSpawnCapacityError
	require.ErrorAs(t, err, &capacityErr)
	require.Equal(t, 1, capacityErr.active)
	require.Equal(t, 1, capacityErr.capacity)
	flushPlayerSpawnEvents(t, shard.eventBus)
	require.Empty(t, entered)
	chunk := shard.chunkManager.GetChunk(types.ChunkCoord{})
	require.Zero(t, chunk.Spatial().DynamicCount())
	require.Equal(t, types.InvalidHandle, shard.world.GetHandleByEntityID(10))

	require.True(t, shard.world.Despawn(blocker))
	ok, handle = shard.TrySpawnPlayer(200, 200, character, setup)
	require.True(t, ok)
	require.True(t, shard.world.Alive(handle))
	require.Equal(t, 1, setupCalls)
	require.Equal(t, []types.Handle{handle}, chunk.Spatial().GetDynamicHandles())
	select {
	case event := <-entered:
		require.Equal(t, types.EntityID(10), event.(*ecs.PlayerEnteredWorldEvent).EntityID)
	case <-time.After(time.Second):
		t.Fatal("successful recovery did not publish player entered world")
	}
	require.Empty(t, entered)
}

func TestPlayerSetupFailureRollsBackBeforeEnteringWorld(t *testing.T) {
	for _, inventoryFailure := range []bool{false, true} {
		name := "setup_error"
		if inventoryFailure {
			name = "inventory_capacity"
		}
		t.Run(name, func(t *testing.T) {
			shard, entered := newPlayerSpawnTestShard(t, 4)
			w := shard.world
			character := repository.Character{ID: 10}
			require.NoError(t, shard.PrepareEntityAOI(context.Background(), 10, 200, 200))
			existing := w.SpawnWithoutExternalID()
			existingContainer := components.InventoryContainer{OwnerID: 10, Kind: constt.InventoryGrid, Key: 7, Items: []components.InvItem{{ItemID: 42}}}
			ecs.AddComponent(w, existing, existingContainer)
			refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
			refIndex.Add(constt.InventoryGrid, 10, 7, existing)
			blocker := w.Spawn(99, nil)
			setupFailure := errors.New("player setup failed")
			setup := func(w *ecs.World, handle types.Handle) error {
				ecs.AddComponent(w, handle, components.Vision{})
				visibility := ecs.GetResource[ecs.VisibilityState](w)
				visibility.VisibleByObserver[handle] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{}}
				if !inventoryFailure {
					return setupFailure
				}
				_, err := inventory.NewInventoryLoader(zap.NewNop()).LoadPlayerInventories(w, 10, []inventory.InventoryDataV1{
					{Kind: uint8(constt.InventoryHand)}, {Kind: uint8(constt.InventoryEquipment)},
				})
				return err
			}
			handle, err := shard.trySpawnPlayerWithPolicy(200, 200, character, setup, SpawnCollisionPolicy{})
			if inventoryFailure {
				require.ErrorIs(t, err, ecs.ErrEntityCapacityExhausted)
			} else {
				require.ErrorIs(t, err, setupFailure)
			}
			require.Equal(t, types.InvalidHandle, handle)
			require.Equal(t, 2, w.EntityCount())
			require.Equal(t, types.InvalidHandle, w.GetHandleByEntityID(10))
			require.Empty(t, ecs.GetResource[ecs.VisibilityState](w).VisibleByObserver)
			require.Empty(t, ecs.GetResource[ecs.CharacterEntities](w).GetAll())
			require.Zero(t, shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().DynamicCount())
			got, ok := ecs.GetComponent[components.InventoryContainer](w, existing)
			require.True(t, ok)
			require.Equal(t, existingContainer, got)
			indexed, ok := refIndex.Lookup(constt.InventoryGrid, 10, 7)
			require.True(t, ok)
			require.Equal(t, existing, indexed)
			flushPlayerSpawnEvents(t, shard.eventBus)
			require.Empty(t, entered)
			if inventoryFailure {
				require.True(t, w.Despawn(blocker))
				handle, err = shard.trySpawnPlayerWithPolicy(200, 200, character, setup, SpawnCollisionPolicy{})
				require.NoError(t, err)
				require.True(t, w.Alive(handle))
				require.Equal(t, 4, w.EntityCount())
			}
		})
	}
}

func connectPlayerSpawnTestClient(t *testing.T) (*network.Client, net.Conn) {
	t.Helper()
	server := network.NewServer(&config.NetworkConfig{ReadTimeout: time.Minute, WriteTimeout: time.Second}, &config.GameConfig{SendChannelBuffer: 32}, zap.NewNop())
	connected := make(chan *network.Client, 1)
	server.SetOnConnect(func(client *network.Client) { connected <- client })
	mux := http.NewServeMux()
	require.NoError(t, server.Start("127.0.0.1:0", mux))
	t.Cleanup(server.Stop)
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	conn, _, _, err := ws.Dial(context.Background(), "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	select {
	case client := <-connected:
		return client, conn
	case <-time.After(time.Second):
		t.Fatal("client did not connect")
		return nil, nil
	}
}

func TestSpawnAndLoginCapacityFailureDoesNotAttachClient(t *testing.T) {
	shard, entered := newPlayerSpawnTestShard(t, 1)
	logs, observed := observer.New(zap.InfoLevel)
	shard.logger = zap.New(logs)
	shard.world.Spawn(99, nil)
	client, conn := connectPlayerSpawnTestClient(t)
	client.CharacterID = 10
	game := &Game{cfg: shard.cfg, logger: shard.logger, shardManager: &ShardManager{shards: map[int]*Shard{0: shard}}}
	game.spawnAndLogin(client, repository.Character{ID: 10, X: 200, Y: 200})
	require.False(t, client.InWorld.Load())
	require.Zero(t, client.StreamEpoch.Load())
	require.Empty(t, shard.Clients)
	require.Empty(t, ecs.GetResource[ecs.CharacterEntities](shard.world).GetAll())
	require.Empty(t, shard.chunkManager.GetEntityActiveChunks(10))
	require.Zero(t, shard.chunkManager.GetEntityEpoch(10))
	require.Equal(t, types.InvalidHandle, shard.world.GetHandleByEntityID(10))
	require.Zero(t, shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().DynamicCount())
	require.Equal(t, 1, observed.FilterMessage("Preparing entity AOI").Len(), "capacity failure must not retry unrelated spawn positions")
	capacityLogs := observed.FilterMessage("Player spawn rejected: entity capacity exhausted").All()
	require.Len(t, capacityLogs, 1)
	require.EqualValues(t, 1, capacityLogs[0].ContextMap()["active_entities"])
	require.EqualValues(t, 1, capacityLogs[0].ContextMap()["entity_capacity"])
	flushPlayerSpawnEvents(t, shard.eventBus)
	require.Empty(t, entered)

	barrier, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_Pong{Pong: &netproto.S2C_Pong{}}})
	require.NoError(t, err)
	require.True(t, client.SendCritical(barrier))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	for index := 0; index < 2; index++ {
		payload, op, err := wsutil.ReadServerData(conn)
		require.NoError(t, err)
		require.Equal(t, ws.OpBinary, op)
		message := &netproto.ServerMessage{}
		require.NoError(t, proto.Unmarshal(payload, message))
		if index == 0 {
			require.NotNil(t, message.GetError())
			require.Equal(t, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, message.GetError().Code)
			require.Contains(t, message.GetError().Message, "entity capacity exhausted")
		} else {
			require.NotNil(t, message.GetPong(), "no EnterWorld or bootstrap packets may be sent")
		}
	}
}
