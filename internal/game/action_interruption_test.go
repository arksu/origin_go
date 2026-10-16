package game

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/actiondefs"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/playerstate"
	"origin/internal/types"
)

type testInactivityAction struct {
	testActionHandler
	admit func(*ecs.World, types.Handle) string
}

func (*testInactivityAction) InterruptOnGameplayAttempt() bool { return true }
func (handler *testInactivityAction) BeforeExecution(w *ecs.World, _ types.EntityID, h types.Handle) string {
	if handler.admit != nil {
		return handler.admit(w, h)
	}
	return ""
}

func newInterruptionActions(t *testing.T, w *ecs.World) (*ActionService, *testInactivityAction) {
	t.Helper()
	handler := &testInactivityAction{testActionHandler: testActionHandler{status: ActionSucceeded}}
	service, err := NewActionService(w, actiondefs.NewRegistry([]actiondefs.Definition{
		{ID: "rest", Target: actiondefs.Target{Kind: actiondefs.TargetNone}, Execution: actiondefs.Execution{Ticks: 50}, Cooldown: 360000},
		{ID: "other", Target: actiondefs.Target{Kind: actiondefs.TargetTile}, Cooldown: 1000},
		{ID: "item", Target: actiondefs.Target{Kind: actiondefs.TargetTile}},
		{ID: "directed", Target: actiondefs.Target{Kind: actiondefs.TargetDirection}},
	}), map[string]ActionHandler{
		"rest": handler, "other": &testActionHandler{unavailable: "ACTION_UNAVAILABLE"},
		"item": &testItemActionHandler{}, "directed": &testActionHandler{},
	}, nil)
	require.NoError(t, err)
	return service, handler
}

func TestActionAdmissionRunsBeforeTimedProgressAndFailureCleansUp(t *testing.T) {
	for _, failure := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		h := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Movement{State: constt.StateMoving, TargetType: constt.TargetPoint, TargetX: 42})
		})
		service, handler := newInterruptionActions(t, w)
		calls := 0
		handler.admit = func(w *ecs.World, h types.Handle) string {
			calls++
			_, started := ecs.GetComponent[components.ActiveCyclicAction](w, h)
			require.False(t, started, "admission must precede progress")
			playerstate.StopMovement(w, h)
			if failure {
				return "ADMISSION_FAILED"
			}
			return ""
		}
		service.Activate(w, 1, h, "rest")
		require.Equal(t, 1, calls)
		require.Zero(t, handler.startCount, "Start must remain a timed completion hook")
		movement, _ := ecs.GetComponent[components.Movement](w, h)
		require.Equal(t, constt.TargetNone, movement.TargetType)
		_, active := ecs.GetComponent[components.ActiveCyclicAction](w, h)
		require.Equal(t, !failure, active)
		require.False(t, service.isOnCooldown(w, h, "rest"))
		if failure {
			require.Equal(t, "idle", service.State(w, h).Phase)
			require.Equal(t, 1, handler.canceled)
		}
	}
}

func TestInactivityActionInterruptedBeforeRejectedActionAdmission(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, refusal := range []string{"requirements", "cooldown", "incapacity"} {
			t.Run(refusal+map[bool]string{false: "/activate", true: "/direct"}[direct], func(t *testing.T) {
				w := ecs.NewWorldForTesting()
				h := w.Spawn(1, nil)
				service, handler := newInterruptionActions(t, w)
				service.Activate(w, 1, h, "rest")
				old, _ := ecs.GetComponent[components.ActiveGameAction](w, h)
				id := "other"
				if refusal == "cooldown" {
					definition, _ := service.definitions.Get(id)
					service.startCooldown(w, h, definition)
				} else if refusal == "incapacity" {
					id = "item"
					ecs.AddComponent(w, h, components.EntityHealth{SHP: 2, HHP: 20, IsLying: true})
				}
				if direct {
					service.StartTargetedOnce(w, 1, h, id, 0, types.InvalidHandle, 0, 0)
				} else {
					service.Activate(w, 1, h, id)
				}
				require.Equal(t, 1, handler.canceled)
				require.Equal(t, "idle", service.State(w, h).Phase)
				service.Complete(w, 1, h, old.Generation, true, "")
				require.False(t, service.isOnCooldown(w, h, "rest"), "stale completion must not charge cooldown")
			})
		}
	}
}

