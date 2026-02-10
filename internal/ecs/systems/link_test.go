package systems

import (
	"context"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

func TestLinkSystemCreatesLinkOnIntentCollision(t *testing.T) {
	eb := newTestEventBus(t)
	defer shutdownTestEventBus(t, eb)

	w := ecs.NewWorldForTesting()
	linkSystem := NewLinkSystem(eb, zap.NewNop())

	playerID := types.EntityID(1001)
	targetID := types.EntityID(2001)

	playerHandle := spawnPlayerForLinkTests(w, playerID, 10, 10, targetID)
	targetHandle := spawnTargetForLinkTests(w, targetID, 12, 10)
	ecs.AddComponent(w, playerHandle, components.Movement{
		State:      constt.StateMoving,
		TargetType: constt.TargetEntity,
		TargetX:    12,
		TargetY:    10,
		Speed:      10,
	})

	linkState := ecs.GetResource[ecs.LinkState](w)
	linkState.SetIntent(playerID, targetID, targetHandle, time.Now())

	var moveEvents []*ecs.ObjectMoveBatchEvent
	moveEventReceived := make(chan struct{}, 1)
	eb.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(ctx context.Context, event eventbus.Event) error {
		if move, ok := event.(*ecs.ObjectMoveBatchEvent); ok {
			moveEvents = append(moveEvents, move)
			select {
			case moveEventReceived <- struct{}{}:
			default:
			}
		}
		return nil
	})

	linkSystem.Update(w, 0.05)

	// Wait for async event to be processed
	select {
	case <-moveEventReceived:
	case <-time.After(100 * time.Millisecond):
		// Continue anyway - the event might not be published if no movement was stopped
	}

	link, ok := linkState.GetLink(playerID)
	if !ok {
		t.Fatalf("expected link to be created")
	}
	if link.PlayerHandle != playerHandle {
		t.Fatalf("unexpected player handle in link: got %v want %v", link.PlayerHandle, playerHandle)
	}
	if link.TargetHandle != targetHandle {
		t.Fatalf("unexpected target handle in link: got %v want %v", link.TargetHandle, targetHandle)
	}
	if link.TargetID != targetID {
		t.Fatalf("unexpected target id in link: got %v want %v", link.TargetID, targetID)
	}
	if _, stillPending := linkState.IntentByPlayer[playerID]; stillPending {
		t.Fatalf("intent must be cleared after successful link")
	}
	if _, hasReverse := linkState.PlayersByTarget[targetID][playerID]; !hasReverse {
		t.Fatalf("reverse index must contain linked player")
	}
	movement, _ := ecs.GetComponent[components.Movement](w, playerHandle)
	if movement.State != constt.StateIdle || movement.TargetType != constt.TargetNone {
		t.Fatalf("movement must be cleared on link creation: state=%v targetType=%v", movement.State, movement.TargetType)
	}
	if len(moveEvents) != 1 {
		t.Fatalf("expected one stop-move event, got %d", len(moveEvents))
	}
	if len(moveEvents[0].Entries) != 1 || moveEvents[0].Entries[0].IsMoving {
		t.Fatalf("expected stop-move entry with isMoving=false")
	}
}

func TestLinkSystemBreaksOnMovementWithMovedReason(t *testing.T) {
	eb := newTestEventBus(t)
	defer shutdownTestEventBus(t, eb)

	w := ecs.NewWorldForTesting()
	linkSystem := NewLinkSystem(eb, zap.NewNop())

	playerID := types.EntityID(1002)
	targetID := types.EntityID(2002)

	playerHandle := spawnPlayerForLinkTests(w, playerID, 10, 10, targetID)
	targetHandle := spawnTargetForLinkTests(w, targetID, 12, 10)

	linkState := ecs.GetResource[ecs.LinkState](w)
	linkState.SetLink(ecs.PlayerLink{
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		PlayerX:      10,
		PlayerY:      10,
		TargetX:      12,
		TargetY:      10,
		CreatedAt:    time.Now(),
	})

	var brokenEvents []*ecs.LinkBrokenEvent
	eb.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityMedium, func(ctx context.Context, event eventbus.Event) error {
		if broken, ok := event.(*ecs.LinkBrokenEvent); ok {
			brokenEvents = append(brokenEvents, broken)
		}
		return nil
	})

	ecs.WithComponent(w, playerHandle, func(t *components.Transform) {
		t.X = 11
		t.Y = 10
	})

	linkSystem.Update(w, 0.05)

	if _, ok := linkState.GetLink(playerID); ok {
		t.Fatalf("link must be broken after player movement")
	}
	if len(brokenEvents) != 1 {
		t.Fatalf("expected 1 link broken event, got %d", len(brokenEvents))
	}
	if brokenEvents[0].BreakReason != ecs.LinkBreakMoved {
		t.Fatalf("unexpected break reason: got %q want %q", brokenEvents[0].BreakReason, ecs.LinkBreakMoved)
	}
}

