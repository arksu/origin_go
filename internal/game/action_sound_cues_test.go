package game

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"origin/internal/actionanimationdefs"
	"origin/internal/actiondefs"
	"origin/internal/config"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/behaviors/contracts"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/sounddefs"
	"origin/internal/types"
)

type actionCueSoundSender struct {
	messages map[types.EntityID][]*netproto.S2C_Sound
}

func (sender *actionCueSoundSender) SendSoundBatch(entityID types.EntityID, _ uint64, batch *netproto.S2C_SoundBatch) soundBatchDelivery {
	for _, sound := range batch.Sounds {
		sender.messages[entityID] = append(sender.messages[entityID], proto.Clone(sound).(*netproto.S2C_Sound))
	}
	return soundBatchDelivery{result: network.AudioSendAccepted, bytes: proto.Size(batch)}
}

func actionCueAudio(t *testing.T) (*SoundEventService, *actionCueSoundSender) {
	t.Helper()
	profiles, err := sounddefs.LoadFromDirectory("../../data/sounds", zap.NewNop())
	require.NoError(t, err)
	sender := &actionCueSoundSender{messages: make(map[types.EntityID][]*netproto.S2C_Sound)}
	service, err := NewSoundEventService(profiles, config.DefaultAudioConfig(), sender)
	require.NoError(t, err)
	return service, sender
}

func installActionCueBinding(t *testing.T, definition actionanimationdefs.Definition, profiles *sounddefs.Registry, source string) *actionanimationdefs.Definition {
	t.Helper()
	definition.SoundCues = []actionanimationdefs.SoundCue{
		{ID: "impact", Phase: 0.6, SoundKey: "chop", Source: source},
		{ID: "quiet", Phase: 0.8, SoundKey: "footstep", Source: "actor"},
	}
	registry, err := actionanimationdefs.NewRegistry([]actionanimationdefs.Definition{definition})
	require.NoError(t, err)
	require.NoError(t, registry.PrepareSoundCues(profiles))
	actionanimationdefs.SetGlobalForTesting(registry)
	binding, exists := registry.Get(definition.Key)
	require.True(t, exists)
	return binding
}

type actionCueBehavior struct {
	key        string
	decision   contracts.BehaviorCycleDecision
	effects    int
	onComplete func(*contracts.BehaviorCycleContext)
}

func (behavior *actionCueBehavior) Key() string { return behavior.key }
func (behavior *actionCueBehavior) OnCycleComplete(ctx *contracts.BehaviorCycleContext) contracts.BehaviorCycleDecision {
	behavior.effects++
	if behavior.onComplete != nil {
		behavior.onComplete(ctx)
	}
	return behavior.decision
}

func contextCueFixture(t *testing.T, duration uint32, decision contracts.BehaviorCycleDecision) (*ecs.World, types.Handle, types.Handle, *ContextActionService, *CyclicActionSystem, *SoundEventService, *actionCueSoundSender, *actionCueBehavior) {
	t.Helper()
	world, player, bindings := animationFixture(t)
	audio, sender := actionCueAudio(t)
	binding := installActionCueBinding(t, bindings[0], audio.profiles, "target")
	target := world.Spawn(202, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 200})
	})
	observer := world.Spawn(303, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 900, Y: 200})
	})
	require.True(t, audio.Attach(world, player, 1, 1))
	require.True(t, audio.Attach(world, observer, 2, 1))
	behavior := &actionCueBehavior{key: binding.Source.Namespace, decision: decision}
	service := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil, testSingleBehaviorRegistry{behavior: behavior}, nil)
	service.SetSoundEventService(audio)
	cyclicaction.StartContext(world, player, components.ActiveCyclicAction{
		BehaviorKey: binding.Source.Namespace, ActionID: binding.Source.ID,
		TargetKind: components.CyclicActionTargetObject, TargetID: 202, TargetHandle: target,
		CycleDurationTicks: duration, CycleIndex: 1,
	})
	ecs.GetResource[ecs.LinkState](world).SetLink(ecs.PlayerLink{PlayerID: 101, TargetID: 202})
	return world, player, target, service, NewCyclicActionSystem(service, nil, nil), audio, sender, behavior
}

func advanceCueTick(world *ecs.World, system *CyclicActionSystem, audio *SoundEventService) {
	timing := ecs.GetResource[ecs.TimeState](world)
	timing.Tick++
	timing.UnixMs += 100
	system.Update(world, .1)
	audio.Flush(world)
}

func TestActionSoundCueCrossesNormalizedThreshold(t *testing.T) {
	for _, test := range []struct {
		phase    float64
		duration uint32
		impact   uint32
	}{{0.6, 20, 12}, {0.6, 13, 8}, {0.14, 50, 7}, {1, 20, 20}} {
		cycle := components.ActiveCyclicAction{
			CycleDurationTicks: test.duration,
			SoundBinding: &actionanimationdefs.Definition{WorldSoundCues: []actionanimationdefs.SoundCue{
				{ID: "impact", Phase: test.phase, SoundKey: "chop", Source: "actor"},
			}},
		}
		cycle.CycleElapsedTicks = test.impact - 1
		require.False(t, actionSoundCueDue(cycle))
		cycle.CycleElapsedTicks = test.impact
		require.True(t, actionSoundCueDue(cycle))
	}
}

