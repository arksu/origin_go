package game

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"origin/internal/actionanimationdefs"
	"origin/internal/config"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/timeutil"
	"origin/internal/types"
)

func newSoundLifecycleShard(t *testing.T, audio config.AudioConfig, capacity uint32) (*Shard, *Game) {
	t.Helper()
	shard, _ := newPlayerSpawnTestShard(t, capacity)
	shard.cfg.Game.Audio = audio
	shard.cfg.Game.TickRate = 10
	shard.cfg.Game.DisconnectDelay = 30
	shard.state = ShardStateRunning
	shard.playerInbox = network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig())
	shard.serverInbox = network.NewServerJobInbox(network.DefaultCommandQueueConfig())
	var err error
	shard.soundEvents, err = NewSoundEventService(soundTestProfiles(t), audio, shard)
	require.NoError(t, err)
	shard.chunkManager.SetPositionObserver(shard.soundEvents)
	timing := ecs.GetResource[ecs.TimeState](shard.world)
	timing.UnixMs, timing.TickPeriod = 10000, 100*time.Millisecond
	game := &Game{
		cfg: shard.cfg, logger: zap.NewNop(), clock: timeutil.NewManualClock(time.Unix(10, 0)),
		shardManager: &ShardManager{shards: map[int]*Shard{0: shard}},
	}
	game.setState(GameStateRunning)
	return shard, game
}

func spawnSoundLifecycleEntity(t *testing.T, shard *Shard, entityID types.EntityID, positionX, positionY int) types.Handle {
	t.Helper()
	require.NoError(t, shard.PrepareEntityAOI(context.Background(), entityID, positionX, positionY))
	success, handle := shard.TrySpawnPlayer(positionX, positionY, repository.Character{ID: int64(entityID)}, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: float64(positionX), Y: float64(positionY)})
		ecs.AddComponent(world, handle, components.ChunkRef{})
		ecs.AddComponent(world, handle, components.EntityInfo{Layer: shard.layer})
	})
	require.True(t, success)
	return handle
}

func readSoundLifecyclePacket(t *testing.T, connection net.Conn, accept func(*netproto.ServerMessage) bool) *netproto.ServerMessage {
	t.Helper()
	message, _ := readSoundLifecycleEnvelope(t, connection, accept)
	return message
}

func readSoundLifecycleEnvelope(t *testing.T, connection net.Conn, accept func(*netproto.ServerMessage) bool) (*netproto.ServerMessage, []byte) {
	t.Helper()
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(time.Second)))
	for range 32 {
		payload, opcode, err := wsutil.ReadServerData(connection)
		require.NoError(t, err)
		require.Equal(t, ws.OpBinary, opcode)
		message := &netproto.ServerMessage{}
		require.NoError(t, proto.Unmarshal(payload, message))
		if accept(message) {
			return message, payload
		}
	}
	t.Fatal("expected packet did not arrive within the bounded read")
	return nil, nil
}

func readSoundLifecycleEntry(t *testing.T, connection net.Conn) *netproto.S2C_PlayerEnterWorld {
	t.Helper()
	return readSoundLifecyclePacket(t, connection, func(message *netproto.ServerMessage) bool { return message.GetPlayerEnterWorld() != nil }).GetPlayerEnterWorld()
}

func readSoundLifecycleBatch(t *testing.T, connection net.Conn) *netproto.S2C_SoundBatch {
	t.Helper()
	return readSoundLifecyclePacket(t, connection, func(message *netproto.ServerMessage) bool { return message.GetSoundBatch() != nil }).GetSoundBatch()
}

func attachSoundLifecycleClient(t *testing.T, game *Game, shard *Shard, client *network.Client, handle types.Handle) {
	t.Helper()
	entityID, exists := shard.world.GetExternalID(handle)
	require.True(t, exists)
	client.CharacterID, client.Layer = entityID, shard.layer
	game.attachClientToWorld(shard, client, entityID, repository.Character{ID: int64(entityID), Layer: shard.layer}, handle)
	listener := shard.soundEvents.listeners.members[handle]
	require.NotNil(t, listener)
	require.Equal(t, client.ID, listener.clientID)
	require.Equal(t, client.StreamEpoch.Load(), listener.streamEpoch)
}