func TestLinkSystemBreaksOldLinkOnRelink(t *testing.T) {
	eb := newTestEventBus(t)
	defer shutdownTestEventBus(t, eb)

	w := ecs.NewWorldForTesting()
	linkSystem := NewLinkSystem(eb, zap.NewNop())

	playerID := types.EntityID(1003)
	targetAID := types.EntityID(2003)
	targetBID := types.EntityID(2004)

	playerHandle := spawnPlayerForLinkTests(w, playerID, 10, 10, targetBID)
	targetAHandle := spawnTargetForLinkTests(w, targetAID, 12, 10)
	targetBHandle := spawnTargetForLinkTests(w, targetBID, 9, 10)

	linkState := ecs.GetResource[ecs.LinkState](w)
	linkState.SetLink(ecs.PlayerLink{
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetAID,
		TargetHandle: targetAHandle,
		PlayerX:      10,
		PlayerY:      10,
		TargetX:      12,
		TargetY:      10,
		CreatedAt:    time.Now(),
	})
	linkState.SetIntent(playerID, targetBID, targetBHandle, time.Now())

	var brokenEvents []*ecs.LinkBrokenEvent
	eb.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityMedium, func(ctx context.Context, event eventbus.Event) error {
		if broken, ok := event.(*ecs.LinkBrokenEvent); ok {
			brokenEvents = append(brokenEvents, broken)
		}
		return nil
	})

	linkSystem.Update(w, 0.05)

	link, ok := linkState.GetLink(playerID)
	if !ok {
		t.Fatalf("expected new link after relink")
	}
	if link.TargetID != targetBID {
		t.Fatalf("unexpected relink target: got %v want %v", link.TargetID, targetBID)
	}
	if _, stale := linkState.PlayersByTarget[targetAID][playerID]; stale {
		t.Fatalf("stale reverse index for old target must be removed")
	}
	if _, hasNew := linkState.PlayersByTarget[targetBID][playerID]; !hasNew {
		t.Fatalf("reverse index must contain new target")
	}
	if len(brokenEvents) != 1 {
		t.Fatalf("expected exactly one break event for relink, got %d", len(brokenEvents))
	}
	if brokenEvents[0].BreakReason != ecs.LinkBreakRelink {
		t.Fatalf("unexpected break reason: got %q want %q", brokenEvents[0].BreakReason, ecs.LinkBreakRelink)
	}
}

func TestLinkSystemBreaksOnTargetDespawn(t *testing.T) {
	eb := newTestEventBus(t)
	defer shutdownTestEventBus(t, eb)

	w := ecs.NewWorldForTesting()
	linkSystem := NewLinkSystem(eb, zap.NewNop())

	playerID := types.EntityID(1004)
	targetID := types.EntityID(2005)

	playerHandle := spawnPlayerForLinkTests(w, playerID, 10, 10, targetID)
	targetHandle := spawnTargetForLinkTests(w, targetID, 12, 10)

	linkState := ecs.GetResource[ecs.LinkState](w)
	linkState.SetLink(ecs.PlayerLink{
		PlayerID:     playerID,
		PlayerHandle: playerHandle,
		TargetID:     targetID,
		TargetHandle: targetHandle,
		PlayerX:      10,
		PlayerY:      10,
		TargetX:      12,
		TargetY:      10,
		CreatedAt:    time.Now(),
	})

	var brokenEvents []*ecs.LinkBrokenEvent
	eb.SubscribeSync(ecs.TopicGameplayLinkBroken, eventbus.PriorityMedium, func(ctx context.Context, event eventbus.Event) error {
		if broken, ok := event.(*ecs.LinkBrokenEvent); ok {
			brokenEvents = append(brokenEvents, broken)
		}
		return nil
	})

	w.Despawn(targetHandle)
	linkSystem.Update(w, 0.05)

	if _, ok := linkState.GetLink(playerID); ok {
		t.Fatalf("link must be broken after target despawn")
	}
	if len(brokenEvents) != 1 {
		t.Fatalf("expected 1 link broken event, got %d", len(brokenEvents))
	}
	if brokenEvents[0].BreakReason != ecs.LinkBreakDespawn {
		t.Fatalf("unexpected break reason: got %q want %q", brokenEvents[0].BreakReason, ecs.LinkBreakDespawn)
	}
}

func spawnPlayerForLinkTests(w *ecs.World, playerID types.EntityID, x, y float64, collidedWith types.EntityID) types.Handle {
	return w.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: x, Y: y})
		ecs.AddComponent(w, h, components.CollisionResult{
			PrevCollidedWith: collidedWith,
		})
	})
}

func spawnTargetForLinkTests(w *ecs.World, targetID types.EntityID, x, y float64) types.Handle {
	return w.Spawn(targetID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: x, Y: y})
	})
}

func newTestEventBus(t *testing.T) *eventbus.EventBus {
	t.Helper()
	cfg := eventbus.DefaultConfig()
	cfg.MinWorkers = 1
	cfg.MaxWorkers = 1
	cfg.Logger = zap.NewNop()
	return eventbus.New(cfg)
}

func shutdownTestEventBus(t *testing.T, eb *eventbus.EventBus) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := eb.Shutdown(ctx); err != nil {
		t.Fatalf("eventbus shutdown failed: %v", err)
	}
}
