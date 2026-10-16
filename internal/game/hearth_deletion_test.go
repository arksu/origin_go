package game

import (
	"context"
	"sync"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/hearth"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func hearthDeletionCharacterRow(t *testing.T, f *hearthTerminalGameFixture, id types.EntityID) string {
	t.Helper()
	var row string
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT to_jsonb(c)::text FROM character c WHERE id=$1`, int64(id)).Scan(&row))
	return row
}

func requireHearthDeletionDurable(t *testing.T, f *hearthTerminalGameFixture, id, homeID types.EntityID) {
	t.Helper()
	var deleted bool
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL FROM character WHERE id=$1`, int64(id)).Scan(&deleted))
	require.True(t, deleted)
	var associations int
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT count(*) FROM character_hearth WHERE character_id=$1`, int64(id)).Scan(&associations))
	require.Zero(t, associations)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	stored, err := f.db.Queries().GetObjectByID(t.Context(), int64(homeID))
	require.NoError(t, err)
	require.Equal(t, campfire.DefID, stored.TypeID)
	require.Equal(t, hearth.Snapshot{State: hearth.NoHome, Terminal: true}, f.runtime.Index.Snapshot(id))
}

func hearthDeletionUnloadedHome(t *testing.T, f *hearthTerminalGameFixture, id types.EntityID) types.ChunkCoord {
	t.Helper()
	coord := types.ChunkCoord{X: 4}
	_, err := f.db.Pool().Exec(t.Context(), `UPDATE object SET x=$1,chunk_x=$2 WHERE id=$3`, 4*constt.ChunkWorldSize+400, coord.X, int64(1000+id))
	require.NoError(t, err)
	f.shards[0].chunkManager.GetChunk(types.ChunkCoord{}).RemoveRawObjectByID(1000 + id)
	f.runtime.Index = hearth.NewIndex()
	require.Nil(t, f.shards[0].chunkManager.GetChunkFast(coord))
	require.Equal(t, hearth.Unknown, f.runtime.Index.Snapshot(id).State)
	return coord
}

func TestHearthDeletionPostgresNoopLeavesOwnershipAndStorageUntouched(t *testing.T) {
	for _, scenario := range []string{"other_account", "absent_character", "already_deleted"} {
		t.Run(scenario, func(t *testing.T) {
			f := newHearthTerminalGameFixture(t)
			f.addCharacter(t, 10, 0)
			params := repository.DeleteCharacterParams{ID: 10, AccountID: 1}
			switch scenario {
			case "other_account":
				params.AccountID = 2
			case "absent_character":
				params.ID = 999
			case "already_deleted":
				_, err := f.db.Pool().Exec(t.Context(), `UPDATE character SET deleted_at=now() WHERE id=10`)
				require.NoError(t, err)
			}
			beforeCharacter := hearthDeletionCharacterRow(t, f, 10)
			beforeHome, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
			require.NoError(t, err)
			beforeIndex := f.runtime.Index.Snapshot(10)
			saves := 0
			f.saver.beforeSave = func(types.EntityID) { saves++ }
			require.NoError(t, f.games[0].DeleteCharacter(t.Context(), params))
			require.Equal(t, beforeCharacter, hearthDeletionCharacterRow(t, f, 10))
			afterHome, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
			require.NoError(t, err)
			require.Equal(t, beforeHome, afterHome)
			require.Equal(t, beforeIndex, f.runtime.Index.Snapshot(10))
			require.Equal(t, hearth.Snapshot{}, f.runtime.Index.Snapshot(999))
			var homeID int64
			require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT object_id FROM character_hearth WHERE character_id=10`).Scan(&homeID))
			require.Equal(t, int64(1010), homeID)
			require.Zero(t, saves)
		})
	}
}

