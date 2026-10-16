package game

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/hearth"
	"origin/internal/game/inventory"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newHearthLoginFallbackFixture(t *testing.T, layer int) (*hearthTerminalGameFixture, *Game, repository.Character) {
	t.Helper()
	f := newHearthTerminalGameFixture(t)
	g := f.games[layer]
	g.ctx = context.Background()
	g.shardManager = &ShardManager{shards: map[int]*Shard{0: f.shards[0], 1: f.shards[1]}}
	g.cfg.Game.NearSpawnTries, g.cfg.Game.RandomSpawnTries = 0, 0
	f.shards[0].objectCollisionFilter = f.runtime.IgnoreObjectCollision
	f.addCharacter(t, 10, layer)
	character, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	home := f.runtime.Index.Snapshot(10).Home
	addHearthCollisionTestObject(f.shards[0], home.Object.ID, home.X, home.Y, []string{"hearth"})
	return f, g, character
}

func requireHearthLoginBody(t *testing.T, shard *Shard, x, y int) types.Handle {
	t.Helper()
	handle := shard.world.GetHandleByEntityID(10)
	require.True(t, shard.world.Alive(handle))
	position, exists := ecs.GetComponent[components.Transform](shard.world, handle)
	require.True(t, exists)
	require.EqualValues(t, x, position.X)
	require.EqualValues(t, y, position.Y)
	info, exists := ecs.GetComponent[components.EntityInfo](shard.world, handle)
	require.True(t, exists)
	require.Equal(t, shard.layer, info.Layer)
	return handle
}

func TestHearthLoginPostgresSavedPositionPrecedesHome(t *testing.T) {
	for _, layer := range []int{0, 1} {
		t.Run(fmt.Sprintf("layer_%d", layer), func(t *testing.T) {
			f, g, character := newHearthLoginFallbackFixture(t, layer)
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, layer
			g.spawnAndLogin(client, character)
			readSoundLifecycleEntry(t, connection)
			require.True(t, client.InWorld.Load())
			require.Equal(t, layer, client.Layer)
			requireHearthLoginBody(t, f.shards[layer], character.X, character.Y)
			require.Equal(t, types.InvalidHandle, f.shards[1-layer].world.GetHandleByEntityID(10))
		})
	}
}

func TestHearthLoginPostgresHomePreservesRuntimeAndReattachesBeforeSavedLayer(t *testing.T) {
	for _, koUntil := range []int64{0, 60000} {
		name := "lying"
		if koUntil != 0 {
			name = "knocked_out"
		}
		t.Run(name, func(t *testing.T) {
			f, g, character := newHearthLoginFallbackFixture(t, 1)
			source, surface := f.shards[1], f.shards[0]
			blocker := addHearthCollisionTestObject(source, 2000, character.X, character.Y, nil)
			cached := playerRuntimeState{
				Health:      components.EntityHealth{SHP: 12.25, HHP: 18.5, IsLying: true, KOUntilUnixMs: koUntil, LyingRevision: 7},
				CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 9000},
			}
			source.offlineHealth.Store(types.EntityID(10), cached)
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, 1
			g.spawnAndLogin(client, character)
			readSoundLifecycleEntry(t, connection)
			home := f.runtime.Index.Snapshot(10).Home
			player := requireHearthLoginBody(t, surface, home.X, home.Y)
			require.Equal(t, 0, client.Layer)
			state, exists := surface.capturePlayerRuntimeState(player)
			require.True(t, exists)
			require.Equal(t, cached, state, "login fallback must preserve lying, knockout and combat state")
			_, exists = source.offlineHealth.Load(types.EntityID(10))
			require.False(t, exists)
			require.Empty(t, source.chunkManager.GetEntityActiveChunks(10))

			// Reconnect must find the actual body even when the character row
			// supplied by authentication still points underground.
			row, err := f.db.Queries().GetCharacter(t.Context(), 10)
			require.NoError(t, err)
			require.Equal(t, 0, row.Layer)
			require.Equal(t, home.X, row.X)
			require.Equal(t, home.Y, row.Y)
			stale := character
			require.True(t, surface.detachClientForLogout(client, g.clock.GameNow(), time.Minute))
			source.world.Despawn(blocker)
			ecs.WithComponent(surface.world, player, func(health *components.EntityHealth) { health.SHP = 8.75 })
			next, nextConnection := connectPlayerSpawnTestClient(t)
			next.CharacterID, next.Layer = 10, stale.Layer
			g.spawnAndLogin(next, stale)
			readSoundLifecycleEntry(t, nextConnection)
			require.Equal(t, 0, next.Layer)
			require.Equal(t, player, requireHearthLoginBody(t, surface, home.X, home.Y))
			require.Equal(t, types.InvalidHandle, source.world.GetHandleByEntityID(10), "saved position must not create a duplicate body")
			state, exists = surface.capturePlayerRuntimeState(player)
			require.True(t, exists)
			cached.Health.SHP = 8.75
			require.Equal(t, cached, state)
			require.False(t, ecs.GetResource[ecs.DetachedEntities](surface.world).IsDetached(10))
		})
	}
}

