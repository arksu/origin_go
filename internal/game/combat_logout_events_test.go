package game

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

func TestCombatActivityAcceptedStartAndCompletion(t *testing.T) {
	for _, action := range []string{"axe_sweep", "axe_strike"} {
		t.Run(action, func(t *testing.T) {
			f := newMeleeFixture(t)
			target := f.creature(t, 3, 60, 50, false)
			f.start(action, 0)
			state, _ := f.service.activity.Capture(f.owner)
			require.Equal(t, ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 1000}, state)
			state, _ = f.service.activity.Capture(target)
			require.Zero(t, state, "starting does not mark a potential victim")
			f.finish()
			for _, handle := range []types.Handle{f.owner, target} {
				state, _ = f.service.activity.Capture(handle)
				require.Equal(t, ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 1600}, state)
			}
		})
	}
	for _, withObject := range []bool{false, true} {
		f := newMeleeFixture(t)
		if withObject {
			f.object(t, 3, 60, 50, 100)
		}
		f.start("axe_sweep", 0)
		f.finish()
		state, _ := f.service.activity.Capture(f.owner)
		require.Equal(t, int64(1600), state.LastCombatEventAtUnixMs, "miss and object contact both mark actor")
	}
}

func TestCombatActivityCancellationAndRejectedCompletion(t *testing.T) {
	f := newMeleeFixture(t)
	f.start("axe_sweep", 0)
	f.actions.Cancel(f.world, 1, f.owner)
	f.finish()
	state, _ := f.service.activity.Capture(f.owner)
	require.Equal(t, int64(1000), state.LastCombatEventAtUnixMs)
	require.Empty(t, f.sender.attacks)
	for _, failed := range []string{"last target", "cost"} {
		t.Run(failed, func(t *testing.T) {
			f := newMeleeFixture(t)
			first := f.creature(t, 3, 55, 50, false)
			last := f.creature(t, 4, 60, 50, false)
			f.start("axe_sweep", 0)
			if failed == "last target" {
				ecs.WithComponent(f.world, last, func(h *components.EntityHealth) { h.HHP = math.NaN() })
			} else {
				ecs.WithComponent(f.world, f.owner, func(s *components.EntityStats) { s.Stamina = 0 })
			}
			f.finish()
			state, _ := f.service.activity.Capture(f.owner)
			require.Equal(t, int64(1000), state.LastCombatEventAtUnixMs)
			for _, handle := range []types.Handle{first, last} {
				state, _ = f.service.activity.Capture(handle)
				require.Zero(t, state)
			}
		})
	}
}

func TestCombatActivityInvalidStartDoesNotEnterCycle(t *testing.T) {
	for _, now := range []int64{-1, math.MaxInt64 - ecs.CombatLogoutHoldMs + 1} {
		f := newMeleeFixture(t)
		ecs.GetResource[ecs.TimeState](f.world).UnixMs = now
		f.start("axe_sweep", 0)
		state, _ := f.service.activity.Capture(f.owner)
		require.Zero(t, state)
		_, running := ecs.GetComponent[components.ActiveCyclicAction](f.world, f.owner)
		require.False(t, running)
		require.Equal(t, 1000.0, f.stamina())
	}
}

func TestCreatureCombatZeroDamageAndEqualTimestampWakeLogout(t *testing.T) {
	f := newCreatureDamageFixture(t)
	clock := ecs.GetResource[ecs.TimeState](f.world)
	clock.Now = time.Unix(100, 0)
	detached := ecs.GetResource[ecs.DetachedEntities](f.world)
	require.NoError(t, detached.PreparePlayer(1, f.owner))
	detached.AddDetachedEntity(1, f.owner, clock.Now, clock.Now)
	var buffer [1]ecs.DetachedCandidate
	require.Len(t, detached.PopDueInto(clock.Now, buffer[:0]), 1)
	detached.ScheduleNext(1, f.owner, clock.Now, time.Second, time.Millisecond)
	before := f.health()
	stats, visual := f.stats.PendingPlayerPushCount(), f.visual.PendingCount()
	result, err := f.service.Apply(f.owner, 0)
	require.NoError(t, err)
	require.Zero(t, result.Damage)
	require.Equal(t, before, f.health())
	require.Equal(t, stats, f.stats.PendingPlayerPushCount())
	require.Equal(t, visual, f.visual.PendingCount())
	require.Len(t, detached.PopDueInto(clock.Now, buffer[:0]), 1)
	state, _ := f.service.activity.Capture(f.owner)
	require.Equal(t, int64(1000), state.LastCombatEventAtUnixMs)
	detached.ScheduleNext(1, f.owner, clock.Now, time.Second, time.Millisecond)
	result, err = f.service.Apply(f.owner, 10)
	require.NoError(t, err)
	require.True(t, result.EnteredKO)
	require.Len(t, detached.PopDueInto(clock.Now, buffer[:0]), 1, "equal event time still wakes a newly KO body")
	state, _ = f.service.activity.Capture(f.owner)
	require.Equal(t, int64(1000), state.LastCombatEventAtUnixMs)
}

func TestCreatureCombatActivityRejectsDeadlineOverflowEvenForZeroDamage(t *testing.T) {
	f := newCreatureDamageFixture(t)
	ecs.GetResource[ecs.TimeState](f.world).UnixMs = math.MaxInt64 - ecs.CombatLogoutHoldMs + 1
	f.assertRejected(t, 0, ErrInvalidCreatureDamageTime)
	state, _ := f.service.activity.Capture(f.owner)
	require.Zero(t, state)
}
