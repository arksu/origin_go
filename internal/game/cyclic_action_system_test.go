package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type testCyclicActionProgressSender struct {
	messages []*netproto.S2C_CyclicActionProgress
	entityID types.EntityID
}

func (s *testCyclicActionProgressSender) SendCyclicActionProgress(entityID types.EntityID, progress *netproto.S2C_CyclicActionProgress) {
	s.entityID = entityID
	s.messages = append(s.messages, progress)
}

func TestCyclicActionSystem_CancelsWhenLinkMissing(t *testing.T) {
	world := ecs.NewWorldForTesting()
	contextActionService := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil)
	system := NewCyclicActionSystem(contextActionService, nil, nil)

	playerID := types.EntityID(1001)
	targetID := types.EntityID(2002)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			BehaviorKey:        "tree",
			ActionID:           contextActionChop,
			TargetKind:         components.CyclicActionTargetObject,
			TargetID:           targetID,
			CycleDurationTicks: 20,
			CycleIndex:         1,
		})
	})

	linkState := ecs.GetResource[ecs.LinkState](world)
	linkState.SetLink(ecs.PlayerLink{
		PlayerID: playerID,
		TargetID: targetID,
	})

	system.Update(world, 0.05)

	actionAfterFirstTick, hasAction := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle)
	if !hasAction {
		t.Fatalf("expected active cyclic action after first tick with valid link")
	}
	if actionAfterFirstTick.CycleElapsedTicks != 1 {
		t.Fatalf("expected cycle progress 1 tick, got %d", actionAfterFirstTick.CycleElapsedTicks)
	}

	linkState.RemoveLink(playerID)
	system.Update(world, 0.05)

	if _, stillHasAction := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle); stillHasAction {
		t.Fatalf("expected active cyclic action to be canceled after link removal")
	}
	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	if !hasMovement {
		t.Fatalf("expected movement component")
	}
	if movement.State != constt.StateIdle {
		t.Fatalf("expected movement state idle after cancel, got %d", movement.State)
	}
}

func TestCyclicActionSystem_SendProgress_IncludesSoundOnCycleEnd(t *testing.T) {
	progressSender := &testCyclicActionProgressSender{}
	system := NewCyclicActionSystem(nil, progressSender, nil)

	const (
		playerID = types.EntityID(5001)
		targetID = types.EntityID(6002)
	)

	action := components.ActiveCyclicAction{
		ActionID:           contextActionChop,
		TargetID:           targetID,
		CycleIndex:         2,
		CycleElapsedTicks:  20,
		CycleDurationTicks: 20,
		FinishSoundKey:     "chop",
	}
	system.sendProgress(playerID, action)

	if len(progressSender.messages) != 1 {
		t.Fatalf("expected one progress message, got %d", len(progressSender.messages))
	}

	message := progressSender.messages[0]
	if message.SoundKey == nil || *message.SoundKey != "chop" {
		t.Fatalf("expected progress sound_key=chop, got %v", message.SoundKey)
	}
}