func TestSoundLifecycleEntryParametersAndFinalSessionGuard(t *testing.T) {
	audio := config.DefaultAudioConfig()
	audio.BaseHearing, audio.FreshnessMs = 1.5, 777
	shard, game := newSoundLifecycleShard(t, audio, 16)
	listener := spawnSoundLifecycleEntity(t, shard, 10, 1200, 100)
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, listener)
	entry := readSoundLifecycleEntry(t, connection)
	require.Equal(t, shard.soundEvents.EffectiveHearing(listener), entry.Audio.Hearing)
	require.Equal(t, uint32(777), entry.Audio.FreshnessMs)
	require.Equal(t, client.StreamEpoch.Load(), entry.StreamEpoch)
	require.True(t, shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop"))
	shard.soundEvents.Flush(shard.world)
	batch := readSoundLifecycleBatch(t, connection)
	require.Len(t, batch.Sounds, 1)
	require.Equal(t, 1500.0, batch.Sounds[0].MaxHearDistance)
	require.Equal(t, entry.StreamEpoch, batch.StreamEpoch)
	require.Equal(t, int64(10000), batch.ServerTimeMs)

	for _, test := range []struct {
		name     string
		clientID uint64
		epoch    uint32
		inWorld  bool
	}{{"old_connection", client.ID + 1, entry.StreamEpoch, true}, {"old_stream", client.ID, entry.StreamEpoch + 1, true}, {"detached", client.ID, entry.StreamEpoch, false}} {
		t.Run(test.name, func(t *testing.T) {
			client.InWorld.Store(test.inWorld)
			result := shard.SendSoundBatch(10, test.clientID, &netproto.S2C_SoundBatch{StreamEpoch: test.epoch, Sounds: batch.Sounds})
			require.Equal(t, network.AudioSendClosed, result.result)
			require.Zero(t, result.bytes, "obsolete sessions must be rejected before serialization")
			client.InWorld.Store(true)
		})
	}
}

func TestSoundLifecycleImmediateRelocationUsesCommittedPosition(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	listener := spawnSoundLifecycleEntity(t, shard, 10, 200, 100)
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, listener)
	readSoundLifecycleEntry(t, connection)
	shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop")
	require.True(t, gameworld.RelocateWorldObjectImmediate(shard.world, shard.chunkManager, nil, listener,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: true}, 1200, 100, zap.NewNop()))
	shard.soundEvents.Flush(shard.world)
	require.Zero(t, shard.soundEvents.LastTick.Recipients, "eligibility must use relocation committed after event creation")
	require.Equal(t, soundCell{4, 0}, shard.soundEvents.listeners.members[listener].cell)
	require.True(t, gameworld.RelocateWorldObjectImmediate(shard.world, shard.chunkManager, nil, listener,
		gameworld.RelocateWorldObjectImmediateOptions{IsTeleport: true}, 500, 100, zap.NewNop()))
	shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop")
	shard.soundEvents.Flush(shard.world)
	batch := readSoundLifecycleBatch(t, connection)
	require.InDelta(t, .5, batch.Sounds[0].GetDistanceGain(), 1e-6)
	require.Equal(t, soundCell{1, 0}, shard.soundEvents.listeners.members[listener].cell)
}

