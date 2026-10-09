package game

import (
	"context"
	"errors"
	"testing"
	"time"

	"origin/internal/config"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/game/inventory"
	gameworld "origin/internal/game/world"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSkullClaimKeepsOneSpatialEntryAcrossVisibleReplacement(t *testing.T) {
	f := newSkullClaimFixture(t)
	cfg := &config.Config{Game: config.GameConfig{ChunkLRUCapacity: 1, ChunkLRUTTL: 60, LoadWorkers: 1, WorldWidthChunks: 1, WorldHeightChunks: 1, Region: 1}}
	chunks := gameworld.NewChunkManager(cfg, nil, f.w, nil, 0, 1, nil, nil, nil, zap.NewNop())
	t.Cleanup(chunks.Stop)
	chunks.RegisterEntity(500, 50, 50, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, chunks.WaitPreloaded(ctx, types.ChunkCoord{}))
	chunks.Update(0)
	chunk := chunks.GetChunkFast(types.ChunkCoord{})
	require.NotNil(t, chunk)
	chunk.Spatial().AddStatic(f.target, 50, 50)
	f.shard.chunkManager = chunks
	f.admit(t)
	require.Equal(t, 1, chunk.Spatial().StaticCount())
	f.persist(t)
	f.service.Update()
	f.requireCompleted(t)
	require.Equal(t, 1, chunk.Spatial().StaticCount(), "visible replacement must not duplicate static spatial membership")
}

func TestSkullClaimRetainsObserversUntilImmediateAppearanceChange(t *testing.T) {
	for _, outcome := range []string{"commit", "ambiguous_retry", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			f := newSkullClaimFixture(t)
			bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1, Logger: zap.NewNop()})
			t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
			f.shard.eventBus = bus
			visibility := ecs.GetResource[ecs.VisibilityState](f.w)
			observer := f.w.Spawn(700, nil)
			observers := []types.Handle{f.recipient, observer}
			nextVision := ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Hour)
			visibility.ObserversByVisibleTarget[f.target] = make(map[types.Handle]struct{})
			for _, handle := range observers {
				visibility.ObserversByVisibleTarget[f.target][handle] = struct{}{}
				visibility.VisibleByObserver[handle] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{f.target: 1}, NextUpdateTime: nextVision}
			}
			quarantines := 0
			f.service.deps.Quarantine = func(target types.Handle) {
				quarantines++
				invalidateEntityVisibility(f.w, 0, target, 1, bus)
				ecs.RemoveComponent[components.Collider](f.w, target)
			}
			appearances := make(chan *ecs.EntityAppearanceChangedEvent, 4)
			despawns := make(chan *ecs.EntityDespawnEvent, 4)
			bus.SubscribeAsync(ecs.TopicGameplayEntityAppearance, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				appearances <- event.(*ecs.EntityAppearanceChangedEvent)
				return nil
			})
			bus.SubscribeAsync(ecs.TopicGameplayEntityDespawn, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
				despawns <- event.(*ecs.EntityDespawnEvent)
				return nil
			})
			requireVisible := func() {
				t.Helper()
				require.Zero(t, quarantines, "a reserved skull source must not enter destruction quarantine")
				require.Len(t, visibility.ObserversByVisibleTarget[f.target], 2)
				for _, handle := range observers {
					state := visibility.VisibleByObserver[handle]
					require.Equal(t, types.EntityID(1), state.Known[f.target])
					require.Equal(t, nextVision, state.NextUpdateTime, "refresh must not rely on periodic vision")
				}
			}
			f.admit(t)
			requireVisible()
			require.True(t, ecs.HasComponent[components.Collider](f.w, f.target), "the pending skeleton keeps its spatial presence")
			info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.target)
			require.Equal(t, uint32(17), info.TypeID)
			if outcome == "ambiguous_retry" {
				f.persister.commitErr = errors.New("lost commit acknowledgement")
			} else if outcome == "rejected" {
				f.persister.metadataErr = inventory.ErrSkullClaimCharacterMissing
			}
			f.persist(t)
			f.service.Update()
			requireVisible()
			if outcome == "ambiguous_retry" {
				require.Empty(t, appearances)
				ecs.GetResource[ecs.TimeState](f.w).Now = ecs.GetResource[ecs.TimeState](f.w).Now.Add(time.Second)
				f.persist(t)
				f.service.Update()
				requireVisible()
			}
			if outcome != "rejected" {
				select {
				case event := <-appearances:
					require.Equal(t, types.EntityID(1), event.TargetID)
					require.Equal(t, f.target, event.TargetHandle)
				case <-time.After(time.Second):
					t.Fatal("committed skeleton appearance waited for the periodic vision pass")
				}
				f.requireCompleted(t)
			} else {
				require.Zero(t, f.service.PendingCount())
				info, _ := ecs.GetComponent[components.EntityInfo](f.w, f.target)
				require.Equal(t, uint32(17), info.TypeID)
				require.True(t, ecs.HasComponent[components.Collider](f.w, f.target))
			}
			require.NoError(t, bus.Shutdown(context.Background()))
			require.Empty(t, despawns, "take skull must never send a temporary despawn")
			require.Empty(t, appearances, "one committed replacement emits exactly one appearance update")
		})
	}
}
