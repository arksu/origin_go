package game

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	gameworld "origin/internal/game/world"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

func spawnChatLifecyclePlayer(t *testing.T, shard *Shard, id types.EntityID, x, y int) types.Handle {
	t.Helper()
	handle := spawnSoundLifecycleEntity(t, shard, id, x, y)
	ecs.GetResource[ecs.CharacterEntities](shard.world).Add(id, handle, time.Unix(100, 0))
	return handle
}

func chatLifecycleRecipients(shard *Shard, x, y float64) []types.EntityID {
	return shard.AppendLocalChatRecipients(shard.world, x, y, 1000, 1000*1000, nil)
}

func TestLocalChatRecipientsFollowMovementAndImmediateRelocation(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	player := spawnChatLifecyclePlayer(t, shard, 10, 1200, 100)
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, player)
	readSoundLifecycleEntry(t, connection)
	require.Empty(t, chatLifecycleRecipients(shard, 0, 100))

	// This is the actual collision commit path, with no visible observers. The
	// position hook must run before TransformUpdateSystem's visibility guard.
	visibility := ecs.GetResource[ecs.VisibilityState](shard.world)
	delete(visibility.ObserversByVisibleTarget, player)
	ecs.AddComponent(shard.world, player, components.CollisionResult{FinalX: 500, FinalY: 100})
	ecs.GetResource[ecs.MovedEntities](shard.world).Add(player, 500, 100)
	transform := systems.NewTransformUpdateSystem(shard.world, shard.chunkManager, nil, zap.NewNop())
	transform.SetPositionObserver(shard.soundEvents)
	transform.Update(shard.world, .1)
	require.Empty(t, visibility.ObserversByVisibleTarget[player])
	require.Equal(t, soundCell{1, 0}, shard.soundEvents.listeners.members[player].cell)
	require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(shard, 0, 100))
	require.Empty(t, chatLifecycleRecipients(shard, 2000, 100), "old cell must no longer produce the moved player")

	require.True(t, gameworld.RelocateWorldObjectImmediate(shard.world, shard.chunkManager, nil, player,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: true}, 1200, 100, zap.NewNop()))
	require.Empty(t, chatLifecycleRecipients(shard, 0, 100))
	require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(shard, 2000, 100))
	require.True(t, gameworld.RelocateWorldObjectImmediate(shard.world, shard.chunkManager, nil, player,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: true}, 500, 100, zap.NewNop()))
	require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(shard, 0, 100))
}

func TestLocalChatRecipientsDisconnectReattachAndLateOldDetach(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	player := spawnChatLifecyclePlayer(t, shard, 10, 200, 100)
	// A shared server gives the two real connections distinct connection IDs.
	connections := newAttackResultFixture(t, 32)
	oldClient, oldConnection := connections.connect(t, 10, 0)
	attachSoundLifecycleClient(t, game, shard, oldClient, player)
	readSoundLifecycleEntry(t, oldConnection)
	require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(shard, 0, 100))

	require.True(t, shard.detachClientForLogout(oldClient, game.clock.GameNow(), time.Minute))
	require.True(t, shard.world.Alive(player))
	require.Contains(t, ecs.GetResource[ecs.CharacterEntities](shard.world).Map, types.EntityID(10),
		"a detached living body remains a character but must not receive chat")
	require.Empty(t, chatLifecycleRecipients(shard, 0, 100))

	replacement, replacementConnection := connections.connect(t, 10, 0)
	replacement.Layer = shard.layer
	require.NotEqual(t, oldClient.ID, replacement.ID)
	require.True(t, game.tryReattachPlayer(replacement, shard, 10, repository.Character{ID: 10}))
	readSoundLifecycleEntry(t, replacementConnection)
	require.Equal(t, player, shard.world.GetHandleByEntityID(10))
	require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(shard, 0, 100))

	require.False(t, shard.detachClientForLogout(oldClient, game.clock.GameNow(), 0))
	shard.soundEvents.Detach(player, oldClient.ID)
	require.Equal(t, replacement.ID, shard.soundEvents.listeners.members[player].clientID)
	require.False(t, ecs.GetResource[ecs.DetachedEntities](shard.world).IsDetached(10))
	require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(shard, 0, 100))
}