func TestContextSoundMarkerUsesAuthoredPhaseAndResetsEachCycle(t *testing.T) {
	for _, test := range []struct{ duration, impact uint32 }{{20, 12}, {13, 8}} {
		world, player, _, _, system, audio, sender, behavior := contextCueFixture(t, test.duration, contracts.BehaviorCycleDecisionContinue)
		for tick := uint32(1); tick <= test.duration*2; tick++ {
			advanceCueTick(world, system, audio)
			want := 0
			if tick >= test.impact {
				want++
			}
			if tick >= test.duration+test.impact {
				want++
			}
			require.Len(t, sender.messages[101], want)
			require.Len(t, sender.messages[303], want, "observer outside ordinary vision must hear the marker")
		}
		require.Equal(t, 2, behavior.effects)
		cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		require.True(t, exists)
		require.Equal(t, uint32(3), cycle.CycleIndex)
		require.Zero(t, cycle.NextSoundCue)
		for _, sound := range sender.messages[303] {
			require.Equal(t, "chop", sound.SoundKey, "local cues must stay off the server")
			require.Equal(t, float64(100), sound.X)
			require.Equal(t, float64(200), sound.Y)
		}
	}
}

func TestContextSoundCancellationBeforeAndAfterMarker(t *testing.T) {
	for _, cancelTick := range []int{11, 12} {
		world, player, _, service, system, audio, sender, behavior := contextCueFixture(t, 20, contracts.BehaviorCycleDecisionContinue)
		for tick := 1; tick < cancelTick; tick++ {
			advanceCueTick(world, system, audio)
		}
		system.Update(world, .1)
		service.cancelActiveCyclicAction(101, player, "manual_movement")
		audio.Flush(world)
		advanceCueTick(world, system, audio)
		want := 0
		if cancelTick == 12 {
			want = 1
		}
		require.Len(t, sender.messages[101], want)
		require.Zero(t, behavior.effects)
	}
}

func TestCompletionSoundUsesPointCapturedBeforeTargetAndCycleDisappear(t *testing.T) {
	for _, decision := range []contracts.BehaviorCycleDecision{contracts.BehaviorCycleDecisionComplete, contracts.BehaviorCycleDecisionCanceled} {
		world, player, target, _, system, audio, sender, behavior := contextCueFixture(t, 20, decision)
		ecs.WithComponent(world, player, func(cycle *components.ActiveCyclicAction) { cycle.CompleteSoundKey = "tree_fall" })
		behavior.onComplete = func(ctx *contracts.BehaviorCycleContext) {
			ctx.World.Despawn(ctx.TargetHandle)
			cyclicaction.Clear(ctx.World, ctx.PlayerHandle)
		}
		for range 20 {
			advanceCueTick(world, system, audio)
		}
		require.False(t, world.Alive(target))
		want := 1
		if decision == contracts.BehaviorCycleDecisionComplete {
			want = 2
		}
		require.Len(t, sender.messages[303], want)
		require.Equal(t, "chop", sender.messages[303][0].SoundKey)
		if want == 2 {
			fall := sender.messages[303][1]
			require.Equal(t, "tree_fall", fall.SoundKey)
			require.Equal(t, float64(100), fall.X)
			require.Equal(t, float64(200), fall.Y)
		}
	}
}

func TestWorldMarkerCannotReplayAfterDuplicateOrBackwardProgress(t *testing.T) {
	world, player, target, _, _, audio, sender, _ := contextCueFixture(t, 20, contracts.BehaviorCycleDecisionContinue)
	cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	cycle.CycleElapsedTicks = 12
	advanceActionSoundCues(world, player, &cycle, audio)
	advanceActionSoundCues(world, player, &cycle, audio)
	cycle.CycleElapsedTicks = 11
	advanceActionSoundCues(world, player, &cycle, audio)
	cycle.CycleElapsedTicks = 12
	advanceActionSoundCues(world, player, &cycle, audio)
	audio.Flush(world)
	require.Len(t, sender.messages[101], 1)
	world.Despawn(target)
	world.Spawn(202, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{X: 999}) })
	cycle.NextSoundCue = 0
	advanceActionSoundCues(world, player, &cycle, audio)
	audio.Flush(world)
	require.Len(t, sender.messages[101], 1, "a stale target handle must not emit at its reused entity ID")
}

