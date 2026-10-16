package game

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/testutil"
	"origin/internal/timeutil"
	"origin/internal/types"

	"github.com/gobwas/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Unlike the ordinary terminal fixture, this fixture owns database-backed chunk
// loaders and lets Game.Stop perform their one and only shutdown.
func newHearthShutdownFixture(t *testing.T, logger *zap.Logger) (*hearthTerminalGameFixture, *Game, func()) {
	t.Helper()
	db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	_, hearthDef := loadHearthBuildRecipeForTest(t)
	f := &hearthTerminalGameFixture{db: db, hearth: hearthDef}
	cfg := &config.Config{Game: config.GameConfig{
		Region: 1, MaxLayers: 2, TickRate: 10, DisconnectDelay: 30,
		ChunkLRUCapacity: 16, ChunkLRUTTL: 60, LoadWorkers: 1, SaveWorkers: 1,
		WorldWidthChunks: 5, WorldHeightChunks: 1, SpawnTimeout: 3 * time.Second,
		SendChannelBuffer: 64, Audio: config.DefaultAudioConfig(),
	}, Network: config.NetworkConfig{ReadTimeout: time.Minute, WriteTimeout: time.Second}}
	_, err := db.Pool().Exec(t.Context(), `INSERT INTO account (id,login,password_hash) VALUES (1,'hearth_shutdown','unused')`)
	require.NoError(t, err)
	for layer := range 2 {
		for x := range cfg.Game.WorldWidthChunks {
			_, err = db.Pool().Exec(t.Context(), `INSERT INTO chunk (region,x,y,layer,last_tick,tiles_data) VALUES (1,$1,0,$2,1,$3)`, x, layer, bytes.Repeat([]byte{types.TileGrass}, constt.ChunkSize*constt.ChunkSize))
			require.NoError(t, err)
		}
	}
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1, Logger: logger})
	ctx, cancel := context.WithCancel(context.Background())
	g := &Game{
		cfg: cfg, db: db, logger: logger, ctx: ctx, cancel: cancel,
		clock:            timeutil.NewManualClock(time.Unix(10, 0)),
		inventoryLoader:  inventory.NewInventoryLoader(logger),
		entityIDManager:  &EntityIDManager{db: db, logger: logger},
		shardManager:     &ShardManager{shards: make(map[int]*Shard), workerPool: NewWorkerPool(1), eventBus: bus},
		networkServer:    network.NewServer(&cfg.Network, &cfg.Game, logger),
		transferInFlight: make(map[types.EntityID]chan struct{}),
	}
	g.serverConstantsData, err = marshalServerConstants(cfg.Game.TickRate)
	require.NoError(t, err)
	for layer := range f.shards {
		shard := &Shard{
			layer: layer, cfg: cfg, db: db, logger: logger, eventBus: bus, state: ShardStateRunning,
			world: ecs.NewWorldWithCapacity(64, bus, layer), Clients: make(map[types.EntityID]*network.Client),
			playerInbox: network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig()),
			serverInbox: network.NewServerJobInbox(network.DefaultCommandQueueConfig()),
		}
		shard.chunkManager = gameworld.NewChunkManager(cfg, db, shard.world, shard, layer, 1, gameworld.NewObjectFactory(nil), nil, bus, logger)
		shard.soundEvents, err = NewSoundEventService(soundTestProfiles(t), cfg.Game.Audio, shard)
		require.NoError(t, err)
		shard.chunkManager.SetPositionObserver(shard.soundEvents)
		timing := ecs.GetResource[ecs.TimeState](shard.world)
		timing.UnixMs, timing.TickPeriod = 10000, 100*time.Millisecond
		f.shards[layer], f.games[layer], g.shardManager.shards[layer] = shard, g, shard
		require.NoError(t, shard.chunkManager.WaitPreloaded(t.Context(), types.ChunkCoord{}))
	}
	surface := f.shards[0]
	f.saver = &hearthTerminalGameSaver{ConstructionSaverDB: hearth.NewConstructionSaverDB(db)}
	f.runtime = ecs.InitResource(surface.world, hearth.NewRuntime(surface.world, surface.chunkManager, db.Queries(), surface.chunkManager.ObjectFactory(), f.saver, behaviors.MustDefaultRegistry(), bus, logger))
	for _, shard := range f.shards {
		shard.permanentDeathNotify = f.runtime.NotifyTerminal
	}
	g.transferService = NewPlayerTransferService(g, logger)
	g.setState(GameStateRunning)
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(g.Stop) }
	t.Cleanup(stop)
	return f, g, stop
}

