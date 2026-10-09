package game

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type testDirectionActionHandler struct {
	testActionHandler
	startedTarget ActionTarget
}

func (handler *testDirectionActionHandler) Start(world *ecs.World, id types.EntityID, player types.Handle, target ActionTarget, generation uint64) ActionResult {
	handler.startedTarget = target
	return handler.testActionHandler.Start(world, id, player, target, generation)
}

func newDirectionActionTest(t *testing.T) (*ecs.World, types.Handle, *ActionService, *testDirectionActionHandler, *testActionSender) {
	t.Helper()
	world := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](world).UnixMs = 10000
	player := world.Spawn(1, func(w *ecs.World, handle types.Handle) {
		ecs.AddComponent(w, handle, components.Transform{})
		ecs.AddComponent(w, handle, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, VelocityX: 4})
		ecs.AddComponent(w, handle, components.EntityStats{Stamina: 1000})
		ecs.AddComponent(w, handle, components.EntityHealth{SHP: 25, HHP: 25})
		ecs.AddComponent(w, handle, components.CharacterProfile{Skills: []string{"test_skill"}})
	})
	registry := actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "direction_test", Target: actiondefs.Target{Kind: actiondefs.TargetDirection},
			Requirements: actiondefs.Requirements{Skills: []string{"test_skill"}},
			Execution:    actiondefs.Execution{Ticks: 6, Stamina: 60}, Cooldown: 2000},
		{ID: "selected", Target: actiondefs.Target{Kind: actiondefs.TargetTile}},
		{ID: "ready", Target: actiondefs.Target{Kind: actiondefs.TargetNone}},
	})
	handler := &testDirectionActionHandler{testActionHandler: testActionHandler{status: ActionSucceeded}}
	sender := &testActionSender{}
	service, err := NewActionService(world, registry, map[string]ActionHandler{
		"direction_test": handler, "selected": &testActionHandler{}, "ready": &testActionHandler{status: ActionSucceeded},
	}, sender)
	require.NoError(t, err)
	return world, player, service, handler, sender
}

func directionTestRequest(angle float32) *netproto.C2S_ActivateAction {
	return &netproto.C2S_ActivateAction{ActionId: "direction_test", AimAngle: &angle, StreamEpoch: 1}
}

func TestDirectionActionTimedCompletion(t *testing.T) {
	for _, angle := range []float64{0, -math.Pi / 2, 0.123456789123456} {
		world, player, service, handler, sender := newDirectionActionTest(t)
		ecs.WithComponent(world, player, func(transform *components.Transform) { transform.Direction = angle })
		commands, inbox := directionalTestCommands(world, service)
		timing := ecs.GetResource[ecs.TimeState](world)
		timing.UnixMs = 10000
		request := &netproto.C2S_ActivateAction{ActionId: "direction_test", StreamEpoch: 1}
		require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
			ClientID: 1, CharacterID: 1, CommandID: 1, CommandType: network.CmdActivateAction,
			ReceivedAt: timing.WallNow, Payload: request,
		}))
		commands.Update(world, .1)
		active, exists := ecs.GetComponent[components.ActiveGameAction](world, player)
		require.True(t, exists)
		require.Equal(t, components.GameActionExecuting, active.Phase)
		require.True(t, validActionAim(active.AimAngle))
		if angle == 0 {
			require.Zero(t, active.AimAngle)
		} else if angle < 0 {
			require.Equal(t, 1.5*math.Pi, active.AimAngle)
		} else {
			require.Equal(t, angle, active.AimAngle, "server precision must not pass through float32")
		}
		movement, _ := ecs.GetComponent[components.Movement](world, player)
		require.Equal(t, constt.TargetNone, movement.TargetType)
		require.Zero(t, movement.VelocityX)
		stale, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		ecs.WithComponent(world, player, func(transform *components.Transform) { transform.Direction = math.Pi })
		require.Equal(t, active.AimAngle, stale.FacingAngle)
		require.True(t, stale.HasFacingAngle)
		cycles := NewCyclicActionSystem(nil, sender, zap.NewNop())
		cycles.SetActionService(service)
		for tick := 1; tick <= 6; tick++ {
			timing.UnixMs += 100
			cycles.Update(world, .1)
			if tick < 6 {
				require.Zero(t, handler.startCount)
				stats, _ := ecs.GetComponent[components.EntityStats](world, player)
				require.Equal(t, 1000.0, stats.Stamina)
				require.Empty(t, service.State(world, player).Cooldowns)
			}
		}
		require.Equal(t, 1, handler.startCount)
		require.Equal(t, active.AimAngle, handler.startedTarget.AimAngle)
		stats, _ := ecs.GetComponent[components.EntityStats](world, player)
		require.Equal(t, 940.0, stats.Stamina)
		require.Equal(t, "idle", service.State(world, player).Phase)
		require.Len(t, sender.progress, 6)
		require.Len(t, sender.finished, 1)
		require.Equal(t, int64(10600), service.State(world, player).Cooldowns[0].StartedAtMs)
		require.Equal(t, int64(12600), service.State(world, player).Cooldowns[0].ExpiresAtMs)
		service.AdvanceCycle(world, 1, player, stale, sender)
		service.Complete(world, 1, player, active.Generation, true, "")
		require.Equal(t, 1, handler.startCount)
		service.Activate(world, 1, player, "ready")
		require.Equal(t, "idle", service.State(world, player).Phase)
	}
}

