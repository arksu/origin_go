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
	contextActionService := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	system := NewCyclicActionSystem(contextActionService, nil, nil)

	playerID := types.EntityID(1001)
	targetID := types.EntityID(2002)
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			BehaviorKey:        "tree",
			ActionID:           testActionChop,
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

func TestCyclicActionSystem_SendProgress_NoSoundKey(t *testing.T) {
	progressSender := &testCyclicActionProgressSender{}
	system := NewCyclicActionSystem(nil, progressSender, nil)

	const (
		playerID = types.EntityID(5001)
		targetID = types.EntityID(6002)
	)

	action := components.ActiveCyclicAction{
		ActionID:           testActionChop,
		TargetID:           targetID,
		CycleIndex:         2,
		CycleElapsedTicks:  20,
		CycleDurationTicks: 20,
		CycleSoundKey:      "chop",
	}
	system.sendProgress(playerID, action)

	if len(progressSender.messages) != 1 {
		t.Fatalf("expected one progress message, got %d", len(progressSender.messages))
	}

	message := progressSender.messages[0]
	if message.ActionId != testActionChop || message.CycleIndex != 2 {
		t.Fatalf("unexpected progress payload: %+v", message)
	}
}

func TestCyclicActionSystem_EmitsCycleSoundToVisibleObservers(t *testing.T) {
	world := ecs.NewWorldForTesting()
	soundSender := &testSoundEventSender{}
	contextActionService := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	contextActionService.SetSoundEventSender(soundSender)
	system := NewCyclicActionSystem(contextActionService, nil, nil)

	const (
		playerID   = types.EntityID(7001)
		targetID   = types.EntityID(8002)
		observerID = types.EntityID(9003)
	)

	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 300, Y: 400})
	})
	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			BehaviorKey:        "missing_behavior",
			ActionID:           testActionChop,
			TargetKind:         components.CyclicActionTargetObject,
			TargetID:           targetID,
			TargetHandle:       targetHandle,
			CycleDurationTicks: 1,
			CycleIndex:         1,
			CycleSoundKey:      "chop",
		})
	})
	observerHandle := world.Spawn(observerID, nil)

	visibilityState := ecs.GetResource[ecs.VisibilityState](world)
	visibilityState.ObserversByVisibleTarget[targetHandle] = map[types.Handle]struct{}{
		playerHandle:   {},
		observerHandle: {},
	}

	linkState := ecs.GetResource[ecs.LinkState](world)
	linkState.SetLink(ecs.PlayerLink{
		PlayerID: playerID,
		TargetID: targetID,
	})

	system.Update(world, 0.05)

	if len(soundSender.messages[playerID]) != 1 {
		t.Fatalf("expected cycle sound for player observer, got %d", len(soundSender.messages[playerID]))
	}
	if len(soundSender.messages[observerID]) != 1 {
		t.Fatalf("expected cycle sound for secondary observer, got %d", len(soundSender.messages[observerID]))
	}
	sound := soundSender.messages[playerID][0]
	if sound.SoundKey != "chop" || sound.X != 300 || sound.Y != 400 {
		t.Fatalf("unexpected cycle sound payload: %+v", sound)
	}
}