func TestHearthLoginPostgresBlockedHomeFallsBackNearSavedLayer(t *testing.T) {
	f, g, character := newHearthLoginFallbackFixture(t, 1)
	g.cfg.Game.NearSpawnTries, g.cfg.Game.NearSpawnRadius = 64, 100
	addHearthCollisionTestObject(f.shards[1], 2000, character.X, character.Y, nil)
	home := f.runtime.Index.Snapshot(10).Home
	addHearthCollisionTestObject(f.shards[0], 2001, home.X, home.Y, nil)
	client, connection := connectPlayerSpawnTestClient(t)
	client.CharacterID, client.Layer = 10, 1
	g.spawnAndLogin(client, character)
	readSoundLifecycleEntry(t, connection)
	require.Equal(t, 1, client.Layer)
	player := f.shards[1].world.GetHandleByEntityID(10)
	require.True(t, f.shards[1].world.Alive(player))
	position, exists := ecs.GetComponent[components.Transform](f.shards[1].world, player)
	require.True(t, exists)
	require.LessOrEqual(t, math.Abs(position.X-float64(character.X)), 100.0)
	require.LessOrEqual(t, math.Abs(position.Y-float64(character.Y)), 100.0)
	require.NotEqual(t, spawnPos{character.X, character.Y}, spawnPos{int(position.X), int(position.Y)})
	require.Equal(t, types.InvalidHandle, f.shards[0].world.GetHandleByEntityID(10))
	require.Empty(t, f.shards[0].chunkManager.GetEntityActiveChunks(10), "failed home reservation must be retired")
}

func TestHearthLoginPostgresNoHomeUsesOrdinarySpawn(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "first_entry"
		if blocked {
			name = "random_fallback"
		}
		t.Run(name, func(t *testing.T) {
			f, g, character := newHearthLoginFallbackFixture(t, 1)
			_, err := f.db.Pool().Exec(t.Context(), `DELETE FROM character_hearth WHERE character_id=10`)
			require.NoError(t, err)
			// Force the production metadata load to establish NoHome from SQL.
			f.runtime.Index = hearth.NewIndex()
			if blocked {
				addHearthCollisionTestObject(f.shards[1], 2000, character.X, character.Y, nil)
				g.cfg.Game.RandomSpawnTries = 16
			}
			before := f.shards[0].world.EntityCount()
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, 1
			g.spawnAndLogin(client, character)
			readSoundLifecycleEntry(t, connection)
			require.True(t, client.InWorld.Load())
			require.Equal(t, 1, client.Layer)
			require.Equal(t, hearth.NoHome, f.runtime.Index.Snapshot(10).State)
			require.Equal(t, before, f.shards[0].world.EntityCount(), "login without ownership must not create a Hearth")
			var homes int
			require.NoError(t, f.db.Pool().QueryRow(t.Context(), `SELECT count(*) FROM character_hearth WHERE character_id=10`).Scan(&homes))
			require.Zero(t, homes)
		})
	}
}

