package systems

import (
	"context"
	"math"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestExpireDetachedRetainsRejectedCaptureUntilRetry(t *testing.T) {
	world := ecs.NewWorldForTesting()
	now := time.Unix(100, 0)
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.Now = now
	player := world.Spawn(10, nil)
	ecs.AddComponent(world, player, components.Transform{X: 9})
	ecs.AddComponent(world, player, components.EntityStats{Stamina: 80, Energy: 900})
	ecs.AddComponent(world, player, components.EntityHealth{SHP: .49, HHP: math.NaN()})
	characters := ecs.GetResource[ecs.CharacterEntities](world)
	characters.Add(10, player, now.Add(time.Minute))
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	expiresAt := now.Add(-time.Second)
	detachedAt := now.Add(-time.Minute)
	detached.AddDetachedEntity(10, player, expiresAt, detachedAt)
	container := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, container, components.InventoryContainer{OwnerID: 10})
	serialized := 0
	var persisted []repository.UpdateCharactersParams
	saver := newCharacterSaver(0, characterSaveInventoryFunc(func(w interface{}, id types.EntityID, handle types.Handle) []InventorySnapshot {
		serialized++
		require.True(t, w.(*ecs.World).Alive(handle))
		require.True(t, w.(*ecs.World).Alive(container))
		return []InventorySnapshot{{CharacterID: int64(id), Version: 3, Data: []byte(`{"items":[]}`)}}
	}), zap.NewNop(), func(_ context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
		persisted = append(persisted, chars)
		return nil
	})
	t.Cleanup(saver.Stop)
	cleanup := 0
	var unregistered []types.EntityID
	logs, observed := observer.New(zap.ErrorLevel)
	system := NewExpireDetachedSystem(zap.New(logs), saver, func(id types.EntityID, handle types.Handle) {
		cleanup++
		require.Equal(t, types.EntityID(10), id)
		require.Equal(t, player, handle)
		require.Equal(t, 1, serialized, "capture must precede cleanup")
		world.Despawn(container)
	}, func(ids []types.EntityID) { unregistered = append(unregistered, ids...) })
	system.Update(world, 0)
	require.True(t, world.Alive(player))
	require.True(t, world.Alive(container))
	require.Contains(t, characters.Map, types.EntityID(10))
	require.Equal(t, expiresAt, detached.Map[10].ExpirationTime)
	require.Equal(t, detachedAt, detached.Map[10].DetachedAt)
	retryAt := now.Add(CharacterSaveCaptureRetryInterval)
	require.Equal(t, retryAt, detached.Map[10].SaveRetryAt)
	require.Zero(t, serialized)
	require.Zero(t, cleanup)
	require.Empty(t, unregistered)
	require.Empty(t, persisted)
	require.Nil(t, characterSavePending(t, saver, 10))
	require.Equal(t, 1, observed.Len())

	ecs.WithComponent(world, player, func(health *components.EntityHealth) { health.HHP = 19.6 })
	clock.Now = retryAt.Add(-time.Nanosecond)
	system.Update(world, 0)
	require.Zero(t, serialized, "fixed state must still respect retry backoff")
	require.Equal(t, 1, observed.Len(), "no repeat logging before retry")
	clock.Now = retryAt
	system.Update(world, 0)
	require.False(t, world.Alive(player))
	require.False(t, world.Alive(container))
	require.NotContains(t, characters.Map, types.EntityID(10))
	require.NotContains(t, detached.Map, types.EntityID(10))
	require.Equal(t, []types.EntityID{10}, unregistered)
	require.Equal(t, 1, cleanup)
	pending := characterSavePending(t, saver, 10)
	require.NotNil(t, pending)
	require.Equal(t, .49, pending.SHP)
	require.Equal(t, 19.6, pending.HHP)
	require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(10), 0))
	require.Len(t, persisted, 1)
	require.Equal(t, []float64{.49}, persisted[0].Shps)
	require.Equal(t, []float64{19.6}, persisted[0].Hhps)
	system.Update(world, 0)
	require.Equal(t, 1, cleanup, "expiry must only clean up once")
}

func TestExpireDetachedRetiresStaleHandleWithoutTouchingReplacement(t *testing.T) {
	world := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.Now = time.Unix(100, 0)
	stale := world.Spawn(10, nil)
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	detached.AddDetachedEntity(10, stale, clock.Now.Add(-time.Second), clock.Now.Add(-time.Minute))
	require.True(t, world.Despawn(stale))
	replacement := world.Spawn(10, nil)
	require.NotEqual(t, stale, replacement)
	characters := ecs.GetResource[ecs.CharacterEntities](world)
	characters.Add(10, replacement, clock.Now.Add(time.Minute))
	container := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, container, components.InventoryContainer{OwnerID: 10})
	callbacks := 0
	system := NewExpireDetachedSystem(zap.NewNop(), nil, func(types.EntityID, types.Handle) { callbacks++ }, func([]types.EntityID) { callbacks++ })
	system.Update(world, 0)
	require.NotContains(t, detached.Map, types.EntityID(10))
	require.Equal(t, replacement, characters.Map[10].Handle)
	require.True(t, world.Alive(replacement))
	require.True(t, world.Alive(container))
	require.Zero(t, callbacks)
}
