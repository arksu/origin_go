package game

import (
	"context"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"origin/internal/actiondefs"
	"origin/internal/characterattrs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/game/hearth"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	"origin/internal/objectdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type hearthReturnTransferFixture struct {
	*hearthTerminalGameFixture
	game       *Game
	source     *Shard
	player     types.Handle
	client     *network.Client
	connection net.Conn
	handler    *hearthReturnActionHandler
	character  repository.Character
}

func newHearthReturnTransferFixture(t *testing.T, sourceLayer int, carrying, lying bool, configure ...func(*hearthReturnTransferFixture)) *hearthReturnTransferFixture {
	t.Helper()
	f := &hearthReturnTransferFixture{hearthTerminalGameFixture: newHearthTerminalGameFixture(t)}
	f.game, f.source = f.games[sourceLayer], f.shards[sourceLayer]
	for _, setup := range configure {
		setup(f)
	}
	g := f.game
	g.ctx, g.transferInFlight = context.Background(), make(map[types.EntityID]chan struct{})
	g.cfg.Game.MaxLayers = 2
	g.objectFactory = f.source.chunkManager.ObjectFactory()
	g.shardManager = &ShardManager{shards: map[int]*Shard{0: f.shards[0], 1: f.shards[1]}}
	g.transferService = NewPlayerTransferService(g, g.logger)
	g.transferService.RegisterParticipant(NewLiftCarryTransferParticipant(g.logger))
	registry := hearthReturnTestDefinitions()
	previous := actiondefs.Global()
	actiondefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { actiondefs.SetGlobalForTesting(previous) })
	for _, shard := range f.shards {
		shard.liftService = NewLiftService(shard.world, shard.chunkManager, shard.eventBus, nil, shard.logger)
		handler := &hearthReturnActionHandler{game: g, shard: shard, index: f.runtime.Index, transfer: g.transferService}
		var err error
		shard.actionService, err = NewActionService(shard.world, registry, map[string]ActionHandler{hearth.ReturnActionID: handler}, shard)
		require.NoError(t, err)
		if shard == f.source {
			f.handler = handler
		}
	}
	f.shards[0].objectCollisionFilter = f.runtime.IgnoreObjectCollision
	f.character = f.addCharacter(t, 10, sourceLayer)
	home := f.runtime.Index.Snapshot(10).Home
	addHearthCollisionTestObject(f.shards[0], home.Object.ID, home.X, home.Y, []string{"hearth"})
	f.client, f.connection = connectPlayerSpawnTestClient(t)
	f.client.CharacterID, f.client.Layer = 10, sourceLayer
	var err error
	f.player, err = g.spawnTeleportedPlayerWithRuntime(f.client, f.source, f.character, 200, 200, false, nil, nil)
	require.NoError(t, err)
	if carrying {
		def, found := objectdefs.Global().GetByKey("box")
		require.True(t, found)
		hp := 100.0
		object, err := g.objectFactory.SpawnWorldObjectFromSnapshot(f.source.world, gameworld.EmbeddedObjectSnapshotV1{
			Version: 1, EntityID: 3000, TypeID: def.DefID, Region: 1, HP: &hp,
		}, gameworld.SnapshotSpawnOptions{X: 200, Y: 200, Layer: sourceLayer, ChunkManager: f.source.chunkManager, BehaviorRegistry: behaviors.MustDefaultRegistry(), Logger: g.logger})
		require.NoError(t, err)
		// Its retained collider is much larger than the player's. Only the player
		// footprint may decide whether the return destination is clear.
		ecs.WithComponent(f.source.world, object, func(collider *components.Collider) { collider.HalfWidth, collider.HalfHeight = 100, 100 })
		result := f.source.liftService.StartLift(f.source.world, 10, f.player, 3000, object)
		require.Equal(t, ActionSucceeded, result.Outcome)
	}
	if lying {
		ecs.WithComponent(f.source.world, f.player, func(health *components.EntityHealth) { health.IsLying, health.LyingRevision = true, 8 })
	}
	f.source.characterSaver = systems.NewCharacterSaver(f.db, 1, inventory.NewInventorySaver(g.logger), g.logger)
	t.Cleanup(f.source.characterSaver.Stop)
	return f
}

