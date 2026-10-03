package game

import (
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"net"
	"origin/internal/actionanimationdefs"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"path/filepath"
	"testing"
	"time"
)

func TestCombatTwoClientPublicRange(t *testing.T) {
	fixture, rangeService := newCombatRangeFixture(t)
	owner, ownerConnection := connectPlayerSpawnTestClient(t)
	observer, observerConnection := connectPlayerSpawnTestClient(t)
	owner.CharacterID = 1
	observer.CharacterID = 9
	owner.InWorld.Store(true)
	observer.InWorld.Store(true)
	owner.StreamEpoch.Store(1)
	observer.StreamEpoch.Store(7)
	observerHandle := fixture.world.Spawn(9, nil)
	shard := &Shard{world: fixture.world, logger: zap.NewNop(), Clients: map[types.EntityID]*network.Client{1: owner, 9: observer}}
	fixture.actions.sender = shard
	fixture.service.OnState = shard.publishCombatState
	fixture.service.OnResult = shard.publishCombatResult
	rangeService.OnTarget = shard.publishCombatTarget
	rangeService.OnRemove = shard.removeCombatFixture
	require.NoError(t, rangeService.Create(fixture.actor, "small"))
	visibility := ecs.GetResource[ecs.VisibilityState](fixture.world)
	visibility.ObserversByVisibleTarget[fixture.actor] = map[types.Handle]struct{}{observerHandle: {}}
	fixtures := rangeService.ranges[fixture.actor].fixtures
	for index, target := range fixtures {
		visibility.ObserversByVisibleTarget[target.handle] = map[types.Handle]struct{}{fixture.actor: {}}
		if index == 0 {
			visibility.ObserversByVisibleTarget[target.handle][observerHandle] = struct{}{}
		}
	}
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	for _, connection := range []net.Conn{ownerConnection, observerConnection} {
		start := readSoundLifecyclePacket(t, connection, func(message *netproto.ServerMessage) bool { return message.GetCombatState() != nil }).GetCombatState()
		require.Equal(t, "windup", start.State.Phase)
		require.Equal(t, float64(1000), start.State.DurationMs)
		require.Equal(t, int64(600), start.State.StrikeAtMs)
	}
	fixture.at(300)
	late := CombatExecutionSnapshot(fixture.world, fixture.actor)
	require.Equal(t, float64(300), late.ElapsedMs)
	require.Equal(t, float64(1), late.LockedDirection.X)
	fixture.at(600)
	fixture.service.Update(fixture.world, .1)
	ownerResult := readSoundLifecyclePacket(t, ownerConnection, func(message *netproto.ServerMessage) bool { return message.GetCombatResult() != nil }).GetCombatResult()
	observerResult := readSoundLifecyclePacket(t, observerConnection, func(message *netproto.ServerMessage) bool { return message.GetCombatResult() != nil }).GetCombatResult()
	require.Len(t, ownerResult.Hits, 3)
	require.Len(t, observerResult.Hits, 1, "result must not disclose invisible targets")
	require.Equal(t, uint32(7), observerResult.StreamEpoch)
	for _, hit := range ownerResult.Hits {
		require.Equal(t, float64(6), hit.Damage)
		require.Equal(t, float64(94), hit.Target.Hp)
	}
	fixture.service.Update(fixture.world, .1)
	for _, target := range fixtures {
		require.Equal(t, float64(94), CombatTargetSnapshot(fixture.world, target.handle).Hp)
	}
	fixture.at(1000)
	fixture.service.Update(fixture.world, .1)
	idle := readSoundLifecyclePacket(t, observerConnection, func(message *netproto.ServerMessage) bool {
		return message.GetCombatState() != nil && message.GetCombatState().State.Phase == "idle"
	}).GetCombatState()
	require.Greater(t, idle.State.Revision, late.Revision)
	rangeService.Remove(fixture.actor)
	removed := readSoundLifecyclePacket(t, observerConnection, func(message *netproto.ServerMessage) bool { return message.GetObjectDespawn() != nil }).GetObjectDespawn()
	require.Equal(t, observerResult.Hits[0].TargetId, removed.EntityId)
	require.Empty(t, visibility.ObserversByVisibleTarget[fixtures[0].handle])
}

