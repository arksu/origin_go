package systems

import (
	"errors"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestExpireDetachedBudgetIncludesBlockedCandidates(t *testing.T) {
	world := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.Now = time.Unix(100, 0)
	clock.TickPeriod = 100 * time.Millisecond
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	for id := types.EntityID(1); id <= 600; id++ {
		handle := world.Spawn(id, nil)
		detached.AddDetachedEntity(id, handle, clock.Now, clock.Now)
	}
	checks := make([]types.EntityID, 0, 600)
	cleanups := 0
	system := NewExpireDetachedSystem(zap.NewNop(), nil,
		func(types.EntityID, types.Handle) { cleanups++ }, nil,
		func(context ecs.LogoutContext) (ecs.LogoutDecision, error) {
			checks = append(checks, context.EntityID)
			return ecs.LogoutDecision{Blocked: true}, nil
		})
	require.Equal(t, 950, system.Priority())
	system.Update(world, 0)
	require.Len(t, checks, 256)
	require.Equal(t, types.EntityID(1), checks[0])
	require.Equal(t, types.EntityID(256), checks[255])
	require.Zero(t, cleanups)
	require.Len(t, detached.Map, 600)
	require.Equal(t, 600, detached.PendingCheckCount())
	system.Update(world, 0)
	require.Len(t, checks, 512)
	require.Equal(t, types.EntityID(257), checks[256])
	system.Update(world, 0)
	require.Len(t, checks, 600)
	system.Update(world, 0)
	require.Len(t, checks, 600, "consumed entries cannot spin while runtime has not advanced")
	clock.Now = clock.Now.Add(time.Second)
	system.Update(world, 0)
	require.Len(t, checks, 856)
}

func TestExpireDetachedErrorsRecheckAndAllowWithoutLosingState(t *testing.T) {
	world := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.Now = time.Unix(100, 0)
	clock.TickPeriod = 100 * time.Millisecond
	player := world.Spawn(10, nil)
	container := world.SpawnWithoutExternalID()
	ecs.AddComponent(world, container, components.InventoryContainer{OwnerID: 10})
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	detachedAt, expiration := clock.Now.Add(-time.Minute), clock.Now
	detached.AddDetachedEntity(10, player, expiration, detachedAt)
	blocked, fail, checks, cleanup := true, true, 0, 0
	policyErr := errors.New("invalid retention state")
	system := NewExpireDetachedSystem(zap.NewNop(), nil,
		func(types.EntityID, types.Handle) { cleanup++; world.Despawn(container) }, nil,
		func(context ecs.LogoutContext) (ecs.LogoutDecision, error) {
			checks++
			require.Equal(t, detachedAt, context.Detached.DetachedAt)
			require.Equal(t, expiration, context.Detached.ExpirationTime)
			require.Equal(t, clock.Now, context.Time.Now)
			if fail {
				return ecs.LogoutDecision{}, policyErr
			}
			return ecs.LogoutDecision{Blocked: blocked, RetryAfter: time.Hour}, nil
		})
	system.Update(world, 0)
	require.Equal(t, 1, checks)
	require.True(t, world.Alive(player))
	require.True(t, world.Alive(container))
	require.Equal(t, 1, detached.PendingCheckCount())
	clock.Now = clock.Now.Add(time.Second - time.Nanosecond)
	system.Update(world, 0)
	require.Equal(t, 1, checks, "policy errors retry after one second")
	clock.Now = clock.Now.Add(time.Nanosecond)
	fail = false
	system.Update(world, 0)
	require.Equal(t, 2, checks)
	blocked = false
	detached.RequestRecheck(player, clock.Now)
	system.Update(world, 0)
	require.Equal(t, 3, checks)
	require.Equal(t, 1, cleanup)
	require.False(t, world.Alive(player))
	require.False(t, world.Alive(container))
	require.Empty(t, detached.Map)
	require.Zero(t, detached.PreparedCount())
	system.Update(world, 0)
	require.Equal(t, 1, cleanup)
}

func TestExpireDetachedRejectsCorruptIdentityBeforePolicyOrSave(t *testing.T) {
	world := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](world)
	clock.Now = time.Unix(100, 0)
	player := world.Spawn(10, nil)
	detached := ecs.GetResource[ecs.DetachedEntities](world)
	detached.AddDetachedEntity(10, player, clock.Now, clock.Now)
	ecs.WithComponent(world, player, func(id *ecs.ExternalID) { id.ID = 20 })
	checks, cleanup := 0, 0
	system := NewExpireDetachedSystem(zap.NewNop(), nil,
		func(types.EntityID, types.Handle) { cleanup++ }, nil,
		func(ecs.LogoutContext) (ecs.LogoutDecision, error) { checks++; return ecs.LogoutDecision{}, nil })
	system.Update(world, 0)
	require.Zero(t, checks)
	require.Zero(t, cleanup)
	require.True(t, world.Alive(player))
	require.Equal(t, 1, detached.PendingCheckCount())
	clock.Now = clock.Now.Add(time.Second)
	ecs.WithComponent(world, player, func(id *ecs.ExternalID) { id.ID = 10 })
	system.Update(world, 0)
	require.Equal(t, 1, checks)
	require.Equal(t, 1, cleanup)
}

func TestExpireDetachedPartialAOIBatchFlushesDespiteUnknownBlocker(t *testing.T) {
	for _, respawn := range []bool{false, true} {
		name := "removed_body"
		if respawn {
			name = "replacement_body"
		}
		t.Run(name, func(t *testing.T) {
			world := ecs.NewWorldForTesting()
			clock := ecs.GetResource[ecs.TimeState](world)
			clock.Now = time.Unix(100, 0)
			detached := ecs.GetResource[ecs.DetachedEntities](world)
			player, blocker := world.Spawn(10, nil), world.Spawn(20, nil)
			detached.AddDetachedEntity(10, player, clock.Now, clock.Now)
			detached.AddDetachedEntity(20, blocker, clock.Now, clock.Now)
			var unregistered []types.EntityID
			system := NewExpireDetachedSystem(zap.NewNop(), nil, nil,
				func(ids []types.EntityID) { unregistered = append(unregistered, ids...) },
				func(context ecs.LogoutContext) (ecs.LogoutDecision, error) {
					return ecs.LogoutDecision{Blocked: context.EntityID == 20}, nil
				})
			system.Update(world, 0)
			require.False(t, world.Alive(player))
			require.True(t, world.Alive(blocker))
			require.Empty(t, unregistered)
			require.Equal(t, []types.EntityID{10}, system.pendingUnregister)
			var replacement types.Handle
			if respawn {
				replacement = world.Spawn(10, nil)
			}
			clock.Now = clock.Now.Add(time.Second - time.Nanosecond)
			system.Update(world, 0)
			require.Empty(t, unregistered)
			clock.Now = clock.Now.Add(time.Nanosecond)
			system.Update(world, 0)
			require.Empty(t, system.pendingUnregister)
			require.True(t, world.Alive(blocker))
			if respawn {
				require.Empty(t, unregistered, "old ID cleanup cannot unregister a newly attached body")
				require.True(t, world.Alive(replacement))
				require.Equal(t, replacement, world.GetHandleByEntityID(10))
			} else {
				require.Equal(t, []types.EntityID{10}, unregistered)
			}
		})
	}
}