func TestSoundLifecycleReattachRefreshesHearingAndCancelsDetachedExpiry(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	player := spawnSoundLifecycleEntity(t, shard, 10, 1200, 100)
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, player)
	firstEntry := readSoundLifecycleEntry(t, connection)
	client.InWorld.Store(false)
	delete(shard.Clients, 10)
	shard.soundEvents.Detach(player, client.ID)
	detached := ecs.GetResource[ecs.DetachedEntities](shard.world)
	detached.AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
	shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop")
	shard.soundEvents.Flush(shard.world)
	require.Zero(t, shard.soundEvents.LastTick.Recipients)
	// Entry parameters come from the current hearing policy, even when the login config snapshot differs.
	refreshed := config.DefaultAudioConfig()
	refreshed.BaseHearing, refreshed.FreshnessMs = 1.5, 900
	var err error
	shard.soundEvents, err = NewSoundEventService(soundTestProfiles(t), refreshed, shard)
	require.NoError(t, err)
	shard.chunkManager.SetPositionObserver(shard.soundEvents)
	require.True(t, game.tryReattachPlayer(client, shard, 10, repository.Character{ID: 10}))
	secondEntry := readSoundLifecycleEntry(t, connection)
	require.Equal(t, firstEntry.StreamEpoch+1, secondEntry.StreamEpoch)
	require.Equal(t, 1.5, secondEntry.Audio.Hearing)
	require.Equal(t, uint32(900), secondEntry.Audio.FreshnessMs)
	_, stillDetached := detached.GetDetachedEntity(10)
	require.False(t, stillDetached)
	require.Equal(t, player, shard.world.GetHandleByEntityID(10))
	require.Equal(t, client.ID, shard.soundEvents.listeners.members[player].clientID)
	shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop")
	shard.soundEvents.Flush(shard.world)
	batch := readSoundLifecycleBatch(t, connection)
	require.Equal(t, secondEntry.StreamEpoch, batch.StreamEpoch)
	require.Equal(t, 1500.0, batch.Sounds[0].MaxHearDistance)
}

func TestSoundLifecycleTransferAndRollbackRefreshMembership(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		name := "target_layer"
		if rollback {
			name = "rollback_after_target_capacity_failure"
		}
		t.Run(name, func(t *testing.T) {
			source, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
			targetAudio := config.DefaultAudioConfig()
			targetAudio.BaseHearing, targetAudio.FreshnessMs = 1.5, 750
			target, _ := newSoundLifecycleShard(t, targetAudio, 1)
			target.layer, target.world.Layer = 1, 1
			player := spawnSoundLifecycleEntity(t, source, 10, 200, 100)
			client, connection := connectPlayerSpawnTestClient(t)
			attachSoundLifecycleClient(t, game, source, client, player)
			firstEntry := readSoundLifecycleEntry(t, connection)
			source.soundEvents.EmitPoint(source.world, soundPoint{200, 100}, "chop")
			transfer := NewPlayerTransferService(game, zap.NewNop())
			snapshot, err := transfer.detachTransferSource(PlayerTransferRequest{PlayerID: 10, SourceLayer: 0, TargetLayer: 1}, source, repository.Character{ID: 10})
			require.NoError(t, err)
			require.False(t, source.world.Alive(player))
			require.False(t, client.InWorld.Load())
			require.Empty(t, source.soundEvents.listeners.members)
			source.soundEvents.Flush(source.world)
			require.Zero(t, source.soundEvents.LastTick.Messages)
			destination := target
			if rollback {
				blocker := target.world.Spawn(99, nil)
				require.NoError(t, target.PrepareEntityAOI(context.Background(), 10, 200, 100))
				ok, failed := target.TrySpawnPlayer(200, 100, repository.Character{ID: 10}, nil)
				require.False(t, ok)
				require.Equal(t, types.InvalidHandle, failed)
				require.Empty(t, target.soundEvents.listeners.members)
				target.world.Despawn(blocker)
				destination = source
			}
			newPlayer := spawnSoundLifecycleEntity(t, destination, 10, snapshot.SourceX, snapshot.SourceY)
			if rollback {
				require.NotEqual(t, player, newPlayer, "rollback respawn must have a fresh entity generation")
			}
			attachSoundLifecycleClient(t, game, destination, client, newPlayer)
			if rollback {
				transfer.restoreParticipantsOnRollback(PlayerTransferRequest{PlayerID: 10}, destination, newPlayer, snapshot.ParticipantStates)
			} else {
				transfer.restoreParticipantsOnTarget(PlayerTransferRequest{PlayerID: 10}, destination, newPlayer, snapshot.ParticipantStates)
			}
			secondEntry := readSoundLifecycleEntry(t, connection)
			require.Equal(t, firstEntry.StreamEpoch+1, secondEntry.StreamEpoch)
			require.Equal(t, destination.soundEvents.EffectiveHearing(newPlayer), secondEntry.Audio.Hearing)
			require.EqualValues(t, destination.soundEvents.config.FreshnessMs, secondEntry.Audio.FreshnessMs)
			require.Equal(t, network.AudioSendClosed, source.SendSoundBatch(10, client.ID, &netproto.S2C_SoundBatch{StreamEpoch: firstEntry.StreamEpoch}).result)
			destination.soundEvents.EmitPoint(destination.world, soundPoint{200, 100}, "chop")
			destination.soundEvents.Flush(destination.world)
			batch := readSoundLifecycleBatch(t, connection)
			require.Equal(t, secondEntry.StreamEpoch, batch.StreamEpoch)
			require.Len(t, batch.Sounds, 1)
			if !rollback {
				source.soundEvents.EmitPoint(source.world, soundPoint{200, 100}, "chop")
				source.soundEvents.Flush(source.world)
				require.Zero(t, source.soundEvents.LastTick.Recipients)
			}
		})
	}
}

