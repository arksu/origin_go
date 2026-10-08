package game

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/characterattrs"
	"origin/internal/config"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

func installMeleeLifecycleReceiver(t *testing.T, shard *Shard) {
	t.Helper()
	fixture := creatureDamageFixtureInWorld(t, shard.world)
	shard.creatureDamage = fixture.service
}

func meleeLifecycleSetup(shard *Shard, x, y int, health components.EntityHealth) func(*ecs.World, types.Handle) {
	return func(world *ecs.World, handle types.Handle) {
		attributes := characterattrs.Default()
		attributes[characterattrs.CON] = 4
		ecs.AddComponent(world, handle, components.Transform{X: float64(x), Y: float64(y)})
		ecs.AddComponent(world, handle, components.ChunkRef{})
		ecs.AddComponent(world, handle, components.EntityInfo{Layer: shard.layer})
		ecs.AddComponent(world, handle, health)
		ecs.AddComponent(world, handle, components.EntityStats{Stamina: 100, Energy: 900})
		ecs.AddComponent(world, handle, components.CharacterProfile{Attributes: attributes})
		ecs.AddComponent(world, handle, components.ActionCooldowns{})
		ecs.AddComponent(world, handle, components.Movement{State: constt.StateIdle})
	}
}

func spawnMeleeLifecyclePlayer(t *testing.T, shard *Shard, id types.EntityID, x, y int, health components.EntityHealth) types.Handle {
	t.Helper()
	require.NoError(t, shard.PrepareEntityAOI(t.Context(), id, x, y))
	ok, handle := shard.TrySpawnPlayer(x, y, repository.Character{ID: int64(id), Layer: shard.layer}, meleeLifecycleSetup(shard, x, y, health))
	require.True(t, ok)
	assertMeleeLifecyclePrepared(t, shard, id, handle)
	return handle
}

func assertMeleeLifecyclePrepared(t *testing.T, shard *Shard, id types.EntityID, handle types.Handle) {
	t.Helper()
	require.True(t, shard.creatureDamage.activity.IsPrepared(handle, id))
	require.True(t, ecs.GetResource[ecs.EntityStatsUpdateState](shard.world).IsPlayerPrepared(id, handle))
	require.True(t, ecs.GetResource[ecs.CharacterVisualDirtyQueue](shard.world).IsPrepared(handle))
	_, err := shard.creatureDamage.prepareDamage(handle, 0)
	require.NoError(t, err, "the exposed living body must already accept combat")
}

func assertMeleeLifecycleReleased(t *testing.T, shard *Shard, id types.EntityID, handle types.Handle) {
	t.Helper()
	_, registered := shard.creatureDamage.activity.Identity(handle)
	require.False(t, registered)
	require.False(t, ecs.GetResource[ecs.EntityStatsUpdateState](shard.world).IsPlayerPrepared(id, handle))
	require.False(t, ecs.GetResource[ecs.CharacterVisualDirtyQueue](shard.world).IsPrepared(handle))
}

func TestMeleeLifecycleSpawnPreparesBeforePublicationAndAttachment(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	installMeleeLifecycleReceiver(t, shard)
	// Async observers use the same locked read boundary as production readers.
	preparedOnPublication := make(chan bool, 1)
	shard.eventBus.SubscribeAsync(ecs.TopicGameplayPlayerEnterWorld, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		entered := event.(*ecs.PlayerEnteredWorldEvent)
		shard.WithWorldRead(func(world *ecs.World) {
			handle := world.GetHandleByEntityID(entered.EntityID)
			preparedOnPublication <- shard.creatureDamage.activity.IsPrepared(handle, entered.EntityID) &&
				ecs.GetResource[ecs.EntityStatsUpdateState](world).IsPlayerPrepared(entered.EntityID, handle) &&
				ecs.GetResource[ecs.CharacterVisualDirtyQueue](world).IsPrepared(handle)
		})
		return nil
	})
	health := components.EntityHealth{SHP: 21.4, HHP: 24.28}
	player := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, health)
	select {
	case prepared := <-preparedOnPublication:
		require.True(t, prepared)
	case <-time.After(time.Second):
		t.Fatal("successful spawn did not publish the prepared body")
	}
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, player)
	require.Equal(t, uint64(10), readSoundLifecycleEntry(t, connection).EntityId)
	assertMeleeLifecyclePrepared(t, shard, 10, player)
	actual, exists := ecs.GetComponent[components.EntityHealth](shard.world, player)
	require.True(t, exists)
	require.Equal(t, health, actual)
}

