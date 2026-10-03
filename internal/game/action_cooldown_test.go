package game

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/actionanimationdefs"
	"origin/internal/actiondefs"
	"origin/internal/config"
	"origin/internal/cyclicaction"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"
)

func TestActionCooldownAdmissionAndIndependence(t *testing.T) {
	w := ecs.NewWorldForTesting()
	clock := ecs.GetResource[ecs.TimeState](w)
	clock.UnixMs = 10000
	player := w.Spawn(1, nil)
	otherPlayer := w.Spawn(2, nil)
	registry := actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "first", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Cooldown: 2000},
		{ID: "second", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Cooldown: 500},
		{ID: "selected", Target: actiondefs.Target{Kind: actiondefs.TargetTile}},
	})
	first, second, selected := &testActionHandler{status: ActionSucceeded}, &testActionHandler{status: ActionSucceeded}, &testActionHandler{}
	sender := &testActionSender{}
	service, err := NewActionService(w, registry, map[string]ActionHandler{"first": first, "second": second, "selected": selected}, sender)
	require.NoError(t, err)
	service.Activate(w, 1, player, "first")
	require.Equal(t, int64(12000), service.State(w, player).Cooldowns[0].ExpiresAtMs)
	require.Equal(t, "idle", service.State(w, player).Phase)
	service.Activate(w, 1, player, "selected")
	before, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
	clock.UnixMs = 11999
	service.Activate(w, 1, player, "first")
	after, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
	require.Equal(t, before, after)
	require.Equal(t, "ACTION_ON_COOLDOWN", sender.alerts[len(sender.alerts)-1].ReasonCode)
	require.Equal(t, 1, first.startCount)
	service.StartTargetedOnce(w, 1, player, "first", 0, 0, 0, 0)
	after, _ = ecs.GetComponent[components.ActiveGameAction](w, player)
	require.Equal(t, before, after)
	service.Activate(w, 2, otherPlayer, "first")
	require.Equal(t, 2, first.startCount)
	service.Activate(w, 1, player, "second")
	require.Equal(t, 1, second.startCount)
	clock.UnixMs = 12000
	service.Activate(w, 1, player, "first")
	require.Equal(t, 3, first.startCount)
	require.Equal(t, int64(14000), service.State(w, player).Cooldowns[0].ExpiresAtMs)
}

func TestActionCooldownRejectedAndAcceptedOutcomes(t *testing.T) {
	for _, outcome := range []ActionOutcome{ActionRejected, ActionApproaching, ActionDeferred, ActionFailed, ActionSucceeded} {
		t.Run(map[ActionOutcome]string{ActionRejected: "rejected", ActionApproaching: "approaching", ActionDeferred: "deferred", ActionFailed: "failed", ActionSucceeded: "succeeded"}[outcome], func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			ecs.GetResource[ecs.TimeState](w).UnixMs = 10000
			player := w.Spawn(1, nil)
			definition := actiondefs.Definition{ID: "test", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Cooldown: 2000}
			handler, sender := &testActionHandler{status: outcome}, &testActionSender{}
			service, err := NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{definition}), map[string]ActionHandler{"test": handler}, sender)
			require.NoError(t, err)
			service.Activate(w, 1, player, "test")
			started := outcome == ActionSucceeded
			require.Equal(t, started, service.isOnCooldown(w, player, "test"))
			service.Cancel(w, 1, player)
			require.Equal(t, started, service.isOnCooldown(w, player, "test"))
		})
	}
}

