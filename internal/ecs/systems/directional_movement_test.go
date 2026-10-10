package systems

import (
	"math"
	"testing"
	"time"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type directionalFixture struct {
	world    *ecs.World
	player   types.Handle
	commands *NetworkCommandSystem
	movement *MovementSystem
}

func newDirectionalFixture(t *testing.T) *directionalFixture {
	t.Helper()
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
		ecs.AddComponent(w, h, components.Movement{Mode: constt.Walk, Speed: 32})
	})
	commands := NewNetworkCommandSystem(nil, nil, nil, nil, nil, nil, nil, 0, zap.NewNop())
	commands.SetDirectionalSessionValidator(func(id types.EntityID, client uint64, epoch uint32) bool { return id == 1 && client == 2 && epoch == 3 })
	ecs.SetResource(w, ecs.TimeState{Now: time.Unix(100, 0), WallNow: time.Unix(10000, 0)})
	return &directionalFixture{w, player, commands, NewMovementSystem(w, nil, zap.NewNop())}
}

func (f *directionalFixture) send(x, y float32, revision uint32, age time.Duration) {
	f.commands.handleMoveDirection(f.world, f.player, &network.PlayerCommand{ClientID: 2, CharacterID: 1, ReceivedAt: ecs.GetResource[ecs.TimeState](f.world).WallNow.Add(-age), Payload: &netproto.MoveDirection{X: x, Y: y, InputRevision: revision, StreamEpoch: 3}})
}
func (f *directionalFixture) state() components.Movement {
	m, _ := ecs.GetComponent[components.Movement](f.world, f.player)
	return m
}
func (f *directionalFixture) advance(duration time.Duration) {
	ts := ecs.GetResource[ecs.TimeState](f.world)
	ts.Now = ts.Now.Add(duration)
	ts.WallNow = ts.WallNow.Add(duration)
}

func TestDirectionSessionAndMalformedInputAreInert(t *testing.T) {
	for _, failure := range []string{"validator", "client", "epoch", "layer", "detached", "dead", "nan", "infinite", "range", "zero", "nil"} {
		t.Run(failure, func(t *testing.T) {
			f := newDirectionalFixture(t)
			f.send(1, 0, 1, 0)
			direction := &netproto.MoveDirection{X: 0, Y: 1, InputRevision: 2, StreamEpoch: 3}
			command := &network.PlayerCommand{ClientID: 2, CharacterID: 1, ReceivedAt: ecs.GetResource[ecs.TimeState](f.world).WallNow, Payload: direction}
			switch failure {
			case "validator":
				f.commands.SetDirectionalSessionValidator(nil)
			case "client":
				command.ClientID = 99
			case "epoch":
				direction.StreamEpoch = 2
			case "layer":
				command.Layer = 99
			case "detached":
				ecs.GetResource[ecs.DetachedEntities](f.world).AddDetachedEntity(1, f.player, time.Time{}, time.Time{})
			case "dead":
				ecs.AddComponent(f.world, f.player, components.EntityHealth{HHP: 0})
			case "nan":
				direction.X = float32(math.NaN())
			case "infinite":
				direction.Y = float32(math.Inf(-1))
			case "range":
				direction.Y = 1.01
			case "zero":
				direction.InputRevision = 0
			case "nil":
				command.Payload = (*netproto.MoveDirection)(nil)
			}
			before := f.state()
			f.commands.handleMoveDirection(f.world, f.player, command)
			if after := f.state(); after != before {
				t.Fatalf("invalid input changed movement: %+v", after)
			}
		})
	}
}

func TestDirectionRevisionOrderingAndSupersession(t *testing.T) {
	f := newDirectionalFixture(t)
	f.send(1, 1, ^uint32(0), 0)
	if math.Abs(math.Hypot(f.state().Direction.X, f.state().Direction.Y)-1) > 1e-12 {
		t.Fatal("not normalized")
	}
	before := f.state()
	f.advance(100 * time.Millisecond)
	f.send(.5, .5, ^uint32(0), 0)
	if f.state() != before {
		t.Fatal("same revision changed serialized vector")
	}
	f.send(0, 1, ^uint32(0), 0)
	if f.state() != before {
		t.Fatal("same revision changed direction")
	}
	f.send(0, 1, 1, 0)
	if f.state().Direction.Revision != 1 || f.state().Direction.Y != 1 {
		t.Fatal("wrap rejected")
	}
	f.send(1, 0, ^uint32(0), 0)
	if f.state().Direction.Y != 1 {
		t.Fatal("old pre-wrap revision accepted")
	}
	f.send(0, 0, 2, 0)
	f.send(0, 1, 2, 0)
	if f.state().TargetType != constt.TargetNone {
		t.Fatal("release revived")
	}
	f.send(1, 0, 3, 0)
	ecs.WithComponent(f.world, f.player, func(m *components.Movement) { m.SetTargetPoint(500, 600) })
	f.send(1, 0, 3, 0)
	f.send(0, 0, 4, 0)
	if f.state().TargetType != constt.TargetPoint {
		t.Fatal("old hold/release stole point route")
	}
	for _, pair := range [][2]uint32{{1, 0}, {1, ^uint32(0)}, {^uint32(0), 1}, {0, 1}, {0x80000001, 1}} {
		got := newerInputRevision(pair[0], pair[1])
		want := pair[0] == 1
		if got != want {
			t.Fatalf("serial ordering %v: %v", pair, got)
		}
	}
}