func TestDirectionActionInvalidRequestPreservesCurrentAction(t *testing.T) {
	for _, test := range []struct {
		name     string
		request  *netproto.C2S_ActivateAction
		client   uint64
		alert    bool
		layer    int
		detached bool
	}{
		{name: "missing epoch and angle", request: &netproto.C2S_ActivateAction{ActionId: "direction_test"}, client: 1},
		{name: "nan", request: directionTestRequest(float32(math.NaN())), client: 1, alert: true},
		{name: "infinity", request: directionTestRequest(float32(math.Inf(1))), client: 1, alert: true},
		{name: "missing epoch", request: &netproto.C2S_ActivateAction{ActionId: "direction_test", AimAngle: new(float32)}, client: 1},
		{name: "old epoch", request: &netproto.C2S_ActivateAction{ActionId: "direction_test", StreamEpoch: 2}, client: 1},
		{name: "old connection", request: &netproto.C2S_ActivateAction{ActionId: "direction_test", StreamEpoch: 1}, client: 2},
		{name: "wrong layer without aim", request: &netproto.C2S_ActivateAction{ActionId: "direction_test", StreamEpoch: 1}, client: 1, layer: 1},
		{name: "detached without aim", request: &netproto.C2S_ActivateAction{ActionId: "direction_test", StreamEpoch: 1}, client: 1, detached: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, player, service, handler, sender := newDirectionActionTest(t)
			service.Activate(world, 1, player, "selected")
			before, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			commands, inbox := directionalTestCommands(world, service)
			if test.detached {
				ecs.GetResource[ecs.DetachedEntities](world).AddDetachedEntity(1, player, ecs.GetResource[ecs.TimeState](world).Now, ecs.GetResource[ecs.TimeState](world).Now)
			}
			require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
				ClientID: test.client, CharacterID: 1, CommandID: 1, CommandType: network.CmdActivateAction,
				ReceivedAt: ecs.GetResource[ecs.TimeState](world).WallNow, Payload: test.request, Layer: test.layer,
			}))
			commands.Update(world, .1)
			after, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			require.Equal(t, before, after)
			require.Zero(t, handler.startCount)
			if test.alert {
				require.Len(t, sender.alerts, 1)
				require.Equal(t, "ACTION_INVALID_TARGET", sender.alerts[0].ReasonCode)
			} else {
				require.Empty(t, sender.alerts)
			}
		})
	}
}

