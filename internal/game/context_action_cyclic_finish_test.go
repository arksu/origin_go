package game

import (
	"context"
	"fmt"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
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

type testRetargetBehavior struct {
	actionID         string
	executeCount     int
	executedTargetID types.EntityID
}

func (b *testRetargetBehavior) Key() string {
	return "test_retarget_behavior"
}

func (b *testRetargetBehavior) ProvideActions(_ *contracts.BehaviorActionListContext) []contracts.ContextAction {
	return []contracts.ContextAction{
		{
			ActionID: b.actionID,
			Title:    "Retarget",
		},
	}
}

func (b *testRetargetBehavior) ExecuteAction(ctx *contracts.BehaviorActionExecuteContext) contracts.BehaviorResult {
	b.executeCount++
	b.executedTargetID = ctx.TargetID
	return contracts.BehaviorResult{OK: true}
}

type testSingleBehaviorRegistry struct {
	behavior contracts.Behavior
}

func (r testSingleBehaviorRegistry) GetBehavior(key string) (contracts.Behavior, bool) {
	if r.behavior == nil || key != r.behavior.Key() {
		return nil, false
	}
	return r.behavior, true
}

func (r testSingleBehaviorRegistry) Keys() []string {
	if r.behavior == nil {
		return nil
	}
	return []string{r.behavior.Key()}
}

func (r testSingleBehaviorRegistry) IsRegisteredBehaviorKey(key string) bool {
	_, ok := r.GetBehavior(key)
	return ok
}

func (r testSingleBehaviorRegistry) ValidateBehaviorKeys(keys []string) error {
	for _, key := range keys {
		if !r.IsRegisteredBehaviorKey(key) {
			return fmt.Errorf("unknown behavior key: %s", key)
		}
	}
	return nil
}

func (r testSingleBehaviorRegistry) InitObjectBehaviors(_ *contracts.BehaviorObjectInitContext, _ []string) error {
	return nil
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
	service := NewContextActionService(world, nil, nil, nil, nil, sender, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)

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
	service := NewContextActionService(world, nil, nil, nil, nil, sender, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)

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
	service := NewContextActionService(world, nil, nil, nil, nil, sender, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)
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

func TestContextActionService_OnLinkBroken_RemovesPendingActionForBrokenTarget(t *testing.T) {
	world := ecs.NewWorldForTesting()
	service := NewContextActionService(world, nil, nil, nil, nil, nil, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)

	const (
		playerID = types.EntityID(9101)
		targetID = types.EntityID(9102)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.PendingContextAction{
			TargetEntityID: targetID,
			ActionID:       "chop",
		})
	})
	if playerHandle == types.InvalidHandle {
		t.Fatalf("expected valid player handle")
	}

	err := service.onLinkBroken(context.Background(), ecs.NewLinkBrokenEvent(world.Layer, playerID, targetID, ecs.LinkBreakMoved))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if _, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle); hasPending {
		t.Fatalf("expected pending context action to be removed for broken target")
	}
}

func TestContextActionService_OnLinkBroken_PreservesRetargetPendingAndCancelsActive(t *testing.T) {
	world := ecs.NewWorldForTesting()
	sender := &testCyclicActionFinishSender{}
	service := NewContextActionService(world, nil, nil, nil, nil, sender, nil, nil, nil, behaviors.MustDefaultRegistry(), nil)

	const (
		playerID  = types.EntityID(9201)
		oldTarget = types.EntityID(9202)
		newTarget = types.EntityID(9203)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID:    testActionChop,
			TargetID:    oldTarget,
			BehaviorKey: "tree",
		})
		ecs.AddComponent(w, h, components.PendingContextAction{
			TargetEntityID: newTarget,
			ActionID:       "chop",
		})
	})
	if playerHandle == types.InvalidHandle {
		t.Fatalf("expected valid player handle")
	}

	err := service.onLinkBroken(context.Background(), ecs.NewLinkBrokenEvent(world.Layer, playerID, oldTarget, ecs.LinkBreakMoved))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	pending, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle)
	if !hasPending {
		t.Fatalf("expected pending context action to be preserved for retarget")
	}
	if pending.TargetEntityID != newTarget {
		t.Fatalf("expected pending target %d, got %d", newTarget, pending.TargetEntityID)
	}
	if _, hasActive := ecs.GetComponent[components.ActiveCyclicAction](world, playerHandle); hasActive {
		t.Fatalf("expected active cyclic action to be canceled on link break")
	}
	if len(sender.messages) != 1 {
		t.Fatalf("expected one cyclic finish message, got %d", len(sender.messages))
	}
	if sender.messages[0].Result != netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED {
		t.Fatalf("expected canceled finish result, got %v", sender.messages[0].Result)
	}
}