func (f *hearthReturnTransferFixture) request(t *testing.T) PlayerTransferRequest {
	t.Helper()
	f.source.mu.Lock()
	defer f.source.mu.Unlock()
	f.source.actionService.Activate(f.source.world, 10, f.player, hearth.ReturnActionID)
	active, exists := ecs.GetComponent[components.ActiveGameAction](f.source.world, f.player)
	require.True(t, exists)
	return f.handler.transferRequest(10, f.player, active.Generation)
}

func (f *hearthReturnTransferFixture) run(t *testing.T, req PlayerTransferRequest) {
	t.Helper()
	require.NoError(t, f.game.transferService.RequestTransfer(req))
	done := make(chan struct{})
	go func() { f.game.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Hearth transfer did not finish")
	}
}

func TestHearthReturnPostgresTransferSuccessAndRollback(t *testing.T) {
	for _, sourceLayer := range []int{0, 1} {
		for _, blocked := range []bool{false, true} {
			t.Run(fmt.Sprintf("layer_%d_blocked_%t", sourceLayer, blocked), func(t *testing.T) {
				f := newHearthReturnTransferFixture(t, sourceLayer, true, true)
				req := f.request(t)
				home := f.runtime.Index.Snapshot(10).Home
				obstacleX := home.X + 20
				if blocked {
					obstacleX = home.X
				}
				addHearthCollisionTestObject(f.shards[0], 4000, obstacleX, home.Y, nil)
				resultCalls := 0
				originalResult := req.OnResult
				req.OnResult = func(shard *Shard, handle types.Handle, err error) {
					resultCalls++
					originalResult(shard, handle, err)
					if err == nil {
						require.False(t, f.client.InWorld.Load(), "cooldown must precede attachment/bootstrap")
						require.Len(t, shard.actionService.State(shard.world, handle).Cooldowns, 1)
					}
				}
				f.run(t, req)
				require.Equal(t, 1, resultCalls)
				resultShard, wantX, wantY := f.shards[0], home.X, home.Y
				if blocked {
					resultShard, wantX, wantY = f.source, 200, 200
				}
				player := resultShard.world.GetHandleByEntityID(10)
				require.True(t, resultShard.world.Alive(player))
				health, exists := ecs.GetComponent[components.EntityHealth](resultShard.world, player)
				require.True(t, exists)
				require.Equal(t, blocked, health.IsLying)
				if blocked {
					require.EqualValues(t, 8, health.LyingRevision)
				}
				position, _ := ecs.GetComponent[components.Transform](resultShard.world, player)
				require.EqualValues(t, wantX, position.X)
				require.EqualValues(t, wantY, position.Y)
				require.Equal(t, !blocked, resultShard.actionService.isOnCooldown(resultShard.world, player, hearth.ReturnActionID))
				carry, exists := ecs.GetComponent[components.LiftCarryState](resultShard.world, player)
				require.True(t, exists)
				require.Equal(t, types.EntityID(3000), carry.ObjectEntityID)
				require.True(t, resultShard.world.Alive(carry.ObjectHandle))
				if sourceLayer != resultShard.layer {
					require.Equal(t, types.InvalidHandle, f.source.world.GetHandleByEntityID(10))
					require.Equal(t, types.InvalidHandle, f.source.world.GetHandleByEntityID(3000))
				}
				row, err := f.db.Queries().GetCharacter(t.Context(), 10)
				require.NoError(t, err)
				require.Equal(t, resultShard.layer, row.Layer)
				require.Equal(t, wantX, row.X)
				require.Equal(t, wantY, row.Y)
				require.False(t, row.DisconnectAt.Valid, "transfer without a socket close is not a disconnect")
				// Ordinary character saving persists the independently committed cooldown.
				if !blocked {
					saver := systems.NewCharacterSaver(f.db, 1, inventory.NewInventorySaver(f.game.logger), f.game.logger)
					defer saver.Stop()
					require.NoError(t, saver.SaveSync(resultShard.world, 10, player))
					row, err = f.db.Queries().GetCharacter(t.Context(), 10)
					require.NoError(t, err)
					cooldowns, err := components.UnmarshalActionCooldowns(row.ActionCooldowns)
					require.NoError(t, err)
					require.EqualValues(t, 370000, cooldowns.ByAction[hearth.ReturnActionID].ExpiresAtMs)
				}
			})
		}
	}
}

