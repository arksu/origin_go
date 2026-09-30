package systems

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"
)

func TestTransformBatchSurvivesNextTickScratchReuse(t *testing.T) {
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	release := make(chan struct{})
	events := make(chan *ecs.ObjectMoveBatchEvent, 2)
	bus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		<-release
		events <- event.(*ecs.ObjectMoveBatchEvent)
		return nil
	})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	defer close(release)
	w := ecs.NewWorldWithCapacity(16, bus, 0)
	target := w.Spawn(2, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{})
		ecs.AddComponent(w, h, components.CollisionResult{FinalX: 10, FinalY: 20})
		ecs.AddComponent(w, h, components.Movement{MoveSeq: 5})
	})
	ecs.GetResource[ecs.VisibilityState](w).ObserversByVisibleTarget[target] = map[types.Handle]struct{}{target: {}}
	ecs.GetResource[ecs.MovedEntities](w).Add(target, 10, 20)
	ecs.GetResource[ecs.TimeState](w).UnixMs = 100
	system := NewTransformUpdateSystem(w, nil, bus, zap.NewNop())
	system.Update(w, 0.1)
	ecs.WithComponent(w, target, func(result *components.CollisionResult) { result.FinalX = 30; result.FinalY = 40 })
	ecs.GetResource[ecs.TimeState](w).UnixMs = 200
	system.Update(w, 0.1)
	// Release the worker only after the same scratch backing array was overwritten.
	release <- struct{}{}
	select {
	case event := <-events:
		require.Len(t, event.Entries, 1)
		require.Equal(t, 10, event.Entries[0].X)
		require.Equal(t, 20, event.Entries[0].Y)
		require.EqualValues(t, 5, event.Entries[0].MoveSeq)
		require.EqualValues(t, 100, event.Entries[0].ServerTimeMs)
	case <-time.After(time.Second):
		t.Fatal("missing movement event")
	}
}