func TestLocalChatRecipientsTransferAndRollback(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		name := "target_layer"
		if rollback {
			name = "rollback_after_target_capacity_failure"
		}
		t.Run(name, func(t *testing.T) {
			source, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
			target, _ := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 1)
			target.layer, target.world.Layer = 1, 1
			player := spawnChatLifecyclePlayer(t, source, 10, 200, 100)
			client, connection := connectPlayerSpawnTestClient(t)
			attachSoundLifecycleClient(t, game, source, client, player)
			readSoundLifecycleEntry(t, connection)
			require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(source, 200, 100))
			require.Empty(t, chatLifecycleRecipients(target, 200, 100))

			transfer := NewPlayerTransferService(game, zap.NewNop())
			snapshot, err := transfer.detachTransferSource(PlayerTransferRequest{PlayerID: 10, SourceLayer: 0, TargetLayer: 1}, source, repository.Character{ID: 10})
			require.NoError(t, err)
			require.False(t, source.world.Alive(player))
			require.Empty(t, chatLifecycleRecipients(source, 200, 100))

			// Exercise the production detach/spawn/attach hooks. The DB-backed
			// asynchronous transfer orchestrator is outside this fixture; choose
			// its destination or rollback branch explicitly after a real failure.
			destination := target
			if rollback {
				blocker := target.world.Spawn(99, nil)
				require.NoError(t, target.PrepareEntityAOI(context.Background(), 10, 200, 100))
				ok, failed := target.TrySpawnPlayer(200, 100, repository.Character{ID: 10}, nil)
				require.False(t, ok)
				require.Equal(t, types.InvalidHandle, failed)
				require.Empty(t, chatLifecycleRecipients(target, 200, 100))
				target.world.Despawn(blocker)
				destination = source
			}
			newPlayer := spawnChatLifecyclePlayer(t, destination, 10, snapshot.SourceX, snapshot.SourceY)
			if rollback {
				require.NotEqual(t, player, newPlayer, "rollback replaces the old handle generation")
			}
			attachSoundLifecycleClient(t, game, destination, client, newPlayer)
			readSoundLifecycleEntry(t, connection)
			require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(destination, 200, 100))
			other := source
			if rollback {
				other = target
			}
			require.Empty(t, chatLifecycleRecipients(other, 200, 100))
			require.Empty(t, destination.AppendLocalChatRecipients(other.world, 200, 100, 1000, 1000*1000, nil),
				"a shard must never query another layer's world")
		})
	}
}

func TestLocalChatRecipientsExcludeDeadObserverWithAudioMembership(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	player := spawnChatLifecyclePlayer(t, shard, 10, 800, 100)
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, player)
	readSoundLifecycleEntry(t, connection)
	require.Equal(t, []types.EntityID{10}, chatLifecycleRecipients(shard, 0, 100))

	// Use the real death system's CharacterEntities removal and the real
	// observer session hook, without the DB/corpse-conversion death handler.
	ecs.AddComponent(shard.world, player, components.EntityHealth{})
	handler := &testPlayerDeathHandler{}
	NewPlayerDeathSystem(handler, PlayerDeathSystemConfig{}).Update(shard.world, 0)
	require.Len(t, handler.calls, 1)
	require.NotContains(t, ecs.GetResource[ecs.CharacterEntities](shard.world).Map, types.EntityID(10))
	require.True(t, shard.enterClientObserverModeAfterPermanentDeath(shard.world, 10))
	require.True(t, client.IsDeadObserverMode())
	require.True(t, shard.world.Alive(player))
	require.Contains(t, shard.soundEvents.listeners.members, player)
	require.Empty(t, chatLifecycleRecipients(shard, 0, 100))
	require.True(t, shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop"))
	shard.soundEvents.Flush(shard.world)
	require.Len(t, readSoundLifecycleBatch(t, connection).Sounds, 1,
		"chat exclusion must not remove the observer's audio membership")
}

func TestLocalChatRecipientsDoNotConsumeAudioStateOrBudgets(t *testing.T) {
	w, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	first := addSoundTestListener(t, w, service, 10, 0, 0)
	second := addSoundTestListener(t, w, service, 20, 800, 0)
	characters := ecs.GetResource[ecs.CharacterEntities](w)
	characters.Add(10, first, time.Unix(100, 0))
	characters.Add(20, second, time.Unix(100, 0))
	shard := &Shard{world: w, soundEvents: service}
	require.True(t, service.EmitPoint(w, soundPoint{}, "chop"))
	service.Flush(w)

	// Prime actual pending events and propagated entries to catch accidental
	// reuse of audio query scratch. Exhausted audio budgets must not cap chat.
	require.True(t, service.EmitPoint(w, soundPoint{}, "tree_fall"))
	require.Empty(t, service.propagate(w, ecs.GetOrCreateStorage[components.Transform](w), service.events[0], 1000))
	service.stats.Cells = uint64(service.config.MaxCellsPerTick)
	service.stats.Candidates = uint64(service.config.MaxCandidatesPerTick)
	service.stats.Entries = uint64(service.config.MaxEntriesPerTick)
	stats, lastTick, sequence := service.stats, service.LastTick, service.sequence
	events := append([]worldSoundEvent(nil), service.events...)
	recipients := append([]*soundListener(nil), service.recipients...)
	firstEntries := append([]soundEntry(nil), service.listeners.members[first].entries...)
	secondEntries := append([]soundEntry(nil), service.listeners.members[second].entries...)
	firstBytes, secondBytes := service.listeners.members[first].encodedBytes, service.listeners.members[second].encodedBytes
	packetCount := len(sender.packets)
	require.NotEmpty(t, events)
	require.Len(t, recipients, 2)
	require.NotEmpty(t, firstEntries)
	require.NotEmpty(t, secondEntries)

	for range 2 {
		require.ElementsMatch(t, []types.EntityID{10, 20}, chatLifecycleRecipients(shard, 0, 0))
	}
	require.Equal(t, stats, service.stats)
	require.Equal(t, lastTick, service.LastTick)
	require.Equal(t, sequence, service.sequence)
	require.Equal(t, events, service.events)
	require.Equal(t, recipients, service.recipients)
	require.Equal(t, firstEntries, service.listeners.members[first].entries)
	require.Equal(t, secondEntries, service.listeners.members[second].entries)
	require.Equal(t, firstBytes, service.listeners.members[first].encodedBytes)
	require.Equal(t, secondBytes, service.listeners.members[second].encodedBytes)
	require.Len(t, sender.packets, packetCount, "recipient selection must not send audio")
}