func TestSoundLifecycleObserverHearsUntilSessionDisconnect(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	observer := spawnSoundLifecycleEntity(t, shard, 10, 800, 100)
	// Another interested entity keeps the corpse's chunk active after its session leaves.
	require.NoError(t, shard.PrepareEntityAOI(context.Background(), 99, 800, 100))
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, observer)
	readSoundLifecycleEntry(t, connection)
	require.True(t, shard.enterClientObserverModeAfterPermanentDeath(shard.world, 10))
	require.True(t, client.IsDeadObserverMode())
	shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop")
	shard.soundEvents.Flush(shard.world)
	require.Len(t, readSoundLifecycleBatch(t, connection).Sounds, 1)
	game.handleDisconnect(client)
	require.True(t, shard.world.Alive(observer), "observer disconnect must not destroy its world object")
	require.Empty(t, shard.soundEvents.listeners.members)
	require.False(t, client.InWorld.Load())
	shard.soundEvents.EmitPoint(shard.world, soundPoint{0, 100}, "chop")
	shard.soundEvents.Flush(shard.world)
	require.Zero(t, shard.soundEvents.LastTick.Messages)
}

func TestSoundLifecycleShardFlushesAuthoredChopBeforeCompletion(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	registry, err := actionanimationdefs.LoadFromDirectory("../../data/action_animations", zap.NewNop())
	require.NoError(t, err)
	require.NoError(t, registry.PrepareSoundCues(shard.soundEvents.profiles))
	previous := actionanimationdefs.Global()
	actionanimationdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { actionanimationdefs.SetGlobalForTesting(previous) })
	performer := spawnSoundLifecycleEntity(t, shard, 10, 100, 100)
	tree := shard.world.Spawn(20, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: 200, Y: 100})
	})
	listener := spawnSoundLifecycleEntity(t, shard, 30, 1000, 100)
	client, connection := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, listener)
	_, entryPayload := readSoundLifecycleEnvelope(t, connection, func(message *netproto.ServerMessage) bool { return message.GetPlayerEnterWorld() != nil })
	type fixtureEmission struct {
		Tick         uint64  `json:"tick"`
		SoundKey     string  `json:"sound_key"`
		DistanceGain float32 `json:"distance_gain"`
		PacketBase64 string  `json:"packet_base64"`
	}
	emissions := make([]fixtureEmission, 0, 3)
	behavior := &actionCueBehavior{key: "tree", decision: contracts.BehaviorCycleDecisionContinue}
	behavior.onComplete = func(ctx *contracts.BehaviorCycleContext) {
		if ctx.Action.CycleIndex == 2 {
			behavior.decision = contracts.BehaviorCycleDecisionComplete
			ctx.World.Despawn(ctx.TargetHandle)
		}
	}
	service := NewContextActionService(shard.world, nil, nil, nil, nil, nil, nil, nil, nil, testSingleBehaviorRegistry{behavior: behavior}, nil)
	service.SetSoundEventService(shard.soundEvents)
	shard.world.AddSystem(NewCyclicActionSystem(service, nil, nil))
	cyclicaction.StartContext(shard.world, performer, components.ActiveCyclicAction{
		BehaviorKey: "tree", ActionID: "chop", CompleteSoundKey: "tree_fall",
		TargetKind: components.CyclicActionTargetObject, TargetID: 20, TargetHandle: tree,
		CycleDurationTicks: 20, CycleIndex: 1,
	})
	ecs.GetResource[ecs.LinkState](shard.world).SetLink(ecs.PlayerLink{PlayerID: 10, TargetID: 20})
	require.Empty(t, ecs.GetResource[ecs.VisibilityState](shard.world).ObserversByVisibleTarget)
	for tick := uint64(1); tick <= 40; tick++ {
		shard.Update(ecs.TimeState{Tick: tick, UnixMs: 10000 + int64(tick)*100, TickPeriod: 100 * time.Millisecond, Delta: .1})
		if tick == 8 || tick == 28 || tick == 40 {
			require.Equal(t, uint64(1), shard.soundEvents.LastTick.Messages)
			packet, payload := readSoundLifecycleEnvelope(t, connection, func(message *netproto.ServerMessage) bool { return message.GetSoundBatch() != nil })
			batch := packet.GetSoundBatch()
			require.Equal(t, int64(10000)+int64(tick)*100, batch.ServerTimeMs)
			require.Len(t, batch.Sounds, 1)
			key := "chop"
			if tick == 40 {
				key = "tree_fall"
			}
			require.Equal(t, key, batch.Sounds[0].SoundKey)
			require.Equal(t, 200.0, batch.Sounds[0].X)
			emissions = append(emissions, fixtureEmission{tick, key, batch.Sounds[0].GetDistanceGain(), base64.StdEncoding.EncodeToString(payload)})
		} else {
			require.Zero(t, shard.soundEvents.LastTick.Events)
		}
		if tick < 20 {
			require.Zero(t, behavior.effects)
		}
	}
	require.Equal(t, 2, behavior.effects)
	require.False(t, shard.world.Alive(tree))
	require.Empty(t, shard.soundEvents.events)
	if fixturePath := os.Getenv("ORIGIN_AUDIO_FIXTURE_PATH"); fixturePath != "" {
		fixture := struct {
			Version          int               `json:"v"`
			EnterWorldBase64 string            `json:"enter_world_base64"`
			Emissions        []fixtureEmission `json:"emissions"`
			Expected         map[string]any    `json:"expected"`
		}{
			Version: 1, EnterWorldBase64: base64.StdEncoding.EncodeToString(entryPayload), Emissions: emissions,
			Expected: map[string]any{"effects": 2, "chop_count": 2, "fall_count": 1, "target_alive": false, "outside_visibility": true},
		}
		encoded, err := json.MarshalIndent(fixture, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(fixturePath, append(encoded, '\n'), 0600))
	}
}

