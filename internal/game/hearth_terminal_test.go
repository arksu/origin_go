package game

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	"origin/internal/game/inventory"
	"origin/internal/objectdefs"
	"origin/internal/persistence"
	"origin/internal/persistence/repository"
	"origin/internal/persistence/testutil"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type hearthTerminalGameSaver struct {
	*hearth.ConstructionSaverDB
	beforeSave func(types.EntityID)
}

func (s *hearthTerminalGameSaver) SaveTerminal(ctx context.Context, character types.EntityID, home hearth.ObjectKey, object *repository.Object) error {
	if s.beforeSave != nil {
		s.beforeSave(character)
	}
	return s.ConstructionSaverDB.SaveTerminal(ctx, character, home, object)
}

type hearthTerminalGameFixture struct {
	db      *persistence.Postgres
	shards  [2]*Shard
	games   [2]*Game
	runtime *hearth.Runtime
	saver   *hearthTerminalGameSaver
	hearth  *objectdefs.ObjectDef
}

func newHearthTerminalGameFixture(t *testing.T) *hearthTerminalGameFixture {
	t.Helper()
	db := testutil.NewPostgres(t, "ORIGIN_OBJECT_HEALTH_TEST_DSN", filepath.Join("..", "..", "migrations", "schema.sql"))
	_, hearthDef := loadHearthBuildRecipeForTest(t)
	f := &hearthTerminalGameFixture{db: db, hearth: hearthDef}
	_, err := db.Pool().Exec(t.Context(), `INSERT INTO account (id,login,password_hash) VALUES (1,'hearth_terminal','unused')`)
	require.NoError(t, err)
	for layer := range f.shards {
		shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 64)
		shard.layer, shard.world.Layer, shard.cfg.Game.Region = layer, layer, 1
		shard.db, game.db = db, db
		game.inventoryLoader = inventory.NewInventoryLoader(shard.logger)
		f.shards[layer], f.games[layer] = shard, game
	}
	surface := f.shards[0]
	f.saver = &hearthTerminalGameSaver{ConstructionSaverDB: hearth.NewConstructionSaverDB(db)}
	f.runtime = ecs.InitResource(surface.world, hearth.NewRuntime(surface.world, surface.chunkManager, db.Queries(), surface.chunkManager.ObjectFactory(), f.saver, behaviors.MustDefaultRegistry(), surface.eventBus, surface.logger))
	for _, shard := range f.shards {
		shard.permanentDeathNotify = f.runtime.NotifyTerminal
		shard.permanentDeathComplete = func(character types.EntityID) error {
			return surface.completeHearthDeath(character)
		}
	}
	return f
}

func (f *hearthTerminalGameFixture) addCharacter(t *testing.T, id types.EntityID, layer int) repository.Character {
	t.Helper()
	_, err := f.db.Pool().Exec(t.Context(), `INSERT INTO character
		(id,account_id,name,region,x,y,layer,heading,stamina,energy,shp,hhp,attributes,exp,skills,discovery)
		VALUES ($1,1,'terminal owner',1,200,200,$2,0,100,900,20,20,'{}','{}','[]','[]')`, int64(id), layer)
	require.NoError(t, err)
	home := hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 1000 + id}, X: 400 + int(id), Y: 400}
	_, err = f.db.Pool().Exec(t.Context(), `INSERT INTO object
		(region,id,type_id,x,y,layer,chunk_x,chunk_y,create_tick,last_tick)
		VALUES (1,$1,$2,$3,$4,0,0,0,1,1)`, int64(home.Object.ID), f.hearth.DefID, home.X, home.Y)
	require.NoError(t, err)
	_, err = f.db.Pool().Exec(t.Context(), `INSERT INTO character_hearth (character_id,object_region,object_id) VALUES ($1,1,$2)`, int64(id), int64(home.Object.ID))
	require.NoError(t, err)
	raw, err := f.db.Queries().GetObjectByID(t.Context(), int64(home.Object.ID))
	require.NoError(t, err)
	f.shards[0].chunkManager.GetChunk(types.ChunkCoord{}).UpsertRawObject(&raw)
	require.NoError(t, f.runtime.Index.LoadIfUnknown(id, &home))
	return repository.Character{ID: int64(id), Name: "terminal owner", Region: 1, X: 200, Y: 200, Layer: layer, Shp: 20, Hhp: 20, Stamina: 100, Energy: 900}
}