func TestCombatAnimationSnapshotUsesRuntimeMilliseconds(t *testing.T) {
	fixture := newCombatFixture(t, 120)
	old := actionanimationdefs.Global()
	registry, err := actionanimationdefs.LoadFromDirectory(filepath.Join("..", "..", "data", "action_animations"), zap.NewNop())
	require.NoError(t, err)
	actionanimationdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { actionanimationdefs.SetGlobalForTesting(old) })
	ecs.AddComponent(fixture.world, fixture.actor, components.Appearance{Resource: "player"})
	ecs.GetResource[ecs.TimeState](fixture.world).TickPeriod = 17 * time.Millisecond
	fixture.arm(t, "axe_aoe")
	fixture.commit(t)
	fixture.at(350)
	sample, err := cyclicaction.Snapshot(fixture.world, fixture.actor)
	require.NoError(t, err)
	require.Equal(t, "axe_aoe", sample.AnimationKey)
	require.Equal(t, float64(350), sample.ElapsedMs)
	require.Equal(t, float64(1000), sample.DurationMs)
	require.Zero(t, sample.TotalTicks)
	require.Equal(t, float64(1), sample.LockedDirection.X)
	fixture.at(600)
	fixture.service.Update(fixture.world, .017)
	recovery, err := cyclicaction.Snapshot(fixture.world, fixture.actor)
	require.NoError(t, err)
	require.Equal(t, sample.Revision, recovery.Revision)
	require.Equal(t, float64(600), recovery.ElapsedMs)
	fixture.service.Interrupt(fixture.actor)
	stopped, err := cyclicaction.Snapshot(fixture.world, fixture.actor)
	require.NoError(t, err)
	require.Empty(t, stopped.AnimationKey)
	require.Greater(t, stopped.Revision, sample.Revision)
}

func TestCombatTwoClientSingleEscapeAndMovingAttacker(t *testing.T) {
	for _, scenario := range []string{"single", "tied", "escape", "moving attacker"} {
		t.Run(scenario, func(t *testing.T) {
			fixture, rangeService := newCombatRangeFixture(t)
			owner, ownerConnection := connectPlayerSpawnTestClient(t)
			observer, observerConnection := connectPlayerSpawnTestClient(t)
			owner.CharacterID, observer.CharacterID = 1, 9
			owner.InWorld.Store(true)
			observer.InWorld.Store(true)
			owner.StreamEpoch.Store(1)
			observer.StreamEpoch.Store(7)
			observerHandle := fixture.world.Spawn(9, nil)
			shard := &Shard{world: fixture.world, logger: zap.NewNop(), Clients: map[types.EntityID]*network.Client{1: owner, 9: observer}}
			fixture.actions.sender = shard
			fixture.service.OnState, fixture.service.OnResult = shard.publishCombatState, shard.publishCombatResult
			rangeService.OnTarget = shard.publishCombatTarget
			preset, action := "small", "axe_single"
			if scenario == "tied" {
				preset = "tied"
			}
			if scenario == "escape" {
				preset, action = "moving", "axe_aoe"
			}
			if scenario == "moving attacker" {
				action = "axe_aoe"
			}
			require.NoError(t, rangeService.Create(fixture.actor, preset))
			visibility := ecs.GetResource[ecs.VisibilityState](fixture.world)
			visibility.ObserversByVisibleTarget[fixture.actor] = map[types.Handle]struct{}{observerHandle: {}}
			targets := rangeService.ranges[fixture.actor].fixtures
			for _, target := range targets {
				visibility.ObserversByVisibleTarget[target.handle] = map[types.Handle]struct{}{fixture.actor: {}, observerHandle: {}}
			}
			fixture.arm(t, action)
			fixture.commit(t)
			cooldown := readSoundLifecyclePacket(t, ownerConnection, func(message *netproto.ServerMessage) bool { return message.GetCombatOwnerState() != nil }).GetCombatOwnerState()
			require.Len(t, cooldown.Cooldowns, 1)
			require.Equal(t, int64(2000), cooldown.Cooldowns[0].ReadyAtMs)
			stats, _ := ecs.GetComponent[components.EntityStats](fixture.world, fixture.actor)
			require.Equal(t, float64(440), stats.Stamina)
			if scenario == "moving attacker" {
				ecs.WithComponent(fixture.world, fixture.actor, func(position *components.Transform) { position.X = 101; position.Direction = 180 })
			}
			fixture.at(600)
			rangeService.Update(fixture.world, .1)
			fixture.service.Update(fixture.world, .1)
			for _, connection := range []net.Conn{ownerConnection, observerConnection} {
				result := readSoundLifecyclePacket(t, connection, func(message *netproto.ServerMessage) bool { return message.GetCombatResult() != nil }).GetCombatResult()
				switch scenario {
				case "single", "tied":
					require.Len(t, result.Hits, 1)
					firstID, _ := fixture.world.GetExternalID(targets[0].handle)
					require.Equal(t, uint64(firstID), result.Hits[0].TargetId)
					require.Equal(t, float64(9), result.Hits[0].Damage)
					require.Equal(t, float64(91), result.Hits[0].Target.Hp)
				case "escape":
					require.False(t, result.Hit)
					require.Empty(t, result.Hits)
				case "moving attacker":
					require.Len(t, result.Hits, 3)
					for _, hit := range result.Hits {
						require.Equal(t, float64(6), hit.Damage)
					}
				}
			}
			current := CombatExecutionSnapshot(fixture.world, fixture.actor)
			require.Equal(t, "recovery", current.Phase)
			require.Equal(t, float64(1), current.LockedDirection.X)
			require.Zero(t, current.LockedDirection.Y)
			fixture.at(1000)
			fixture.service.Update(fixture.world, .1)
			require.Equal(t, "idle", CombatExecutionSnapshot(fixture.world, fixture.actor).Phase)
		})
	}
}
