package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type testCyclicActionFinishSender struct {
	messages []*netproto.S2C_CyclicActionFinished
	entityID types.EntityID
}

func (s *testCyclicActionFinishSender) SendCyclicActionFinished(entityID types.EntityID, finished *netproto.S2C_CyclicActionFinished) {
	s.entityID = entityID
	s.messages = append(s.messages, finished)
}

func TestContextActionService_CancelActiveCyclicAction_SendsCanceled(t *testing.T) {
	world := ecs.NewWorldForTesting()
	sender := &testCyclicActionFinishSender{}
	service := NewContextActionService(world, nil, nil, nil, sender, nil, nil, nil, nil)

	const (
		playerID = types.EntityID(1001)
		targetID = types.EntityID(2002)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID:    contextActionChop,
			TargetID:    targetID,
			CycleIndex:  3,
			BehaviorKey: "tree",
		})
	})

	service.cancelActiveCyclicAction(playerID, playerHandle, "link_broken")

	if _, has := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle); has {
		t.Fatalf("expected active cyclic action to be removed")
	}

	movement, hasMovement := ecs.GetComponent[components.Movement](world, playerHandle)
	if !hasMovement {
		t.Fatalf("expected movement component to exist")
	}
	if movement.State != constt.StateIdle {
		t.Fatalf("expected movement state to be idle, got %d", movement.State)
	}

	if len(sender.messages) != 1 {
		t.Fatalf("expected exactly one finished message, got %d", len(sender.messages))
	}
	message := sender.messages[0]
	if sender.entityID != playerID {
		t.Fatalf("expected finished message for player %d, got %d", playerID, sender.entityID)
	}
	if message.Result != netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED {
		t.Fatalf("expected canceled result, got %v", message.Result)
	}
	if message.ReasonCode == nil || *message.ReasonCode != "link_broken" {
		t.Fatalf("expected canceled reason_code=link_broken, got %v", message.ReasonCode)
	}
}

func TestContextActionService_CompleteActiveCyclicAction_SendsCompleted(t *testing.T) {
	world := ecs.NewWorldForTesting()
	sender := &testCyclicActionFinishSender{}
	service := NewContextActionService(world, nil, nil, nil, sender, nil, nil, nil, nil)

	const (
		playerID = types.EntityID(3003)
		targetID = types.EntityID(4004)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID:    contextActionChop,
			TargetID:    targetID,
			CycleIndex:  6,
			BehaviorKey: "tree",
		})
	})

	service.completeActiveCyclicAction(playerID, playerHandle)

	if _, has := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle); has {
		t.Fatalf("expected active cyclic action to be removed")
	}

	if len(sender.messages) != 1 {
		t.Fatalf("expected exactly one finished message, got %d", len(sender.messages))
	}
	message := sender.messages[0]
	if message.Result != netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_COMPLETED {
		t.Fatalf("expected completed result, got %v", message.Result)
	}
	if message.ReasonCode != nil {
		t.Fatalf("expected no reason_code for completed result, got %v", *message.ReasonCode)
	}
}
