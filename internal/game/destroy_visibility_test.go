package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"

	"go.uber.org/zap"
)

type visibilityTestDeleter struct{ err error }

func (d visibilityTestDeleter) DeleteObject(int, types.EntityID) error { return d.err }

func TestDestroyNotifiesObserversWithoutVisionTick(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "deleted", true: "persistence failure"}[failed], func(t *testing.T) {
			bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1, Logger: zap.NewNop()})
			defer bus.Shutdown(context.Background())
			w := ecs.NewWorldForTesting()
			target := w.Spawn(777, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityInfo{Region: 1, Layer: 2})
			})
			other := w.Spawn(778, nil)
			vis := ecs.GetResource[ecs.VisibilityState](w)
			vis.ObserversByVisibleTarget[target] = make(map[types.Handle]struct{})
			for _, id := range []types.EntityID{42, 43} {
				observer := w.Spawn(id, nil)
				vis.ObserversByVisibleTarget[target][observer] = struct{}{}
				vis.VisibleByObserver[observer] = ecs.ObserverVisibility{
					Known:          map[types.Handle]types.EntityID{target: 777, other: 778},
					NextUpdateTime: time.Now().Add(time.Hour),
				}
			}
			events := make(chan *ecs.EntityDespawnEvent, 4)
			bus.SubscribeAsync(ecs.TopicGameplayEntityDespawn, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				events <- event.(*ecs.EntityDespawnEvent)
				return nil
			})
			chat := &mockChatDeliveryService{messages: make(map[types.EntityID]string)}
			handler := NewChatAdminCommandHandler(nil, nil, chat, nil, nil, nil, nil, nil, bus, zap.NewNop())
			deleter := visibilityTestDeleter{}
			if failed {
				deleter.err = errors.New("database unavailable")
			}
			handler.SetObjectDeleter(deleter)
			ecs.GetResource[ecs.PendingAdminDestroy](w).Set(42)
			handler.ExecutePendingDestroy(w, 42, 777)

			if w.Alive(target) != failed {
				t.Fatal("unexpected target lifetime")
			}
			for observer, state := range vis.VisibleByObserver {
				_, known := state.Known[target]
				if known != failed || state.Known[other] != 778 {
					t.Fatalf("incorrect visibility for observer %v: %+v", observer, state.Known)
				}
			}
			if failed {
				if len(vis.ObserversByVisibleTarget[target]) != 2 {
					t.Fatal("failed deletion changed observer index")
				}
			} else {
				if _, exists := vis.ObserversByVisibleTarget[target]; exists {
					t.Fatal("deleted target still indexed by visibility")
				}
				seen := make(map[types.EntityID]bool)
				for range 2 {
					select {
					case event := <-events:
						if event.TargetID != 777 || event.Layer != 2 || (event.ObserverID != 42 && event.ObserverID != 43) || seen[event.ObserverID] {
							t.Fatalf("incorrect despawn event: %+v", event)
						}
						seen[event.ObserverID] = true
					case <-time.After(time.Second):
						t.Fatal("despawn notification requires a periodic vision update")
					}
				}
			}
			bus.Shutdown(context.Background())
			select {
			case event := <-events:
				t.Fatalf("unexpected despawn event: %+v", event)
			default:
			}
		})
	}
}