func TestDirectionActionCancellation(t *testing.T) {
	for _, cause := range []string{"cancel", "switch", "movement", "point movement", "skill", "stamina", "ko", "lying", "stun", "failed completion"} {
		t.Run(cause, func(t *testing.T) {
			world, player, service, handler, sender := newDirectionActionTest(t)
			service.ActivateRequest(world, 1, player, directionTestRequest(0))
			active, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			stale, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
			service.AdvanceCycle(world, 1, player, stale, sender)
			switch cause {
			case "cancel":
				service.Cancel(world, 1, player)
			case "switch":
				service.Activate(world, 1, player, "ready")
			case "movement":
				commands, inbox := directionalTestCommands(world, service)
				enqueueTestDirection(t, world, inbox, 1)
				commands.Update(world, .1)
			case "point movement":
				commands, inbox := directionalTestCommands(world, service)
				require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
					ClientID: 1, CharacterID: 1, CommandID: 1, CommandType: network.CmdMapClick,
					ReceivedAt: ecs.GetResource[ecs.TimeState](world).WallNow,
					Payload:    &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_PRIMARY, X: 12, Y: 24},
				}))
				commands.Update(world, .1)
			case "skill":
				ecs.WithComponent(world, player, func(profile *components.CharacterProfile) { profile.Skills = nil })
			case "stamina":
				ecs.WithComponent(world, player, func(stats *components.EntityStats) { stats.Stamina = 59 })
			case "ko":
				ecs.WithComponent(world, player, func(health *components.EntityHealth) { health.KOUntilUnixMs = 60000 })
			case "lying":
				ecs.WithComponent(world, player, func(health *components.EntityHealth) { health.IsLying = true })
			case "stun":
				ecs.WithComponent(world, player, func(movement *components.Movement) { movement.State = constt.StateStunned })
			case "failed completion":
				handler.status = ActionFailed
				for tick := 1; tick < 6; tick++ {
					cycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
					service.AdvanceCycle(world, 1, player, cycle, sender)
				}
			}
			service.Recheck(world, 1, player)
			before, _ := ecs.GetComponent[components.EntityStats](world, player)
			service.AdvanceCycle(world, 1, player, stale, sender)
			service.Complete(world, 1, player, active.Generation, true, "")
			after, _ := ecs.GetComponent[components.EntityStats](world, player)
			require.Equal(t, before.Stamina, after.Stamina)
			require.Empty(t, service.State(world, player).Cooldowns)
			require.Equal(t, "idle", service.State(world, player).Phase)
			_, executing := ecs.GetComponent[components.ActiveCyclicAction](world, player)
			require.False(t, executing)
			if cause != "failed completion" {
				require.Zero(t, handler.startCount)
			}
		})
	}
}

func TestDirectionActionUsesHeadingInsteadOfLegacyAim(t *testing.T) {
	for _, aim := range []float32{0, 1, -2, math.MaxFloat32} {
		world, player, service, _, _ := newDirectionActionTest(t)
		ecs.WithComponent(world, player, func(transform *components.Transform) { transform.Direction = -0.75 })
		service.ActivateRequest(world, 1, player, directionTestRequest(aim))
		active, exists := ecs.GetComponent[components.ActiveGameAction](world, player)
		require.True(t, exists)
		require.Equal(t, 2*math.Pi-0.75, active.AimAngle)
		transform, _ := ecs.GetComponent[components.Transform](world, player)
		require.Equal(t, -0.75, transform.Direction)
	}
}

func TestDirectionInvalidHeadingPreservesActionAndMovement(t *testing.T) {
	for _, cause := range []string{"missing", "nan", "positive infinity", "negative infinity"} {
		t.Run(cause, func(t *testing.T) {
			world, player, service, handler, sender := newDirectionActionTest(t)
			service.Activate(world, 1, player, "selected")
			switch cause {
			case "missing":
				ecs.RemoveComponent[components.Transform](world, player)
			default:
				invalid := map[string]float64{"nan": math.NaN(), "positive infinity": math.Inf(1), "negative infinity": math.Inf(-1)}[cause]
				ecs.WithComponent(world, player, func(transform *components.Transform) { transform.Direction = invalid })
			}
			before, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			beforeMovement, _ := ecs.GetComponent[components.Movement](world, player)
			generation := service.nextGeneration
			service.ActivateRequest(world, 1, player, &netproto.C2S_ActivateAction{ActionId: "direction_test"})
			after, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			afterMovement, _ := ecs.GetComponent[components.Movement](world, player)
			require.Equal(t, before, after)
			require.Equal(t, beforeMovement, afterMovement)
			require.Equal(t, generation, service.nextGeneration)
			require.Zero(t, handler.startCount)
			require.Equal(t, "ACTION_INVALID_TARGET", sender.alerts[0].ReasonCode)
		})
	}
}

