package game

import (
	"context"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/game/hearth"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestHearthRecoveryPostgresOrdinaryShutdownPreservesIndependentFinalSaves(t *testing.T) {
	logCore, logs := observer.New(zap.InfoLevel)
	logger := zap.New(logCore, zap.WithFatalHook(zapcore.WriteThenPanic))
	f, g, stop := newHearthShutdownFixture(t, logger)
	f.addCharacter(t, 10, 0)
	surface := f.shards[0]
	surface.characterSaver = systems.NewCharacterSaver(f.db, 1, inventory.NewInventorySaver(logger), logger)
	campfire, found := objectdefs.Global().GetByKey("campfire")
	require.True(t, found)
	playerDef, found := objectdefs.Global().GetByKey("player")
	require.True(t, found)

	// The first replacement loads and converts a distant raw home. The second
	// replaces a live home, leaving both persistence paths dirty at shutdown.
	oldCoord := types.ChunkCoord{X: 4}
	oldHome := hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 1010}, X: 4*constt.ChunkWorldSize + 400, Y: 400}
	_, err := f.db.Pool().Exec(t.Context(), `UPDATE object SET x=$1,chunk_x=4 WHERE id=1010`, oldHome.X)
	require.NoError(t, err)
	surface.chunkManager.GetChunk(types.ChunkCoord{}).RemoveRawObjectByID(1010)
	_, err = f.runtime.Index.ReplaceHome(10, oldHome)
	require.NoError(t, err)
	player := spawnSoundLifecycleEntity(t, surface, 10, 200, 200)
	func() {
		surface.mu.Lock()
		defer surface.mu.Unlock()
		ecs.WithComponent(surface.world, player, func(info *components.EntityInfo) {
			info.TypeID, info.Region = uint32(playerDef.DefID), 1
		})
		ecs.AddComponent(surface.world, player, components.EntityHealth{SHP: 15.5, HHP: 20})
		ecs.AddComponent(surface.world, player, components.EntityStats{Stamina: 63, Energy: 812})
		ecs.AddComponent(surface.world, player, components.CharacterProfile{})
		ecs.GetResource[ecs.CharacterEntities](surface.world).Add(10, player, time.Time{})
		ecs.GetResource[ecs.TimeState](surface.world).Tick = 123
		registry := behaviors.MustDefaultRegistry()
		for _, id := range []types.EntityID{2010, 3010} {
			site := surface.world.Spawn(id, func(w *ecs.World, handle types.Handle) {
				ecs.AddComponent(w, handle, components.EntityInfo{TypeID: constt.BuildObjectTypeID, Region: 1})
				ecs.AddComponent(w, handle, components.Transform{X: float64(id / 10), Y: 400})
				ecs.AddComponent(w, handle, components.ChunkRef{})
				ecs.AddComponent(w, handle, components.ObjectInternalState{})
				ecs.AddComponent(w, handle, components.Appearance{Resource: "build_site"})
			})
			require.NotEqual(t, types.InvalidHandle, site)
			surface.chunkManager.AddStaticToChunkSpatial(site, 0, 0, int(id/10), 400)
			result := f.runtime.CompleteConstruction(&contracts.ConstructionContext{
				World: surface.world, PlayerID: 10, PlayerHandle: player,
				TargetID: id, TargetHandle: site, ResultType: uint32(f.hearth.DefID),
			}, func() bool {
				return gameworld.TransformObjectToDefInPlace(surface.world, id, site, f.hearth,
					gameworld.TransformObjectInPlaceOptions{BehaviorRegistry: registry, Chunks: surface.chunkManager})
			})
			require.True(t, result.OK, "%+v", result)
		}
		require.NotEqual(t, types.ChunkStateActive, surface.chunkManager.GetChunkFast(oldCoord).GetState())
		require.Equal(t, types.InvalidHandle, surface.world.GetHandleByEntityID(1010))

		// These changes happen after the synchronous Hearth commits, so checking
		// them after restart requires the ordinary character and chunk final saves.
		ecs.WithComponent(surface.world, player, func(position *components.Transform) { position.X, position.Y = 250, 260 })
		liveCampfire := surface.world.GetHandleByEntityID(2010)
		ecs.WithComponent(surface.world, liveCampfire, func(state *components.ObjectInternalState) {
			state.HP, state.IsDirty = 19.5, true
		})
		rawChunk := surface.chunkManager.GetChunkFast(oldCoord)
		rawCampfire := *rawChunk.GetRawObjectByID(1010)
		rawCampfire.Hp.Float64 = 17.5
		rawChunk.UpsertRawObject(&rawCampfire)
	}()
	for _, id := range []int64{1010, 2010} {
		stored, err := f.db.Queries().GetObjectByID(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, campfire.DefID, stored.TypeID)
		require.Equal(t, float64(campfire.HP), stored.Hp.Float64)
	}

	// Hold a real character row lock to expose the existing final-save ordering.
	// Chunk shutdown must not overtake CharacterSaver.Stop while its write waits.
	blocker, err := f.db.Pool().Begin(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback(context.Background()) })
	_, err = blocker.Exec(t.Context(), `SELECT id FROM character WHERE id=10 FOR UPDATE`)
	require.NoError(t, err)
	stopped := make(chan struct{})
	go func() { defer close(stopped); stop() }()
	t.Cleanup(func() {
		_ = blocker.Rollback(context.Background())
		waitHearthTerminal(t, stopped)
	})
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.db.Pool().QueryRow(t.Context(), `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, int32(blocker.Conn().PgConn().PID())).Scan(&blocked)
		return err == nil && blocked
	}, 3*time.Second, 10*time.Millisecond, "the ordinary final character write must reach PostgreSQL")
	require.Len(t, logs.FilterMessage("All character snapshots accepted").All(), 1)
	requireHearthTerminalBlocked(t, stopped)
	for _, id := range []int64{1010, 2010} {
		stored, err := f.db.Queries().GetObjectByID(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, float64(campfire.HP), stored.Hp.Float64, "chunk final saves must wait for the character saver")
	}
	require.NoError(t, blocker.Commit(t.Context()))
	waitHearthTerminal(t, stopped)
	require.Equal(t, GameStateStopped, g.State())
	require.Empty(t, logs.FilterLevelExact(zap.ErrorLevel).All())

	character, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, 250, character.X)
	require.Equal(t, 260, character.Y)
	require.Equal(t, 63.0, character.Stamina)
	require.Equal(t, 812.0, character.Energy)
	require.Equal(t, 15.5, character.Shp)
	var homeCount int
	require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT count(*) FROM character_hearth WHERE character_id=10`).Scan(&homeCount))
	require.Equal(t, 1, homeCount)

	// A new runtime and real loader must recover solely from committed rows.
	restarted := &Shard{layer: 0, cfg: g.cfg, db: f.db, logger: logger, world: ecs.NewWorldWithCapacity(32, nil, 0)}
	factory := gameworld.NewObjectFactory(nil)
	restarted.chunkManager = gameworld.NewChunkManager(g.cfg, f.db, restarted.world, restarted, 0, 1, factory, behaviors.MustDefaultRegistry(), nil, logger)
	t.Cleanup(restarted.chunkManager.Stop)
	runtime := ecs.InitResource(restarted.world, hearth.NewRuntime(restarted.world, restarted.chunkManager, f.db.Queries(), factory,
		hearth.NewConstructionSaverDB(f.db), behaviors.MustDefaultRegistry(), nil, logger))
	restarted.chunkManager.SetActivationPreparer(runtime.PrepareChunkActivation)
	require.NoError(t, restarted.PrepareEntityAOI(t.Context(), 10, character.X, character.Y))
	func() {
		restarted.mu.Lock()
		defer restarted.mu.Unlock()
		require.NoError(t, runtime.LoadHome(10, f.hearth.DefID))
		require.Equal(t, hearth.CurrentHome, runtime.Index.Snapshot(10).State)
		require.Equal(t, hearth.ObjectKey{Region: 1, ID: 3010}, runtime.Index.Snapshot(10).Home.Object)
		require.True(t, runtime.IgnoreObjectCollision(10, restarted.world.GetHandleByEntityID(3010)))
		require.False(t, runtime.IgnoreObjectCollision(10, restarted.world.GetHandleByEntityID(2010)))
		live, found := ecs.GetComponent[components.ObjectInternalState](restarted.world, restarted.world.GetHandleByEntityID(2010))
		require.True(t, found)
		require.Equal(t, 19.5, live.HP)
	}()
	require.NoError(t, restarted.chunkManager.WaitPreloaded(t.Context(), oldCoord))
	raw := restarted.chunkManager.GetChunkFast(oldCoord).GetRawObjectByID(1010)
	require.NotNil(t, raw)
	require.Equal(t, campfire.DefID, raw.TypeID)
	require.Equal(t, 17.5, raw.Hp.Float64)
	_, owned := runtime.Index.Owner(hearth.ObjectKey{Region: 1, ID: 1010})
	require.False(t, owned)
}