func TestMeleeLifecycleInvalidSpawnNeverPublishesOrRegisters(t *testing.T) {
	shard, entered := newPlayerSpawnTestShard(t, 8)
	installMeleeLifecycleReceiver(t, shard)
	require.NoError(t, shard.PrepareEntityAOI(t.Context(), 10, 200, 100))
	ok, handle := shard.TrySpawnPlayer(200, 100, repository.Character{ID: 10},
		meleeLifecycleSetup(shard, 200, 100, components.EntityHealth{SHP: 1, HHP: math.NaN()}))
	require.False(t, ok)
	require.Equal(t, types.InvalidHandle, handle)
	require.Equal(t, types.InvalidHandle, shard.world.GetHandleByEntityID(10))
	require.Equal(t, 1, shard.creatureDamage.activity.PreparedCount(), "only the unrelated fixture body stays prepared")
	require.Empty(t, ecs.GetResource[ecs.CharacterEntities](shard.world).GetAll())
	require.Zero(t, shard.chunkManager.GetChunk(types.ChunkCoord{}).Spatial().DynamicCount())
	flushPlayerSpawnEvents(t, shard.eventBus)
	require.Empty(t, entered)
}

func TestMeleeLifecycleDetachedReattachPreservesIdempotentRegistration(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
	installMeleeLifecycleReceiver(t, shard)
	player := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, components.EntityHealth{SHP: 21.4, HHP: 24.28})
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, player)
	firstEntry := readSoundLifecycleEntry(t, connection)
	// These are the existing detached-body operations from handleDisconnect.
	client.InWorld.Store(false)
	delete(shard.Clients, 10)
	shard.soundEvents.Detach(player, client.ID)
	detached := ecs.GetResource[ecs.DetachedEntities](shard.world)
	detached.AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
	ecs.ForgetPlayerStatsState(shard.world, 10)
	assertMeleeLifecyclePrepared(t, shard, 10, player)
	preparedCount := shard.creatureDamage.activity.PreparedCount()
	require.NoError(t, shard.prepareCreatureCombatTarget(player))
	require.NoError(t, shard.prepareCreatureCombatTarget(player))
	require.Equal(t, preparedCount, shard.creatureDamage.activity.PreparedCount())

	require.True(t, game.tryReattachPlayer(client, shard, 10, repository.Character{ID: 10}))
	secondEntry := readSoundLifecycleEntry(t, connection)
	require.Equal(t, firstEntry.StreamEpoch+1, secondEntry.StreamEpoch)
	require.True(t, client.InWorld.Load())
	_, remainsDetached := detached.GetDetachedEntity(10)
	require.False(t, remainsDetached)
	require.Equal(t, player, shard.world.GetHandleByEntityID(10))
	require.Equal(t, preparedCount, shard.creatureDamage.activity.PreparedCount())
	assertMeleeLifecyclePrepared(t, shard, 10, player)
}

func TestMeleeLifecycleRejectedReattachRetainsBodyAndCacheUntilRetry(t *testing.T) {
	for _, missingHealth := range []bool{false, true} {
		name := "invalid_health"
		if missingHealth {
			name = "missing_health"
		}
		t.Run(name, func(t *testing.T) {
			shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
			installMeleeLifecycleReceiver(t, shard)
			health := components.EntityHealth{SHP: 21.4, HHP: 24.28}
			player := spawnMeleeLifecyclePlayer(t, shard, 10, 200, 100, health)
			detached := ecs.GetResource[ecs.DetachedEntities](shard.world)
			detached.AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
			entry, _ := detached.GetDetachedEntity(10)
			shard.offlineHealth.Store(types.EntityID(10), playerRuntimeState{Health: health})
			// A surviving body from before combat initialization may need registration.
			shard.creatureDamage.releaseTarget(player)
			if missingHealth {
				ecs.RemoveComponent[components.EntityHealth](shard.world, player)
			} else {
				ecs.WithComponent(shard.world, player, func(current *components.EntityHealth) { current.HHP = math.NaN() })
			}
			client, connection := connectPlayerSpawnTestClient(t)
			client.CharacterID = 10
			// A rejected restoration is handled; it must not fall back to a second spawn.
			require.True(t, game.tryReattachPlayer(client, shard, 10, repository.Character{ID: 10}))
			require.False(t, client.InWorld.Load())
			require.Zero(t, client.StreamEpoch.Load())
			require.Empty(t, shard.Clients)
			actualEntry, stillDetached := detached.GetDetachedEntity(10)
			require.True(t, stillDetached)
			require.Equal(t, entry, actualEntry)
			cached, exists := shard.offlineHealth.Load(types.EntityID(10))
			require.True(t, exists)
			require.Equal(t, playerRuntimeState{Health: health}, cached)
			require.Equal(t, player, shard.world.GetHandleByEntityID(10))
			assertMeleeLifecycleReleased(t, shard, 10, player)

			ecs.AddComponent(shard.world, player, health)
			require.True(t, game.tryReattachPlayer(client, shard, 10, repository.Character{ID: 10}))
			require.Equal(t, uint64(10), readSoundLifecycleEntry(t, connection).EntityId)
			require.True(t, client.InWorld.Load())
			_, stillDetached = detached.GetDetachedEntity(10)
			require.False(t, stillDetached)
			_, exists = shard.offlineHealth.Load(types.EntityID(10))
			require.False(t, exists)
			assertMeleeLifecyclePrepared(t, shard, 10, player)
		})
	}
}