func TestInactivityPolicyLeavesOrdinaryActionsAndInvalidAimAlone(t *testing.T) {
	w := ecs.NewWorldForTesting()
	h := w.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.Transform{}) })
	service, handler := newInterruptionActions(t, w)
	ecs.AddComponent(w, h, components.ActiveGameAction{ActionID: "other", Generation: 7, Phase: components.GameActionSelecting})
	service.Interrupt(w, 1, h)
	require.Equal(t, uint64(7), service.State(w, h).ActionGeneration)
	service.Activate(w, 1, h, "rest")
	invalid := float32(math.NaN())
	service.ActivateRequest(w, 1, h, &netproto.C2S_ActivateAction{ActionId: "directed", AimAngle: &invalid})
	require.Equal(t, "rest", service.State(w, h).ActionId)
	require.Zero(t, handler.canceled)
	require.Zero(t, testing.AllocsPerRun(100, func() { service.Interrupt(w, 1, types.InvalidHandle) }))
	ecs.AddComponent(w, h, components.ActiveGameAction{ActionID: "other", Generation: 9, Phase: components.GameActionSelecting})
	require.Zero(t, testing.AllocsPerRun(100, func() { service.Interrupt(w, 1, h) }))
}

type interruptionCarryService struct{}

func (interruptionCarryService) IsPlayerCarrying(*ecs.World, types.Handle) bool { return true }
func (interruptionCarryService) SendCarryLockedWarning(types.EntityID)          {}

func TestInactivityActionNetworkAttemptsAndExemptions(t *testing.T) {
	for _, test := range []struct {
		name      string
		kind      network.CommandType
		payload   any
		interrupt bool
	}{
		{"primary while lying", network.CmdMapClick, &netproto.MapClick{X: 30}, true},
		{"secondary while lying", network.CmdMapClick, &netproto.MapClick{Button: netproto.MapClickButton_MAP_CLICK_BUTTON_SECONDARY}, true},
		{"context rejected", network.CmdSelectContextAction, &netproto.SelectContextAction{ActionId: "open", EntityId: 90}, true},
		{"craft one while carrying", network.CmdStartCraftOne, &netproto.C2S_StartCraftOne{CraftKey: "unavailable"}, true},
		{"craft many while carrying", network.CmdStartCraftMany, &netproto.C2S_StartCraftMany{CraftKey: "unavailable", Cycles: 1}, true},
		{"build while carrying", network.CmdStartBuild, &netproto.C2S_BuildStart{BuildKey: "unavailable"}, true},
		{"build progress while carrying", network.CmdBuildProgress, &netproto.C2S_BuildProgress{EntityId: 90}, true},
		{"build take back while carrying", network.CmdBuildTakeBack, &netproto.C2S_BuildTakeBack{EntityId: 90}, true},
		{"chat", network.CmdChat, &network.ChatCommandPayload{Text: "hello"}, false},
		{"personal window", network.CmdOpenWindow, &netproto.C2S_OpenWindow{Name: "inventory"}, false},
		{"craft window", network.CmdOpenWindow, &netproto.C2S_OpenWindow{Name: "craft"}, false},
		{"invalid map button", network.CmdMapClick, &netproto.MapClick{Button: 100}, false},
		{"invalid craft payload", network.CmdStartCraftOne, (*netproto.C2S_StartCraftOne)(nil), false},
		{"invalid context payload", network.CmdSelectContextAction, (*netproto.SelectContextAction)(nil), false},
		{"invalid cancel payload", network.CmdCancelAction, (*netproto.C2S_CancelAction)(nil), false},
		{"cancel", network.CmdCancelAction, &netproto.C2S_CancelAction{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, current := range []bool{false, true} {
				w := ecs.NewWorldForTesting()
				h := w.Spawn(1, func(w *ecs.World, h types.Handle) {
					ecs.AddComponent(w, h, components.EntityHealth{SHP: 2, HHP: 20, IsLying: true})
					ecs.AddComponent(w, h, components.Movement{})
				})
				service, handler := newInterruptionActions(t, w)
				service.Activate(w, 1, h, "rest")
				queue := network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig())
				commands := systems.NewNetworkCommandSystem(queue, network.NewServerJobInbox(network.DefaultCommandQueueConfig()), nil, nil, nil, nil, nil, 0, zap.NewNop())
				commands.SetActionService(service)
				commands.SetLiftCommandService(interruptionCarryService{})
				commands.SetPlayerConnectionValidator(func(*ecs.World, types.EntityID, uint64) bool { return current })
				require.NoError(t, queue.Enqueue(&network.PlayerCommand{ClientID: 2, CharacterID: 1, CommandID: 1, CommandType: test.kind, Payload: test.payload}))
				commands.Update(w, 0)
				require.Equal(t, current && test.interrupt, handler.canceled != 0)
				require.False(t, service.isOnCooldown(w, h, "rest"))
			}
		})
	}
}