func TestDirectionReceiptDeadlineAndRetirement(t *testing.T) {
	for _, age := range []time.Duration{0, 200 * time.Millisecond, 799 * time.Millisecond, 800 * time.Millisecond, time.Second} {
		t.Run(age.String(), func(t *testing.T) {
			f := newDirectionalFixture(t)
			f.send(1, 0, 1, age)
			if age >= DirectionalInputTTL {
				f.send(1, 0, 1, 0)
				if f.state().TargetType != constt.TargetNone {
					t.Fatal("backlog restarted")
				}
				return
			}
			deadline := ecs.GetResource[ecs.TimeState](f.world).Now.Add(DirectionalInputTTL - age)
			if f.state().Direction.ExpiresAt != deadline {
				t.Fatal("queue age ignored")
			}
			ecs.WithComponent(f.world, f.player, func(m *components.Movement) { m.State = constt.StateIdle })
			f.advance(DirectionalInputTTL - age)
			f.movement.Update(f.world, .1)
			if f.state().TargetType != constt.TargetNone {
				t.Fatal("idle intent survived exact deadline")
			}
			f.send(1, 0, 1, 0)
			if f.state().TargetType != constt.TargetNone {
				t.Fatal("expired revision revived")
			}
			f.send(1, 0, 2, 0)
			if f.state().TargetType != constt.TargetDirection {
				t.Fatal("fresh press rejected")
			}
		})
	}
	for _, delay := range []time.Duration{799 * time.Millisecond, 800 * time.Millisecond} {
		f := newDirectionalFixture(t)
		f.send(1, 0, 1, 0)
		f.advance(delay)
		f.send(1, 0, 1, 0)
		if (f.state().TargetType == constt.TargetDirection) != (delay < DirectionalInputTTL) {
			t.Fatalf("refresh expiry boundary %v", delay)
		}
	}
	// A refresh received before the old deadline can be drained after it, within its own TTL.
	f := newDirectionalFixture(t)
	f.send(1, 0, 1, 0)
	f.advance(900 * time.Millisecond)
	f.send(1, 0, 1, 200*time.Millisecond)
	if f.state().TargetType != constt.TargetDirection || !f.state().Direction.ExpiresAt.Equal(time.Unix(101, 500000000)) {
		t.Fatalf("valid queued refresh lost: %+v", f.state())
	}
}

func TestDirectionRestrictionsRetireInputWithoutCancelingAction(t *testing.T) {
	for _, restriction := range []string{"stun", "stamina"} {
		t.Run(restriction, func(t *testing.T) {
			f := newDirectionalFixture(t)
			router := &testActionClickRouter{}
			f.commands.SetActionService(router)
			ecs.AddComponent(f.world, f.player, components.ActiveGameAction{Phase: components.GameActionExecuting})
			switch restriction {
			case "stun":
				ecs.WithComponent(f.world, f.player, func(m *components.Movement) { m.State = constt.StateStunned })
			case "death":
				ecs.AddComponent(f.world, f.player, components.EntityHealth{HHP: 0, SHP: 0})
			case "stamina":
				ecs.AddComponent(f.world, f.player, components.EntityStats{Stamina: 0, Energy: 1000})
			}
			f.send(1, 0, 1, 0)
			if router.cancelCalls != 0 || f.state().TargetType != constt.TargetNone || f.state().Direction.Revision != 1 {
				t.Fatal("rejected input has effects")
			}
			ecs.RemoveComponent[components.EntityHealth](f.world, f.player)
			ecs.RemoveComponent[components.EntityStats](f.world, f.player)
			ecs.WithComponent(f.world, f.player, func(m *components.Movement) { m.State = constt.StateIdle })
			f.send(1, 0, 1, 0)
			if router.cancelCalls != 0 || f.state().TargetType != constt.TargetNone {
				t.Fatal("recovery resumed retired hold")
			}
		})
	}
}

