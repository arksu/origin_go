package systems

import (
	"context"
	"math"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/entityhealth"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCharacterSaveRejectsHealthBeforeSerializationAndPreservesPending(t *testing.T) {
	for _, test := range []struct {
		name    string
		health  components.EntityHealth
		missing bool
	}{
		{name: "missing", missing: true},
		{name: "negative_shp", health: components.EntityHealth{SHP: -.01, HHP: 24.28}},
		{name: "negative_hhp", health: components.EntityHealth{SHP: 21.4, HHP: -1}},
		{name: "nan_shp", health: components.EntityHealth{SHP: math.NaN(), HHP: 24.28}},
		{name: "nan_hhp", health: components.EntityHealth{SHP: 21.4, HHP: math.NaN()}},
		{name: "positive_infinity", health: components.EntityHealth{SHP: 21.4, HHP: math.Inf(1)}},
		{name: "negative_infinity", health: components.EntityHealth{SHP: math.Inf(-1), HHP: 24.28}},
	} {
		for _, mode := range []string{"save", "detached", "sync"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				world, player := newCharacterSaveTestPlayer()
				if test.missing {
					ecs.RemoveComponent[components.EntityHealth](world, player)
				} else {
					ecs.AddComponent(world, player, test.health)
				}
				core, logs := observer.New(zap.DebugLevel)
				serialized, persisted := 0, 0
				saver := newCharacterSaver(0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot {
					serialized++
					return nil
				}), zap.New(core), func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error {
					persisted++
					return nil
				})
				previous := characterSaveTestSnapshot(10, 1)
				previous.SHP, previous.HHP = .25, .49
				require.True(t, saver.enqueueSnapshot(previous))
				pending := characterSavePending(t, saver, 10)
				want := entityhealth.ErrInvalidPools
				if test.missing {
					want = ErrMissingCharacterHealth
				}
				invoke := func() error {
					switch mode {
					case "sync":
						return saver.SaveSync(world, 10, player)
					case "detached":
						return saver.SaveDetached(world, 10, player)
					default:
						return saver.Save(world, 10, player)
					}
				}
				require.ErrorIs(t, invoke(), want)
				require.Equal(t, float64(0), testing.AllocsPerRun(1000, func() {
					if invoke() != want {
						panic("unexpected capture error")
					}
				}))
				require.Zero(t, serialized, "invalid health must reject before inventory serialization")
				require.Zero(t, persisted)
				require.Zero(t, logs.Len(), "missing profile would log if profile serialization ran")
				require.True(t, world.Alive(player))
				require.Same(t, pending, characterSavePending(t, saver, 10))
				actual, present := ecs.GetComponent[components.EntityHealth](world, player)
				require.Equal(t, !test.missing, present)
				if present {
					require.Equal(t, math.Float64bits(test.health.SHP), math.Float64bits(actual.SHP))
					require.Equal(t, math.Float64bits(test.health.HHP), math.Float64bits(actual.HHP))
				}
				require.NoError(t, saver.flushPending(context.Background(), saver.queueForCharacter(10), 0))
				require.Equal(t, 1, persisted, "last valid pending snapshot must remain flushable")
			})
		}
	}
}

func TestCharacterHealthCaptureExactAndAllocationFree(t *testing.T) {
	world, player := newCharacterSaveTestPlayer()
	saver := &CharacterSaver{}
	for _, health := range []components.EntityHealth{
		{SHP: 21.4, HHP: 24.28, IsLying: true},
		{SHP: .25, HHP: .49}, {SHP: 81.0 / 13, HHP: 24.28},
		{SHP: math.SmallestNonzeroFloat64, HHP: math.MaxFloat64}, {},
	} {
		ecs.AddComponent(world, player, health)
		require.Equal(t, float64(0), testing.AllocsPerRun(1000, func() {
			shp, hhp, lying, err := saver.resolveHealthSnapshotValues(world, player)
			if err != nil || shp != health.SHP || hhp != health.HHP || lying != health.IsLying {
				panic("health changed during capture")
			}
		}))
	}
	world.Despawn(player)
	replacement := world.Spawn(10, nil)
	ecs.AddComponent(world, replacement, components.EntityHealth{SHP: 21.4, HHP: 24.28})
	_, err := saver.captureSnapshot(world, 10, player)
	require.ErrorIs(t, err, ErrInvalidCharacterHandle)
	require.True(t, world.Alive(replacement))
}

