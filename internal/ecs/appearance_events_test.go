package ecs

import (
	"context"
	"testing"
	"time"

	"origin/internal/eventbus"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

func TestPublishEntityAppearanceChangedUsesObserversAtPublication(t *testing.T) {
	w := NewWorldForTesting()
	w.Layer = 2
	target := w.Spawn(20, nil)
	observer := w.Spawn(10, nil)
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	updates := make(chan *EntityAppearanceChangedEvent, 4)
	bus.SubscribeAsync(TopicGameplayEntityAppearance, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		updates <- event.(*EntityAppearanceChangedEvent)
		return nil
	})

	PublishEntityAppearanceChanged(w, bus, 20, target)
	visibility := GetResource[VisibilityState](w)
	visibility.ObserversByVisibleTarget[target] = map[types.Handle]struct{}{observer: {}}
	PublishEntityAppearanceChanged(w, bus, 20, target)
	delete(visibility.ObserversByVisibleTarget, target)
	PublishEntityAppearanceChanged(w, bus, 20, target)
	PublishEntityAppearanceChanged(w, nil, 20, target)

	flushed := make(chan struct{})
	bus.SubscribeAsync("test.appearance.flush", eventbus.PriorityMedium, func(_ context.Context, _ eventbus.Event) error {
		close(flushed)
		return nil
	})
	bus.PublishAsync(eventbus.NewEvent("test.appearance.flush", nil), eventbus.PriorityMedium)
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("appearance event queue did not drain")
	}
	require.NoError(t, bus.Shutdown(context.Background()))
	require.Len(t, updates, 1, "only the change authored with an observer should enter the eventbus")
	event := <-updates
	require.Equal(t, types.EntityID(20), event.TargetID)
	require.Equal(t, target, event.TargetHandle)
	require.Equal(t, 2, event.Layer)
}
