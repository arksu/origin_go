package cyclicaction

import (
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"origin/internal/actionanimationdefs"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
	"os"
	"testing"
	"time"
)

func fixture(t *testing.T) (*ecs.World, types.Handle, []actionanimationdefs.Definition) {
	t.Helper()
	contents, err := os.ReadFile("../../tests/fixtures/action_animations/bindings.json")
	require.NoError(t, err)
	definitions, err := actionanimationdefs.Parse(contents, "fixture.json")
	require.NoError(t, err)
	registry, err := actionanimationdefs.NewRegistry(definitions)
	require.NoError(t, err)
	previous := actionanimationdefs.Global()
	actionanimationdefs.SetGlobalForTesting(registry)
	t.Cleanup(func() { actionanimationdefs.SetGlobalForTesting(previous) })
	w := ecs.NewWorldForTesting()
	handle := w.Spawn(101, nil)
	ecs.AddComponent(w, handle, components.Appearance{Resource: "player"})
	timing := ecs.GetResource[ecs.TimeState](w)
	timing.TickPeriod, timing.UnixMs = 100*time.Millisecond, 10000
	return w, handle, definitions
}

func TestDataBindingsUseTheSameLifecycleAndActualCounters(t *testing.T) {
	w, handle, definitions := fixture(t)
	queue := ecs.GetResource[ecs.ActionAnimationDirtyQueue](w)
	idle, err := Snapshot(w, handle)
	require.NoError(t, err)
	require.Zero(t, idle.Revision)
	for index, definition := range definitions {
		cycle := components.ActiveCyclicAction{CycleDurationTicks: 20, CycleIndex: 1, StartedTick: uint64(index), HasTargetPosition: true, TargetX: 12, TargetY: 34}
		Start(w, handle, cycle, definition.Source)
		Start(w, handle, cycle, definition.Source)
		state, err := Snapshot(w, handle)
		require.NoError(t, err)
		require.Equal(t, definition.Key, state.AnimationKey)
		require.Equal(t, uint64(index*2+1), state.Revision)
		require.Equal(t, float64(2000), float64(state.TotalTicks)*state.TickDurationMs)
		require.Len(t, queue.Drain(0, nil), 1)
		ecs.WithComponent(w, handle, func(cycle *components.ActiveCyclicAction) { cycle.CycleElapsedTicks = 7 })
		state, err = Snapshot(w, handle)
		require.NoError(t, err)
		require.Equal(t, uint32(7), state.ElapsedTicks)
		require.Zero(t, queue.PendingCount())
		if definition.Facing == "target" {
			require.Equal(t, int32(12), state.TargetPosition.X)
		}
		Clear(w, handle)
		Clear(w, handle)
		idle, err = Snapshot(w, handle)
		require.NoError(t, err)
		require.Empty(t, idle.AnimationKey)
		require.Equal(t, uint64(index*2+2), idle.Revision)
		queue.Drain(0, nil)
	}
}

func TestSuccessorReplacementAndIncarnation(t *testing.T) {
	w, handle, definitions := fixture(t)
	cycle := components.ActiveCyclicAction{CycleDurationTicks: 20, CycleIndex: 1}
	Start(w, handle, cycle, definitions[0].Source)
	first, err := Snapshot(w, handle)
	require.NoError(t, err)
	ecs.WithComponent(w, handle, func(cycle *components.ActiveCyclicAction) {
		cycle.CycleElapsedTicks = 0
		cycle.CycleIndex++
		cycle.StartedTick++
	})
	Continue(w, handle)
	Continue(w, handle)
	ecs.GetResource[ecs.TimeState](w).TickPeriod = 16666667 * time.Nanosecond
	next, err := Snapshot(w, handle)
	require.NoError(t, err)
	require.Equal(t, first.Revision+1, next.Revision)
	require.InDelta(t, 16.666667, next.TickDurationMs, 1e-9)
	Start(w, handle, cycle, actionanimationdefs.Source{Kind: "menu", ID: "unmapped"})
	idle, err := Snapshot(w, handle)
	require.NoError(t, err)
	require.Equal(t, next.Revision+1, idle.Revision)
	require.Empty(t, idle.AnimationKey)
	w.Despawn(handle)
	state, err := Snapshot(w, handle)
	require.NoError(t, err)
	require.Nil(t, state)
	recreated := w.Spawn(101, nil)
	ecs.AddComponent(w, recreated, components.Appearance{Resource: "player"})
	fresh, err := Snapshot(w, recreated)
	require.NoError(t, err)
	require.NotEqual(t, idle.Generation, fresh.Generation)
	require.Zero(t, fresh.Revision)
}