func (f *hearthTerminalGameFixture) requireDurableDeath(t *testing.T, id types.EntityID) {
	t.Helper()
	var deleted bool
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL FROM character WHERE id=$1`, int64(id)).Scan(&deleted))
	require.True(t, deleted)
	var associations int
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT count(*) FROM character_hearth WHERE character_id=$1`, int64(id)).Scan(&associations))
	require.Zero(t, associations)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	home, err := f.db.Queries().GetObjectByID(t.Context(), int64(1000+id))
	require.NoError(t, err)
	require.Equal(t, campfire.DefID, home.TypeID)
	corpseDef, found := objectdefs.Global().GetByKey("player_dead")
	require.True(t, found)
	corpse, err := f.db.Queries().GetObjectByID(t.Context(), int64(id))
	require.NoError(t, err)
	require.Equal(t, corpseDef.DefID, corpse.TypeID)
	require.True(t, f.runtime.Index.Snapshot(id).Terminal)
	require.Empty(t, f.shards[corpse.Layer].permanentDeathIDs)
}

func waitHearthTerminal[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("Hearth terminal owning call did not reach the expected boundary")
		var zero T
		return zero
	}
}

func requireHearthTerminalBlocked(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		t.Fatal("owning call returned before terminal persistence completed")
	default:
	}
}

