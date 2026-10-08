package game

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

type logoutTestPolicy struct {
	decision ecs.LogoutDecision
	err      error
}

func (p *logoutTestPolicy) Check(ecs.LogoutContext) (ecs.LogoutDecision, error) {
	return p.decision, p.err
}

type logoutFixture struct {
	world    *ecs.World
	handle   types.Handle
	clock    *ecs.TimeState
	detached *ecs.DetachedEntities
	activity *ecs.CombatActivityState
	service  *PlayerLogoutService
	other    *logoutTestPolicy
}

func newLogoutFixture(t testing.TB) *logoutFixture {
	t.Helper()
	w := ecs.NewWorldForTesting()
	h := w.Spawn(1, nil)
	ecs.AddComponent(w, h, components.EntityHealth{SHP: 10, HHP: 50})
	f := &logoutFixture{world: w, handle: h, clock: ecs.GetResource[ecs.TimeState](w),
		detached: ecs.GetResource[ecs.DetachedEntities](w), activity: ecs.GetResource[ecs.CombatActivityState](w),
		other: &logoutTestPolicy{}}
	f.clock.Now, f.clock.UnixMs = time.Unix(100, 0), 100_000
	require.True(t, f.activity.Prepare(h, 1))
	combat, err := NewCombatLogoutPolicy(w)
	require.NoError(t, err)
	f.service, err = NewPlayerLogoutService(w, NewDisconnectDelayLogoutPolicy(), combat, f.other)
	require.NoError(t, err)
	require.NoError(t, f.service.PreparePlayer(h))
	f.detached.AddDetachedEntity(1, h, f.clock.Now, f.clock.Now)
	return f
}

func (f *logoutFixture) context() ecs.LogoutContext {
	detached, _ := f.detached.GetDetachedEntity(1)
	return ecs.LogoutContext{EntityID: 1, Handle: f.handle, Detached: detached, Time: *f.clock}
}

func TestPlayerLogoutPoliciesComposeIndependentBlockers(t *testing.T) {
	f := newLogoutFixture(t)
	decision, err := f.service.Check(f.context())
	require.NoError(t, err)
	require.False(t, decision.Blocked)
	f.detached.RemoveDetachedEntity(1)
	f.detached.AddDetachedEntity(1, f.handle, f.clock.Now.Add(10*time.Second), f.clock.Now)
	decision, err = f.service.Check(f.context())
	require.NoError(t, err)
	require.Equal(t, ecs.LogoutDecision{Blocked: true, RetryAfter: 10 * time.Second}, decision)
	f.activity.RecordPreparedEvent(f.handle, f.clock.UnixMs)
	decision, err = f.service.Check(f.context())
	require.NoError(t, err)
	require.Equal(t, ecs.LogoutDecision{Blocked: true, RetryAfter: time.Second}, decision)
	f.clock.Now = f.clock.Now.Add(31 * time.Second)
	f.clock.UnixMs += 31_000
	f.other.decision.Blocked = true // A future system need not know a deadline.
	decision, err = f.service.Check(f.context())
	require.NoError(t, err)
	require.Equal(t, ecs.LogoutDecision{Blocked: true, RetryAfter: time.Second}, decision)
	f.other.decision.Blocked = false
	decision, err = f.service.Check(f.context())
	require.NoError(t, err)
	require.False(t, decision.Blocked)
	f.detached.SetSaveRetryAt(1, f.handle, f.clock.Now.Add(5*time.Second))
	decision, err = f.service.Check(f.context())
	require.NoError(t, err)
	require.Equal(t, ecs.LogoutDecision{Blocked: true, RetryAfter: 5 * time.Second}, decision)
}

