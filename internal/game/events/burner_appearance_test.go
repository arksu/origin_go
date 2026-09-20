package events

import (
	"context"
	"testing"
	"time"

	"origin/internal/ecs"
	"origin/internal/ecs/components"
	ecssystems "origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/game/behaviors"
	"origin/internal/game/behaviors/contracts"
	gameworld "origin/internal/game/world"
	netproto "origin/internal/network/proto"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestBurnerBuildAndIgnitionProduceAppearanceUpsertPayloads(t *testing.T) {
	for _, key := range []string{"campfire", "test-hearth"} {
		t.Run(key, func(t *testing.T) {
			previous := objectdefs.Global()
			t.Cleanup(func() { objectdefs.SetGlobalForTesting(previous) })
			def := objectdefs.ObjectDef{
				DefID: 16, Key: key, Resource: key + "/unlit", BehaviorOrder: []string{"burner"},
				Station:      &objectdefs.StationDef{InitialState: "unlit"},
				BurnerConfig: &objectdefs.BurnerBehaviorConfig{InitialFuel: 5, FuelCapacity: 5, TicksPerFuel: 10},
			}
			objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{def}))
			bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
			t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
			updates := make(chan *ecs.EntityAppearanceChangedEvent, 4)
			bus.SubscribeAsync(ecs.TopicGameplayEntityAppearance, eventbus.PriorityMedium, func(_ context.Context, e eventbus.Event) error {
				updates <- e.(*ecs.EntityAppearanceChangedEvent)
				return nil
			})
			w := ecs.NewWorld(bus, 0)
			player := w.Spawn(1, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityStats{Stamina: 50})
			})
			target := w.Spawn(2, func(w *ecs.World, h types.Handle) {
				ecs.AddComponent(w, h, components.EntityInfo{TypeID: 1001})
				ecs.AddComponent(w, h, components.Transform{})
				ecs.AddComponent(w, h, components.ObjectInternalState{})
				ecs.AddComponent(w, h, components.Appearance{Resource: "build"})
			})
			ecs.GetResource[ecs.VisibilityState](w).ObserversByVisibleTarget[target] = map[types.Handle]struct{}{player: {}}
			dispatcher := &NetworkVisibilityDispatcher{logger: zap.NewNop()}
			assertUpsert := func(resource string) {
				t.Helper()
				select {
				case event := <-updates:
					require.True(t, targetVisibleToObserver(w, 1, event.TargetID))
					spawn := dispatcher.buildObjectSpawn(w, event.TargetID, event.TargetHandle)
					require.NotNil(t, spawn)
					encoded, err := proto.Marshal(&netproto.ServerMessage{Payload: &netproto.ServerMessage_ObjectSpawn{ObjectSpawn: spawn}})
					require.NoError(t, err)
					var received netproto.ServerMessage
					require.NoError(t, proto.Unmarshal(encoded, &received))
					require.Equal(t, resource, received.GetObjectSpawn().ResourcePath)
					require.EqualValues(t, 2, received.GetObjectSpawn().EntityId)
				case <-time.After(time.Second):
					t.Fatal("missing appearance upsert event")
				}
			}
			require.True(t, gameworld.TransformObjectToDefInPlace(w, 2, target, &def, gameworld.TransformObjectInPlaceOptions{
				BehaviorRegistry: behaviors.MustDefaultRegistry(), EventBus: bus,
			}))
			assertUpsert(key + "/unlit")
			burner, _ := behaviors.MustDefaultRegistry().GetBehavior("burner")
			actions := burner.(contracts.ContextActionProvider).ProvideActions(&contracts.BehaviorActionListContext{World: w, PlayerHandle: player, TargetHandle: target})
			require.Len(t, actions, 1)
			decision := burner.(contracts.CyclicActionHandler).OnCycleComplete(&contracts.BehaviorCycleContext{
				World: w, PlayerID: 1, PlayerHandle: player, TargetID: 2, TargetHandle: target,
				ActionID: actions[0].ActionID, Deps: &contracts.ExecutionDeps{EventBus: bus},
			})
			require.Equal(t, contracts.BehaviorCycleDecisionComplete, decision)
			assertUpsert(key + "/burning")
			ecssystems.RecomputeObjectBehaviorsNow(w, bus, nil, behaviors.MustDefaultRegistry(), []types.Handle{target})
			require.Equal(t, key+"/burning", dispatcher.buildObjectSpawn(w, 2, target).ResourcePath)
			require.NoError(t, bus.Shutdown(context.Background()))
			require.Empty(t, updates, "unchanged recompute does not send duplicate updates")
		})
	}
}