func TestHearthLoginPostgresHomeFailuresDoNotTryOrdinaryFallback(t *testing.T) {
	for _, failure := range []string{"capacity", "setup"} {
		t.Run(failure, func(t *testing.T) {
			f, g, character := newHearthLoginFallbackFixture(t, 1)
			g.cfg.Game.NearSpawnTries, g.cfg.Game.NearSpawnRadius = 64, 100
			addHearthCollisionTestObject(f.shards[1], 2000, character.X, character.Y, nil)
			if failure == "capacity" {
				for f.shards[0].world.EntityCount() < f.shards[0].world.EntityCapacity() {
					f.shards[0].world.SpawnWithoutExternalID()
				}
			} else {
				character.ActionCooldowns = []byte(`{"invalid":true}`)
			}
			cached := playerRuntimeState{Health: components.EntityHealth{SHP: 12.25, HHP: 18.5, IsLying: true}}
			f.shards[1].offlineHealth.Store(types.EntityID(10), cached)
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, 1
			g.spawnAndLogin(client, character)
			message := readSoundLifecyclePacket(t, connection, func(message *netproto.ServerMessage) bool { return message.GetError() != nil })
			require.Equal(t, netproto.ErrorCode_ERROR_CODE_INTERNAL_ERROR, message.GetError().Code)
			require.Contains(t, message.GetError().Message, failure)
			require.False(t, client.InWorld.Load())
			for _, shard := range f.shards {
				require.Equal(t, types.InvalidHandle, shard.world.GetHandleByEntityID(10))
				require.Empty(t, shard.chunkManager.GetEntityActiveChunks(10))
				require.Empty(t, ecs.GetResource[ecs.CharacterEntities](shard.world).GetAll())
			}
			actual, exists := f.shards[1].offlineHealth.Load(types.EntityID(10))
			require.True(t, exists)
			require.Equal(t, cached, actual)
		})
	}
}

func TestHearthLoginPostgresDisconnectBeforeAttachKeepsSurfaceBody(t *testing.T) {
	f, g, character := newHearthLoginFallbackFixture(t, 1)
	source, surface := f.shards[1], f.shards[0]
	addHearthCollisionTestObject(source, 2000, character.X, character.Y, nil)
	cached := playerRuntimeState{Health: components.EntityHealth{SHP: 12.25, HHP: 18.5, IsLying: true, LyingRevision: 8}, CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 9000}}
	source.offlineHealth.Store(types.EntityID(10), cached)
	client, _ := connectPlayerSpawnTestClient(t)
	client.CharacterID, client.Layer = 10, 1
	closed := false
	surface.world.AddComponentObserver(components.InventoryOwnerComponentID, func(handle types.Handle) {
		if id, exists := surface.world.GetExternalID(handle); exists && id == 10 && !closed {
			// Inventory SQL has finished. Setup will complete on this body, but
			// the connection cannot attach or publish initial world snapshots.
			closed = true
			client.Close()
		}
	})
	g.spawnAndLogin(client, character)
	require.True(t, closed)
	require.False(t, client.InWorld.Load())
	home := f.runtime.Index.Snapshot(10).Home
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, row.Layer, "successful relocation must persist despite client cancellation")
	require.Equal(t, home.X, row.X)
	require.Equal(t, home.Y, row.Y)
	player := requireHearthLoginBody(t, surface, home.X, home.Y)
	require.True(t, ecs.GetResource[ecs.DetachedEntities](surface.world).IsDetached(10))
	state, exists := surface.capturePlayerRuntimeState(player)
	require.True(t, exists)
	require.Equal(t, cached, state)
	actual, exists := source.offlineHealth.Load(types.EntityID(10))
	require.True(t, exists)
	require.Equal(t, cached, actual, "cancellation must retain the unconsumed source cache")
	// The body may change while detached; it wins over the older cache and row.
	ecs.WithComponent(surface.world, player, func(health *components.EntityHealth) { health.SHP = 4.5 })
	next, connection := connectPlayerSpawnTestClient(t)
	next.CharacterID, next.Layer = 10, row.Layer
	g.spawnAndLogin(next, row)
	readSoundLifecycleEntry(t, connection)
	require.Equal(t, 0, next.Layer)
	require.Equal(t, player, requireHearthLoginBody(t, surface, home.X, home.Y))
	state, exists = surface.capturePlayerRuntimeState(player)
	require.True(t, exists)
	cached.Health.SHP = 4.5
	require.Equal(t, cached, state)
	_, exists = source.offlineHealth.Load(types.EntityID(10))
	require.False(t, exists)
	require.Equal(t, types.InvalidHandle, source.world.GetHandleByEntityID(10))
}