func TestPeriodicCaptureFailureReschedulesAndKeepsCounters(t *testing.T) {
	world, player := newCharacterSaveTestPlayer()
	now := time.Unix(100, 0)
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.Now = now
	characters := ecs.GetResource[ecs.CharacterEntities](world)
	characters.Add(10, player, now)
	characters.UpdateSaveTime(10, now.Add(-time.Minute), now)
	before := characters.Map[10]
	ecs.WithComponent(world, player, func(h *components.EntityHealth) { h.HHP = -1 })
	saver := newCharacterSaver(0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot { return nil }), zap.NewNop(), func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error {
		return nil
	})
	system := NewCharacterSaveSystem(saver, time.Minute, zap.NewNop())
	system.Update(world, 0)
	failed := characters.Map[10]
	require.Equal(t, before.LastSaveAt, failed.LastSaveAt)
	require.Equal(t, before.SavesCount, failed.SavesCount)
	require.Equal(t, now.Add(CharacterSaveCaptureRetryInterval), failed.NextSaveAt)
	require.Equal(t, 1, characters.PendingSaveCount(), "PopDue entry must return to scheduling after failure")
	require.Nil(t, characterSavePending(t, saver, 10))
	ecs.WithComponent(world, player, func(h *components.EntityHealth) { h.HHP = 24.28 })
	clock.Now = failed.NextSaveAt.Add(-time.Nanosecond)
	system.Update(world, 0)
	require.Nil(t, characterSavePending(t, saver, 10))
	clock.Now = failed.NextSaveAt
	system.Update(world, 0)
	require.Equal(t, 21.4, characterSavePending(t, saver, 10).SHP)
	require.Equal(t, 24.28, characterSavePending(t, saver, 10).HHP)
	require.Equal(t, before.SavesCount+1, characters.Map[10].SavesCount)
	require.Equal(t, clock.Now, characters.Map[10].LastSaveAt)
	require.Equal(t, 1, characters.PendingSaveCount())
}

func TestSaveAllReportsRejectionsAndDrainsValidSnapshots(t *testing.T) {
	world, player := newCharacterSaveTestPlayer()
	characters := ecs.GetResource[ecs.CharacterEntities](world)
	characters.Add(10, player, time.Time{})
	for _, id := range []types.EntityID{20, 30} {
		handle := world.Spawn(id, nil)
		if id == 20 {
			ecs.AddComponent(world, handle, components.EntityHealth{SHP: 1, HHP: math.NaN()})
		}
		characters.Add(id, handle, time.Time{})
	}
	writes := 0
	saver := newCharacterSaver(0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot { return nil }), zap.NewNop(), func(_ context.Context, chars repository.UpdateCharactersParams, _ repository.UpsertInventoriesParams) error {
		writes++
		require.Equal(t, []int{10}, chars.Ids)
		require.Equal(t, []float64{21.4}, chars.Shps)
		require.Equal(t, []float64{24.28}, chars.Hhps)
		return nil
	})
	err := saver.SaveAll(world)
	require.ErrorIs(t, err, ErrMissingCharacterHealth)
	require.ErrorIs(t, err, entityhealth.ErrInvalidPools)
	require.ErrorContains(t, err, "character 20")
	require.ErrorContains(t, err, "character 30")
	require.NotNil(t, characterSavePending(t, saver, 10))
	saver.Stop()
	require.Equal(t, 1, writes)
	require.Nil(t, characterSavePending(t, saver, 10))
	for _, tracked := range characters.Map {
		require.True(t, world.Alive(tracked.Handle))
	}
}

func TestDetachedAndPeriodicFailuresShareCaptureRetryDeadline(t *testing.T) {
	for _, first := range []string{"periodic", "expiry"} {
		t.Run(first, func(t *testing.T) {
			world, player := newCharacterSaveTestPlayer()
			now := time.Unix(100, 0)
			clock := ecs.GetResource[ecs.TimeState](world)
			clock.Now = now
			characters := ecs.GetResource[ecs.CharacterEntities](world)
			nextPeriodic := now
			if first == "expiry" {
				nextPeriodic = now.Add(time.Second)
			}
			characters.Add(10, player, nextPeriodic)
			detached := ecs.GetResource[ecs.DetachedEntities](world)
			detached.AddDetachedEntity(10, player, now.Add(-time.Second), now.Add(-time.Minute))
			ecs.WithComponent(world, player, func(h *components.EntityHealth) { h.HHP = -1 })
			serialized, cleaned := 0, 0
			saver := newCharacterSaver(0, characterSaveInventoryFunc(func(interface{}, types.EntityID, types.Handle) []InventorySnapshot {
				serialized++
				return nil
			}), zap.NewNop(), func(context.Context, repository.UpdateCharactersParams, repository.UpsertInventoriesParams) error {
				return nil
			})
			periodic := NewCharacterSaveSystem(saver, time.Minute, zap.NewNop())
			expiry := NewExpireDetachedSystem(zap.NewNop(), saver, func(types.EntityID, types.Handle) { cleaned++ }, nil)
			periodic.Update(world, 0)
			expiry.Update(world, 0)
			retryAt := now.Add(CharacterSaveCaptureRetryInterval)
			require.Equal(t, retryAt, characters.Map[10].NextSaveAt)
			require.Equal(t, retryAt, detached.Map[10].SaveRetryAt)
			require.Zero(t, characters.Map[10].SavesCount)
			ecs.WithComponent(world, player, func(h *components.EntityHealth) { h.HHP = 24.28 })
			clock.Now = retryAt.Add(-time.Nanosecond)
			periodic.Update(world, 0)
			expiry.Update(world, 0)
			require.Zero(t, serialized, "neither system may recapture before the shared retry deadline")
			require.True(t, world.Alive(player))
			clock.Now = retryAt
			periodic.Update(world, 0)
			expiry.Update(world, 0)
			require.Greater(t, serialized, 0)
			require.Equal(t, 1, cleaned)
			require.False(t, world.Alive(player))
			periodic.Update(world, 0)
			expiry.Update(world, 0)
			require.Equal(t, 1, cleaned)
		})
	}
}