func TestHearthDeletionPostgresOfflineUnknownLoadsUnloadedHomeAndDuplicateIsNoop(t *testing.T) {
	f, game, _ := newHearthShutdownFixture(t, zap.NewNop())
	f.addCharacter(t, 10, 1)
	coord := hearthDeletionUnloadedHome(t, f, 10)
	saves := 0
	f.saver.beforeSave = func(types.EntityID) { saves++ }
	params := repository.DeleteCharacterParams{ID: 10, AccountID: 1}
	require.NoError(t, game.DeleteCharacter(t.Context(), params))
	requireHearthDeletionDurable(t, f, 10, 1010)
	loaded := f.shards[0].chunkManager.GetChunkFast(coord)
	require.NotNil(t, loaded)
	require.NotEqual(t, types.ChunkStateActive, loaded.GetState())
	require.NotNil(t, loaded.GetRawObjectByID(1010))
	require.Equal(t, types.InvalidHandle, f.shards[0].world.GetHandleByEntityID(1010), "terminal loading must not activate the distant home")
	beforeCharacter := hearthDeletionCharacterRow(t, f, 10)
	beforeHome, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
	require.NoError(t, err)
	require.NoError(t, game.DeleteCharacter(t.Context(), params))
	require.Equal(t, beforeCharacter, hearthDeletionCharacterRow(t, f, 10))
	afterHome, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
	require.NoError(t, err)
	require.Equal(t, beforeHome, afterHome)
	require.Equal(t, 1, saves)
}

func TestHearthDeletionPostgresOnlineBodyAndSessionSurvive(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	surface, game := f.shards[0], f.games[0]
	player := spawnSoundLifecycleEntity(t, surface, 10, 200, 200)
	ecs.AddComponent(surface.world, player, components.EntityHealth{HHP: 20, SHP: 20})
	ecs.GetResource[ecs.CharacterEntities](surface.world).Add(10, player, time.Time{})
	client, _ := connectPlayerSpawnTestClient(t)
	client.CharacterID, client.Layer = 10, 0
	client.InWorld.Store(true)
	surface.Clients[10] = client
	_, err := f.db.Pool().Exec(t.Context(), `UPDATE character SET is_online=true WHERE id=10`)
	require.NoError(t, err)
	health, _ := ecs.GetComponent[components.EntityHealth](surface.world, player)
	position, _ := ecs.GetComponent[components.Transform](surface.world, player)
	characterEntry := ecs.GetResource[ecs.CharacterEntities](surface.world).Map[10]
	require.NoError(t, game.DeleteCharacter(t.Context(), repository.DeleteCharacterParams{ID: 10, AccountID: 1}))
	requireHearthDeletionDurable(t, f, 10, 1010)
	require.Equal(t, player, surface.world.GetHandleByEntityID(10))
	require.True(t, surface.world.Alive(player))
	afterHealth, ok := ecs.GetComponent[components.EntityHealth](surface.world, player)
	require.True(t, ok)
	require.Equal(t, health, afterHealth)
	afterPosition, _ := ecs.GetComponent[components.Transform](surface.world, player)
	require.Equal(t, position, afterPosition)
	require.Equal(t, characterEntry, ecs.GetResource[ecs.CharacterEntities](surface.world).Map[10])
	require.Same(t, client, surface.Clients[10])
	require.True(t, client.InWorld.Load())
	require.Equal(t, types.EntityID(10), client.CharacterID)
	var online bool
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT is_online FROM character WHERE id=10`).Scan(&online))
	require.True(t, online, "deletion must preserve the existing online policy")
}

func TestHearthDeletionPostgresKnownNoHomeNeedsNoHearthIO(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	_, err := f.db.Pool().Exec(t.Context(), `DELETE FROM character_hearth WHERE character_id=10`)
	require.NoError(t, err)
	// Missing Hearth I/O dependencies make an accidental lookup, chunk load or
	// terminal save fail; the authorized character UPDATE still uses the real DB.
	*f.runtime = hearth.NewRuntime(f.shards[0].world, nil, nil, nil, nil, nil, nil, zap.NewNop())
	require.NoError(t, f.runtime.Index.LoadIfUnknown(10, nil))
	require.NoError(t, f.games[0].DeleteCharacter(t.Context(), repository.DeleteCharacterParams{ID: 10, AccountID: 1}))
	require.Equal(t, hearth.Snapshot{State: hearth.NoHome, Terminal: true}, f.runtime.Index.Snapshot(10))
	var deleted bool
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL FROM character WHERE id=10`).Scan(&deleted))
	require.True(t, deleted)
	stored, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
	require.NoError(t, err)
	require.Equal(t, f.hearth.DefID, stored.TypeID)
}