func TestHearthReturnPostgresTransferPreservesFreshCharacter(t *testing.T) {
	wantStats := components.EntityStats{Stamina: 675.25, Energy: 432.5}
	wantProfile := components.CharacterProfile{
		Attributes: characterattrs.Values{
			characterattrs.INT: 2, characterattrs.STR: 3, characterattrs.PER: 4,
			characterattrs.PSY: 5, characterattrs.AGI: 6, characterattrs.CON: 7,
			characterattrs.CHA: 8, characterattrs.DEX: 9, characterattrs.WIL: 10,
		},
		Experience: components.CharacterExperience{LP: 987, Nature: 65, Industry: 43, Combat: 21},
		Skills:     []string{"carpentry", "woodcutting"},
		Discovery:  []string{"branch", "stone"},
	}
	for _, sourceLayer := range []int{0, 1} {
		for _, rollback := range []bool{false, true} {
			t.Run(fmt.Sprintf("layer_%d_rollback_%t", sourceLayer, rollback), func(t *testing.T) {
				f := newHearthReturnTransferFixture(t, sourceLayer, true, false)
				// Leave PostgreSQL at the previous checkpoint while the live character
				// has spent energy and gained attributes, experience and discoveries.
				f.source.mu.Lock()
				ecs.WithComponent(f.source.world, f.player, func(stats *components.EntityStats) { *stats = wantStats })
				ecs.WithComponent(f.source.world, f.player, func(profile *components.CharacterProfile) { *profile = wantProfile })
				ecs.WithComponent(f.source.world, f.player, func(position *components.Transform) { position.Direction = math.Pi / 2 })
				f.source.mu.Unlock()
				oldRow, err := f.db.Queries().GetCharacter(t.Context(), 10)
				require.NoError(t, err)
				require.NotEqual(t, wantStats.Stamina, oldRow.Stamina)
				require.NotEqual(t, wantStats.Energy, oldRow.Energy)
				require.Zero(t, oldRow.Heading)
				require.JSONEq(t, `{}`, string(oldRow.Exp))
				require.JSONEq(t, `[]`, string(oldRow.Discovery))

				req := f.request(t)
				home := f.runtime.Index.Snapshot(10).Home
				if rollback {
					addHearthCollisionTestObject(f.shards[0], 4000, home.X, home.Y, nil)
				}
				var resultErr error
				onResult := req.OnResult
				req.OnResult = func(shard *Shard, handle types.Handle, err error) {
					resultErr = err
					onResult(shard, handle, err)
				}
				f.run(t, req)
				resultShard := f.shards[0]
				if rollback {
					require.Error(t, resultErr)
					resultShard = f.source
				} else {
					require.NoError(t, resultErr)
				}

				assertSavedCharacter := func() {
					t.Helper()
					row, err := f.db.Queries().GetCharacter(t.Context(), 10)
					require.NoError(t, err)
					assert.Equal(t, wantStats.Stamina, row.Stamina)
					assert.Equal(t, wantStats.Energy, row.Energy)
					assert.EqualValues(t, 90, row.Heading)
					attributes, _ := characterattrs.FromRaw(row.Attributes)
					experience, err := components.UnmarshalCharacterExperience(row.Exp)
					require.NoError(t, err)
					skills, err := components.UnmarshalStringSet(row.Skills)
					require.NoError(t, err)
					discovery, err := components.UnmarshalStringSet(row.Discovery)
					require.NoError(t, err)
					assert.Equal(t, wantProfile, components.CharacterProfile{
						Attributes: attributes, Experience: experience, Skills: skills, Discovery: discovery,
					})
				}
				assertSavedCharacter()
				resultShard.mu.Lock()
				defer resultShard.mu.Unlock()
				player := resultShard.world.GetHandleByEntityID(10)
				require.True(t, resultShard.world.Alive(player))
				stats, exists := ecs.GetComponent[components.EntityStats](resultShard.world, player)
				require.True(t, exists)
				assert.Equal(t, wantStats, stats)
				profile, exists := ecs.GetComponent[components.CharacterProfile](resultShard.world, player)
				require.True(t, exists)
				assert.Equal(t, wantProfile, profile)
				position, exists := ecs.GetComponent[components.Transform](resultShard.world, player)
				require.True(t, exists)
				assert.InDelta(t, math.Pi/2, position.Direction, 1e-12)
				// A later ordinary save must not overwrite the fresh checkpoint with
				// stale state from either the target body or a rollback body.
				require.NoError(t, f.source.characterSaver.SaveSync(resultShard.world, 10, player))
				assertSavedCharacter()
			})
		}
	}
}