func TestDirectionEightDirectionsAndOneFinalStep(t *testing.T) {
	for _, vector := range [][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}, {-1.0 / 3, -1}, {1, -1.0 / 3}, {1.0 / 3, 1}, {-1, 1.0 / 3}, {1, 0}} {
		f := newDirectionalFixture(t)
		f.send(vector[0], vector[1], 1, 0)
		ecs.WithComponent(f.world, f.player, func(m *components.Movement) { m.State = constt.StateIdle })
		f.movement.Update(f.world, .1)
		moves := ecs.GetResource[ecs.MovedEntities](f.world)
		if moves.Count != 1 || math.Abs(math.Hypot(moves.IntentX[0]-100, moves.IntentY[0]-100)-3.2) > 1e-10 {
			t.Fatalf("bad step for %v", vector)
		}
	}
	for _, sequence := range []string{"start-stop-start", "turn-turn", "release", "stop-map"} {
		f := newDirectionalFixture(t)
		f.send(1, 0, 1, 0)
		wantX, wantY := 103.2, 100.0
		switch sequence {
		case "start-stop-start":
			f.send(0, 0, 2, 0)
			f.send(0, 1, 3, 0)
			wantX, wantY = 100, 103.2
		case "turn-turn":
			f.send(0, 1, 2, 0)
			f.send(-1, 0, 3, 0)
			wantX = 96.8
		case "release":
			f.send(0, 0, 2, 0)
			wantX = 100
		case "stop-map":
			f.send(0, 0, 2, 0)
			f.commands.handleMapClick(f.world, f.player, &network.PlayerCommand{CharacterID: 1, Payload: &netproto.MapClick{X: 200, Y: 100}})
		}
		NewResetSystem(zap.NewNop()).Update(f.world, .1)
		f.movement.Update(f.world, .1)
		moves := ecs.GetResource[ecs.MovedEntities](f.world)
		if moves.Count != 1 || math.Abs(moves.IntentX[0]-wantX) > 1e-10 || math.Abs(moves.IntentY[0]-wantY) > 1e-10 {
			t.Fatalf("%s: %+v", sequence, moves)
		}
	}
}

func TestDirectionActionPhasesAndIntentCleanup(t *testing.T) {
	for _, phase := range []components.GameActionPhase{components.GameActionSelecting, components.GameActionApproaching, components.GameActionExecuting} {
		t.Run(string(phase), func(t *testing.T) {
			f := newDirectionalFixture(t)
			router := &testActionClickRouter{}
			f.commands.SetActionService(router)
			ecs.AddComponent(f.world, f.player, components.ActiveGameAction{Phase: phase, Generation: 9})
			ecs.AddComponent(f.world, f.player, components.PendingInteraction{TargetEntityID: 8})
			ecs.AddComponent(f.world, f.player, components.PendingContextAction{TargetEntityID: 8})
			links := ecs.GetResource[ecs.LinkState](f.world)
			links.IntentByPlayer[1] = ecs.LinkIntent{TargetID: 8}
			ecs.GetResource[ecs.PendingAdminDestroy](f.world).Set(1)
			f.send(1, 0, 1, 0)
			active, exists := ecs.GetComponent[components.ActiveGameAction](f.world, f.player)
			if (phase == components.GameActionSelecting) != exists || (exists && active.Generation != 9) {
				t.Fatal("wrong action phase outcome")
			}
			wantCancels := 1
			if phase == components.GameActionSelecting {
				wantCancels = 0
			}
			f.advance(200 * time.Millisecond)
			f.send(1, 0, 1, 0)
			f.send(0, 0, 2, 0)
			if router.cancelCalls != wantCancels {
				t.Fatalf("canceled %d times", router.cancelCalls)
			}
			if _, exists := ecs.GetComponent[components.PendingInteraction](f.world, f.player); exists {
				t.Fatal("pickup survived")
			}
			if _, exists := ecs.GetComponent[components.PendingContextAction](f.world, f.player); exists {
				t.Fatal("context intent survived")
			}
			if _, exists := links.IntentByPlayer[1]; exists {
				t.Fatal("link intent survived")
			}
			if !ecs.GetResource[ecs.PendingAdminDestroy](f.world).Get(1) {
				t.Fatal("WASD consumed administrator selection")
			}
		})
	}
}

