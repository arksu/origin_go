package game

import (
	"context"
	"fmt"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestPublishReattachSpawnsBatchesOnlyLiveTargets(t *testing.T) {
	for _, count := range []int{0, 3} {
		t.Run(fmt.Sprintf("live_targets_%d", count), func(t *testing.T) {
			w := ecs.NewWorldForTesting()
			w.Layer = 2
			bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
			t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
			shard := &Shard{world: w, eventBus: bus}
			known := make(map[types.Handle]types.EntityID)
			want := make([]ecs.SpawnBatchEntry, 0, count)
			for index := range count {
				id := types.EntityID(20 + index)
				handle := w.Spawn(id, nil)
				known[handle] = id
				want = append(want, ecs.SpawnBatchEntry{EntityID: id, Handle: handle})
			}
			dead := w.Spawn(99, nil)
			known[dead] = 99
			w.Despawn(dead)
			batches := make(chan *ecs.EntitySpawnBatchEvent, 2)
			singles := make(chan eventbus.Event, 4)
			bus.SubscribeAsync(ecs.TopicGameplayEntitySpawnBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				batches <- event.(*ecs.EntitySpawnBatchEvent)
				return nil
			})
			bus.SubscribeAsync(ecs.TopicGameplayEntitySpawn, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				singles <- event
				return nil
			})

			publishReattachSpawns(shard, 10, known)
			clear(known)
			flushed := make(chan struct{})
			bus.SubscribeAsync("test.reattach.flush", eventbus.PriorityMedium, func(_ context.Context, _ eventbus.Event) error {
				close(flushed)
				return nil
			})
			bus.PublishAsync(eventbus.NewEvent("test.reattach.flush", nil), eventbus.PriorityMedium)
			select {
			case <-flushed:
			case <-time.After(time.Second):
				t.Fatal("reattach event queue did not drain")
			}
			require.NoError(t, bus.Shutdown(context.Background()))
			require.Empty(t, singles)
			if count == 0 {
				require.Empty(t, batches)
				return
			}
			require.Len(t, batches, 1)
			batch := <-batches
			require.Equal(t, types.EntityID(10), batch.ObserverID)
			require.Equal(t, 2, batch.Layer)
			require.ElementsMatch(t, want, batch.Entries)
		})
	}
}
