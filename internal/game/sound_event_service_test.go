package game

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	netproto "origin/internal/network/proto"
	"origin/internal/types"
)

type testSoundSender struct {
	messages map[types.EntityID][]*netproto.S2C_Sound
}

func (s *testSoundSender) SendSound(entityID types.EntityID, sound *netproto.S2C_Sound) {
	if s.messages == nil {
		s.messages = make(map[types.EntityID][]*netproto.S2C_Sound)
	}
	s.messages[entityID] = append(s.messages[entityID], sound)
}

func TestSoundEventService_EmitForVisibleTarget(t *testing.T) {
	world := ecs.NewWorldForTesting()
	sender := &testSoundSender{}
	service := NewSoundEventService(sender, nil)

	const (
		targetID    = types.EntityID(1001)
		observerAID = types.EntityID(2002)
		observerBID = types.EntityID(3003)
	)

	targetHandle := world.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 120.5, Y: -42.25})
	})
	observerAHandle := world.Spawn(observerAID, nil)
	observerBHandle := world.Spawn(observerBID, nil)

	visibilityState := ecs.GetResource[ecs.VisibilityState](world)
	visibilityState.ObserversByVisibleTarget[targetHandle] = map[types.Handle]struct{}{
		observerAHandle: {},
		observerBHandle: {},
	}

	service.EmitForVisibleTarget(world, targetHandle, "tree_fall")

	if len(sender.messages[observerAID]) != 1 {
		t.Fatalf("expected one sound for observer A, got %d", len(sender.messages[observerAID]))
	}
	if len(sender.messages[observerBID]) != 1 {
		t.Fatalf("expected one sound for observer B, got %d", len(sender.messages[observerBID]))
	}

	payload := sender.messages[observerAID][0]
	if payload.SoundKey != "tree_fall" {
		t.Fatalf("unexpected sound key: %q", payload.SoundKey)
	}
	if payload.X != 120.5 || payload.Y != -42.25 {
		t.Fatalf("unexpected sound position: x=%f y=%f", payload.X, payload.Y)
	}
	if payload.MaxHearDistance != constt.DefaultMaxHearDistance {
		t.Fatalf("unexpected max hear distance: %f", payload.MaxHearDistance)
	}
}

func TestSoundEventService_EmitForVisibleTarget_SkipsWhenNoObservers(t *testing.T) {
	world := ecs.NewWorldForTesting()
	sender := &testSoundSender{}
	service := NewSoundEventService(sender, nil)

	targetHandle := world.Spawn(types.EntityID(4004), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 1, Y: 2})
	})

	service.EmitForVisibleTarget(world, targetHandle, "chop")

	if len(sender.messages) != 0 {
		t.Fatalf("expected no sound messages, got %+v", sender.messages)
	}
}