func TestHearthReturnPostgresCharacterRefreshFailurePreservesSource(t *testing.T) {
	f := newHearthReturnTransferFixture(t, 1, true, false)
	req := f.request(t)
	oldRow, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	wantStats := components.EntityStats{Stamina: 612.5, Energy: 333.25}
	ecs.WithComponent(f.source.world, f.player, func(stats *components.EntityStats) { *stats = wantStats })
	carry, exists := ecs.GetComponent[components.LiftCarryState](f.source.world, f.player)
	require.True(t, exists)
	owner, exists := ecs.GetComponent[components.InventoryOwner](f.source.world, f.player)
	require.True(t, exists)
	require.NotEmpty(t, owner.Inventories)

	// SaveSync has its own context and succeeds; only the subsequent refresh
	// uses the canceled game context. No destructive participant may run.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.game.ctx = ctx
	snapshot, err := f.game.transferService.detachTransferSource(req, f.source, oldRow)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, snapshot.ParticipantStates)
	require.Same(t, f.client, f.source.Clients[10])
	require.True(t, f.client.InWorld.Load())
	require.Equal(t, 1, f.client.Layer)
	require.True(t, f.source.world.Alive(f.player))
	require.Equal(t, f.player, f.source.world.GetHandleByEntityID(10))
	currentCarry, exists := ecs.GetComponent[components.LiftCarryState](f.source.world, f.player)
	require.True(t, exists)
	require.Equal(t, carry, currentCarry)
	require.True(t, f.source.world.Alive(carry.ObjectHandle))
	currentOwner, exists := ecs.GetComponent[components.InventoryOwner](f.source.world, f.player)
	require.True(t, exists)
	require.Equal(t, owner, currentOwner)
	for _, link := range owner.Inventories {
		require.True(t, f.source.world.Alive(link.Handle))
	}
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, wantStats.Stamina, row.Stamina)
	require.Equal(t, wantStats.Energy, row.Energy)
}