type meleeLifecycleTransferParticipant struct {
	t                            *testing.T
	captured, restored, rollback bool
}

func (*meleeLifecycleTransferParticipant) Key() string { return "combat_lifecycle_probe" }

func (p *meleeLifecycleTransferParticipant) CaptureSource(_ *Game, shard *Shard, req PlayerTransferRequest, handle types.Handle) (any, error) {
	assertMeleeLifecyclePrepared(p.t, shard, req.PlayerID, handle)
	p.captured = true
	return nil, nil
}

func (p *meleeLifecycleTransferParticipant) RestoreTarget(_ *Game, shard *Shard, req PlayerTransferRequest, handle types.Handle, _ any) error {
	assertMeleeLifecyclePrepared(p.t, shard, req.PlayerID, handle)
	p.restored = true
	return nil
}

func (p *meleeLifecycleTransferParticipant) RestoreSourceRollback(_ *Game, shard *Shard, req PlayerTransferRequest, handle types.Handle, _ any) error {
	assertMeleeLifecyclePrepared(p.t, shard, req.PlayerID, handle)
	p.rollback = true
	return nil
}

func (*meleeLifecycleTransferParticipant) OnTargetRestoreFailure(*Game, *Shard, PlayerTransferRequest, types.Handle, any, error) {
}