func TestContextActionService_RetargetFlow_BrokenOldLinkThenLinkCreatedExecutesNewAction(t *testing.T) {
	world := ecs.NewWorldForTesting()
	eb := newGameTestEventBus(t)
	defer shutdownGameTestEventBus(t, eb)

	sender := &testCyclicActionFinishSender{}
	behavior := &testRetargetBehavior{actionID: "retarget_action"}
	registry := testSingleBehaviorRegistry{behavior: behavior}
	service := NewContextActionService(world, eb, nil, nil, nil, sender, nil, nil, nil, registry, nil)

	const (
		playerID  = types.EntityID(9301)
		oldTarget = types.EntityID(9302)
		newTarget = types.EntityID(9303)
	)

	playerHandle := world.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Movement{State: constt.StateInteracting})
		ecs.AddComponent(w, h, components.ActiveCyclicAction{
			ActionID:    testActionChop,
			TargetID:    oldTarget,
			BehaviorKey: "tree",
		})
		ecs.AddComponent(w, h, components.PendingContextAction{
			TargetEntityID: newTarget,
			ActionID:       behavior.actionID,
		})
	})
	if playerHandle == types.InvalidHandle {
		t.Fatalf("expected valid player handle")
	}

	world.Spawn(oldTarget, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 10})
	})
	world.Spawn(newTarget, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 12, Y: 10})
		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:    1,
			Behaviors: []string{behavior.Key()},
		})
	})

	if err := service.onLinkBroken(context.Background(), ecs.NewLinkBrokenEvent(world.Layer, playerID, oldTarget, ecs.LinkBreakMoved)); err != nil {
		t.Fatalf("onLinkBroken failed: %v", err)
	}
	if _, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle); !hasPending {
		t.Fatalf("expected pending action for new target after old link break")
	}

	if err := service.onLinkCreated(context.Background(), ecs.NewLinkCreatedEvent(world.Layer, playerID, newTarget)); err != nil {
		t.Fatalf("onLinkCreated failed: %v", err)
	}

	if _, hasPending := ecs.GetComponent[components.PendingContextAction](world, playerHandle); hasPending {
		t.Fatalf("expected pending action to be consumed on new link creation")
	}
	if behavior.executeCount != 1 {
		t.Fatalf("expected one new-target action execution, got %d", behavior.executeCount)
	}
	if behavior.executedTargetID != newTarget {
		t.Fatalf("expected execution target %d, got %d", newTarget, behavior.executedTargetID)
	}
	if len(sender.messages) != 1 || sender.messages[0].Result != netproto.CyclicActionFinishResult_CYCLIC_ACTION_FINISH_RESULT_CANCELED {
		t.Fatalf("expected old cyclic action to be canceled exactly once")
	}
}

func newGameTestEventBus(t *testing.T) *eventbus.EventBus {
	t.Helper()
	cfg := eventbus.DefaultConfig()
	cfg.MinWorkers = 1
	cfg.MaxWorkers = 1
	return eventbus.New(cfg)
}

func shutdownGameTestEventBus(t *testing.T, eb *eventbus.EventBus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := eb.Shutdown(ctx); err != nil {
		t.Fatalf("eventbus shutdown failed: %v", err)
	}
}