func TestHearthReturnPostgresCharacterRefreshDeadlinePreservesSource(t *testing.T) {
	f := newHearthReturnTransferFixture(t, 1, true, true)
	req := f.request(t)
	wantStats := components.EntityStats{Stamina: 612.5, Energy: 333.25}
	ecs.WithComponent(f.source.world, f.player, func(stats *components.EntityStats) { *stats = wantStats })
	carry, exists := ecs.GetComponent[components.LiftCarryState](f.source.world, f.player)
	require.True(t, exists)
	owner, exists := ecs.GetComponent[components.InventoryOwner](f.source.world, f.player)
	require.True(t, exists)
	require.NotEmpty(t, owner.Inventories)
	owner.Inventories = append([]components.InventoryLink(nil), owner.Inventories...)
	wantHealth, exists := ecs.GetComponent[components.EntityHealth](f.source.world, f.player)
	require.True(t, exists)

	// The parent has no deadline. Cancellation below is only failure-safe cleanup,
	// so a successful test must observe the reread's own five-second deadline.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.game.ctx = ctx
	_, hasDeadline := ctx.Deadline()
	require.False(t, hasDeadline)
	rowLock, err := f.db.Pool().Begin(t.Context())
	require.NoError(t, err)
	defer rowLock.Rollback(context.Background())
	var rowLockPID int
	require.NoError(t, rowLock.QueryRow(t.Context(), `SELECT pg_backend_pid() FROM character WHERE id=10 FOR UPDATE`).Scan(&rowLockPID))
	tableLock, err := f.db.Pool().Begin(t.Context())
	require.NoError(t, err)
	defer tableLock.Rollback(context.Background())
	var tableLockPID int
	require.NoError(t, tableLock.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&tableLockPID))
	lockCtx, cancelLock := context.WithCancel(t.Context())
	lockDone := make(chan struct{})
	var lockErr error
	lockStarted := false
	defer func() {
		cancel()
		cancelLock()
		_ = rowLock.Rollback(context.Background())
		if lockStarted {
			<-lockDone
		}
		_ = tableLock.Rollback(context.Background())
		f.game.wg.Wait()
	}()
	var resultErr error
	resultCalls := 0
	onResult := req.OnResult
	req.OnResult = func(shard *Shard, handle types.Handle, err error) {
		resultCalls++
		resultErr = err
		onResult(shard, handle, err)
	}
	require.NoError(t, f.game.transferService.RequestTransfer(req))
	done := make(chan struct{})
	go func() { f.game.wg.Wait(); close(done) }()

	// The initial character SELECT passes the row lock, then the real SaveSync
	// UPDATE blocks. Queue an exclusive table lock behind that save transaction.
	var savePID int
	require.Eventually(t, func() bool {
		return f.db.Pool().QueryRow(t.Context(), `SELECT pid FROM pg_stat_activity
			WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE '%-- name: UpdateCharacters :exec%'`, rowLockPID).Scan(&savePID) == nil
	}, 2*time.Second, 10*time.Millisecond)
	lockStarted = true
	go func() {
		_, lockErr = tableLock.Exec(lockCtx, `LOCK TABLE character IN ACCESS EXCLUSIVE MODE`)
		close(lockDone)
	}()
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.db.Pool().QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE pid=$1 AND $2=ANY(pg_blocking_pids(pid)))`, tableLockPID, savePID).Scan(&blocked)
		return err == nil && blocked
	}, 2*time.Second, 10*time.Millisecond)
	refreshStarted := time.Now()
	require.NoError(t, rowLock.Rollback(t.Context()))
	select {
	case <-lockDone:
		require.NoError(t, lockErr)
	case <-time.After(2 * time.Second):
		t.Fatal("character table lock was not acquired after SaveSync")
	}
	// SaveSync has committed; only its following GetCharacter is now blocked.
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.db.Pool().QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE $1=ANY(pg_blocking_pids(pid)) AND query LIKE '%-- name: GetCharacter :one%')`, tableLockPID).Scan(&blocked)
		return err == nil && blocked
	}, 2*time.Second, 10*time.Millisecond)
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("saved character reread did not enforce its own five-second deadline")
	}
	require.NoError(t, ctx.Err(), "the game context must remain active")
	require.Equal(t, 1, resultCalls)
	require.ErrorIs(t, resultErr, context.DeadlineExceeded)
	require.ErrorContains(t, resultErr, "reload saved transfer character")
	require.GreaterOrEqual(t, time.Since(refreshStarted), 5*time.Second)
	require.Same(t, f.client, f.source.Clients[10])
	require.True(t, f.client.InWorld.Load())
	require.Equal(t, 1, f.client.Layer)
	require.True(t, f.source.world.Alive(f.player))
	require.Equal(t, f.player, f.source.world.GetHandleByEntityID(10))
	currentCarry, exists := ecs.GetComponent[components.LiftCarryState](f.source.world, f.player)
	require.True(t, exists)
	require.Equal(t, carry, currentCarry)
	require.True(t, f.source.world.Alive(carry.ObjectHandle))
	currentOwner, exists := ecs.GetComponent[components.InventoryOwner](f.source.world, f.player)
	require.True(t, exists)
	require.Equal(t, owner, currentOwner)
	for _, link := range owner.Inventories {
		require.True(t, f.source.world.Alive(link.Handle))
	}
	health, exists := ecs.GetComponent[components.EntityHealth](f.source.world, f.player)
	require.True(t, exists)
	require.Equal(t, wantHealth, health)
	require.False(t, ecs.HasComponent[components.ActiveGameAction](f.source.world, f.player))
	require.False(t, ecs.HasComponent[components.ActiveCyclicAction](f.source.world, f.player))
	require.False(t, f.source.actionService.isOnCooldown(f.source.world, f.player, hearth.ReturnActionID))
	require.Equal(t, types.InvalidHandle, f.shards[0].world.GetHandleByEntityID(10))
	require.Equal(t, types.InvalidHandle, f.shards[0].world.GetHandleByEntityID(3000))

	// Keep the blocker held until the transfer has failed and source preservation
	// is checked; only then allow ordinary reads to confirm SaveSync committed.
	require.NoError(t, tableLock.Rollback(t.Context()))
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, wantStats.Stamina, row.Stamina)
	require.Equal(t, wantStats.Energy, row.Energy)
	require.Equal(t, 1, row.Layer)
}

