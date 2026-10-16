package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHearthLoginPostgresPositionWriteAllowsAttachedPlayerDeath(t *testing.T) {
	f, g, character := newHearthLoginFallbackFixture(t, 1)
	g.cfg.Game.SpawnTimeout = 2 * time.Second
	source, surface := f.shards[1], f.shards[0]
	addHearthCollisionTestObject(source, 2000, character.X, character.Y, nil)
	home := f.runtime.Index.Snapshot(10).Home
	homeHandle := surface.world.GetHandleByEntityID(home.Object.ID)
	// The fixture's collision-only Hearth must also serialize during death cleanup.
	ecs.AddComponent(surface.world, homeHandle, components.ChunkRef{})
	client, connection := connectPlayerSpawnTestClient(t)
	client.CharacterID, client.Layer = 10, 1
	lethal := &hearthReturnLethalUpdate{
		BaseSystem: ecs.NewBaseSystem("login lethal update", 1), shard: surface, client: client,
	}
	surface.world.AddSystem(lethal)
	surface.world.AddSystem(NewPlayerDeathSystem(surface, PlayerDeathSystemConfig{}))
	require.NoError(t, f.db.Queries().SetCharacterOnline(t.Context(), 10))

	rowLock, err := f.db.Pool().Begin(t.Context())
	require.NoError(t, err)
	defer rowLock.Rollback(context.Background())
	var blockerPID int32
	require.NoError(t, rowLock.QueryRow(t.Context(), `SELECT pg_backend_pid() FROM character WHERE id=10 FOR UPDATE`).Scan(&blockerPID))
	loginDone, updateDone := make(chan struct{}), make(chan struct{})
	updateStarted := false
	defer func() {
		// Release SQL before waiting, including when a boundary assertion fails.
		_ = rowLock.Rollback(context.Background())
		<-loginDone
		if updateStarted {
			<-updateDone
		}
	}()
	go func() {
		g.spawnAndLogin(client, character)
		close(loginDone)
	}()

	// Observe actual SQL lock waits; no sleep or production test hook determines
	// when spawning has finished or when the real death system has run.
	blockers := []int32{blockerPID}
	waitForBlockedStatement := func(name string) int32 {
		t.Helper()
		var pid int32
		require.Eventually(t, func() bool {
			return f.db.Pool().QueryRow(t.Context(), `SELECT pid FROM pg_stat_activity
				WHERE wait_event_type='Lock' AND pg_blocking_pids(pid) && $1::int[]
				AND query LIKE $2`, blockers, "%-- name: "+name+" :exec%").Scan(&pid) == nil
		}, 2*time.Second, 10*time.Millisecond, "%s must wait for the character row lock", name)
		return pid
	}
	positionPID := waitForBlockedStatement("UpdateCharacterPositionAndLayer")
	blockers = append(blockers, positionPID)
	lockAvailable := surface.mu.TryLock()
	if lockAvailable {
		surface.mu.Unlock()
	}
	require.True(t, lockAvailable, "the position checkpoint must not hold the owning shard lock")
	updateStarted = true
	go func() {
		surface.Update(ecs.TimeState{Tick: 1, UnixMs: 10000})
		close(updateDone)
	}()
	// Permanent death converts the player to a corpse before SetCharacterOffline.
	// Waiting here proves that death ran while the login checkpoint was blocked.
	waitForBlockedStatement("SetCharacterOffline")
	select {
	case <-loginDone:
		t.Fatal("login position checkpoint completed before the death boundary")
	default:
	}
	// The spawn deadline may expire while its independent checkpoint is blocked.
	// Returning from that checkpoint must not queue the already attached corpse
	// for logout or send another ordinary EnterWorld.
	time.Sleep(g.cfg.Game.SpawnTimeout)
	require.NoError(t, rowLock.Rollback(t.Context()))
	waitHearthTerminal(t, loginDone)
	waitHearthTerminal(t, updateDone)

	assert.True(t, lethal.observed.living)
	assert.True(t, lethal.observed.bound, "the first update after login spawn must see the attached connection")
	corpse := requireHearthLoginBody(t, surface, home.X, home.Y)
	assert.Equal(t, lethal.observed.handle, corpse)
	corpseDef, found := objectdefs.Global().GetByKey("player_dead")
	require.True(t, found)
	info, found := ecs.GetComponent[components.EntityInfo](surface.world, corpse)
	require.True(t, found)
	assert.Equal(t, uint32(corpseDef.DefID), info.TypeID)
	assert.False(t, ecs.HasComponent[components.EntityHealth](surface.world, corpse))
	assert.NotContains(t, ecs.GetResource[ecs.CharacterEntities](surface.world).Map, types.EntityID(10))
	assert.False(t, ecs.GetResource[ecs.DetachedEntities](surface.world).IsDetached(10), "expired spawn work must not queue the corpse for logout")
	assert.True(t, client.IsDeadObserverMode(), "the login connection must receive the standard permanent-death observer flow")
	assert.Same(t, client, surface.Clients[10])
	assert.Equal(t, 0, client.Layer)
	assert.Equal(t, types.InvalidHandle, source.world.GetHandleByEntityID(10))
	f.requireDurableDeath(t, 10)
	requireHearthReturnDeathPackets(t, &hearthReturnTransferFixture{
		hearthTerminalGameFixture: f, client: client, connection: connection,
	}, 1)
}

func TestHearthLoginPostgresAttachmentDeathCompletesAfterUnlock(t *testing.T) {
	for _, path := range []string{"saved_position", "home"} {
		t.Run(path, func(t *testing.T) {
			f, g, character := newHearthLoginFallbackFixture(t, 1)
			target := f.shards[1]
			if path == "home" {
				addHearthCollisionTestObject(target, 2000, character.X, character.Y, nil)
				target = f.shards[0]
			}
			home := f.runtime.Index.Snapshot(10).Home
			homeHandle := f.shards[0].world.GetHandleByEntityID(home.Object.ID)
			ecs.AddComponent(f.shards[0].world, homeHandle, components.ChunkRef{})
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, 1
			lethal := &hearthReturnLethalUpdate{shard: target, client: client}
			target.world.AddComponentObserver(components.InventoryOwnerComponentID, func(handle types.Handle) {
				if id, exists := target.world.GetExternalID(handle); exists && id == 10 {
					// Setup is complete enough for ordinary death, but attachment
					// must still bind the connection before handling depleted HHP.
					lethal.Update(target.world, 0)
				}
			})
			originalComplete := target.permanentDeathComplete
			completedUnlocked := false
			target.permanentDeathComplete = func(id types.EntityID) error {
				completedUnlocked = target.mu.TryLock()
				if !completedUnlocked {
					return errors.New("login death cleanup attempted before owning shard unlock")
				}
				target.mu.Unlock()
				return originalComplete(id)
			}
			g.spawnAndLogin(client, character)
			require.True(t, completedUnlocked, "both login spawn paths must finish synchronous death cleanup after unlocking")
			assert.True(t, target.world.Alive(lethal.observed.handle))
			assert.False(t, ecs.HasComponent[components.EntityHealth](target.world, lethal.observed.handle))
			assert.True(t, client.IsDeadObserverMode())
			assert.Same(t, client, target.Clients[10])
			assert.Equal(t, target.layer, client.Layer)
			f.requireDurableDeath(t, 10)
			requireHearthReturnDeathPackets(t, &hearthReturnTransferFixture{
				hearthTerminalGameFixture: f, client: client, connection: connection,
			}, 1)
		})
	}
}