func TestHearthLoginPostgresCrossLayerExpiryReloadsSurfaceState(t *testing.T) {
	f, g, character := newHearthLoginFallbackFixture(t, 1)
	source, surface := f.shards[1], f.shards[0]
	addHearthCollisionTestObject(source, 2000, character.X, character.Y, nil)
	client, connection := connectPlayerSpawnTestClient(t)
	client.CharacterID, client.Layer = 10, 1
	g.spawnAndLogin(client, character)
	readSoundLifecycleEntry(t, connection)
	home := f.runtime.Index.Snapshot(10).Home
	player := requireHearthLoginBody(t, surface, home.X, home.Y)
	latest := playerRuntimeState{
		Health:      components.EntityHealth{SHP: 4.5, HHP: 18.5, IsLying: true, LyingRevision: 9},
		CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 9000},
	}
	ecs.AddComponent(surface.world, player, latest.Health)
	require.NoError(t, ecs.GetResource[ecs.CombatActivityState](surface.world).Restore(player, latest.CombatState))
	saver := systems.NewCharacterSaver(f.db, 1, inventory.NewInventorySaver(g.logger), g.logger)
	t.Cleanup(saver.Stop)
	require.NoError(t, saver.SaveSync(surface.world, 10, player))
	row, err := f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, row.Layer, "ordinary final save must retain the separately persisted destination layer")
	require.Equal(t, home.X, row.X)
	require.Equal(t, home.Y, row.Y)
	require.True(t, surface.detachClientForLogout(client, time.Time{}, 0))
	expiry := systems.NewExpireDetachedSystem(g.logger, saver, surface.onDetachedEntityExpired, surface.onDetachedEntitiesExpired)
	expiry.Update(surface.world, 0)
	require.False(t, surface.world.Alive(player))
	saver.Stop()
	cached, exists := surface.offlineHealth.Load(types.EntityID(10))
	require.True(t, exists)
	require.Equal(t, latest, cached)
	row, err = f.db.Queries().GetCharacter(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, 0, row.Layer)
	next, nextConnection := connectPlayerSpawnTestClient(t)
	next.CharacterID, next.Layer = 10, row.Layer
	g.spawnAndLogin(next, row)
	readSoundLifecycleEntry(t, nextConnection)
	require.Equal(t, 0, next.Layer)
	restored := requireHearthLoginBody(t, surface, home.X, home.Y)
	actual, exists := surface.capturePlayerRuntimeState(restored)
	require.True(t, exists)
	require.Equal(t, latest, actual)
	require.Equal(t, types.InvalidHandle, source.world.GetHandleByEntityID(10))
}

