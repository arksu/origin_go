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

type testSoundEventSender struct {
	messages map[types.EntityID][]*netproto.S2C_Sound
}

const testActionChop = "chop"

func (s *testSoundEventSender) SendSound(entityID types.EntityID, sound *netproto.S2C_Sound) {
	if s.messages == nil {
		s.messages = make(map[types.EntityID][]*netproto.S2C_Sound)
	}
	s.messages[entityID] = append(s.messages[entityID], sound)
}

func TestContextActionService_CancelActiveCyclicAction_SendsCanceled(t *testing.T) {
	world := ecs.NewWorldForTesting()
	sender := &testCyclicActionFinishSender{}
	service := NewContextActionService(world, nil, nil, nil, sender, nil, nil, nil, nil, nil)

	const (
		playerID = types.EntityID(1001)
		targetID = types.EntityID(2002)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID:         testActionChop,
			TargetID:         targetID,
			CycleIndex:       3,
			BehaviorKey:      "tree",
			CycleSoundKey:    "chop",
			CompleteSoundKey: "tree_fall",
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
	service := NewContextActionService(world, nil, nil, nil, sender, nil, nil, nil, nil, nil)

	const (
		playerID = types.EntityID(3003)
		targetID = types.EntityID(4004)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID:         testActionChop,
			TargetID:         targetID,
			CycleIndex:       6,
			BehaviorKey:      "tree",
			CycleSoundKey:    "chop",
			CompleteSoundKey: "tree_fall",
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

func TestContextActionService_CompleteActiveCyclicAction_EmitsCompleteSoundToVisibleObservers(t *testing.T) {
	world := ecs.NewWorldForTesting()
	sender := &testCyclicActionFinishSender{}
	soundSender := &testSoundEventSender{}
	service := NewContextActionService(world, nil, nil, nil, sender, nil, nil, nil, nil, nil)
	service.SetSoundEventSender(soundSender)

	const (
		playerID   = types.EntityID(7001)
		targetID   = types.EntityID(8002)
		observerID = types.EntityID(9003)
	)

	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 100, Y: 200})
	})
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID:         testActionChop,
			TargetID:         targetID,
			TargetHandle:     targetHandle,
			TargetKind:       components.CyclicActionTargetObject,
			CycleIndex:       6,
			BehaviorKey:      "tree",
			CompleteSoundKey: "tree_fall",
		})
	})
	observerHandle := world.Spawn(observerID, nil)

	visibilityState := ecs.GetResource[ecs.VisibilityState](world)
	visibilityState.ObserversByVisibleTarget[targetHandle] = map[types.Handle]struct{}{
		playerHandle:   {},
		observerHandle: {},
	}

	service.completeActiveCyclicAction(playerID, playerHandle)

	if len(soundSender.messages[playerID]) != 1 {
		t.Fatalf("expected complete sound for player observer, got %d", len(soundSender.messages[playerID]))
	}
	if len(soundSender.messages[observerID]) != 1 {
		t.Fatalf("expected complete sound for secondary observer, got %d", len(soundSender.messages[observerID]))
	}
	sound := soundSender.messages[playerID][0]
	if sound.SoundKey != "tree_fall" || sound.X != 100 || sound.Y != 200 {
		t.Fatalf("unexpected sound payload: %+v", sound)
	}
}
