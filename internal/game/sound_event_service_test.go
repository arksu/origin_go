package game

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/sounddefs"
	"origin/internal/types"
)

type testSoundSender struct {
	messages map[types.EntityID][]*netproto.S2C_Sound
	packets  [][]byte
	result   network.AudioSendResult
}

func (sender *testSoundSender) SendSoundBatch(entityID types.EntityID, _ uint64, batch *netproto.S2C_SoundBatch) soundBatchDelivery {
	encoded, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_SoundBatch{SoundBatch: batch}})
	if err != nil {
		panic(err)
	}
	if sender.result != network.AudioSendAccepted {
		return soundBatchDelivery{result: sender.result}
	}
	if sender.messages == nil {
		sender.messages = make(map[types.EntityID][]*netproto.S2C_Sound)
	}
	sender.packets = append(sender.packets, encoded)
	for _, sound := range batch.Sounds {
		sender.messages[entityID] = append(sender.messages[entityID], proto.Clone(sound).(*netproto.S2C_Sound))
	}
	return soundBatchDelivery{result: sender.result, bytes: len(encoded)}
}

func soundTestProfiles(testingContext testing.TB) *sounddefs.Registry {
	testingContext.Helper()
	registry, err := sounddefs.NewRegistry([]sounddefs.Profile{
		{Key: "chop", Mode: "world", Loudness: 1000, Volume: .9, Files: []string{"sound/chop/chop_01.mp3"}, Priority: 10, MaxVoices: 32, MaxVoicesPerSource: 4},
		{Key: "tree_fall", Mode: "world", Loudness: 1400, Volume: .95, Files: []string{"sound/tree_fall/tree_fall_01.mp3"}, Priority: 20, MaxVoices: 32, MaxVoicesPerSource: 4},
		{Key: "steps", Mode: "local", Loudness: 120, Volume: .3, Files: []string{"sound/steps/step_01.wav"}, Priority: 0, MaxVoices: 32, MaxVoicesPerSource: 4, LocalAttenuation: &sounddefs.LocalAttenuation{NearGain: .9, FarGain: .8, Shape: 4}},
	})
	require.NoError(testingContext, err)
	return registry
}

func newSoundTestService(testingContext testing.TB, audio config.AudioConfig) (*ecs.World, *SoundEventService, *testSoundSender) {
	testingContext.Helper()
	world := ecs.NewWorldWithCapacity(30000, nil, 0)
	ecs.GetResource[ecs.TimeState](world).UnixMs = 1000
	sender := &testSoundSender{}
	service, err := NewSoundEventService(soundTestProfiles(testingContext), audio, sender)
	require.NoError(testingContext, err)
	return world, service, sender
}

func addSoundTestListener(testingContext testing.TB, world *ecs.World, service *SoundEventService, id types.EntityID, positionX, positionY float64) types.Handle {
	testingContext.Helper()
	handle := world.Spawn(id, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: positionX, Y: positionY})
	})
	require.True(testingContext, service.Attach(world, handle, uint64(id), 1))
	return handle
}

func TestWorldSoundOutsideVisibilityAndStrictHearingBoundary(t *testing.T) {
	world, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	addSoundTestListener(t, world, service, 1, 800, 0)
	addSoundTestListener(t, world, service, 2, 1000, 0)
	addSoundTestListener(t, world, service, 3, 1001, 0)
	addSoundTestListener(t, world, service, 4, 500, 0)
	require.Empty(t, ecs.GetResource[ecs.VisibilityState](world).ObserversByVisibleTarget)
	require.True(t, service.EmitPoint(world, soundPoint{}, "chop"))
	service.Flush(world)
	require.Len(t, sender.messages[1], 1)
	require.Empty(t, sender.messages[2])
	require.Empty(t, sender.messages[3])
	require.InDelta(t, .5, sender.messages[4][0].GetDistanceGain(), 1e-6)
	require.InDelta(t, .104, sender.messages[1][0].GetDistanceGain(), 1e-6)
	require.Equal(t, uint64(2), service.LastTick.Recipients)
}