func TestPublicStateProtocolRoundTrips(t *testing.T) {
	for _, state := range []*netproto.CharacterActionAnimationState{
		{Generation: "-1:4294967297", Revision: 9007199254740993, AnimationKey: "opaque/key", TotalTicks: 20, ElapsedTicks: 7, TickDurationMs: 100.25, ServerTimeMs: 1700000000000, TargetPosition: &netproto.Position{X: 12, Y: 34}},
		{Generation: "-1:4294967297", Revision: 9007199254740994, ServerTimeMs: 1700000000020},
	} {
		message := &netproto.ServerMessage{Payload: &netproto.ServerMessage_CharacterActionAnimation{CharacterActionAnimation: &netproto.S2C_CharacterActionAnimation{EntityId: 101, StreamEpoch: 7, State: state}}}
		encoded, err := proto.Marshal(message)
		require.NoError(t, err)
		decoded := &netproto.ServerMessage{}
		require.NoError(t, proto.Unmarshal(encoded, decoded))
		require.True(t, proto.Equal(message, decoded))
		spawn := &netproto.S2C_ObjectSpawn{EntityId: 101, ActionAnimation: state}
		encoded, err = proto.Marshal(spawn)
		require.NoError(t, err)
		decodedSpawn := &netproto.S2C_ObjectSpawn{}
		require.NoError(t, proto.Unmarshal(encoded, decodedSpawn))
		require.True(t, proto.Equal(spawn, decodedSpawn))
	}
}

func TestSoundCueCursorBelongsToInstalledCycle(t *testing.T) {
	world, handle, definitions := fixture(t)
	binding, exists := actionanimationdefs.Global().Resolve(definitions[0].Source)
	require.True(t, exists)
	binding.WorldSoundCues = []actionanimationdefs.SoundCue{{ID: "impact", Phase: .6, SoundKey: "chop", Source: "actor"}}
	installation := components.ActiveCyclicAction{CycleDurationTicks: 20, CycleIndex: 1}
	Start(world, handle, installation, binding.Source)
	cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, handle)
	require.True(t, exists)
	require.Same(t, binding, cycle.SoundBinding)
	ecs.WithComponent(world, handle, func(current *components.ActiveCyclicAction) {
		current.CycleElapsedTicks = 12
		current.NextSoundCue = 1
	})
	cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](world, handle)
	before, err := Snapshot(world, handle)
	require.NoError(t, err)
	Start(world, handle, cycle, binding.Source)
	after, err := Snapshot(world, handle)
	require.NoError(t, err)
	require.Equal(t, before.Revision, after.Revision)
	cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](world, handle)
	require.Equal(t, 1, cycle.NextSoundCue, "duplicate installation must retain consumed cues")
	Start(world, handle, installation, binding.Source)
	cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](world, handle)
	require.Equal(t, uint32(12), cycle.CycleElapsedTicks, "stale installation must retain current progress")
	require.Equal(t, 1, cycle.NextSoundCue)
	stillSame, err := Snapshot(world, handle)
	require.NoError(t, err)
	require.Equal(t, before.Revision, stillSame.Revision)
	ecs.WithComponent(world, handle, func(current *components.ActiveCyclicAction) {
		current.CycleIndex++
		current.CycleElapsedTicks = 0
		current.StartedTick++
	})
	Continue(world, handle)
	cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](world, handle)
	require.Zero(t, cycle.NextSoundCue)
	ecs.WithComponent(world, handle, func(current *components.ActiveCyclicAction) { current.NextSoundCue = 1 })
	Continue(world, handle)
	cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](world, handle)
	require.Equal(t, 1, cycle.NextSoundCue, "duplicate continuation must not re-arm markers")
}
