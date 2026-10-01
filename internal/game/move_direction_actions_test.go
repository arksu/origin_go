package game

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/game/behaviors"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

func directionalTestCommands(w *ecs.World, actions *ActionService) (*systems.NetworkCommandSystem, *network.PlayerCommandInbox) {
	inbox := network.NewPlayerCommandInbox(network.CommandQueueConfig{MaxQueueSize: 20, MaxPacketsPerSecond: 40, MaxCommandsPerTickPerClient: 20})
	commands := systems.NewNetworkCommandSystem(inbox, network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20}), nil, nil, nil, nil, 0, zap.NewNop())
	commands.SetDirectionalSessionValidator(func(id types.EntityID, client uint64, epoch uint32) bool { return id == 1 && client == 1 && epoch == 1 })
	commands.SetActionService(actions)
	ecs.SetResource(w, ecs.TimeState{Now: time.Unix(100, 0), WallNow: time.Unix(10000, 0)})
	return commands, inbox
}

func TestDirectionCarryFollowsAcrossChunkBoundary(t *testing.T) {
	w, lift, bus, received := newLiftFollowTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, coord := range []types.ChunkCoord{{}, {X: 1}} {
		if err := lift.chunkManager.WaitPreloaded(ctx, coord); err != nil {
			t.Fatal(err)
		}
		lift.chunkManager.GetChunk(coord).SetState(types.ChunkStateActive)
	}
	start := float64(constt.ChunkWorldSize) - 1
	player, object := spawnFollowPair(w, 1, 11, start, start)
	ecs.AddComponent(w, player, components.ChunkRef{})
	ecs.AddComponent(w, player, components.EntityInfo{})
	ecs.AddComponent(w, player, components.CollisionResult{})
	ecs.AddComponent(w, player, components.EntityStats{Stamina: 500, Energy: 1000})
	movement := components.Movement{Speed: 32, Mode: constt.Run}
	movement.SetDirection(1, 0, 1, ecs.GetResource[ecs.TimeState](w).Now.Add(time.Second))
	ecs.AddComponent(w, player, movement)
	source := lift.chunkManager.GetChunk(types.ChunkCoord{})
	source.Spatial().AddDynamic(player, int(start), 40)
	source.Spatial().AddDynamic(object, int(start), 40)
	systems.NewMovementSystem(w, lift.chunkManager, zap.NewNop()).Update(w, .1)
	systems.NewCollisionSystem(w, lift.chunkManager, zap.NewNop(), 0, 2*constt.ChunkWorldSize, 0, 2*constt.ChunkWorldSize, 0).Update(w, .1)
	systems.NewTransformUpdateSystem(w, lift.chunkManager, bus, zap.NewNop()).Update(w, .1)
	systems.NewChunkSystem(lift.chunkManager, zap.NewNop()).Update(w, .1)
	systems.NewLiftCarryFollowSystem(w, lift, bus, zap.NewNop()).Update(w, .1)
	playerPosition, _ := ecs.GetComponent[components.Transform](w, player)
	objectPosition, _ := ecs.GetComponent[components.Transform](w, object)
	playerChunk, _ := ecs.GetComponent[components.ChunkRef](w, player)
	objectChunk, _ := ecs.GetComponent[components.ChunkRef](w, object)
	if playerPosition.X <= start || playerPosition.X != objectPosition.X || playerPosition.Y != objectPosition.Y || playerChunk.CurrentChunkX != 1 || objectChunk.CurrentChunkX != 1 {
		t.Fatalf("carry did not follow: player=%+v object=%+v chunks=%+v/%+v", playerPosition, objectPosition, playerChunk, objectChunk)
	}
	events := drainLiftFollowEvents(t, bus, received)
	if len(events) != 1 || len(events[0].Entries) != 1 || events[0].Entries[0].CarriedByEntityID != 1 {
		t.Fatalf("wrong carry broadcast: %+v", events)
	}
}

func enqueueTestDirection(t *testing.T, w *ecs.World, inbox *network.PlayerCommandInbox, revision uint32, sequences ...uint64) {
	t.Helper()
	sequence := uint64(revision)
	if len(sequences) > 0 {
		sequence = sequences[0]
	}
	if err := inbox.Enqueue(&network.PlayerCommand{ClientID: 1, CharacterID: 1, CommandID: sequence, CommandType: network.CmdMoveDirection, ReceivedAt: ecs.GetResource[ecs.TimeState](w).WallNow, Payload: &netproto.MoveDirection{X: 1, InputRevision: revision, StreamEpoch: 1}}); err != nil {
		t.Fatal(err)
	}
}

func TestDirectionCancelsLastActionTickAndRejectsLateCompletion(t *testing.T) {
	w, player, service, handler, sender := newRepeatActionTest(t, true, false, 0)
	first, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	service.AdvanceCycle(w, 1, player, first, sender)
	stale, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	commands, inbox := directionalTestCommands(w, service)
	enqueueTestDirection(t, w, inbox, 1)
	commands.Update(w, .1)
	service.AdvanceCycle(w, 1, player, stale, sender)
	stats, _ := ecs.GetComponent[components.EntityStats](w, player)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	if handler.effects != 0 || stats.Stamina != 1000 || movement.TargetType != constt.TargetDirection || len(sender.finished) != 1 || service.State(w, player).Phase != "idle" {
		t.Fatal("manual movement did not cancel unfinished cycle")
	}
	service.Activate(w, 1, player, "repeat_test")
	service.HandleArmedClick(w, 1, player, 0, 0, 1, 1)
	current, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	service.AdvanceCycle(w, 1, player, stale, sender)
	after, _ := ecs.GetComponent[components.ActiveCyclicAction](w, player)
	if after != current || handler.effects != 0 {
		t.Fatal("late completion affected replacement")
	}
}