func TestWorldSoundHearingScalingAndSourceLifetime(t *testing.T) {
	audio := config.DefaultAudioConfig()
	audio.BaseHearing = 1.5
	world, service, sender := newSoundTestService(t, audio)
	addSoundTestListener(t, world, service, 1, 1200, 0)
	source := world.Spawn(10, nil)
	service.EmitPoint(world, soundPoint{}, "chop")
	world.Despawn(source)
	service.Flush(world)
	require.Len(t, sender.messages[1], 1)
	require.Equal(t, 1500.0, sender.messages[1][0].MaxHearDistance)
	require.Equal(t, 1.5, service.EffectiveHearing(0))
}

func TestSoundListenerIndexMembershipAndNegativeCells(t *testing.T) {
	world, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	first := addSoundTestListener(t, world, service, 1, -.1, -256.1)
	second := addSoundTestListener(t, world, service, 2, -2, -270)
	third := addSoundTestListener(t, world, service, 3, -3, -280)
	cell := soundCell{-1, -2}
	require.Equal(t, cell, service.listeners.members[first].cell)
	service.Detach(second, 2)
	require.Len(t, service.listeners.cells[cell], 2)
	require.Equal(t, 1, service.listeners.members[third].slot)
	service.OnPositionCommitted(first, -5, -300)
	require.Equal(t, 0, service.listeners.members[first].slot)
	ecs.WithComponent(world, first, func(position *components.Transform) { position.X = 4000; position.Y = 0 })
	service.OnPositionCommitted(first, 4000, 0)
	service.Detach(third, 3)
	require.NotContains(t, service.listeners.cells, cell)
	service.EmitPoint(world, soundPoint{}, "chop")
	service.Flush(world)
	require.Empty(t, sender.messages)
	service.Detach(first, 999)
	require.Contains(t, service.listeners.members, first, "stale connection cannot detach its replacement")
	service.Detach(first, 1)
	require.Empty(t, service.listeners.cells)
}

func TestWorldSoundUsesCollisionCommitBeforeVisibilityGuard(t *testing.T) {
	world, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	listener := addSoundTestListener(t, world, service, 1, 1800, 0)
	ecs.AddComponent(world, listener, components.CollisionResult{FinalX: 500, FinalY: 0})
	moved := ecs.GetResource[ecs.MovedEntities](world)
	moved.Add(listener, 500, 0)
	transform := systems.NewTransformUpdateSystem(world, nil, nil, zap.NewNop())
	transform.SetPositionObserver(service)
	transform.Update(world, .1)
	require.Equal(t, soundCell{1, 0}, service.listeners.members[listener].cell)
	service.EmitPoint(world, soundPoint{}, "chop")
	service.Flush(world)
	require.Len(t, sender.messages[1], 1)
	require.InDelta(t, .5, sender.messages[1][0].GetDistanceGain(), 1e-6)
}

func TestSoundDetachTransferRollbackAndReusedHandle(t *testing.T) {
	world, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	listener := addSoundTestListener(t, world, service, 1, 0, 0)
	service.Detach(listener, 1)
	service.EmitPoint(world, soundPoint{}, "chop")
	service.Flush(world)
	require.Empty(t, sender.messages)
	world.Despawn(listener)
	replacement := world.Spawn(1, func(world *ecs.World, handle types.Handle) {
		ecs.AddComponent(world, handle, components.Transform{X: 3000})
	})
	require.NotEqual(t, listener, replacement)
	require.True(t, service.Attach(world, replacement, 2, 2))
	service.Detach(replacement, 1)
	require.Contains(t, service.listeners.members, replacement)
	ecs.WithComponent(world, replacement, func(position *components.Transform) { position.X = 100 })
	service.OnPositionCommitted(replacement, 100, 0)
	service.EmitPoint(world, soundPoint{}, "chop")
	service.Flush(world)
	require.Len(t, sender.messages[1], 1)
	var wire netproto.ServerMessage
	require.NoError(t, proto.Unmarshal(sender.packets[0], &wire))
	require.Equal(t, uint32(2), wire.GetSoundBatch().StreamEpoch)
}