func TestTimedCooldownStartsOnlyAfterSuccessfulCompletion(t *testing.T) {
	w, player, service, terrain, sender := newPlowTest(t, types.TileGrass, 1)
	clock := ecs.GetResource[ecs.TimeState](w)
	clock.UnixMs = 10000
	ecs.WithComponent(w, player, func(position *components.Transform) { position.X = 1 })
	service.HandleArmedClick(w, 1, player, 0, 0, 6, 6)
	require.Equal(t, "approaching", service.State(w, player).Phase)
	require.Empty(t, service.State(w, player).Cooldowns)
	active, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
	clock.UnixMs = 11000
	ecs.WithComponent(w, player, func(position *components.Transform) { position.X = 6 })
	definition, _ := service.definitions.Get("plow_tile")
	service.beginTileCycle(w, 1, player, definition, active, ActionTarget{X: 6, Y: 6})
	require.Empty(t, service.State(w, player).Cooldowns)
	cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	clock.UnixMs += 100
	service.AdvanceCycle(w, 1, player, cycle, sender)
	require.Equal(t, "executing", service.State(w, player).Phase)
	service.Cancel(w, 1, player)
	require.Empty(t, service.State(w, player).Cooldowns)
	service.AdvanceCycle(w, 1, player, cycle, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](w, player)
	require.Equal(t, float64(1000), stats.Stamina)
	require.Equal(t, byte(types.TileGrass), terrain.chunk.SnapshotTiles().Tiles[0])

	// A canceled attempt can be retried immediately, without advancing the clock.
	service.Activate(w, 1, player, "plow_tile")
	service.HandleArmedClick(w, 1, player, 0, 0, 6, 6)
	require.Equal(t, "executing", service.State(w, player).Phase)
	service.Complete(w, 1, player, active.Generation, true, "")
	for range 19 {
		clock.UnixMs += 100
		cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](w, player)
		service.AdvanceCycle(w, 1, player, cycle, sender)
		require.Empty(t, service.State(w, player).Cooldowns)
	}
	stats, _ = ecs.GetComponent[components.EntityStats](w, player)
	require.Equal(t, float64(1000), stats.Stamina)
	clock.UnixMs += 100
	cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](w, player)
	service.AdvanceCycle(w, 1, player, cycle, sender)
	stats, _ = ecs.GetComponent[components.EntityStats](w, player)
	require.Equal(t, float64(750), stats.Stamina)
	require.Equal(t, byte(types.TilePlowed), terrain.chunk.SnapshotTiles().Tiles[0])
	state := sender.states[len(sender.states)-1]
	require.Equal(t, "selecting", state.Phase)
	require.Len(t, state.Cooldowns, 1)
	require.Equal(t, clock.UnixMs, state.Cooldowns[0].StartedAtMs)
	require.Equal(t, clock.UnixMs+int64(definition.Cooldown), state.Cooldowns[0].ExpiresAtMs)
	service.Cancel(w, 1, player)
	require.True(t, service.isOnCooldown(w, player, "plow_tile"))
}

func TestDeferredCooldownRequiresSuccessfulCostCharge(t *testing.T) {
	for _, outcome := range []string{"success", "failed", "canceled", "unaffordable"} {
		t.Run(outcome, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			clock := ecs.GetResource[ecs.TimeState](w)
			clock.UnixMs = 10000
			player := w.Spawn(1, nil)
			ecs.AddComponent(w, player, components.EntityStats{Stamina: 1000})
			definition := actiondefs.Definition{ID: "test", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Execution: actiondefs.Execution{Stamina: 100}, Cooldown: 2000}
			handler, sender := &testActionHandler{status: ActionDeferred}, &testActionSender{}
			service, err := NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{definition}), map[string]ActionHandler{"test": handler}, sender)
			require.NoError(t, err)
			service.Activate(w, 1, player, "test")
			require.Empty(t, service.State(w, player).Cooldowns)
			active, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
			clock.UnixMs = 11000
			switch outcome {
			case "canceled":
				service.Cancel(w, 1, player)
			case "unaffordable":
				ecs.WithComponent(w, player, func(stats *components.EntityStats) { stats.Stamina = 0 })
			}
			service.Complete(w, 1, player, active.Generation, outcome != "failed", "")
			// Duplicate or late completion must not charge again or extend cooldown.
			clock.UnixMs++
			service.Complete(w, 1, player, active.Generation, true, "")
			state := service.State(w, player)
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			if outcome == "success" {
				require.Equal(t, float64(900), stats.Stamina)
				require.Len(t, state.Cooldowns, 1)
				require.Equal(t, int64(11000), state.Cooldowns[0].StartedAtMs)
				require.Equal(t, int64(13000), state.Cooldowns[0].ExpiresAtMs)
			} else {
				require.Empty(t, state.Cooldowns)
				if outcome == "unaffordable" {
					require.Zero(t, stats.Stamina)
					require.Equal(t, "LOW_STAMINA", sender.alerts[len(sender.alerts)-1].ReasonCode)
				} else {
					require.Equal(t, float64(1000), stats.Stamina)
				}
			}
		})
	}
}