func TestMeleeLifecycleTransferAndRollbackPrepareNewGeneration(t *testing.T) {
	for _, test := range []struct {
		name             string
		rollback, closed bool
	}{
		{"target_layer", false, false},
		{"rollback_after_target_capacity_failure", true, false},
		{"closed_after_source_capture_target", false, true},
		{"closed_after_source_capture_rollback", true, true},
	} {
		rollback := test.rollback
		t.Run(test.name, func(t *testing.T) {
			source, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 8)
			installMeleeLifecycleReceiver(t, source)
			installLogoutLifecycleService(t, source)
			target, _ := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 3)
			target.layer, target.world.Layer = 1, 1
			installMeleeLifecycleReceiver(t, target)
			installLogoutLifecycleService(t, target)
			health := components.EntityHealth{SHP: 21.4, HHP: 24.28}
			player := spawnMeleeLifecyclePlayer(t, source, 10, 200, 100, health)
			client, connection := connectPlayerSpawnTestClient(t)
			attachSoundLifecycleClient(t, game, source, client, player)
			firstEntry := readSoundLifecycleEntry(t, connection)
			activity := ecs.GetResource[ecs.CombatActivityState](source.world)
			require.NoError(t, activity.ValidateEvent(player, 10, 10_000))
			activity.RecordPreparedEvent(player, 10_000)
			expectedCombat := ecs.CombatState{HasEvent: true, LastCombatEventAtUnixMs: 10_000}
			transfer := NewPlayerTransferService(game, zap.NewNop())
			participant := &meleeLifecycleTransferParticipant{t: t}
			transfer.RegisterParticipant(participant)
			req := PlayerTransferRequest{PlayerID: 10, SourceLayer: 0, TargetLayer: 1}
			snapshot, err := transfer.detachTransferSource(req, source, repository.Character{ID: 10})
			require.NoError(t, err)
			require.True(t, participant.captured)
			require.Equal(t, health, snapshot.RuntimeState.Health)
			require.Equal(t, expectedCombat, snapshot.RuntimeState.CombatState)
			cached, exists := source.offlineHealth.Load(types.EntityID(10))
			require.True(t, exists)
			require.Equal(t, snapshot.RuntimeState, cached)
			require.False(t, source.world.Alive(player))
			assertMeleeLifecycleReleased(t, source, 10, player)
			_, err = source.creatureDamage.Apply(player, 1)
			require.ErrorIs(t, err, ErrInvalidCreatureTarget)
			require.False(t, client.InWorld.Load())
			if test.closed {
				client.Close()
			}
			destination := target
			if rollback {
				blocker := target.world.Spawn(99, nil)
				require.NoError(t, target.PrepareEntityAOI(t.Context(), 10, 200, 100))
				ok, failed := target.TrySpawnPlayer(200, 100, repository.Character{ID: 10}, meleeLifecycleSetup(target, 200, 100, snapshot.RuntimeState.Health))
				require.False(t, ok)
				require.Equal(t, types.InvalidHandle, failed)
				require.Equal(t, 1, target.creatureDamage.activity.PreparedCount())
				require.Equal(t, types.InvalidHandle, target.world.GetHandleByEntityID(10))
				require.True(t, target.world.Despawn(blocker))
				destination = source
			}
			// Both target spawn and rollback use this shared spawnPlayerLocked hook.
			require.NoError(t, destination.PrepareEntityAOI(t.Context(), 10, snapshot.SourceX, snapshot.SourceY))
			preparedOnPublication := make(chan ecs.CombatState, 1)
			destination.eventBus.SubscribeAsync(ecs.TopicGameplayPlayerEnterWorld, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				entered := event.(*ecs.PlayerEnteredWorldEvent)
				destination.WithWorldRead(func(world *ecs.World) {
					if entered.EntityID == 10 {
						current, _ := ecs.GetResource[ecs.CombatActivityState](world).Capture(world.GetHandleByEntityID(10))
						preparedOnPublication <- current
					}
				})
				return nil
			})
			ok, newPlayer := destination.TrySpawnPlayer(snapshot.SourceX, snapshot.SourceY, repository.Character{ID: 10}, func(world *ecs.World, handle types.Handle) {
				meleeLifecycleSetup(destination, snapshot.SourceX, snapshot.SourceY, snapshot.RuntimeState.Health)(world, handle)
				require.NoError(t, restorePlayerCombatState(world, handle, 10, snapshot.RuntimeState.CombatState))
			})
			require.True(t, ok)
			select {
			case actual := <-preparedOnPublication:
				require.Equal(t, expectedCombat, actual)
			case <-time.After(time.Second):
				t.Fatal("restored combat state was not published")
			}
			if rollback {
				require.NotEqual(t, player, newPlayer)
				assertMeleeLifecycleReleased(t, source, 10, player)
			}
			if rollback {
				transfer.restoreParticipantsOnRollback(req, destination, newPlayer, snapshot.ParticipantStates)
				require.True(t, participant.rollback)
				require.False(t, participant.restored)
			} else {
				transfer.restoreParticipantsOnTarget(req, destination, newPlayer, snapshot.ParticipantStates)
				require.True(t, participant.restored)
				require.False(t, participant.rollback)
			}
			if test.closed {
				require.False(t, game.attachClientToWorld(destination, client, 10, repository.Character{ID: 10}, newPlayer))
				require.True(t, ecs.GetResource[ecs.DetachedEntities](destination.world).IsDetached(10))
				require.Empty(t, destination.Clients)
			} else {
				attachSoundLifecycleClient(t, game, destination, client, newPlayer)
				secondEntry := readSoundLifecycleEntry(t, connection)
				require.Equal(t, firstEntry.StreamEpoch+1, secondEntry.StreamEpoch)
			}
			assertMeleeLifecyclePrepared(t, destination, 10, newPlayer)
			actualHealth, exists := ecs.GetComponent[components.EntityHealth](destination.world, newPlayer)
			require.True(t, exists)
			require.Equal(t, health, actualHealth)
			actualCombat, prepared := ecs.GetResource[ecs.CombatActivityState](destination.world).Capture(newPlayer)
			require.True(t, prepared)
			require.Equal(t, expectedCombat, actualCombat)
			require.True(t, destination.world.Despawn(newPlayer))
			assertMeleeLifecycleReleased(t, destination, 10, newPlayer)
		})
	}
}