func TestSoundLifecycleOverloadPreservesGameplayComparedWithAudioDisabled(t *testing.T) {
	type result struct {
		stamina  float64
		effects  int
		states   []*netproto.S2C_ActionStateChanged
		progress []*netproto.S2C_CyclicActionProgress
		finished []*netproto.S2C_CyclicActionFinished
	}
	run := func(enabled bool) result {
		world, player, service, handler, progress, audio, _ := menuCueFixture(t, 20, true)
		audio.config.MaxEventsPerTick, audio.config.MaxEntriesPerTick, audio.config.MaxEntriesPerBatch = 1, 1, 1
		if !enabled {
			service.SetSoundEventService(nil)
		}
		var eventDrops uint64
		for range 40 {
			if enabled {
				for range 100 {
					audio.EmitPoint(world, soundPoint{}, "tree_fall")
				}
			}
			cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
			require.True(t, exists)
			service.AdvanceCycle(world, 101, player, cycle, progress)
			audio.Flush(world)
			eventDrops += audio.LastTick.EventDrops
			require.Empty(t, audio.events)
		}
		if enabled {
			require.Positive(t, eventDrops)
		}
		stats, _ := ecs.GetComponent[components.EntityStats](world, player)
		return result{stats.Stamina, handler.startCount, progress.states, progress.progress, progress.finished}
	}
	enabled, disabled := run(true), run(false)
	require.Equal(t, disabled, enabled)
	require.Equal(t, 66.0, enabled.stamina)
	require.Equal(t, 2, enabled.effects)
	require.Len(t, enabled.progress, 40)
}
