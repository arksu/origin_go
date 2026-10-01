package game

import (
	"testing"
	"time"

	"go.uber.org/zap"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/timeutil"
	"origin/internal/types"
)

func TestDirectionQueuedSessionValidationAndFreshEpoch(t *testing.T) {
	for _, transition := range []string{"disconnect", "replacement", "epoch", "detach", "dead"} {
		t.Run(transition, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.Movement{Speed: 32, Mode: constt.Walk})
			})
			client := &network.Client{ID: 1, CharacterID: 1}
			client.InWorld.Store(true)
			client.StreamEpoch.Store(1)
			shard := &Shard{Clients: map[types.EntityID]*network.Client{1: client}}
			commands, inbox := directionalTestCommands(w, nil)
			commands.SetDirectionalSessionValidator(shard.validDirectionalSession)
			enqueueTestDirection(t, w, inbox, 1)
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
				ecs.GetResource[ecs.DetachedEntities](w).AddDetachedEntity(1, player, time.Time{}, time.Time{})
			case "dead":
				ecs.AddComponent(w, player, components.EntityHealth{HHP: 0})
			}
			commands.Update(w, .1)
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			if movement.TargetType != constt.TargetNone || movement.Direction.Revision != 0 {
				t.Fatal("queued obsolete session changed state")
			}
		})
	}
	w := ecs.NewWorldForTesting()
	player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{Speed: 32, Mode: constt.Walk})
	})
	client := &network.Client{ID: 1, CharacterID: 1}
	client.InWorld.Store(true)
	client.StreamEpoch.Store(1)
	shard := &Shard{Clients: map[types.EntityID]*network.Client{1: client}}
	commands, inbox := directionalTestCommands(w, nil)
	commands.SetDirectionalSessionValidator(shard.validDirectionalSession)
	enqueueTestDirection(t, w, inbox, 100)
	commands.Update(w, .1)
	client.StreamEpoch.Store(2)
	if err := inbox.Enqueue(&network.PlayerCommand{ClientID: 1, CharacterID: 1, CommandID: 101, CommandType: network.CmdMoveDirection, ReceivedAt: ecs.GetResource[ecs.TimeState](w).WallNow, Payload: &netproto.MoveDirection{Y: 1, InputRevision: 1, StreamEpoch: 2}}); err != nil {
		t.Fatal(err)
	}
	commands.Update(w, .1)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	if movement.TargetType != constt.TargetDirection || movement.Direction.Revision != 1 || movement.Direction.StreamEpoch != 2 || movement.Direction.Y != 1 {
		t.Fatal("new epoch cannot establish fresh input")
	}
}

func TestDirectionTransferDetachAndReattachRejectOldHold(t *testing.T) {
	// Transfer, teleport and rollback share detachTransferSource/attachClientToWorld.
	shard, _ := newPlayerSpawnTestShard(t, 32)
	w := shard.world
	client, _ := connectPlayerSpawnTestClient(t)
	client.CharacterID = 1
	client.InWorld.Store(true)
	client.StreamEpoch.Store(1)
	shard.Clients[1] = client
	commands, inbox := directionalTestCommands(w, nil)
	shard.playerInbox = inbox
	shard.serverInbox = network.NewServerJobInbox(network.CommandQueueConfig{MaxQueueSize: 20})
	commands.SetDirectionalSessionValidator(shard.validDirectionalSession)
	spawn := func() types.Handle {
		return w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Transform{X: 100, Y: 100})
			ecs.AddComponent(w, h, components.Movement{Speed: 32, Mode: constt.Walk})
		})
	}
	player := spawn()
	var sequence uint64
	send := func(revision, epoch uint32) {
		t.Helper()
		sequence++
		if err := inbox.Enqueue(&network.PlayerCommand{ClientID: client.ID, CharacterID: 1, CommandID: sequence, CommandType: network.CmdMoveDirection,
			ReceivedAt: ecs.GetResource[ecs.TimeState](w).WallNow, Payload: &netproto.MoveDirection{X: 1, InputRevision: revision, StreamEpoch: epoch}}); err != nil {
			t.Fatal(err)
		}
	}
	send(100, 1)
	commands.Update(w, .1)
	send(100, 1)
	g := &Game{cfg: shard.cfg, logger: zap.NewNop(), clock: timeutil.NewManualClock(time.Unix(10000, 0))}
	transfer := NewPlayerTransferService(g, zap.NewNop())
	snapshot, err := transfer.detachTransferSource(PlayerTransferRequest{PlayerID: 1}, shard, repository.Character{ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	commands.Update(w, .1)
	if w.Alive(player) || client.InWorld.Load() || len(snapshot.ParticipantStates) != 0 {
		t.Fatal("source direction survived detach")
	}
	player = spawn()
	g.attachClientToWorld(shard, snapshot.Client, 1, snapshot.Character, player)
	if client.StreamEpoch.Load() != 2 {
		t.Fatal("reattach did not rotate stream epoch")
	}
	send(101, 1)
	commands.Update(w, .1)
	movement, _ := ecs.GetComponent[components.Movement](w, player)
	if movement.TargetType != constt.TargetNone || movement.Direction.Revision != 0 {
		t.Fatal("old epoch restarted movement after respawn")
	}
	send(1, 2)
	commands.Update(w, .1)
	movement, _ = ecs.GetComponent[components.Movement](w, player)
	if movement.TargetType != constt.TargetDirection || movement.Direction.Revision != 1 {
		t.Fatal("fresh keypress rejected after reattach")
	}
}

func TestDirectionKnockoutAndDetachCleanupRetireHold(t *testing.T) {
	for _, knockout := range []bool{false, true} {
		w := ecs.NewWorldForTesting()
		player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
			ecs.AddComponent(w, h, components.Movement{Speed: 32, Mode: constt.Walk})
		})
		commands, inbox := directionalTestCommands(w, nil)
		enqueueTestDirection(t, w, inbox, 1)
		commands.Update(w, .1)
		if knockout {
			applyStunnedStateAndClearActions(w, player)
			movement, _ := ecs.GetComponent[components.Movement](w, player)
			if movement.State != constt.StateStunned {
				t.Fatal("cleanup removed stun")
			}
			clearStunnedState(w, player)
		} else {
			systems.StopMovementForDetached(w, player)
		}
		enqueueTestDirection(t, w, inbox, 1, 2)
		commands.Update(w, .1)
		movement, _ := ecs.GetComponent[components.Movement](w, player)
		if movement.TargetType != constt.TargetNone || movement.Direction.Revision != 1 {
			t.Fatal("retired heartbeat restarted movement")
		}
		enqueueTestDirection(t, w, inbox, 2, 3)
		commands.Update(w, .1)
		movement, _ = ecs.GetComponent[components.Movement](w, player)
		if movement.TargetType != constt.TargetDirection {
			t.Fatal("fresh input rejected after recovery")
		}
	}
}