func TestHearthDeletionPostgresCancellationAfterDeleteStillWaitsForCleanup(t *testing.T) {
	f, game, _ := newHearthShutdownFixture(t, zap.NewNop())
	f.addCharacter(t, 10, 1)
	hearthDeletionUnloadedHome(t, f, 10)
	entered, release, done := make(chan types.EntityID, 1), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.saver.beforeSave = func(id types.EntityID) { entered <- id; <-release }
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() {
		defer close(done)
		result <- game.DeleteCharacter(ctx, repository.DeleteCharacterParams{ID: 10, AccountID: 1})
	}()
	t.Cleanup(func() { unblock(); waitHearthTerminal(t, done) })
	require.Equal(t, types.EntityID(10), waitHearthTerminal(t, entered))
	var deleted bool
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL FROM character WHERE id=10`).Scan(&deleted))
	require.True(t, deleted, "authorized soft deletion must precede the terminal transaction")
	cancel()
	requireHearthTerminalBlocked(t, done)
	if f.shards[0].mu.TryLock() {
		f.shards[0].mu.Unlock()
		t.Fatal("deletion released the surface shard before terminal persistence completed")
	}
	stored, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
	require.NoError(t, err)
	require.Equal(t, f.hearth.DefID, stored.TypeID)
	unblock()
	waitHearthTerminal(t, done)
	require.NoError(t, waitHearthTerminal(t, result))
	requireHearthDeletionDurable(t, f, 10, 1010)
}

type hearthDeletionConstructionSaver struct {
	*hearth.ConstructionSaverDB
	beforeSave func()
}

func (s *hearthDeletionConstructionSaver) SaveConstruction(ctx context.Context, character types.EntityID, next, previous *repository.Object) error {
	s.beforeSave()
	return s.ConstructionSaverDB.SaveConstruction(ctx, character, next, previous)
}

func hearthDeletionConstructionContext(t *testing.T, f *hearthTerminalGameFixture, id types.EntityID) contracts.ConstructionContext {
	t.Helper()
	w := f.shards[0].world
	player := w.GetHandleByEntityID(10)
	if player == types.InvalidHandle {
		player = w.Spawn(10, func(w *ecs.World, handle types.Handle) {
			ecs.AddComponent(w, handle, components.EntityHealth{HHP: 20, SHP: 20})
		})
	}
	site := w.Spawn(id, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{TypeID: constt.BuildObjectTypeID, Region: 1})
		ecs.AddComponent(w, handle, components.Transform{X: 700, Y: 700})
		ecs.AddComponent(w, handle, components.ChunkRef{})
		ecs.AddComponent(w, handle, components.ObjectInternalState{})
		ecs.AddComponent(w, handle, components.Appearance{Resource: "build_site"})
	})
	require.NotEqual(t, types.InvalidHandle, site)
	f.shards[0].chunkManager.GetChunk(types.ChunkCoord{}).Spatial().AddStatic(site, 700, 700)
	return contracts.ConstructionContext{World: w, PlayerID: 10, PlayerHandle: player, TargetID: id, TargetHandle: site, ResultType: uint32(f.hearth.DefID)}
}

func TestHearthDeletionPostgresConstructionCommitPrecedesDeleteAndLateBuildIsRefused(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	surface, game := f.shards[0], f.games[0]
	index := f.runtime.Index
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	saver := &hearthDeletionConstructionSaver{ConstructionSaverDB: hearth.NewConstructionSaverDB(f.db), beforeSave: func() { close(entered); <-release }}
	*f.runtime = hearth.NewRuntime(surface.world, surface.chunkManager, f.db.Queries(), surface.chunkManager.ObjectFactory(), saver, behaviors.MustDefaultRegistry(), surface.eventBus, surface.logger)
	f.runtime.Index = index
	construction := hearthDeletionConstructionContext(t, f, 2000)
	constructionDone, deletionDone := make(chan struct{}), make(chan struct{})
	constructionResult := make(chan contracts.BehaviorResult, 1)
	deletionResult := make(chan error, 1)
	go func() {
		defer close(constructionDone)
		surface.mu.Lock()
		defer surface.mu.Unlock()
		constructionResult <- f.runtime.CompleteConstruction(&construction, func() bool {
			return gameworld.TransformObjectToDefInPlace(surface.world, construction.TargetID, construction.TargetHandle, f.hearth, gameworld.TransformObjectInPlaceOptions{BehaviorRegistry: behaviors.MustDefaultRegistry()})
		})
	}()
	t.Cleanup(func() { unblock(); waitHearthTerminal(t, constructionDone) })
	waitHearthTerminal(t, entered)
	go func() {
		defer close(deletionDone)
		deletionResult <- game.DeleteCharacter(t.Context(), repository.DeleteCharacterParams{ID: 10, AccountID: 1})
	}()
	t.Cleanup(func() { unblock(); waitHearthTerminal(t, deletionDone) })
	require.Never(t, func() bool {
		var deleted bool
		err := f.db.Pool().QueryRow(t.Context(), `SELECT deleted_at IS NOT NULL FROM character WHERE id=10`).Scan(&deleted)
		require.NoError(t, err)
		return deleted
	}, 100*time.Millisecond, 5*time.Millisecond, "DELETE SQL must wait for the owning construction save, not only its cleanup")
	requireHearthTerminalBlocked(t, deletionDone)
	unblock()
	waitHearthTerminal(t, constructionDone)
	require.True(t, waitHearthTerminal(t, constructionResult).OK)
	waitHearthTerminal(t, deletionDone)
	require.NoError(t, waitHearthTerminal(t, deletionResult))
	requireHearthDeletionDurable(t, f, 10, 2000)
	oldHome, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
	require.NoError(t, err)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	require.Equal(t, campfire.DefID, oldHome.TypeID)
	late := hearthDeletionConstructionContext(t, f, 2001)
	charged := false
	surface.mu.Lock()
	lateResult := f.runtime.CompleteConstruction(&late, func() bool { charged = true; return true })
	surface.mu.Unlock()
	require.False(t, lateResult.OK)
	require.False(t, charged)
	info, _ := ecs.GetComponent[components.EntityInfo](surface.world, late.TargetHandle)
	require.Equal(t, uint32(constt.BuildObjectTypeID), info.TypeID)
}

func TestHearthDeletionPostgresAcceptedCleanupBlocksShardStopAndLateDeleteIsRefused(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	shardsStopping := make(chan struct{}, 1)
	logger := zap.New(core, zap.WithFatalHook(zapcore.WriteThenGoexit), zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Message == "Stopping shardManager" {
			shardsStopping <- struct{}{}
		}
		return nil
	}))
	f, game, stop := newHearthShutdownFixture(t, logger)
	f.addCharacter(t, 10, 1)
	f.addCharacter(t, 20, 0)
	hearthDeletionUnloadedHome(t, f, 10)
	entered, release, deleted, stopped := make(chan types.EntityID, 1), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.saver.beforeSave = func(id types.EntityID) { entered <- id; <-release }
	result := make(chan error, 1)
	go func() {
		defer close(deleted)
		result <- game.DeleteCharacter(t.Context(), repository.DeleteCharacterParams{ID: 10, AccountID: 1})
	}()
	t.Cleanup(func() { unblock(); waitHearthTerminal(t, deleted) })
	require.Equal(t, types.EntityID(10), waitHearthTerminal(t, entered))
	go func() { defer close(stopped); stop() }()
	t.Cleanup(func() { unblock(); waitHearthTerminal(t, stopped) })
	waitHearthTerminal(t, shardsStopping)
	select {
	case <-stopped:
		t.Fatal("Game.Stop returned while the admitted deletion still owned the surface shard")
	case <-time.After(100 * time.Millisecond):
	}
	if f.shards[0].mu.TryLock() {
		f.shards[0].mu.Unlock()
		t.Fatal("admitted deletion must retain the surface lock through cleanup")
	}
	unblock()
	waitHearthTerminal(t, deleted)
	require.NoError(t, waitHearthTerminal(t, result))
	waitHearthTerminal(t, stopped)
	requireHearthDeletionDurable(t, f, 10, 1010)
	require.Empty(t, logs.FilterLevelExact(zap.FatalLevel).All())
	require.Equal(t, GameStateStopped, game.State())
	before := hearthDeletionCharacterRow(t, f, 20)
	require.Error(t, game.DeleteCharacter(t.Context(), repository.DeleteCharacterParams{ID: 20, AccountID: 1}))
	require.Equal(t, before, hearthDeletionCharacterRow(t, f, 20))
	require.Equal(t, hearth.Snapshot{}, f.runtime.Index.Snapshot(20))
}

func TestHearthDeletionPostgresUnavailableOwnerAndCanceledRequestDoNotDelete(t *testing.T) {
	for _, scenario := range []string{"surface_stopping", "missing_surface", "missing_runtime", "canceled_request"} {
		t.Run(scenario, func(t *testing.T) {
			f := newHearthTerminalGameFixture(t)
			f.addCharacter(t, 10, 0)
			game, surface := f.games[0], f.shards[0]
			before := hearthDeletionCharacterRow(t, f, 10)
			beforeIndex := f.runtime.Index.Snapshot(10)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch scenario {
			case "surface_stopping":
				surface.state = ShardStateStopping
			case "missing_surface":
				delete(game.shardManager.shards, 0)
			case "missing_runtime":
				surface.world = ecs.NewWorldWithCapacity(8, surface.eventBus, 0)
			case "canceled_request":
				cancel()
			}
			require.Error(t, game.DeleteCharacter(ctx, repository.DeleteCharacterParams{ID: 10, AccountID: 1}))
			require.Equal(t, before, hearthDeletionCharacterRow(t, f, 10))
			require.Equal(t, beforeIndex, f.runtime.Index.Snapshot(10))
		})
	}
}

func TestHearthDeletionPostgresDeleteErrorLeavesOwnershipAndStorageUntouched(t *testing.T) {
	f := newHearthTerminalGameFixture(t)
	f.addCharacter(t, 10, 0)
	game, surface := f.games[0], f.shards[0]
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core, zap.WithFatalHook(zapcore.WriteThenPanic))
	game.logger = logger
	index := f.runtime.Index
	*f.runtime = hearth.NewRuntime(surface.world, surface.chunkManager, f.db.Queries(), surface.chunkManager.ObjectFactory(), f.saver, behaviors.MustDefaultRegistry(), surface.eventBus, logger)
	f.runtime.Index = index
	// Reject the actual UPDATE in PostgreSQL, independently of cancellation or
	// the game admission checks. This constraint exists only in the test schema.
	_, err := f.db.Pool().Exec(t.Context(), `ALTER TABLE character ADD CONSTRAINT reject_test_deletion CHECK (deleted_at IS NULL)`)
	require.NoError(t, err)
	beforeCharacter := hearthDeletionCharacterRow(t, f, 10)
	beforeHome, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
	require.NoError(t, err)
	beforeAssociation, err := f.db.Queries().GetSavedCharacterHearth(t.Context(), 10)
	require.NoError(t, err)
	beforeIndex := f.runtime.Index.Snapshot(10)
	saves := 0
	f.saver.beforeSave = func(types.EntityID) { saves++ }
	require.NotPanics(t, func() {
		err = game.DeleteCharacter(t.Context(), repository.DeleteCharacterParams{ID: 10, AccountID: 1})
	})
	require.ErrorContains(t, err, "reject_test_deletion")
	require.Empty(t, logs.FilterLevelExact(zap.FatalLevel).All())
	require.Equal(t, beforeCharacter, hearthDeletionCharacterRow(t, f, 10))
	afterHome, err := f.db.Queries().GetObjectByID(t.Context(), 1010)
	require.NoError(t, err)
	require.Equal(t, beforeHome, afterHome)
	afterAssociation, err := f.db.Queries().GetSavedCharacterHearth(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, beforeAssociation, afterAssociation)
	require.Equal(t, beforeIndex, f.runtime.Index.Snapshot(10))
	require.Zero(t, saves)
}
