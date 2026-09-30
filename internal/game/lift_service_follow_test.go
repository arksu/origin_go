package game

import (
	"context"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"go.uber.org/zap"
)

func newLiftFollowTest(t *testing.T) (*ecs.World, *LiftService, *eventbus.EventBus, chan *ecs.ObjectMoveBatchEvent) {
	t.Helper()
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).UnixMs = 456789
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { _ = bus.Shutdown(context.Background()) })
	received := make(chan *ecs.ObjectMoveBatchEvent, 16)
	bus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		received <- event.(*ecs.ObjectMoveBatchEvent)
		return nil
	})
	cfg := &config.Config{Game: config.GameConfig{
		ChunkLRUCapacity: 1, ChunkLRUTTL: 60, WorldWidthChunks: 2, WorldHeightChunks: 2,
	}}
	manager := gameworld.NewChunkManager(cfg, nil, w, nil, 0, 1, nil, nil, bus, zap.NewNop())
	t.Cleanup(manager.Stop)
	service := NewLiftService(w, manager, bus, nil, nil)
	return w, service, bus, received
}

func drainLiftFollowEvents(t *testing.T, bus *eventbus.EventBus, received <-chan *ecs.ObjectMoveBatchEvent) []*ecs.ObjectMoveBatchEvent {
	t.Helper()
	// A same-priority marker waits for prior handlers without canceling their contexts.
	bus.PublishAsync(ecs.NewObjectMoveBatchEvent(-1, nil), eventbus.PriorityMedium)
	var events []*ecs.ObjectMoveBatchEvent
	timeout := time.After(time.Second)
	for {
		select {
		case event := <-received:
			if event.Layer == -1 {
				return events
			}
			events = append(events, event)
		case <-timeout:
			t.Fatal("movement event drain timed out")
		}
	}
}

func spawnFollowPair(w *ecs.World, playerID, objectID types.EntityID, playerX, objectX float64) (types.Handle, types.Handle) {
	player := w.Spawn(playerID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: playerX, Y: 40})
	})
	object := w.Spawn(objectID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: objectX, Y: 40, Direction: 1.25})
		ecs.AddComponent(w, h, components.ChunkRef{})
		ecs.AddComponent(w, h, components.EntityInfo{Behaviors: []string{"lift"}})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
		ecs.AddComponent(w, h, components.LiftedObjectState{CarrierPlayerID: playerID, CarrierHandle: player})
	})
	ecs.AddComponent(w, player, components.LiftCarryState{ObjectEntityID: objectID, ObjectHandle: object})
	return player, object
}

func TestLiftServiceFollowEmitsOneBatchForMovingCarries(t *testing.T) {
	w, service, bus, received := newLiftFollowTest(t)
	objects := make([]types.Handle, 0, 3)
	for index := 0; index < 3; index++ {
		playerX := float64(20 + index*10)
		objectX := playerX - 5
		if index == 2 {
			objectX = playerX
		}
		_, object := spawnFollowPair(w, types.EntityID(index+1), types.EntityID(index+11), playerX, objectX)
		objects = append(objects, object)
	}
	system := systems.NewLiftCarryFollowSystem(w, service, bus, nil)
	system.Update(w, 0.1)
	for index, object := range objects {
		position, _ := ecs.GetComponent[components.Transform](w, object)
		if position.X != float64(20+index*10) || position.Y != 40 {
			t.Fatalf("carry %d did not move synchronously: %+v", index, position)
		}
	}
	// All three now match their carrier; another pass must publish nothing.
	system.Update(w, 0.1)
	events := drainLiftFollowEvents(t, bus, received)
	if len(events) != 1 {
		t.Fatalf("got %d events, want one periodic batch without individual duplicates", len(events))
	}
	event := events[0]
	if len(event.Entries) != 2 {
		t.Fatalf("got %d carry entries, want two moving objects", len(event.Entries))
	}
	seen := make(map[types.EntityID]bool)
	for _, entry := range event.Entries {
		index := int(entry.EntityID) - 11
		if index < 0 || index >= 2 || seen[entry.EntityID] {
			t.Fatalf("unexpected or duplicate carried object: %+v", entry)
		}
		seen[entry.EntityID] = true
		if entry.Handle != objects[index] || entry.CarriedByEntityID != types.EntityID(index+1) ||
			entry.ServerTimeMs != 456789 || entry.Heading != 1.25 || entry.MoveSeq != 0 ||
			entry.IsTeleport || entry.IsMoving || entry.TargetX != nil || entry.TargetY != nil ||
			entry.VelocityX != 0 || entry.VelocityY != 0 {
			t.Fatalf("periodic carry entry changed relocation fields: %+v", entry)
		}
	}
}