func TestHearthLoginPostgresPositionWriteFailureKeepsAuthoritativeBody(t *testing.T) {
	for _, layer := range []int{0, 1} {
		t.Run(fmt.Sprintf("layer_%d", layer), func(t *testing.T) {
			f, g, character := newHearthLoginFallbackFixture(t, layer)
			core, logs := observer.New(zap.WarnLevel)
			g.logger = zap.New(core)
			_, err := f.db.Pool().Exec(t.Context(), `
				CREATE FUNCTION reject_login_position_write() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'login position write rejected'; END $$;
				CREATE TRIGGER reject_login_position_write BEFORE UPDATE ON character
				FOR EACH ROW EXECUTE FUNCTION reject_login_position_write()`)
			require.NoError(t, err)
			addHearthCollisionTestObject(f.shards[layer], 2000, character.X, character.Y, nil)
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID, client.Layer = 10, layer
			g.spawnAndLogin(client, character)
			readSoundLifecycleEntry(t, connection)
			home := f.runtime.Index.Snapshot(10).Home
			player := requireHearthLoginBody(t, f.shards[0], home.X, home.Y)
			require.True(t, client.InWorld.Load())
			require.Equal(t, 0, client.Layer)
			require.Equal(t, layer, logs.FilterMessage("Login fallback: failed to persist final position/layer").Len(), "same-layer login must not perform the extra position write")
			row, err := f.db.Queries().GetCharacter(t.Context(), 10)
			require.NoError(t, err)
			require.Equal(t, layer, row.Layer, "failed ordinary checkpoint leaves the old durable layer")
			require.True(t, f.shards[0].detachClientForLogout(client, g.clock.GameNow(), time.Minute))
			next, nextConnection := connectPlayerSpawnTestClient(t)
			next.CharacterID, next.Layer = 10, row.Layer
			g.spawnAndLogin(next, row)
			readSoundLifecycleEntry(t, nextConnection)
			require.Equal(t, 0, next.Layer)
			require.Equal(t, player, requireHearthLoginBody(t, f.shards[0], home.X, home.Y))
			require.Equal(t, types.InvalidHandle, f.shards[1].world.GetHandleByEntityID(10))
		})
	}
}

func TestHearthLoginPostgresAttachmentRetiresCacheBeforeUnlock(t *testing.T) {
	f, g, character := newHearthLoginFallbackFixture(t, 1)
	source, surface := f.shards[1], f.shards[0]
	old := playerRuntimeState{
		Health:      components.EntityHealth{SHP: 12.25, HHP: 18.5, IsLying: true, LyingRevision: 8},
		CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 9000},
	}
	source.offlineHealth.Store(types.EntityID(10), old)
	surface.offlineHealth.Store(types.EntityID(10), old)
	client, connection := connectPlayerSpawnTestClient(t)
	client.CharacterID = 10
	home := f.runtime.Index.Snapshot(10).Home
	player, err := g.spawnTeleportedPlayerWithRuntime(client, surface, character, home.X, home.Y, false, &old, func(types.Handle, repository.Character) {})
	require.NoError(t, err)

	surface.mu.Lock()
	attached := g.attachClientToWorldLocked(surface, client, 10, character, player)
	_, sourceCachedBeforeUnlock := source.offlineHealth.Load(types.EntityID(10))
	_, targetCachedBeforeUnlock := surface.offlineHealth.Load(types.EntityID(10))
	surface.mu.Unlock()
	require.True(t, attached)
	require.False(t, sourceCachedBeforeUnlock, "old-layer cache must retire before another owning call can run")
	require.False(t, targetCachedBeforeUnlock)
	readSoundLifecycleEntry(t, connection)

	// A following owning call can now update and expire the body. Its fresh
	// snapshot must survive; attachment has no cleanup left after its unlock.
	latest := playerRuntimeState{
		Health:      components.EntityHealth{SHP: 4.5, HHP: 18.5, IsLying: true, LyingRevision: 9},
		CombatState: ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 10000},
	}
	require.True(t, surface.detachClientForLogout(client, time.Time{}, 0))
	saver := systems.NewCharacterSaver(f.db, 1, inventory.NewInventorySaver(g.logger), g.logger)
	t.Cleanup(saver.Stop)
	expiry := systems.NewExpireDetachedSystem(g.logger, saver, surface.onDetachedEntityExpired, surface.onDetachedEntitiesExpired)
	surface.mu.Lock()
	ecs.AddComponent(surface.world, player, latest.Health)
	err = ecs.GetResource[ecs.CombatActivityState](surface.world).Restore(player, latest.CombatState)
	expiry.Update(surface.world, 0)
	alive := surface.world.Alive(player)
	surface.mu.Unlock()
	require.NoError(t, err)
	require.False(t, alive)
	saver.Stop()
	cached, exists := surface.offlineHealth.Load(types.EntityID(10))
	require.True(t, exists, "a later logout snapshot must not be erased by delayed attachment cleanup")
	require.Equal(t, latest, cached)
}