func TestHearthReturnPostgresQueuedHomeReplacement(t *testing.T) {
	f := newHearthReturnTransferFixture(t, 1, true, false)
	req := f.request(t)
	latest := hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 1011}, X: 700, Y: 600}
	f.shards[0].mu.Lock()
	_, err := f.runtime.Index.ReplaceHome(10, latest)
	require.NoError(t, err)
	addHearthCollisionTestObject(f.shards[0], latest.Object.ID, latest.X, latest.Y, []string{"hearth"})
	f.shards[0].mu.Unlock()
	f.run(t, req)
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, latest.X, row.X)
	require.Equal(t, latest.Y, row.Y)
	require.Equal(t, 0, row.Layer)
	carried, err := f.db.Queries().GetObjectByID(t.Context(), 3000)
	require.NoError(t, err)
	require.Equal(t, latest.X, carried.X)
	require.Equal(t, latest.Y, carried.Y)
	require.Equal(t, 0, carried.Layer)
}

func TestHearthReturnPostgresTimedCompletionDefersTransferSuccess(t *testing.T) {
	f := newHearthReturnTransferFixture(t, 1, false, true)
	f.source.mu.Lock()
	f.source.actionService.Activate(f.source.world, 10, f.player, hearth.ReturnActionID)
	for range 50 {
		cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](f.source.world, f.player)
		require.True(t, exists)
		f.source.actionService.AdvanceCycle(f.source.world, 10, f.player, cycle, nil)
	}
	cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](f.source.world, f.player)
	require.True(t, exists)
	require.True(t, cycle.ActionCompletionStarted)
	require.True(t, f.source.world.Alive(f.player), "queued transfer cannot detach before the source lock is released")
	require.False(t, f.source.actionService.isOnCooldown(f.source.world, f.player, hearth.ReturnActionID))
	f.source.mu.Unlock()
	f.game.wg.Wait()
	targetPlayer := f.shards[0].world.GetHandleByEntityID(10)
	require.True(t, f.shards[0].world.Alive(targetPlayer))
	require.True(t, f.shards[0].actionService.isOnCooldown(f.shards[0].world, targetPlayer, hearth.ReturnActionID))
}