func TestInactivityActionStandUpSessionAndOrdinaryDisconnect(t *testing.T) {
	for _, current := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		h := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.EntityHealth{SHP: 2, HHP: 20, IsLying: true})
		})
		service, handler := newInterruptionActions(t, w)
		service.Activate(w, 1, h, "rest")
		client := &network.Client{ID: 2, CharacterID: 1}
		client.InWorld.Store(true)
		client.StreamEpoch.Store(7)
		shard := &Shard{world: w, actionService: service, Clients: map[types.EntityID]*network.Client{1: client}, logger: zap.NewNop()}
		epoch := uint32(6)
		if current {
			epoch = 7
		}
		shard.queueStandUp(w, h, &network.PlayerCommand{ClientID: 2, CharacterID: 1, Payload: &netproto.StandUp{StreamEpoch: epoch}})
		require.Equal(t, current, handler.canceled != 0)
		require.NotNil(t, shard.pendingStandUps[1], "stale epoch still needs the existing stats ack")
		if current {
			service.Activate(w, 1, h, "rest")
		}
		old := &network.Client{ID: 3, CharacterID: 1}
		require.False(t, shard.detachClientForLogout(old, time.Unix(100, 0), time.Minute))
		require.Equal(t, "rest", service.State(w, h).ActionId)
		require.True(t, shard.detachClientForLogout(client, time.Unix(100, 0), time.Minute))
		require.Equal(t, "idle", service.State(w, h).Phase)
		require.False(t, service.isOnCooldown(w, h, "rest"))
		require.True(t, w.Alive(h))
	}
}

func TestInactivityActionDamageAndZeroDamageCombat(t *testing.T) {
	for _, draw := range []float64{0, 2} {
		fixture := newCreatureDamageFixture(t)
		service, handler := newInterruptionActions(t, fixture.world)
		service.Activate(fixture.world, 1, fixture.owner, "rest")
		fixture.service.SetActionInterrupter(service.Interrupt)
		_, err := fixture.service.Apply(fixture.owner, math.NaN())
		require.Error(t, err)
		require.Zero(t, handler.canceled)
		_, err = fixture.service.Apply(fixture.owner, draw)
		require.NoError(t, err)
		require.Equal(t, 1, handler.canceled)
		require.Equal(t, "idle", service.State(fixture.world, fixture.owner).Phase)
	}
}

func TestInactivityActionStarvationBeforeHealthRestoration(t *testing.T) {
	for _, knockoutEnding := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		health := components.EntityHealth{SHP: 1, HHP: 20}
		if knockoutEnding {
			health.KOUntilUnixMs, health.IsLying = 1000, true
		}
		h := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, health)
			ecs.AddComponent(w, h, components.EntityStats{Energy: 400})
		})
		*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Tick: 100, UnixMs: 1000}
		ecs.GetResource[ecs.CharacterEntities](w).Add(1, h, time.Time{})
		service, handler := newInterruptionActions(t, w)
		service.Activate(w, 1, h, "rest")
		system := NewPlayerDeathSystem(nil, PlayerDeathSystemConfig{StarvationDamageIntervalTicks: 100, ShpRegenIntervalTicks: 100})
		system.SetActionInterrupter(service.Interrupt)
		system.Update(w, 0)
		after, _ := ecs.GetComponent[components.EntityHealth](w, h)
		if knockoutEnding {
			require.Equal(t, health.SHP, after.SHP, "KO completion masks the preceding damage")
		}
		require.Equal(t, 1, handler.canceled)
		require.Equal(t, "idle", service.State(w, h).Phase)
	}
}