func TestWorldSoundRejectsForeignLayerStaleAndLocalEvents(t *testing.T) {
	world, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	addSoundTestListener(t, world, service, 1, 0, 0)
	foreign := ecs.NewWorldWithCapacity(8, nil, 1)
	service.EmitPoint(foreign, soundPoint{}, "chop")
	service.EmitPoint(world, soundPoint{}, "chop")
	require.False(t, service.EmitPoint(world, soundPoint{}, "steps"))
	require.False(t, service.EmitPoint(world, soundPoint{math.NaN(), 0}, "chop"))
	ecs.GetResource[ecs.TimeState](world).UnixMs = 1501
	service.Flush(world)
	require.Empty(t, sender.messages)
	require.Equal(t, uint64(2), service.LastTick.StaleEvents)
	require.Equal(t, uint64(1), service.LastTick.InvalidEvents)
	require.Empty(t, service.events)
	service.Flush(world)
	require.Zero(t, service.LastTick.Events)
}

func TestSoundPriorityEntryAndByteBudgets(t *testing.T) {
	audio := config.DefaultAudioConfig()
	audio.MaxEventsPerTick = 2
	audio.MaxEntriesPerBatch = 1
	world, service, sender := newSoundTestService(t, audio)
	addSoundTestListener(t, world, service, 1, 0, 0)
	service.EmitPoint(world, soundPoint{1, 0}, "chop")
	service.EmitPoint(world, soundPoint{2, 0}, "chop")
	service.EmitPoint(world, soundPoint{3, 0}, "tree_fall")
	service.EmitPoint(world, soundPoint{4, 0}, "chop")
	service.Flush(world)
	require.Len(t, sender.messages[1], 1)
	require.Equal(t, "tree_fall", sender.messages[1][0].SoundKey)
	require.Equal(t, uint64(2), service.LastTick.EventDrops)
	require.Equal(t, uint64(1), service.LastTick.EntryDrops)

	audio.MaxBatchBytes = 8
	world, service, sender = newSoundTestService(t, audio)
	addSoundTestListener(t, world, service, 1, 0, 0)
	service.EmitPoint(world, soundPoint{}, "chop")
	service.Flush(world)
	require.Empty(t, sender.messages)
	require.Equal(t, uint64(1), service.LastTick.ByteDrops)
}

func TestSoundGlobalWorkLimitsStopDenseQueries(t *testing.T) {
	for _, limit := range []string{"cells", "candidates", "entries", "full_batches"} {
		t.Run(limit, func(t *testing.T) {
			audio := config.DefaultAudioConfig()
			switch limit {
			case "cells":
				audio.MaxCellsPerTick = 2
			case "candidates":
				audio.MaxCandidatesPerTick = 5
			case "entries":
				audio.MaxEntriesPerTick = 3
			case "full_batches":
				audio.MaxCandidatesPerTick = 15
				audio.MaxEntriesPerBatch = 1
			}
			world, service, _ := newSoundTestService(t, audio)
			for listener := 1; listener <= 10; listener++ {
				addSoundTestListener(t, world, service, types.EntityID(listener), 0, 0)
			}
			for event := 0; event < 5; event++ {
				service.EmitPoint(world, soundPoint{}, "chop")
			}
			service.Flush(world)
			require.LessOrEqual(t, service.LastTick.Cells, uint64(audio.MaxCellsPerTick))
			require.LessOrEqual(t, service.LastTick.Candidates, uint64(audio.MaxCandidatesPerTick))
			require.LessOrEqual(t, service.LastTick.Entries, uint64(audio.MaxEntriesPerTick))
			require.Positive(t, service.LastTick.WorkDrops)
			require.Equal(t, uint64(1), service.LastTick.TruncatedQueries)
			require.Equal(t, uint64(1), service.LastTick.CellBudgetCutoffs+service.LastTick.CandidateBudgetCutoffs+service.LastTick.GlobalEntryBudgetCutoffs)
			switch limit {
			case "cells":
				require.Equal(t, uint64(1), service.LastTick.CellBudgetCutoffs)
			case "candidates", "full_batches":
				require.Equal(t, uint64(1), service.LastTick.CandidateBudgetCutoffs)
			case "entries":
				require.Equal(t, uint64(1), service.LastTick.GlobalEntryBudgetCutoffs)
			}
			require.Empty(t, service.events)
		})
	}
}

