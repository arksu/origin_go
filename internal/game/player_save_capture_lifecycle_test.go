package game

import (
	"math"
	"testing"
	"time"

	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type captureRetryInventoryRecorder struct {
	disconnectInventoryRecorder
	calls int
}

func (r *captureRetryInventoryRecorder) SerializeInventories(world interface{}, id types.EntityID, handle types.Handle) []systems.InventorySnapshot {
	r.calls++
	return r.disconnectInventoryRecorder.SerializeInventories(world, id, handle)
}

func TestZeroDelayQueuedDisconnectRetainsOwnerAndInventoriesAfterCaptureRejection(t *testing.T) {
	for _, missingHealth := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonfinite_health", true: "missing_health"}[missingHealth], func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			now := time.Unix(100, 0)
			clock := ecs.GetResource[ecs.TimeState](world)
			clock.Now = now
			player := world.Spawn(10, nil)
			ecs.AddComponent(world, player, components.Transform{})
			ecs.AddComponent(world, player, components.EntityStats{Stamina: 80, Energy: 900})
			ecs.AddComponent(world, player, components.Movement{TargetType: constt.TargetPoint, State: constt.StateMoving, VelocityX: 1})
			if !missingHealth {
				ecs.AddComponent(world, player, components.EntityHealth{SHP: .49, HHP: math.NaN()})
			}
			index := ecs.GetResource[ecs.InventoryRefIndex](world)
			container := world.SpawnWithoutExternalID()
			ecs.AddComponent(world, container, components.InventoryContainer{OwnerID: 10, Kind: constt.InventoryGrid, Width: 2, Height: 2})
			index.Add(constt.InventoryGrid, 10, 0, container)
			ecs.AddComponent(world, player, components.InventoryOwner{Inventories: []components.InventoryLink{{OwnerID: 10, Kind: constt.InventoryGrid, Handle: container}}})
			characters := ecs.GetResource[ecs.CharacterEntities](world)
			characters.Add(10, player, now.Add(time.Second))
			recorder := &captureRetryInventoryRecorder{disconnectInventoryRecorder: disconnectInventoryRecorder{containerHandles: []types.Handle{container}}}
			// No workers or DB writes: capture acceptance is the deletion boundary.
			saver := systems.NewCharacterSaver(nil, 0, recorder, zap.NewNop())
			client := &network.Client{ID: 1, CharacterID: 10}
			shard := &Shard{world: world, characterSaver: saver, logger: zap.NewNop(), Clients: map[types.EntityID]*network.Client{10: client}}
			require.True(t, shard.detachClientForLogout(client, now, 0))
			system := systems.NewExpireDetachedSystem(zap.NewNop(), saver, shard.onDetachedEntityExpired, nil)
			system.Update(world, 0)
			require.True(t, world.Alive(player))
			require.True(t, world.Alive(container))
			require.Equal(t, player, characters.Map[10].Handle)
			require.Zero(t, recorder.calls)
			indexed, found := index.Lookup(constt.InventoryGrid, 10, 0)
			require.True(t, found)
			require.Equal(t, container, indexed)
			entry, detached := ecs.GetResource[ecs.DetachedEntities](world).GetDetachedEntity(10)
			require.True(t, detached)
			require.Equal(t, now, entry.ExpirationTime)
			require.Equal(t, now, entry.DetachedAt)
			require.Equal(t, now.Add(systems.CharacterSaveCaptureRetryInterval), entry.SaveRetryAt)
			require.Equal(t, entry.SaveRetryAt, characters.Map[10].NextSaveAt)
			require.Zero(t, characters.Map[10].SavesCount)
			movement, _ := ecs.GetComponent[components.Movement](world, player)
			require.Equal(t, constt.TargetNone, movement.TargetType)
			require.Zero(t, movement.VelocityX)
			_, cached := shard.offlineHealth.Load(types.EntityID(10))
			require.False(t, cached, "rejected capture must not run detach cleanup")

			ecs.AddComponent(world, player, components.EntityHealth{SHP: .49, HHP: 19.6})
			clock.Now = entry.SaveRetryAt.Add(-time.Nanosecond)
			system.Update(world, 0)
			require.True(t, world.Alive(player))
			require.Zero(t, recorder.calls)
			clock.Now = entry.SaveRetryAt
			system.Update(world, 0)
			require.Equal(t, 1, recorder.calls)
			require.True(t, recorder.aliveAtSave)
			require.True(t, recorder.containersAliveAtSave)
			require.False(t, world.Alive(player))
			require.False(t, world.Alive(container))
			require.NotContains(t, characters.Map, types.EntityID(10))
			require.NotContains(t, ecs.GetResource[ecs.DetachedEntities](world).Map, types.EntityID(10))
			_, found = index.Lookup(constt.InventoryGrid, 10, 0)
			require.False(t, found)
		})
	}
}

type shutdownCaptureLockRecorder struct {
	shard  *Shard
	calls  int
	locked bool
}

func (r *shutdownCaptureLockRecorder) SerializeInventories(interface{}, types.EntityID, types.Handle) []systems.InventorySnapshot {
	r.calls++
	if r.shard.mu.TryLock() {
		r.shard.mu.Unlock()
	} else {
		r.locked = true
	}
	return nil
}

func TestShardShutdownCapturesUnderLockAndReportsRejectedSnapshots(t *testing.T) {
	world := ecs.NewWorldForTesting()
	characters := ecs.GetResource[ecs.CharacterEntities](world)
	for _, id := range []types.EntityID{10, 20} {
		player := world.Spawn(id, nil)
		ecs.AddComponent(world, player, components.Transform{})
		ecs.AddComponent(world, player, components.EntityStats{Stamina: 80, Energy: 900})
		health := components.EntityHealth{SHP: .49, HHP: 19.6}
		if id == 20 {
			health.HHP = math.NaN()
		}
		ecs.AddComponent(world, player, health)
		characters.Add(id, player, time.Unix(100, 0))
	}
	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	recorder := &shutdownCaptureLockRecorder{}
	// A stopped empty saver makes enqueue fail without creating a DB worker.
	saver := systems.NewCharacterSaver(nil, 0, recorder, logger)
	saver.Stop()
	cfg := &config.Config{}
	cfg.Game.ChunkLRUCapacity = 1
	manager := gameworld.NewChunkManager(cfg, nil, world, nil, 0, 1, nil, nil, nil, zap.NewNop())
	shard := &Shard{world: world, characterSaver: saver, chunkManager: manager, logger: logger}
	recorder.shard = shard
	shard.Stop()
	require.Equal(t, 1, recorder.calls)
	require.True(t, recorder.locked, "shutdown capture must hold the owner shard lock")
	require.True(t, shard.mu.TryLock(), "shutdown must leave the owner lock released")
	shard.mu.Unlock()
	errors := observed.FilterMessage("Shutdown character snapshots rejected").All()
	require.Len(t, errors, 1)
	require.Contains(t, errors[0].ContextMap()["error"], "character 10")
	require.Contains(t, errors[0].ContextMap()["error"], "character 20")
	for _, tracked := range characters.Map {
		require.True(t, world.Alive(tracked.Handle))
	}
}