func TestHearthTerminalPostgresDeathOwningCallsWaitWithSourceUnlocked(t *testing.T) {
	for _, entry := range []string{"update", "attach", "reattach", "teleport"} {
		t.Run(entry, func(t *testing.T) {
			f := newHearthTerminalGameFixture(t)
			source, game := f.shards[1], f.games[1]
			character := f.addCharacter(t, 10, 1)
			client, _ := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, 1
			var handle types.Handle
			if entry != "teleport" {
				handle = spawnSoundLifecycleEntity(t, source, 10, 200, 200)
				ecs.WithComponent(source.world, handle, func(info *components.EntityInfo) { info.Region = 1 })
				ecs.AddComponent(source.world, handle, components.Appearance{Resource: "player"})
				ecs.AddComponent(source.world, handle, components.EntityHealth{HHP: 20, SHP: 20})
				ecs.GetResource[ecs.CharacterEntities](source.world).Add(10, handle, time.Time{})
			}
			if entry == "update" {
				require.True(t, game.attachClientToWorld(source, client, 10, character, handle))
				source.world.AddSystem(NewPlayerDeathSystem(source, PlayerDeathSystemConfig{}))
			}
			if entry == "reattach" {
				ecs.GetResource[ecs.DetachedEntities](source.world).AddDetachedEntity(10, handle, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
			}
			if entry != "teleport" {
				ecs.WithComponent(source.world, handle, func(health *components.EntityHealth) { health.HHP, health.SHP = 0, 0 })
			}
			type notification struct{ lockHeld, livingComponents, pending bool }
			notified := make(chan notification, 1)
			source.permanentDeathNotify = func(id types.EntityID) error {
				unlocked := source.mu.TryLock()
				if unlocked {
					source.mu.Unlock()
				}
				err := f.runtime.NotifyTerminal(id)
				notified <- notification{!unlocked, ecs.HasComponent[components.EntityHealth](source.world, source.world.GetHandleByEntityID(id)), f.runtime.Index.Snapshot(id).PendingTerminal}
				return err
			}
			entered, release, done := make(chan types.EntityID, 1), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			f.saver.beforeSave = func(id types.EntityID) { entered <- id; <-release }
			result := make(chan error, 1)
			go func() {
				defer close(done)
				switch entry {
				case "update":
					source.Update(ecs.TimeState{Tick: 1, UnixMs: 10000})
				case "attach":
					game.attachClientToWorld(source, client, 10, character, handle)
				case "reattach":
					game.tryReattachPlayer(client, source, 10, character)
				case "teleport":
					_, err := game.spawnTeleportedPlayerWithRuntime(client, source, character, 200, 200, true, &playerRuntimeState{Health: components.EntityHealth{}}, nil)
					result <- err
				}
			}()
			require.Equal(t, notification{true, true, true}, waitHearthTerminal(t, notified), "notification precedes corpse mutation under the source lock")
			require.Equal(t, types.EntityID(10), waitHearthTerminal(t, entered))
			requireHearthTerminalBlocked(t, done)
			require.True(t, source.mu.TryLock(), "the source shard must be available while surface persistence waits")
			corpse := source.world.GetHandleByEntityID(10)
			hasHealth := ecs.HasComponent[components.EntityHealth](source.world, corpse)
			pending := append([]types.EntityID(nil), source.permanentDeathIDs...)
			source.mu.Unlock()
			require.False(t, hasHealth)
			require.Empty(t, pending, "the owning call must detach its batch before unlocking")
			var deleted bool
			require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL FROM character WHERE id=10`).Scan(&deleted))
			require.True(t, deleted, "ordinary permanent-death persistence precedes terminal completion")
			home, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
			require.NoError(t, err)
			require.Equal(t, f.hearth.DefID, home.TypeID, "the gated terminal save has not committed yet")
			unblock()
			waitHearthTerminal(t, done)
			if entry == "teleport" {
				require.NoError(t, waitHearthTerminal(t, result))
			}
			f.requireDurableDeath(t, 10)
		})
	}
}

func TestHearthTerminalPostgresSimultaneousLayersAndManagerWait(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	for layer, id := range []types.EntityID{10, 20} {
		f.addCharacter(t, id, layer)
	}
	for layer, id := range []types.EntityID{10, 20} {
		shard, game := f.shards[layer], f.games[layer]
		player := spawnSoundLifecycleEntity(t, shard, id, 200, 200)
		ecs.WithComponent(shard.world, player, func(info *components.EntityInfo) { info.Region = 1 })
		ecs.AddComponent(shard.world, player, components.Appearance{Resource: "player"})
		ecs.AddComponent(shard.world, player, components.EntityHealth{HHP: 20, SHP: 20})
		ecs.GetResource[ecs.CharacterEntities](shard.world).Add(id, player, time.Time{})
		client, _ := connectPlayerSpawnTestClient(t)
		client.CharacterID, client.Layer = id, layer
		require.True(t, game.attachClientToWorld(shard, client, id, repository.Character{ID: int64(id), Layer: layer}, player))
		ecs.WithComponent(shard.world, player, func(health *components.EntityHealth) { health.HHP, health.SHP = 0, 0 })
		shard.world.AddSystem(NewPlayerDeathSystem(shard, PlayerDeathSystemConfig{}))
	}
	notified, requested, saving := make(chan types.EntityID, 2), make(chan types.EntityID, 2), make(chan types.EntityID, 2)
	releaseDeaths, releaseSave := make(chan struct{}), make(chan struct{}, 2)
	var releaseDeathsOnce sync.Once
	unblockDeaths := func() { releaseDeathsOnce.Do(func() { close(releaseDeaths) }) }
	var releaseSaveOnce sync.Once
	unblockSave := func() { releaseSaveOnce.Do(func() { close(releaseSave) }) }
	t.Cleanup(unblockDeaths)
	t.Cleanup(unblockSave)
	for _, shard := range f.shards {
		shard.permanentDeathNotify = func(id types.EntityID) error {
			err := f.runtime.NotifyTerminal(id)
			notified <- id
			<-releaseDeaths
			return err
		}
		shard.permanentDeathComplete = func(id types.EntityID) error {
			requested <- id
			return f.shards[0].completeHearthDeath(id)
		}
	}
	f.saver.beforeSave = func(id types.EntityID) { saving <- id; <-releaseSave }
	manager := &ShardManager{shards: map[int]*Shard{0: f.shards[0], 1: f.shards[1]}, workerPool: NewWorkerPool(2)}
	done := make(chan struct{})
	var result ShardUpdateResult
	go func() { defer close(done); result = manager.Update(ecs.TimeState{Tick: 1, UnixMs: 10000}) }()
	require.ElementsMatch(t, []types.EntityID{10, 20}, []types.EntityID{waitHearthTerminal(t, notified), waitHearthTerminal(t, notified)})
	for _, id := range []types.EntityID{10, 20} {
		require.True(t, f.runtime.Index.Snapshot(id).PendingTerminal)
		require.False(t, f.runtime.Index.Snapshot(id).CanReturn())
	}
	unblockDeaths()
	require.ElementsMatch(t, []types.EntityID{10, 20}, []types.EntityID{waitHearthTerminal(t, requested), waitHearthTerminal(t, requested)})
	first := waitHearthTerminal(t, saving)
	requireHearthTerminalBlocked(t, done)
	require.True(t, f.shards[1].mu.TryLock(), "an underground source must unlock before waiting for surface completion")
	f.shards[1].mu.Unlock()
	releaseSave <- struct{}{}
	second := waitHearthTerminal(t, saving)
	require.NotEqual(t, first, second)
	requireHearthTerminalBlocked(t, done)
	releaseSave <- struct{}{}
	waitHearthTerminal(t, done)
	manager.workerPool.Stop()
	require.Len(t, result.ShardDurations, 2)
	f.requireDurableDeath(t, 10)
	f.requireDurableDeath(t, 20)
}

type hearthTerminalBatchSystem struct {
	ecs.BaseSystem
	shard *Shard
	calls int
}

func (s *hearthTerminalBatchSystem) Update(*ecs.World, float64) {
	s.calls++
	if s.calls == 1 {
		s.shard.permanentDeathIDs = append(s.shard.permanentDeathIDs, 10, 20)
	} else if s.calls == 2 {
		s.shard.permanentDeathIDs = append(s.shard.permanentDeathIDs, 30, 40)
	}
}

func TestHearthTerminalOverlappingUpdatesOwnDetachedBatches(t *testing.T) {
	shard, _ := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	// Spare capacity deliberately makes resetting to [:0] unsafe: the next
	// owning call would overwrite the first call's still-unprocessed second ID.
	shard.permanentDeathIDs = make([]types.EntityID, 0, 4)
	shard.world.AddSystem(&hearthTerminalBatchSystem{BaseSystem: ecs.NewBaseSystem("terminal batch fixture", 1), shard: shard})
	entered, completed := make(chan struct{}), make(chan types.EntityID, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	shard.permanentDeathComplete = func(id types.EntityID) error {
		if id == 10 {
			close(entered)
			<-release
		}
		completed <- id
		return nil
	}
	firstDone, secondDone := make(chan struct{}), make(chan struct{})
	go func() { defer close(firstDone); shard.Update(ecs.TimeState{Tick: 1}) }()
	waitHearthTerminal(t, entered)
	go func() { defer close(secondDone); shard.Update(ecs.TimeState{Tick: 2}) }()
	waitHearthTerminal(t, secondDone)
	requireHearthTerminalBlocked(t, firstDone)
	unblock()
	waitHearthTerminal(t, firstDone)
	var actual []types.EntityID
	for range 4 {
		actual = append(actual, waitHearthTerminal(t, completed))
	}
	require.Equal(t, []types.EntityID{30, 40, 10, 20}, actual)
	require.Empty(t, shard.permanentDeathIDs)
}