func TestHearthReturnPostgresEarlyFailuresRetireDeferredAction(t *testing.T) {
	for _, mode := range []string{"target_missing", "character_missing", "canceled", "save_failed", "disconnected", "ordinary_lying"} {
		t.Run(mode, func(t *testing.T) {
			f := newHearthReturnTransferFixture(t, 1, true, true)
			req := f.request(t)
			switch mode {
			case "target_missing":
				delete(f.game.shardManager.shards, 0)
			case "character_missing":
				_, err := f.db.Pool().Exec(t.Context(), `DELETE FROM character_hearth WHERE character_id=10; DELETE FROM character WHERE id=10`)
				require.NoError(t, err)
			case "canceled":
				f.source.actionService.Cancel(f.source.world, 10, f.player)
			case "save_failed":
				f.source.characterSaver.Stop()
			case "disconnected":
				f.client.Close()
			case "ordinary_lying":
				req.SourcePolicy = nil // Ordinary stairs/admin incapacity remains strict.
			}
			f.run(t, req)
			require.True(t, f.source.world.Alive(f.player))
			require.False(t, ecs.HasComponent[components.ActiveGameAction](f.source.world, f.player))
			require.True(t, ecs.HasComponent[components.LiftCarryState](f.source.world, f.player))
			require.False(t, f.source.actionService.isOnCooldown(f.source.world, f.player, hearth.ReturnActionID))
		})
	}
}

func TestHearthReturnPostgresDisconnectAfterSpawnCommitsCooldown(t *testing.T) {
	f := newHearthReturnTransferFixture(t, 1, true, true)
	req := f.request(t)
	spawn := req.SpawnTarget
	req.SpawnTarget = func(shard *Shard, character repository.Character, runtime playerRuntimeState, finalize playerSpawnFinalizer) (types.Handle, repository.Character, error) {
		return spawn(shard, character, runtime, func(handle types.Handle, actual repository.Character) {
			f.client.Close()
			finalize(handle, actual)
		})
	}
	f.run(t, req)
	player := f.shards[0].world.GetHandleByEntityID(10)
	require.True(t, f.shards[0].world.Alive(player))
	// The existing disconnected-body path may force-drop carry at the target.
	// Its object still exists there, and body relocation remains a success.
	require.True(t, f.shards[0].world.Alive(f.shards[0].world.GetHandleByEntityID(3000)))
	require.True(t, f.shards[0].actionService.isOnCooldown(f.shards[0].world, player, hearth.ReturnActionID))
	require.Equal(t, types.InvalidHandle, f.source.world.GetHandleByEntityID(10))
}

func TestHearthReturnPostgresDisconnectDuringSourceSave(t *testing.T) {
	f := newHearthReturnTransferFixture(t, 1, true, true)
	req := f.request(t)
	transaction, err := f.db.Pool().Begin(t.Context())
	require.NoError(t, err)
	defer transaction.Rollback(context.Background())
	var blockerPID int
	require.NoError(t, transaction.QueryRow(t.Context(), `SELECT pg_backend_pid() FROM character WHERE id=10 FOR UPDATE`).Scan(&blockerPID))
	require.NoError(t, f.game.transferService.RequestTransfer(req))
	// The real SaveSync write is blocked behind this row lock, after its first
	// source validation. Closing the socket must still prevent carry capture.
	require.Eventually(t, func() bool {
		var blocked bool
		err := f.db.Pool().QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, blockerPID).Scan(&blocked)
		return err == nil && blocked
	}, 2*time.Second, 10*time.Millisecond)
	f.client.Close()
	require.NoError(t, transaction.Rollback(t.Context()))
	f.game.wg.Wait()
	require.True(t, f.source.world.Alive(f.player))
	require.True(t, ecs.HasComponent[components.LiftCarryState](f.source.world, f.player))
	require.False(t, ecs.HasComponent[components.ActiveGameAction](f.source.world, f.player))
	require.False(t, f.source.actionService.isOnCooldown(f.source.world, f.player, hearth.ReturnActionID))
}

