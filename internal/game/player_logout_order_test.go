package game

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
)

// Register expiry first to exercise real priority sorting: a due disconnected
// victim must receive the sixth-tick hit before logout checks its old deadline.
func TestMeleeCompletionPrecedesDueCombatLogout(t *testing.T) {
	for _, action := range []string{"axe_sweep", "axe_strike"} {
		t.Run(action, func(t *testing.T) {
			f := newMeleeFixture(t)
			target := f.creature(t, 3, 60, 50, true)
			clock := ecs.GetResource[ecs.TimeState](f.world)
			clock.Now, clock.UnixMs, clock.TickPeriod = time.Unix(100, 0), 100_000, 100*time.Millisecond
			policy, err := NewCombatLogoutPolicy(f.world)
			require.NoError(t, err)
			logout, err := NewPlayerLogoutService(f.world, NewDisconnectDelayLogoutPolicy(), policy)
			require.NoError(t, err)
			require.NoError(t, logout.PreparePlayer(target))
			activity := ecs.GetResource[ecs.CombatActivityState](f.world)
			require.NoError(t, activity.Restore(target, ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 70_600}))
			detached := ecs.GetResource[ecs.DetachedEntities](f.world)
			detached.AddDetachedEntity(3, target, clock.Now, clock.Now)
			expiry := systems.NewExpireDetachedSystem(zap.NewNop(), nil, nil, nil, logout.Check)
			f.world.AddSystem(expiry)
			f.world.AddSystem(NewPlayerDeathSystem(nil, PlayerDeathSystemConfig{}))
			f.world.AddSystem(f.cyclic)
			f.start(action, 0)
			for i := 0; i < 6; i++ {
				clock.Tick++
				clock.Now = clock.Now.Add(clock.TickPeriod)
				clock.UnixMs += 100
				if i == 5 {
					logout.RequestRecheck(target)
				}
				f.world.Update(.1)
			}
			require.True(t, f.world.Alive(target))
			health, exists := ecs.GetComponent[components.EntityHealth](f.world, target)
			require.True(t, exists)
			require.Less(t, health.SHP, 25.0)
			state, prepared := activity.Capture(target)
			require.True(t, prepared)
			require.Equal(t, int64(100_600), state.LastCombatEventAtUnixMs)
			require.True(t, detached.IsDetached(3))
			require.Len(t, f.sender.attacks, 1)

			clock.Now = clock.Now.Add(30 * time.Second)
			clock.UnixMs += 30_000
			expiry.Update(f.world, .1)
			require.False(t, f.world.Alive(target))
			require.False(t, detached.IsDetached(3))
			require.False(t, activity.IsPrepared(target, 3))
		})
	}
}