func TestDirectionCancelsContextAndLinklessCraft(t *testing.T) {
	for _, craft := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting, Speed: 32, Mode: constt.Walk})
			ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
			ecs.AddComponent(w, h, components.ActiveCyclicAction{BehaviorKey: "tree", ActionID: "chop", CycleIndex: 3})
			if craft {
				ecs.AddComponent(w, h, components.ActiveCraft{CraftKey: "test", RemainingCycles: 1})
			}
		})
		links := ecs.GetResource[ecs.LinkState](w)
		if !craft {
			links.SetLink(ecs.PlayerLink{PlayerID: 1, PlayerHandle: player, TargetID: 2})
		}
		sender := &testCyclicActionFinishSender{}
		contextActions := NewContextActionService(w, nil, nil, nil, nil, sender, nil, nil, nil, behaviors.MustDefaultRegistry(), zap.NewNop())
		commands, inbox := directionalTestCommands(w, nil)
		commands.SetManualMovementCanceler(contextActions)
		enqueueTestDirection(t, w, inbox, 1)
		commands.Update(w, .1)
		if _, exists := ecs.GetComponent[components.ActiveCyclicAction](w, player); exists {
			t.Fatal("cycle survived")
		}
		if _, exists := ecs.GetComponent[components.ActiveCraft](w, player); exists {
			t.Fatal("craft survived")
		}
		if _, exists := links.GetLink(1); exists {
			t.Fatal("active link survived")
		}
		movement, _ := ecs.GetComponent[components.Movement](w, player)
		if movement.TargetType != constt.TargetDirection || movement.State != constt.StateMoving || len(sender.messages) != 1 || sender.messages[0].Result != netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED {
			t.Fatal("cleanup callback clobbered direction or cancellation missing")
		}
	}
}

func TestDirectionCancelsLiftDownAndKeepsCarry(t *testing.T) {
	fixture := newRMBLiftTest(t)
	pending := fixture.click(t, 0, 1043, 1087)
	fixture.commands.SetDirectionalSessionValidator(func(types.EntityID, uint64, uint32) bool { return true })
	timing := ecs.GetResource[ecs.TimeState](fixture.world)
	// Existing fixture uses zero TimeState and receipt, keeping both time domains aligned.
	if !timing.WallNow.IsZero() {
		t.Fatal("unexpected fixture clock")
	}
	fixture.send(t, network.CmdMoveDirection, &netproto.MoveDirection{X: 1, InputRevision: 1, StreamEpoch: 1})
	fixture.actions.Complete(fixture.world, 1, fixture.player, pending.ActionGeneration, true, "")
	if _, exists := ecs.GetComponent[components.PendingLiftTransition](fixture.world, fixture.player); exists {
		t.Fatal("placement survived")
	}
	carry, exists := ecs.GetComponent[components.LiftCarryState](fixture.world, fixture.player)
	collider, _ := ecs.GetComponent[components.Collider](fixture.world, fixture.player)
	movement, _ := ecs.GetComponent[components.Movement](fixture.world, fixture.player)
	if !exists || carry.ObjectHandle != fixture.object || collider.Phantom != nil || movement.TargetType != constt.TargetDirection {
		t.Fatalf("invalid carry after cancel: %+v", carry)
	}
}

func TestDirectionPreservesSelectionAndCancelsTileApproach(t *testing.T) {
	for _, approaching := range []bool{false, true} {
		w, player, actions, handler, _ := newTileApproachTest(t)
		if approaching {
			actions.HandleArmedClick(w, 1, player, 0, 0, 1000, 1000)
		}
		commands, inbox := directionalTestCommands(w, actions)
		enqueueTestDirection(t, w, inbox, 1)
		commands.Update(w, .1)
		actions.Recheck(w, 1, player)
		wantPhase := "selecting"
		if approaching {
			wantPhase = "idle"
		}
		stats, _ := ecs.GetComponent[components.EntityStats](w, player)
		movement, _ := ecs.GetComponent[components.Movement](w, player)
		if actions.State(w, player).Phase != wantPhase || handler.startCount != 0 || stats.Stamina != 250 || movement.TargetType != constt.TargetDirection {
			t.Fatalf("invalid tile handoff: %+v %+v", actions.State(w, player), movement)
		}
	}
}

func TestDirectionCancelsLiftApproachAndLateFinalization(t *testing.T) {
	w, player, object, lift, actions, _ := newNoColliderLiftActionTest(t)
	actions.Activate(w, 1, player, "lift")
	actions.HandleArmedClick(w, 1, player, 3, object, 1000, 1000)
	pending, exists := ecs.GetComponent[components.PendingLiftTransition](w, player)
	if !exists {
		t.Fatal("lift approach not established")
	}
	commands, inbox := directionalTestCommands(w, actions)
	enqueueTestDirection(t, w, inbox, 1)
	commands.Update(w, .1)
	lift.FinalizePendingLiftTransition(w, 1, player, pending)
	if _, exists := ecs.GetComponent[components.PendingLiftTransition](w, player); exists {
		t.Fatal("lift approach survived manual input")
	}
	if _, exists := ecs.GetComponent[components.LiftCarryState](w, player); exists {
		t.Fatal("late finalization picked up object")
	}
	position, _ := ecs.GetComponent[components.Transform](w, object)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	if position.X != 1000 || position.Y != 1000 || movement.TargetType != constt.TargetDirection || actions.State(w, player).Phase != "idle" {
		t.Fatal("canceled lift affected object or movement")
	}
}