func menuCueFixture(t *testing.T, duration int, repeat bool) (*ecs.World, types.Handle, *ActionService, *testActionHandler, *testActionSender, *SoundEventService, *actionCueSoundSender) {
	t.Helper()
	world, player, bindings := animationFixture(t)
	audio, sounds := actionCueAudio(t)
	binding := bindings[0]
	binding.Source = actionanimationdefs.Source{Kind: "menu", ID: "cue_menu"}
	installActionCueBinding(t, binding, audio.profiles, "actor")
	require.True(t, audio.Attach(world, player, 1, 1))
	ecs.AddComponent(world, player, components.EntityStats{Stamina: 100})
	targetKind := actiondefs.TargetNone
	if repeat {
		targetKind = actiondefs.TargetTile
	}
	definitions := actiondefs.NewRegistry([]actiondefs.Definition{{
		ID: "cue_menu", Target: actiondefs.Target{Kind: targetKind},
		Execution: actiondefs.Execution{Ticks: duration, Stamina: 17, Repeat: repeat},
	}})
	handler, sender := &testActionHandler{status: ActionSucceeded}, &testActionSender{}
	service, err := NewActionService(world, definitions, map[string]ActionHandler{"cue_menu": handler}, sender)
	require.NoError(t, err)
	service.SetSoundEventService(audio)
	service.Activate(world, 101, player, "cue_menu")
	if repeat {
		service.HandleArmedClick(world, 101, player, 0, 0, 6, 6)
	}
	return world, player, service, handler, sender, audio, sounds
}

func TestMenuWorldMarkerKeepsEffectsAndCostsAtCompletion(t *testing.T) {
	for _, test := range []struct{ duration, impact int }{{20, 12}, {13, 8}} {
		world, player, service, handler, progress, audio, sounds := menuCueFixture(t, test.duration, false)
		for tick := 1; tick <= test.duration; tick++ {
			cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
			require.True(t, exists)
			service.AdvanceCycle(world, 101, player, cycle, progress)
			service.AdvanceCycle(world, 101, player, cycle, progress)
			audio.Flush(world)
			want := 0
			if tick >= test.impact {
				want = 1
			}
			require.Len(t, sounds.messages[101], want)
			if tick < test.duration {
				stats, _ := ecs.GetComponent[components.EntityStats](world, player)
				require.Equal(t, float64(100), stats.Stamina)
				require.Zero(t, handler.startCount)
			}
		}
		stats, _ := ecs.GetComponent[components.EntityStats](world, player)
		require.Equal(t, float64(83), stats.Stamina)
		require.Equal(t, 1, handler.startCount)
		require.Len(t, progress.progress, test.duration)
	}
}

func TestMenuCueRepeatResetAndInvalidTargetRejection(t *testing.T) {
	world, player, service, handler, progress, audio, sounds := menuCueFixture(t, 20, true)
	for tick := 1; tick <= 40; tick++ {
		cycle, exists := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		require.True(t, exists)
		service.AdvanceCycle(world, 101, player, cycle, progress)
		audio.Flush(world)
	}
	require.Len(t, sounds.messages[101], 2)
	require.Equal(t, 2, handler.startCount)
	cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	require.Zero(t, cycle.NextSoundCue)
	for range 11 {
		cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](world, player)
		service.AdvanceCycle(world, 101, player, cycle, progress)
	}
	handler.targetReason = func(ActionTarget) string { return "BAD_TARGET" }
	cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](world, player)
	service.AdvanceCycle(world, 101, player, cycle, progress)
	audio.Flush(world)
	require.Len(t, sounds.messages[101], 2, "invalid target must not emit a marker")
	require.Equal(t, 2, handler.startCount)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	require.Equal(t, float64(66), stats.Stamina)
}

func TestMenuCueReplacementRejectsOldCycle(t *testing.T) {
	world, player, service, handler, progress, audio, sounds := menuCueFixture(t, 20, false)
	for range 11 {
		cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		service.AdvanceCycle(world, 101, player, cycle, progress)
	}
	oldCycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	service.Activate(world, 101, player, "cue_menu")
	newCycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	require.NotEqual(t, oldCycle.ActionGeneration, newCycle.ActionGeneration)
	service.AdvanceCycle(world, 101, player, oldCycle, progress)
	audio.Flush(world)
	require.Empty(t, sounds.messages[101])
	unchangedCycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
	require.Equal(t, newCycle, unchangedCycle)
	for range 12 {
		cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		service.AdvanceCycle(world, 101, player, cycle, progress)
	}
	service.Cancel(world, 101, player)
	audio.Flush(world)
	require.Len(t, sounds.messages[101], 1, "the completed contact survives later cancellation")
	require.Zero(t, handler.startCount)
	stats, _ := ecs.GetComponent[components.EntityStats](world, player)
	require.Equal(t, float64(100), stats.Stamina)
}

func TestWorldCueHelperSkipsUnmappedAndLocalOnlyBindings(t *testing.T) {
	world, player, _, _, _, audio, sounds, _ := contextCueFixture(t, 20, contracts.BehaviorCycleDecisionContinue)
	for _, binding := range []*actionanimationdefs.Definition{
		nil,
		{LocalSoundCues: []actionanimationdefs.SoundCue{{ID: "quiet", Phase: 0.6, SoundKey: "footstep", Source: "actor"}}},
	} {
		cycle := components.ActiveCyclicAction{SoundBinding: binding, CycleDurationTicks: 20, CycleElapsedTicks: 20}
		advanceActionSoundCues(world, player, &cycle, audio)
		require.Zero(t, cycle.NextSoundCue)
	}
	audio.Flush(world)
	require.Empty(t, sounds.messages)
}