func TestLiftServiceFollowRefreshesCachedHandleAndCleansInvalidCarry(t *testing.T) {
	w, service, bus, received := newLiftFollowTest(t)
	player, object := spawnFollowPair(w, 1, 11, 20, 20)
	ecs.WithComponent(w, player, func(carry *components.LiftCarryState) { carry.ObjectHandle = types.InvalidHandle })
	carry, _ := ecs.GetComponent[components.LiftCarryState](w, player)
	if entry := service.SyncLiftCarryFollow(w, 1, player, carry); entry != nil {
		t.Fatalf("stationary carry produced movement: %+v", entry)
	}
	carry, exists := ecs.GetComponent[components.LiftCarryState](w, player)
	if !exists || carry.ObjectHandle != object {
		t.Fatalf("stationary carry did not refresh cached handle: %+v", carry)
	}
	ecs.RemoveComponent[components.LiftedObjectState](w, object)
	if entry := service.SyncLiftCarryFollow(w, 1, player, carry); entry != nil {
		t.Fatalf("invalid carry produced movement: %+v", entry)
	}
	if _, exists := ecs.GetComponent[components.LiftCarryState](w, player); exists {
		t.Fatal("missing lifted state did not clear player's carry")
	}
	ecs.AddComponent(w, player, components.LiftCarryState{ObjectEntityID: 12})
	carry, _ = ecs.GetComponent[components.LiftCarryState](w, player)
	service.SyncLiftCarryFollow(w, 1, player, carry)
	if _, exists := ecs.GetComponent[components.LiftCarryState](w, player); exists {
		t.Fatal("missing carried object did not clear player's carry")
	}
	events := drainLiftFollowEvents(t, bus, received)
	if len(events) != 0 {
		t.Fatal("stationary or invalid carry published movement")
	}
}

func TestLiftServiceSamePositionPickupAndDropsStillPublishImmediately(t *testing.T) {
	w, service, bus, received := newLiftFollowTest(t)
	player, object := spawnFollowPair(w, 1, 11, 20, 20)
	ecs.RemoveComponent[components.LiftCarryState](w, player)
	ecs.RemoveComponent[components.LiftedObjectState](w, object)
	ecs.AddComponent(w, object, components.Collider{})
	service.chunkActive = func(types.ChunkCoord) bool { return true }
	if result := service.StartLift(w, 1, player, 11, object); result.Outcome != ActionSucceeded {
		t.Fatalf("pickup failed: %+v", result)
	}
	if !service.ForceDropCarryAtPlayerPosition(w, 1, player, true) {
		t.Fatal("forced drop failed")
	}
	if result := service.StartLift(w, 1, player, 11, object); result.Outcome != ActionSucceeded {
		t.Fatalf("second pickup failed: %+v", result)
	}
	if !service.finalizeLiftPutDown(w, 1, player, components.PendingLiftTransition{TargetX: 20, TargetY: 40}) {
		t.Fatal("put-down failed")
	}
	events := drainLiftFollowEvents(t, bus, received)
	if len(events) != 4 {
		t.Fatalf("got %d transition events, want two pickups and two drops without a periodic pass", len(events))
	}
	carried, dropped := 0, 0
	for _, event := range events {
		if len(event.Entries) != 1 {
			t.Fatalf("immediate transition entries: %+v", event.Entries)
		}
		entry := event.Entries[0]
		if entry.EntityID != 11 || entry.X != 20 || entry.Y != 40 || entry.MoveSeq != 0 || entry.IsTeleport {
			t.Fatalf("immediate transition fields changed: %+v", entry)
		}
		switch entry.CarriedByEntityID {
		case 1:
			carried++
		case 0:
			dropped++
		default:
			t.Fatalf("unexpected carrier: %+v", entry)
		}
	}
	if carried != 2 || dropped != 2 {
		t.Fatalf("transitions = %d carried, %d dropped; want two of each", carried, dropped)
	}
}