func cooldownRepeatFixture(t *testing.T, duration uint32) (*ecs.World, types.Handle, *ActionService, *repeatTestHandler, *testActionSender) {
	t.Helper()
	w, player, service, handler, sender := newRepeatActionTest(t, true, false, 0)
	service.Cancel(w, 1, player)
	*sender = testActionSender{}
	definition, _ := service.definitions.Get("repeat_test")
	definition.Cooldown = duration
	clock := ecs.GetResource[ecs.TimeState](w)
	clock.UnixMs = 10000
	clock.TickPeriod = 100 * time.Millisecond
	service.Activate(w, 1, player, "repeat_test")
	service.HandleArmedClick(w, 1, player, 0, 0, 6, 6)
	return w, player, service, handler, sender
}

func TestRepeatedCooldownSpacesEachCycle(t *testing.T) {
	for _, duration := range []uint32{0, 100, 200, 500} {
		t.Run((time.Duration(duration) * time.Millisecond).String(), func(t *testing.T) {
			w, player, service, handler, sender := cooldownRepeatFixture(t, duration)
			clock := ecs.GetResource[ecs.TimeState](w)
			require.Empty(t, service.State(w, player).Cooldowns)
			for range 2 {
				clock.UnixMs += 100
				clock.Tick++
				cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
				service.AdvanceCycle(w, 1, player, cycle, sender)
			}
			require.Equal(t, 1, handler.effects)
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			require.Equal(t, float64(900), stats.Stamina)
			if duration > 0 {
				require.Equal(t, "cooldown_wait", service.State(w, player).Phase)
				state := service.State(w, player)
				require.Len(t, state.Cooldowns, 1)
				require.Equal(t, int64(10200), state.Cooldowns[0].StartedAtMs)
				require.Equal(t, int64(10200)+int64(duration), state.Cooldowns[0].ExpiresAtMs)
				staleCycle, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
				active, _ := ecs.GetComponent[components.ActiveGameAction](w, player)
				require.False(t, service.CanCommit(w, 1, player, active.Generation))
				service.Complete(w, 1, player, active.Generation, true, "")
				clock.UnixMs = state.Cooldowns[0].ExpiresAtMs - 1
				service.Recheck(w, 1, player)
				service.AdvanceCycle(w, 1, player, staleCycle, sender)
				require.Equal(t, 1, handler.effects)
				stats, _ = ecs.GetComponent[components.EntityStats](w, player)
				require.Equal(t, float64(900), stats.Stamina)
				require.Empty(t, sender.finished)
				clock.UnixMs++
				service.Recheck(w, 1, player)
				service.AdvanceCycle(w, 1, player, staleCycle, sender)
				require.Equal(t, 1, handler.effects)
			}
			require.Equal(t, "executing", service.State(w, player).Phase)
			cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
			require.Equal(t, uint32(2), cycle.CycleIndex)
			require.Zero(t, cycle.CycleElapsedTicks)
			require.Empty(t, service.State(w, player).Cooldowns)
			for range 2 {
				clock.UnixMs += 100
				clock.Tick++
				cycle, _ = ecs.GetComponent[components.ActiveCyclicAction](w, player)
				service.AdvanceCycle(w, 1, player, cycle, sender)
			}
			require.Equal(t, 2, handler.effects)
			stats, _ = ecs.GetComponent[components.EntityStats](w, player)
			require.Equal(t, float64(800), stats.Stamina)
			state := service.State(w, player)
			if duration > 0 {
				require.Equal(t, "cooldown_wait", state.Phase)
				require.Len(t, state.Cooldowns, 1)
				require.Equal(t, clock.UnixMs, state.Cooldowns[0].StartedAtMs)
				require.Equal(t, clock.UnixMs+int64(duration), state.Cooldowns[0].ExpiresAtMs)
			} else {
				require.Equal(t, "executing", state.Phase)
				require.Empty(t, state.Cooldowns)
			}
			require.Empty(t, sender.finished)
		})
	}
}