type hearthShutdownTransferDeath struct{}

func (hearthShutdownTransferDeath) Key() string { return "hearth_shutdown_death" }
func (hearthShutdownTransferDeath) CaptureSource(*Game, *Shard, PlayerTransferRequest, types.Handle) (any, error) {
	return nil, nil
}
func (hearthShutdownTransferDeath) RestoreTarget(_ *Game, shard *Shard, _ PlayerTransferRequest, handle types.Handle, _ any) error {
	ecs.WithComponent(shard.world, handle, func(health *components.EntityHealth) { health.HHP, health.SHP = 0, 0 })
	return nil
}
func (hearthShutdownTransferDeath) RestoreSourceRollback(*Game, *Shard, PlayerTransferRequest, types.Handle, any) error {
	return nil
}
func (hearthShutdownTransferDeath) OnTargetRestoreFailure(*Game, *Shard, PlayerTransferRequest, types.Handle, any, error) {
}

func TestHearthTerminalPostgresGameStopWaitsForAdmittedOwningCalls(t *testing.T) {
	for _, entry := range []string{"auth", "transfer"} {
		t.Run(entry, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)
			networkStopped, shardsStopping := make(chan struct{}, 1), make(chan struct{}, 1)
			logger := zap.New(core, zap.WithFatalHook(zapcore.WriteThenGoexit), zap.Hooks(func(entry zapcore.Entry) error {
				switch entry.Message {
				case "WebSocket server stopped":
					networkStopped <- struct{}{}
				case "Stopping shardManager":
					shardsStopping <- struct{}{}
				}
				return nil
			}))
			f, g, stop := newHearthShutdownFixture(t, logger)
			character := f.addCharacter(t, 10, 1)
			homeCoord := types.ChunkCoord{X: 4}
			home := hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 1010}, X: 4*constt.ChunkWorldSize + 400, Y: 400}
			_, err := f.db.Pool().Exec(t.Context(), `UPDATE object SET x=$1,chunk_x=4 WHERE id=1010`, home.X)
			require.NoError(t, err)
			f.shards[0].chunkManager.GetChunk(types.ChunkCoord{}).RemoveRawObjectByID(1010)
			_, err = f.runtime.Index.ReplaceHome(10, home)
			require.NoError(t, err)

			connected := make(chan *network.Client, 1)
			g.networkServer.SetOnConnect(func(client *network.Client) { connected <- client })
			mux := http.NewServeMux()
			require.NoError(t, g.networkServer.Start("127.0.0.1:0", mux))
			httpServer := httptest.NewServer(mux)
			t.Cleanup(httpServer.Close)
			connection, _, _, err := ws.Dial(t.Context(), "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws")
			require.NoError(t, err)
			t.Cleanup(func() { _ = connection.Close() })
			client := waitHearthTerminal(t, connected)

			entered, completed := make(chan struct{}, 1), make(chan struct{})
			releaseCompletion, releaseSave := make(chan struct{}), make(chan struct{})
			var completionOnce, saveOnce sync.Once
			unblockCompletion := func() { completionOnce.Do(func() { close(releaseCompletion) }) }
			unblockSave := func() { saveOnce.Do(func() { close(releaseSave) }) }
			t.Cleanup(unblockCompletion)
			t.Cleanup(unblockSave)
			deathShard := f.shards[1]
			deathShard.permanentDeathComplete = func(id types.EntityID) error {
				defer close(completed)
				entered <- struct{}{}
				<-releaseCompletion
				f.shards[0].mu.Lock()
				defer f.shards[0].mu.Unlock()
				return f.runtime.CompleteTerminal(id)
			}
			saving := make(chan types.EntityID, 1)
			f.saver.beforeSave = func(id types.EntityID) { saving <- id; <-releaseSave }

			source := deathShard
			if entry == "transfer" {
				source = f.shards[0]
			}
			player := spawnSoundLifecycleEntity(t, source, 10, 200, 200)
			ecs.WithComponent(source.world, player, func(info *components.EntityInfo) { info.Region = 1 })
			ecs.AddComponent(source.world, player, components.Appearance{Resource: "player"})
			ecs.AddComponent(source.world, player, components.EntityHealth{HHP: 20, SHP: 20})
			if entry == "auth" {
				ecs.WithComponent(source.world, player, func(health *components.EntityHealth) { health.HHP, health.SHP = 0, 0 })
				ecs.GetResource[ecs.DetachedEntities](source.world).AddDetachedEntity(10, player, g.clock.GameNow().Add(time.Minute), g.clock.GameNow())
				_, err = f.db.Pool().Exec(t.Context(), `UPDATE character SET auth_token='shutdown-login',is_online=false WHERE id=10`)
				require.NoError(t, err)
				g.handleAuth(client, 1, &netproto.C2S_Auth{Token: "shutdown-login"})
			} else {
				client.CharacterID, client.Layer = 10, 0
				require.True(t, g.attachClientToWorld(source, client, 10, character, player))
				g.transferService.RegisterParticipant(hearthShutdownTransferDeath{})
				require.NoError(t, g.transferService.RequestTransfer(PlayerTransferRequest{PlayerID: 10, SourceLayer: 0, TargetLayer: 1, TargetX: 200, TargetY: 200, IgnoreObjectCollision: true}))
			}
			waitHearthTerminal(t, entered)
			t.Cleanup(func() {
				unblockCompletion()
				unblockSave()
				waitHearthTerminal(t, completed)
			})
			require.True(t, deathShard.mu.TryLock(), "terminal completion must have released the death shard")
			deathShard.mu.Unlock()
			require.Nil(t, f.shards[0].chunkManager.GetChunkFast(homeCoord), "the home must require a chunk load after shutdown begins")
			require.True(t, f.runtime.Index.Snapshot(10).PendingTerminal)

			stopped := make(chan struct{})
			go func() { defer close(stopped); stop() }()
			waitHearthTerminal(t, networkStopped)
			select {
			case <-shardsStopping:
				t.Error("Game.Stop began stopping shards before the admitted terminal owning call completed")
			case <-time.After(100 * time.Millisecond):
			}
			assert.Empty(t, logs.FilterMessage("chunk manager stopped").All(), "all shard chunk managers must remain running")
			requireHearthTerminalBlocked(t, stopped)
			unblockCompletion()
			select {
			case id := <-saving:
				require.Equal(t, types.EntityID(10), id)
			case <-completed:
				t.Fatalf("terminal completion ended before saving: %v", logs.FilterLevelExact(zap.FatalLevel).All())
			case <-time.After(5 * time.Second):
				t.Fatal("terminal completion did not load the home and reach its save")
			}
			requireHearthTerminalBlocked(t, stopped)
			require.Empty(t, logs.FilterMessage("Stopping shardManager").All(), "shutdown must also await the terminal commit")
			stored, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
			require.NoError(t, err)
			require.Equal(t, f.hearth.DefID, stored.TypeID, "the gated terminal transaction has not committed")
			unblockSave()
			waitHearthTerminal(t, completed)
			waitHearthTerminal(t, stopped)
			require.Equal(t, GameStateStopped, g.State())
			require.Empty(t, logs.FilterLevelExact(zap.FatalLevel).All())
			require.Len(t, logs.FilterMessage("chunk manager stopped").All(), 2)
			f.requireDurableDeath(t, 10)
			loaded := f.shards[0].chunkManager.GetChunkFast(homeCoord)
			require.NotNil(t, loaded)
			require.NotEqual(t, types.ChunkStateActive, loaded.GetState())
			require.NotNil(t, loaded.GetRawObjectByID(1010))
			require.Equal(t, types.InvalidHandle, f.shards[0].world.GetHandleByEntityID(1010), "terminal loading must not activate the distant home")
		})
	}
}