func TestInactivityActionAdminDamageOnlyWhenHealthFalls(t *testing.T) {
	for _, damage := range []string{"0", "-1", "2"} {
		w := ecs.NewWorldForTesting()
		h := w.Spawn(1, func(w *ecs.World, h types.Handle) { ecs.AddComponent(w, h, components.EntityHealth{SHP: 10, HHP: 20}) })
		service, handler := newInterruptionActions(t, w)
		service.Activate(w, 1, h, "rest")
		admin := &ChatAdminCommandHandler{lifeDeathFactor: 1, chatDelivery: &mockChatDeliveryService{messages: map[types.EntityID]string{}}}
		admin.SetActionInterrupter(service.Interrupt)
		admin.handleDamage(w, 1, h, []string{damage})
		require.Equal(t, damage == "2", handler.canceled != 0)
	}
}

func TestInactivityActionFreshDirectionalIntentBeforeMovementRefusal(t *testing.T) {
	for _, scenario := range []string{"lying", "stunned", "exhausted", "release", "refresh", "expired", "stale revision", "stale epoch", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			now := time.Unix(100, 0)
			*ecs.GetResource[ecs.TimeState](w) = ecs.TimeState{Now: now, WallNow: now}
			h := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Transform{})
				movement := components.Movement{Mode: constt.Walk}
				movement.ResetDirectionSession(2, 3)
				movement.SetDirection(1, 0, 2, now.Add(systems.DirectionalInputTTL))
				movement.Direction.InputX = 1
				ecs.AddComponent(w, h, movement)
			})
			service, handler := newInterruptionActions(t, w)
			service.Activate(w, 1, h, "rest")
			direction := &netproto.MoveDirection{X: 1, InputRevision: 3, StreamEpoch: 3}
			received := now
			switch scenario {
			case "lying":
				ecs.AddComponent(w, h, components.EntityHealth{SHP: 2, HHP: 20, IsLying: true})
			case "stunned":
				ecs.WithComponent(w, h, func(m *components.Movement) { m.State = constt.StateStunned })
			case "exhausted":
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 0, Energy: 0})
			case "release":
				direction.X = 0
			case "refresh":
				direction.InputRevision = 2
			case "expired":
				received = now.Add(-systems.DirectionalInputTTL)
			case "stale revision":
				direction.InputRevision = 1
			case "stale epoch":
				direction.StreamEpoch = 2
			case "malformed":
				direction.X = float32(math.NaN())
			}
			queue := network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig())
			commands := systems.NewNetworkCommandSystem(queue, network.NewServerJobInbox(network.DefaultCommandQueueConfig()), nil, nil, nil, nil, nil, 0, zap.NewNop())
			commands.SetActionService(service)
			commands.SetDirectionalSessionValidator(func(id types.EntityID, client uint64, epoch uint32) bool { return id == 1 && client == 2 && epoch == 3 })
			require.NoError(t, queue.Enqueue(&network.PlayerCommand{ClientID: 2, CharacterID: 1, CommandID: 1, CommandType: network.CmdMoveDirection, Payload: direction, ReceivedAt: received}))
			commands.Update(w, 0)
			interrupted := scenario == "lying" || scenario == "stunned" || scenario == "exhausted"
			require.Equal(t, interrupted, handler.canceled != 0)
			position, _ := ecs.GetComponent[components.Transform](w, h)
			require.Zero(t, position.X)
			require.Zero(t, position.Y)
			if interrupted {
				movement, _ := ecs.GetComponent[components.Movement](w, h)
				require.NotEqual(t, constt.TargetDirection, movement.TargetType)
			}
		})
	}
}

func TestInactivityActionOnlyAcceptedSwimTransitionInterrupts(t *testing.T) {
	for _, carrying := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		h := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk})
			if carrying {
				ecs.AddComponent(w, h, components.LiftCarryState{})
			}
		})
		service, handler := newInterruptionActions(t, w)
		service.Activate(w, 1, h, "rest")
		queue := network.NewPlayerCommandInbox(network.DefaultCommandQueueConfig())
		commands := systems.NewNetworkCommandSystem(queue, network.NewServerJobInbox(network.DefaultCommandQueueConfig()), nil, nil, nil, nil, nil, 0, zap.NewNop())
		commands.SetActionService(service)
		require.NoError(t, queue.Enqueue(&network.PlayerCommand{ClientID: 2, CharacterID: 1, CommandID: 1, CommandType: network.CmdSetMovementMode, Payload: &netproto.C2S_MovementMode{Mode: netproto.MovementMode_MOVE_MODE_SWIM}}))
		commands.Update(w, 0)
		require.Equal(t, !carrying, handler.canceled != 0)
	}
}