func TestHearthReturnPostgresDestinationRepreparesChangedAOI(t *testing.T) {
	f := newHearthReturnTransferFixture(t, 1, false, false)
	target := f.shards[0]
	target.cfg.Game.WorldWidthChunks = 2
	coord := types.ChunkCoord{X: 1}
	require.NoError(t, target.chunkManager.WaitPreloaded(t.Context(), coord))
	chunk := target.chunkManager.GetChunk(coord)
	tiles := make([]byte, constt.ChunkSize*constt.ChunkSize)
	for index := range tiles {
		tiles[index] = types.TileGrass
	}
	require.NoError(t, chunk.RestoreTiles(tiles, 0, 0))
	latest := hearth.Home{Object: hearth.ObjectKey{Region: 1, ID: 1011}, X: constt.ChunkWorldSize + 200, Y: 400}
	homeHandle := target.world.Spawn(latest.Object.ID, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.EntityInfo{Region: 1, Layer: 0, Behaviors: []string{"hearth"}})
		ecs.AddComponent(w, handle, components.Transform{X: float64(latest.X), Y: float64(latest.Y)})
	})
	chunk.Spatial().AddStatic(homeHandle, latest.X, latest.Y)
	resolutions := 0
	resolve := func(shard *Shard) (playerSpawnDestination, error) {
		resolutions++
		if resolutions == 2 {
			// This is the first locked check after preparing the old AOI.
			_, err := f.runtime.Index.ReplaceHome(10, latest)
			require.NoError(t, err)
		}
		return resolveHearthSpawnDestinationLocked(shard, 10)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	handle, character, err := f.game.spawnPlayerAtResolvedDestination(ctx, target, f.character, nil, resolve,
		func(shard *Shard, destination playerSpawnDestination) error {
			return validateHearthSpawnDestinationLocked(shard, 10, destination)
		}, SpawnCollisionPolicy{}, nil)
	require.NoError(t, err)
	require.True(t, target.world.Alive(handle))
	require.Equal(t, 3, resolutions)
	require.Equal(t, latest.X, character.X)
	require.Equal(t, latest.Y, character.Y)
	actualChunk, exists := target.chunkManager.GetEntityChunk(10)
	require.True(t, exists)
	require.Same(t, chunk, actualChunk, "RegisterEntity must not retain the first unpublished AOI")
}

func TestHearthReturnPostgresDestinationIdentityAndTimeout(t *testing.T) {
	for _, invalid := range []string{"region", "behavior", "position", "missing", "revoked", "timeout"} {
		t.Run(invalid, func(t *testing.T) {
			f := newHearthReturnTransferFixture(t, 1, false, false)
			target := f.shards[0]
			home := f.runtime.Index.Snapshot(10).Home
			invalidate := func() {
				homeHandle := target.world.GetHandleByEntityID(home.Object.ID)
				switch invalid {
				case "region":
					ecs.WithComponent(target.world, homeHandle, func(info *components.EntityInfo) { info.Region++ })
				case "behavior":
					ecs.WithComponent(target.world, homeHandle, func(info *components.EntityInfo) { info.Behaviors = []string{"burner"} })
				case "position":
					ecs.WithComponent(target.world, homeHandle, func(position *components.Transform) { position.X++ })
				case "missing":
					target.world.Despawn(homeHandle)
				case "revoked":
					_, err := f.runtime.Index.RevokeHome(10)
					require.NoError(t, err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			resolutions := 0
			resolve := func(shard *Shard) (playerSpawnDestination, error) {
				resolutions++
				if resolutions == 2 {
					invalidate()
				}
				if invalid == "timeout" && resolutions == 2 {
					cancel()
					home.X++
					_, err := f.runtime.Index.ReplaceHome(10, home)
					require.NoError(t, err)
				}
				return resolveHearthSpawnDestinationLocked(shard, 10)
			}
			handle, _, err := f.game.spawnPlayerAtResolvedDestination(ctx, target, f.character, nil, resolve,
				func(shard *Shard, destination playerSpawnDestination) error {
					return validateHearthSpawnDestinationLocked(shard, 10, destination)
				}, SpawnCollisionPolicy{}, nil)
			require.Error(t, err)
			require.Equal(t, types.InvalidHandle, handle)
			require.Equal(t, types.InvalidHandle, target.world.GetHandleByEntityID(10))
			_, registered := target.chunkManager.GetEntityChunk(10)
			require.False(t, registered)
		})
	}
}