func TestCombatLogoutDeadlineAndTimeDomains(t *testing.T) {
	for _, test := range []struct {
		name    string
		now     int64
		ko      int64
		last    int64
		blocked bool
	}{
		{name: "last millisecond", now: 129_999, last: 100_000, blocked: true},
		{name: "combat equality permits", now: 130_000, last: 100_000},
		{name: "KO equality retains", now: 130_000, ko: 130_000, last: 100_000, blocked: true},
		{name: "KO finished", now: 130_001, ko: 130_000, last: 100_000},
		{name: "wall clock backwards", now: 99_000, last: 100_000, blocked: true},
		{name: "long deadline bounded hint", now: 100_000, ko: math.MaxInt64, last: 100_000, blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newLogoutFixture(t)
			require.NoError(t, f.activity.Restore(f.handle, ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: test.last}))
			ecs.WithComponent(f.world, f.handle, func(h *components.EntityHealth) { h.KOUntilUnixMs = test.ko; h.IsLying = true })
			f.clock.UnixMs = test.now
			decision, err := f.service.Check(f.context())
			require.NoError(t, err)
			require.Equal(t, test.blocked, decision.Blocked)
			if test.blocked {
				require.Equal(t, time.Second, decision.RetryAfter)
			}
		})
	}
	f := newLogoutFixture(t)
	f.clock.UnixMs = 0
	f.activity.RecordPreparedEvent(f.handle, 0)
	decision, err := f.service.Check(f.context())
	require.NoError(t, err)
	require.True(t, decision.Blocked, "UnixMs zero is a real accepted event")
}

func TestPlayerLogoutRejectsChangedIdentityAndInvalidState(t *testing.T) {
	for _, invalidate := range []func(*logoutFixture){
		func(f *logoutFixture) { f.clock.UnixMs = -1 },
		func(f *logoutFixture) {
			ecs.WithComponent(f.world, f.handle, func(h *components.EntityHealth) { h.SHP = math.NaN() })
		},
		func(f *logoutFixture) {
			ecs.WithComponent(f.world, f.handle, func(h *components.EntityHealth) { h.SHP = h.HHP + 1 })
		},
		func(f *logoutFixture) {
			ecs.WithComponent(f.world, f.handle, func(h *components.EntityHealth) { h.KOUntilUnixMs = -1 })
		},
		func(f *logoutFixture) { f.activity.Release(f.handle) },
		func(f *logoutFixture) { f.other.err = errors.New("test blocker failed") },
	} {
		f := newLogoutFixture(t)
		invalidate(f)
		decision, err := f.service.Check(f.context())
		require.Error(t, err)
		require.Equal(t, ecs.LogoutDecision{Blocked: true, RetryAfter: time.Second}, decision)
		require.True(t, f.world.Alive(f.handle))
		require.True(t, f.detached.IsDetached(1))
	}
	f := newLogoutFixture(t)
	staleContext := f.context()
	f.detached.SetSaveRetryAt(1, f.handle, f.clock.Now.Add(time.Second))
	_, err := f.service.Check(staleContext)
	require.ErrorIs(t, err, ErrInvalidPlayerLogoutTarget)
	f.world.Despawn(f.handle)
	require.False(t, f.detached.IsPrepared(1, f.handle))
	_, err = f.service.Check(staleContext)
	require.ErrorIs(t, err, ErrInvalidPlayerLogoutTarget)
}

func TestPlayerLogoutPreparedOperationsAllocateNothing(t *testing.T) {
	f := newLogoutFixture(t)
	context := f.context()
	var err error
	var decision ecs.LogoutDecision
	for _, state := range []string{"allow", "combat", "unknown", "error"} {
		t.Run(state, func(t *testing.T) {
			if state == "combat" {
				f.activity.RecordPreparedEvent(f.handle, f.clock.UnixMs)
			}
			f.other.decision.Blocked = state == "unknown"
			if state == "error" {
				f.other.err = ErrInvalidPlayerLogoutHealth
			}
			require.Zero(t, testing.AllocsPerRun(100, func() {
				decision, err = f.service.Check(context)
				f.service.RequestRecheck(f.handle)
			}))
			if state == "error" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if state != "allow" {
				require.True(t, decision.Blocked)
			}
		})
	}
}