func TestDirectionModesAndRestrictionsMatchClickMovement(t *testing.T) {
	for _, mode := range []constt.MoveMode{constt.Crawl, constt.Walk, constt.Run, constt.FastRun, constt.Swim} {
		for _, condition := range []string{"normal", "carry", "low-stamina", "overstuffed", "exhausted", "stun", "ko", "lying"} {
			f := newDirectionalFixture(t)
			ecs.AddComponent(f.world, f.player, components.EntityStats{Stamina: 1000, Energy: 1000})
			ecs.WithComponent(f.world, f.player, func(m *components.Movement) { m.Mode = mode })
			f.send(1, 0, 1, 0)
			expected := (&components.Movement{Mode: mode, Speed: 32}).GetCurrentSpeed()
			switch condition {
			case "carry":
				ecs.AddComponent(f.world, f.player, components.LiftCarryState{})
				if mode >= constt.Run {
					expected = 32
				}
			case "low-stamina":
				ecs.WithComponent(f.world, f.player, func(s *components.EntityStats) { s.Stamina = 75 })
				expected = 16
			case "overstuffed":
				ecs.WithComponent(f.world, f.player, func(s *components.EntityStats) { s.Energy = 1001 })
				expected = 16
			case "exhausted":
				ecs.WithComponent(f.world, f.player, func(s *components.EntityStats) { s.Stamina = 0 })
				expected = 0
			case "stun":
				ecs.WithComponent(f.world, f.player, func(m *components.Movement) { m.State = constt.StateStunned })
				expected = 0
			case "ko":
				ecs.AddComponent(f.world, f.player, components.EntityHealth{HHP: 100, SHP: 0, KOUntilUnixMs: 60_000, IsLying: true})
				expected = 0
			case "lying":
				ecs.AddComponent(f.world, f.player, components.EntityHealth{HHP: 100, SHP: 5, IsLying: true})
				expected = 0
			}
			f.movement.Update(f.world, .1)
			moves := ecs.GetResource[ecs.MovedEntities](f.world)
			if moves.Count != 1 || math.Abs(moves.IntentX[0]-100-expected*.1) > 1e-9 {
				t.Fatalf("mode %v condition %s: count=%d x=%v expectedStep=%v", mode, condition, moves.Count, moves.IntentX[:moves.Count], expected*.1)
			}
			if expected == 0 && f.state().TargetType != constt.TargetNone {
				t.Fatal("restricted hold retained")
			}
		}
	}
}

func TestDirectionPopulationInputQueueBounded(t *testing.T) {
	const population = 200
	world := ecs.NewWorldForTesting()
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 500, MaxPacketsPerSecond: 40, MaxCommandsPerTickPerClient: 20})
	commands := NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 500}), nil, nil, nil, nil, nil, 0, zap.NewNop())
	commands.SetDirectionalSessionValidator(func(id types.EntityID, client uint64, epoch uint32) bool { return uint64(id) == client && epoch == 1 })
	for id := 1; id <= population; id++ {
		world.Spawn(types.EntityID(id), func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Movement{Speed: 32, Mode: constt.Walk})
			ecs.AddComponent(w, h, components.Transform{})
		})
	}
	mover := NewMovementSystem(world, nil, zap.NewNop())
	sequence := uint64(0)
	for tick := 0; tick <= 20; tick++ {
		now := time.Unix(100, 0).Add(time.Duration(tick) * 100 * time.Millisecond)
		ecs.SetResource(world, ecs.TimeState{Now: now, WallNow: now})
		if tick%2 == 0 {
			sequence++
			revision := uint32(1)
			directionX := float32(1)
			if tick == 20 {
				revision = 2
				directionX = 0
			}
			for id := 1; id <= population; id++ {
				if err := inbox.Enqueue(&network.PlayerCommand{ClientID: uint64(id), CharacterID: types.EntityID(id), CommandID: sequence, CommandType: network.CmdMoveDirection, ReceivedAt: now, Payload: &netproto.MoveDirection{X: directionX, InputRevision: revision, StreamEpoch: 1}}); err != nil {
					t.Fatal(err)
				}
			}
		}
		commands.Update(world, .1)
		if pending := inbox.Drain(); len(pending) != 0 {
			t.Fatalf("queue backlog at tick %d: %d", tick, len(pending))
		}
		ecs.GetResource[ecs.MovedEntities](world).Count = 0
		mover.Update(world, .1)
		if count := ecs.GetResource[ecs.MovedEntities](world).Count; count != population {
			t.Fatalf("tick %d movement entries=%d", tick, count)
		}
	}
	received, dropped, processed := inbox.Stats()
	if dropped != 0 || received != 2200 || processed != received {
		t.Fatalf("input accounting: %d/%d/%d", received, dropped, processed)
	}
	t.Logf("200 players, 10Hz, 2s: input=%d dropped=%d processed=%d, max enqueued burst=200, backlog after each drain=0", received, dropped, processed)
}