func TestActionGenerationExhaustionPreservesCurrentExecution(t *testing.T) {
	for _, direct := range []bool{false, true} {
		world, player, service, _, sender := newDirectionActionTest(t)
		service.nextGeneration = math.MaxUint64 - 1
		service.Activate(world, 1, player, "direction_test")
		before, exists := ecs.GetComponent[components.ActiveGameAction](world, player)
		require.True(t, exists)
		require.EqualValues(t, uint64(math.MaxUint64), before.Generation)
		beforeMovement, _ := ecs.GetComponent[components.Movement](world, player)
		beforeCycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		if direct {
			service.StartTargetedOnce(world, 1, player, "selected", 0, types.InvalidHandle, 1, 2)
		} else {
			service.Activate(world, 1, player, "ready")
		}
		after, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
		afterMovement, _ := ecs.GetComponent[components.Movement](world, player)
		afterCycle, _ := ecs.GetComponent[components.ActiveCyclicAction](world, player)
		require.Equal(t, before, after)
		require.Equal(t, beforeMovement, afterMovement)
		require.Equal(t, beforeCycle, afterCycle)
		require.EqualValues(t, uint64(math.MaxUint64), service.nextGeneration)
		require.Equal(t, "ACTION_FAILED", sender.alerts[0].ReasonCode)
		require.Empty(t, sender.finished)
	}
}

func TestDirectionStartUsesCommittedHeadingBeforeSameTickWASD(t *testing.T) {
	world, player, service, _, _ := newDirectionActionTest(t)
	ecs.WithComponent(world, player, func(transform *components.Transform) { transform.Direction = 0.25 })
	commands, inbox := directionalTestCommands(world, service)
	require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
		ClientID: 1, CharacterID: 1, CommandID: 1, CommandType: network.CmdMoveDirection,
		ReceivedAt: ecs.GetResource[ecs.TimeState](world).WallNow,
		Payload:    &netproto.MoveDirection{X: -1, InputRevision: 1, StreamEpoch: 1},
	}))
	require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
		ClientID: 1, CharacterID: 1, CommandID: 2, CommandType: network.CmdActivateAction,
		ReceivedAt: ecs.GetResource[ecs.TimeState](world).WallNow,
		Payload:    &netproto.C2S_ActivateAction{ActionId: "direction_test", StreamEpoch: 1},
	}))
	commands.Update(world, .1)
	active, exists := ecs.GetComponent[components.ActiveGameAction](world, player)
	require.True(t, exists)
	require.Equal(t, 0.25, active.AimAngle)
	transform, _ := ecs.GetComponent[components.Transform](world, player)
	require.Equal(t, 0.25, transform.Direction)
	movement, _ := ecs.GetComponent[components.Movement](world, player)
	require.Equal(t, constt.TargetNone, movement.TargetType)
}

func TestQueuedDirectionActivationRevalidatesCurrentSession(t *testing.T) {
	for _, transition := range []string{"disconnect", "replacement", "epoch", "detach"} {
		t.Run(transition, func(t *testing.T) {
			world, player, service, handler, _ := newDirectionActionTest(t)
			service.Activate(world, 1, player, "selected")
			before, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			client := &network.Client{ID: 1, CharacterID: 1}
			client.InWorld.Store(true)
			client.StreamEpoch.Store(1)
			shard := &Shard{Clients: map[types.EntityID]*network.Client{1: client}}
			commands, inbox := directionalTestCommands(world, service)
			commands.SetDirectionalSessionValidator(shard.validDirectionalSession)
			require.NoError(t, inbox.Enqueue(&network.PlayerCommand{
				ClientID: 1, CharacterID: 1, CommandID: 1, CommandType: network.CmdActivateAction,
				ReceivedAt: ecs.GetResource[ecs.TimeState](world).WallNow,
				Payload:    &netproto.C2S_ActivateAction{ActionId: "direction_test", StreamEpoch: 1},
			}))
			switch transition {
			case "disconnect":
				client.InWorld.Store(false)
			case "replacement":
				replacement := &network.Client{ID: 2, CharacterID: 1}
				replacement.InWorld.Store(true)
				replacement.StreamEpoch.Store(1)
				shard.Clients[1] = replacement
			case "epoch":
				client.StreamEpoch.Store(2)
			case "detach":
				now := ecs.GetResource[ecs.TimeState](world).Now
				ecs.GetResource[ecs.DetachedEntities](world).AddDetachedEntity(1, player, now, now)
			}
			commands.Update(world, .1)
			after, _ := ecs.GetComponent[components.ActiveGameAction](world, player)
			require.Equal(t, before, after)
			require.Zero(t, handler.startCount)
		})
	}
}
