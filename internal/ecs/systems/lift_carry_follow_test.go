package systems

import (
	"context"
	"sync"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"
)

type testLiftCarryCoordinator struct {
	entries map[types.EntityID]*ecs.MoveBatchEntry
	visits  int
}

func (c *testLiftCarryCoordinator) SyncLiftCarryFollow(_ *ecs.World, playerID types.EntityID, _ types.Handle, _ components.LiftCarryState) *ecs.MoveBatchEntry {
	c.visits++
	return c.entries[playerID]
}

func TestLiftCarryFollowSystemBatchesMovingCarriesAndOwnsPublishedEntries(t *testing.T) {
	w := ecs.NewWorldForTesting()
	for playerID := types.EntityID(1); playerID <= 3; playerID++ {
		w.Spawn(playerID, func(w *ecs.World, handle types.Handle) {
			ecs.AddComponent(w, handle, components.LiftCarryState{ObjectEntityID: playerID + 10})
		})
	}
	coordinator := &testLiftCarryCoordinator{entries: map[types.EntityID]*ecs.MoveBatchEntry{
		1: {EntityID: 11, CarriedByEntityID: 1, X: 100},
		2: {EntityID: 12, CarriedByEntityID: 2, X: 200},
	}}
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	defer bus.Shutdown(context.Background())
	started := make(chan struct{}, 2)
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	received := make(chan *ecs.ObjectMoveBatchEvent, 3)
	bus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		started <- struct{}{}
		<-resume
		received <- event.(*ecs.ObjectMoveBatchEvent)
		return nil
	})
	system := NewLiftCarryFollowSystem(w, coordinator, bus, nil)
	system.Update(w, 0.1)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first carry batch did not reach handler")
	}
	coordinator.entries[1].X = 110
	coordinator.entries[2].X = 220
	system.Update(w, 0.1)
	// Reuse the producer buffer before the first queued event is consumed.
	release()
	var first *ecs.ObjectMoveBatchEvent
	select {
	case first = <-received:
	case <-time.After(time.Second):
		t.Fatal("first carry batch did not finish")
	}
	if len(first.Entries) != 2 {
		t.Fatalf("got %d entries, want only two moving carries", len(first.Entries))
	}
	for _, entry := range first.Entries {
		if entry.X != int(entry.CarriedByEntityID)*100 {
			t.Fatalf("next update overwrote queued entry: %+v", entry)
		}
	}
	select {
	case second := <-received:
		if len(second.Entries) != 2 {
			t.Fatalf("got %d entries for second pass", len(second.Entries))
		}
		for _, entry := range second.Entries {
			if entry.X != int(entry.CarriedByEntityID)*110 {
				t.Fatalf("second pass lost movement: %+v", entry)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("second carry batch did not finish")
	}
	coordinator.entries = nil
	system.Update(w, 0.1)
	if coordinator.visits != 9 {
		t.Fatalf("follow coordinator visits = %d, want 9", coordinator.visits)
	}
	if len(system.moves) != 0 {
		t.Fatal("stationary carries generated entries")
	}
	if err := bus.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(received) != 0 {
		t.Fatal("stationary carry pass generated a message")
	}
}
