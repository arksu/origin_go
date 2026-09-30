package systems

import (
	"context"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestVisionPublishesOneSpawnBatchPerObserver(t *testing.T) {
	for _, mode := range []string{"normal", "forced"} {
		t.Run(mode, func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			w.Layer = 2
			ecs.SetResource(w, ecs.TimeState{Now: time.Unix(100, 0)})
			chunk := newTestChunk(types.ChunkCoord{})
			bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
			t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
			batches := make(chan *ecs.EntitySpawnBatchEvent, 8)
			singles := make(chan eventbus.Event, 8)
			bus.SubscribeAsync(ecs.TopicGameplayEntitySpawnBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				batches <- event.(*ecs.EntitySpawnBatchEvent)
				return nil
			})
			bus.SubscribeAsync(ecs.TopicGameplayEntitySpawn, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				singles <- event
				return nil
			})
			visibility := ecs.GetResource[ecs.VisibilityState](w)
			observers := make([]types.Handle, 0, 2)
			want := make(map[types.EntityID][]ecs.SpawnBatchEntry)
			for observerIndex, positionX := range []int{200, 700} {
				observerID := types.EntityID(10 + observerIndex)
				observer := w.Spawn(observerID, nil)
				ecs.AddComponent(w, observer, components.Transform{X: float64(positionX), Y: 200})
				ecs.AddComponent(w, observer, components.Vision{Radius: 100, Power: 100})
				ecs.AddComponent(w, observer, components.ChunkRef{})
				ecs.AddComponent(w, observer, components.EntityInfo{Layer: w.Layer})
				chunk.Spatial().AddDynamic(observer, positionX, 200)
				visibility.VisibleByObserver[observer] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{observer: observerID}}
				visibility.ObserversByVisibleTarget[observer] = map[types.Handle]struct{}{observer: {}}
				observers = append(observers, observer)
				for targetIndex := range 2 {
					id := types.EntityID(100 + observerIndex*10 + targetIndex)
					target := w.Spawn(id, nil)
					targetX := positionX + 10 + targetIndex*10
					ecs.AddComponent(w, target, components.Transform{X: float64(targetX), Y: 200})
					ecs.AddComponent(w, target, components.EntityInfo{Layer: w.Layer})
					chunk.Spatial().AddStatic(target, targetX, 200)
					want[observerID] = append(want[observerID], ecs.SpawnBatchEntry{EntityID: id, Handle: target})
				}
			}
			system := NewVisionSystem(w, &testChunkManager{chunk: chunk}, bus, false, zap.NewNop())
			if mode == "forced" {
				for _, observer := range observers {
					system.ForceUpdateForObserver(w, observer)
				}
			} else {
				system.Update(w, 0)
			}
			// A second result with no new targets must not publish an empty batch.
			for _, observer := range observers {
				system.ForceUpdateForObserver(w, observer)
			}
			flushBatchPublicationEvents(t, bus)
			require.NoError(t, bus.Shutdown(context.Background()))
			require.Empty(t, singles)
			require.Len(t, batches, 2)
			seen := make(map[types.EntityID]bool)
			for range 2 {
				batch := <-batches
				require.False(t, seen[batch.ObserverID], "each observer should receive one event")
				seen[batch.ObserverID] = true
				require.Equal(t, w.Layer, batch.Layer)
				require.ElementsMatch(t, want[batch.ObserverID], batch.Entries)
			}
		})
	}
}

func flushBatchPublicationEvents(t *testing.T, bus *eventbus.EventBus) {
	t.Helper()
	flushed := make(chan struct{})
	bus.SubscribeAsync("test.batch_publication.flush", eventbus.PriorityMedium, func(_ context.Context, _ eventbus.Event) error {
		close(flushed)
		return nil
	})
	bus.PublishAsync(eventbus.NewEvent("test.batch_publication.flush", nil), eventbus.PriorityMedium)
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("publication event queue did not drain")
	}
}