func TestSoundRemotePopulationDoesNotChangeRoutingWork(t *testing.T) {
	world, service, _ := newSoundTestService(t, config.DefaultAudioConfig())
	addSoundTestListener(t, world, service, 1, 10, 10)
	service.EmitPoint(world, soundPoint{}, "chop")
	service.Flush(world)
	before := service.LastTick
	for remote := 2; remote <= 1002; remote++ {
		addSoundTestListener(t, world, service, types.EntityID(remote), 100000+float64(remote), 100000)
		world.Spawn(types.EntityID(remote+10000), nil)
	}
	service.EmitPoint(world, soundPoint{}, "chop")
	service.Flush(world)
	require.Equal(t, before.Cells, service.LastTick.Cells)
	require.Equal(t, before.Candidates, service.LastTick.Candidates)
	require.Equal(t, before.Recipients, service.LastTick.Recipients)
}

func TestSoundQueuedPacketsOwnBytesAcrossNextTick(t *testing.T) {
	world, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	addSoundTestListener(t, world, service, 1, 0, 0)
	service.EmitPoint(world, soundPoint{10, 20}, "chop")
	service.Flush(world)
	first := append([]byte(nil), sender.packets[0]...)
	ecs.GetResource[ecs.TimeState](world).UnixMs += 100
	service.EmitPoint(world, soundPoint{30, 40}, "tree_fall")
	service.Flush(world)
	require.Equal(t, first, sender.packets[0])
	require.NotEqual(t, sender.packets[0], sender.packets[1])
	for _, encoded := range sender.packets {
		require.LessOrEqual(t, len(encoded), service.config.MaxBatchBytes)
	}
}

func TestSoundTransportDropsAndLocalOnlyWorkload(t *testing.T) {
	world, service, sender := newSoundTestService(t, config.DefaultAudioConfig())
	addSoundTestListener(t, world, service, 1, 0, 0)
	for step := 0; step < 10000; step++ {
		require.False(t, service.EmitPoint(world, soundPoint{}, "steps"))
	}
	service.Flush(world)
	require.Zero(t, service.LastTick.Events)
	require.Zero(t, service.LastTick.Cells)
	require.Empty(t, sender.messages)
	for _, result := range []network.AudioSendResult{network.AudioSendFull, network.AudioSendClosed} {
		sender.result = result
		service.EmitPoint(world, soundPoint{}, "chop")
		service.Flush(world)
		require.Equal(t, uint64(1), service.LastTick.TransportDrops+service.LastTick.Disconnected)
	}
}

type benchmarkSoundSender struct{}

func (benchmarkSoundSender) SendSoundBatch(_ types.EntityID, _ uint64, batch *netproto.S2C_SoundBatch) soundBatchDelivery {
	started := time.Now()
	encoded, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_SoundBatch{SoundBatch: batch}})
	if err != nil {
		panic(err)
	}
	return soundBatchDelivery{result: network.AudioSendAccepted, bytes: len(encoded), encodeTime: time.Since(started)}
}