func TestTerminalRepeatedCycleRetainsCompletionCooldown(t *testing.T) {
	for _, stop := range []string{"handler", "stamina"} {
		t.Run(stop, func(t *testing.T) {
			w, player, service, handler, sender := cooldownRepeatFixture(t, 500)
			startingStamina := float64(1000)
			if stop == "handler" {
				handler.stopAt = 1
			} else {
				startingStamina = 100
				ecs.WithComponent(w, player, func(stats *components.EntityStats) { stats.Stamina = startingStamina })
			}
			ecs.GetResource[ecs.TimeState](w).UnixMs = 10200
			advanceRepeatTestCycle(t, w, player, service, sender)
			state := service.State(w, player)
			require.Equal(t, "idle", state.Phase)
			require.Equal(t, 1, handler.effects)
			stats, _ := ecs.GetComponent[components.EntityStats](w, player)
			require.Equal(t, startingStamina-100, stats.Stamina)
			require.Len(t, state.Cooldowns, 1)
			require.Equal(t, int64(10200), state.Cooldowns[0].StartedAtMs)
			require.Equal(t, int64(10700), state.Cooldowns[0].ExpiresAtMs)
			require.Len(t, sender.finished, 1)
			require.Equal(t, netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_COMPLETED, sender.finished[0].Result)
		})
	}
}

func TestCooldownWaitCancellationAndRevalidation(t *testing.T) {
	for _, stop := range []string{"cancel", "stamina", "position"} {
		t.Run(stop, func(t *testing.T) {
			w, player, service, handler, sender := cooldownRepeatFixture(t, 500)
			advanceRepeatTestCycle(t, w, player, service, sender)
			require.Equal(t, "cooldown_wait", service.State(w, player).Phase)
			switch stop {
			case "cancel":
				service.Cancel(w, 1, player)
			case "stamina":
				ecs.WithComponent(w, player, func(stats *components.EntityStats) { stats.Stamina = 0 })
			case "position":
				ecs.WithComponent(w, player, func(position *components.Transform) { position.X = 100 })
			}
			require.True(t, service.isOnCooldown(w, player, "repeat_test"))
			ecs.GetResource[ecs.TimeState](w).UnixMs = 10500
			service.Recheck(w, 1, player)
			require.Equal(t, "idle", service.State(w, player).Phase)
			require.False(t, ecs.HasComponent[components.ActiveCyclicAction](w, player))
			require.Equal(t, 1, handler.effects)
			require.Len(t, sender.finished, 1)
			require.Equal(t, netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED, sender.finished[0].Result)
		})
	}
}

func TestDeferredLiftStartsCooldownOnlyAtCommit(t *testing.T) {
	w, player, target, lift, service, _ := newNoColliderLiftActionTest(t)
	definition, _ := service.definitions.Get("lift")
	definition.Cooldown = 2000
	clock := ecs.GetResource[ecs.TimeState](w)
	clock.UnixMs = 10000
	service.Activate(w, 1, player, "lift")
	service.HandleArmedClick(w, 1, player, 3, target, 1000, 1000)
	require.Empty(t, service.State(w, player).Cooldowns)
	pending, _ := ecs.GetComponent[components.PendingLiftTransition](w, player)
	clock.UnixMs = 11000
	ecs.WithComponent(w, player, func(position *components.Transform) { position.X, position.Y = 1000, 1000 })
	lift.FinalizePendingLiftTransition(w, 1, player, pending)
	require.True(t, lift.IsPlayerCarrying(w, player))
	require.Equal(t, int64(13000), service.State(w, player).Cooldowns[0].ExpiresAtMs)
}

func TestRestoreCooldownsDropsExpiredAndRemovedActions(t *testing.T) {
	previous := actiondefs.Global()
	actiondefs.SetGlobalForTesting(actiondefs.NewRegistry([]actiondefs.Definition{{ID: "active"}, {ID: "expired"}}))
	t.Cleanup(func() { actiondefs.SetGlobalForTesting(previous) })
	cooldowns, err := loadCharacterActionCooldowns([]byte(`{"active":{"startedAtMs":1000,"expiresAtMs":5000},"expired":{"startedAtMs":1000,"expiresAtMs":2000},"removed":{"startedAtMs":1000,"expiresAtMs":9000}}`), 2000)
	require.NoError(t, err)
	require.Len(t, cooldowns.ByAction, 1)
	require.Equal(t, int64(5000), cooldowns.ByAction["active"].ExpiresAtMs)
	for _, raw := range []string{`null`, `[]`, `{"active":{"startedAtMs":1000,"expiresAtMs":500}}`, `{"active":null}`} {
		_, err := loadCharacterActionCooldowns([]byte(raw), 2000)
		require.Error(t, err)
	}
}