func BenchmarkWorldSoundRouting(b *testing.B) {
	for _, workload := range []struct {
		name                   string
		nearby, remote, events int
		move                   bool
	}{
		{"sparse", 8, 0, 1, false}, {"remote1000", 8, 1000, 1, false}, {"remote10000", 8, 10000, 1, false},
		{"objects10000", 8, 0, 1, false}, {"dense", 1000, 0, 16, false}, {"overload", 2000, 0, 128, false}, {"moving", 128, 0, 4, true},
		{"event_overload", 8, 0, 2048, false}, {"cell_overload", 0, 0, 1024, false}, {"full_batches", 1000, 0, 128, false},
	} {
		b.Run(workload.name, func(b *testing.B) {
			audio := config.DefaultAudioConfig()
			if workload.name == "full_batches" {
				audio.MaxEntriesPerBatch = 1
			}
			world, service, _ := newSoundTestService(b, audio)
			service.sender = benchmarkSoundSender{}
			if workload.name == "objects10000" {
				for object := 0; object < 10000; object++ {
					world.Spawn(types.EntityID(10000+object), nil)
				}
			}
			handles := make([]types.Handle, 0, workload.nearby)
			for listener := 1; listener <= workload.nearby; listener++ {
				handles = append(handles, addSoundTestListener(b, world, service, types.EntityID(listener), float64(listener%10)*10, 0))
			}
			for remote := 0; remote < workload.remote; remote++ {
				addSoundTestListener(b, world, service, types.EntityID(workload.nearby+remote+1), 100000+float64(remote), 100000)
			}
			runTick := func(iteration int) {
				if workload.move {
					for _, handle := range handles {
						positionX := float64((iteration % 2) * 300)
						ecs.WithComponent(world, handle, func(position *components.Transform) { position.X = positionX })
						service.OnPositionCommitted(handle, positionX, 0)
					}
				}
				for event := 0; event < workload.events; event++ {
					key := "chop"
					if workload.name == "cell_overload" {
						key = "tree_fall"
					}
					service.EmitPoint(world, soundPoint{}, key)
				}
				service.Flush(world)
			}
			for warmup := 0; warmup < 4; warmup++ {
				runTick(warmup)
			}
			b.ReportAllocs()
			var propagationTime, encodeTime time.Duration
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				runTick(iteration)
				propagationTime += service.LastTick.PropagationTime
				encodeTime += service.LastTick.EncodeTime
			}
			b.StopTimer()
			b.ReportMetric(float64(service.LastTick.Cells), "cells/tick")
			b.ReportMetric(float64(service.LastTick.Candidates), "candidates/tick")
			b.ReportMetric(float64(service.LastTick.Recipients), "recipients/tick")
			b.ReportMetric(float64(service.LastTick.Bytes), "bytes/tick")
			b.ReportMetric(float64(service.LastTick.Events), "events/tick")
			b.ReportMetric(float64(service.LastTick.Entries), "entries/tick")
			b.ReportMetric(float64(service.LastTick.EntryDrops), "entry-drops/tick")
			b.ReportMetric(float64(service.LastTick.Messages), "messages/tick")
			b.ReportMetric(float64(service.LastTick.WorkDrops+service.LastTick.EventDrops), "event-drops/tick")
			b.ReportMetric(float64(propagationTime.Nanoseconds())/float64(b.N), "propagation-ns/tick")
			b.ReportMetric(float64(encodeTime.Nanoseconds())/float64(b.N), "encoding-ns/tick")
			if service.LastTick.Cells > uint64(service.config.MaxCellsPerTick) || service.LastTick.Candidates > uint64(service.config.MaxCandidatesPerTick) || service.LastTick.Entries > uint64(service.config.MaxEntriesPerTick) {
				b.Fatal(fmt.Sprintf("work budget exceeded: %+v", service.LastTick))
			}
		})
	}
}