func TestCooldownWaitClearsAndResumesAnimation(t *testing.T) {
	_, _, bindings := animationFixture(t)
	binding := bindings[0]
	binding.Source = actionanimationdefs.Source{Kind: "menu", ID: "repeat_test"}
	registry, err := actionanimationdefs.NewRegistry([]actionanimationdefs.Definition{binding})
	require.NoError(t, err)
	actionanimationdefs.SetGlobalForTesting(registry)
	w, player, service, _, sender := cooldownRepeatFixture(t, 500)
	ecs.AddComponent(w, player, components.Appearance{Resource: "player"})
	started, err := cyclicaction.Snapshot(w, player)
	require.NoError(t, err)
	require.Equal(t, binding.Key, started.AnimationKey)
	advanceRepeatTestCycle(t, w, player, service, sender)
	paused, err := cyclicaction.Snapshot(w, player)
	require.NoError(t, err)
	require.Empty(t, paused.AnimationKey)
	require.Greater(t, paused.Revision, started.Revision)
	require.Empty(t, sender.finished)
	ecs.GetResource[ecs.TimeState](w).UnixMs = 10500
	service.Recheck(w, 1, player)
	resumed, err := cyclicaction.Snapshot(w, player)
	require.NoError(t, err)
	require.Equal(t, binding.Key, resumed.AnimationKey)
	require.Greater(t, resumed.Revision, paused.Revision)
}

func TestCooldownReattachPreservesLiveDeadline(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	player := spawnSoundLifecycleEntity(t, shard, 10, 200, 100)
	client, _ := connectPlayerSpawnTestClient(t)
	client.CharacterID = 10
	now := ecs.GetResource[ecs.TimeState](shard.world).UnixMs
	expected := components.ActionCooldown{StartedAtMs: now, ExpiresAtMs: now + 2000}
	ecs.AddComponent(shard.world, player, components.ActionCooldowns{ByAction: map[string]components.ActionCooldown{"plow_tile": expected}})
	ecs.GetResource[ecs.DetachedEntities](shard.world).AddDetachedEntity(10, player, game.clock.GameNow().Add(time.Minute), game.clock.GameNow())
	// A stale persisted snapshot must never replace the surviving runtime record.
	require.True(t, game.tryReattachPlayer(client, shard, 10, repository.Character{ID: 10, ActionCooldowns: []byte(`{}`)}))
	actual, ok := ecs.GetComponent[components.ActionCooldowns](shard.world, player)
	require.True(t, ok)
	require.Equal(t, expected, actual.ByAction["plow_tile"])
}

func TestCooldownTransferCapturesDeadlineBeforeDespawn(t *testing.T) {
	shard, game := newSoundLifecycleShard(t, config.DefaultAudioConfig(), 16)
	player := spawnSoundLifecycleEntity(t, shard, 10, 200, 100)
	client, _ := connectPlayerSpawnTestClient(t)
	attachSoundLifecycleClient(t, game, shard, client, player)
	now := ecs.GetResource[ecs.TimeState](shard.world).UnixMs
	expected := components.ActionCooldown{StartedAtMs: now, ExpiresAtMs: now + 2000}
	ecs.AddComponent(shard.world, player, components.ActionCooldowns{ByAction: map[string]components.ActionCooldown{"plow_tile": expected}})
	transfer := NewPlayerTransferService(game, zap.NewNop())
	snapshot, err := transfer.detachTransferSource(PlayerTransferRequest{PlayerID: 10, SourceLayer: 0, TargetLayer: 1}, shard, repository.Character{ID: 10, ActionCooldowns: []byte(`{}`)})
	require.NoError(t, err)
	require.False(t, shard.world.Alive(player))
	captured, err := components.UnmarshalActionCooldowns(snapshot.Character.ActionCooldowns)
	require.NoError(t, err)
	require.Equal(t, expected, captured.ByAction["plow_tile"])
}
