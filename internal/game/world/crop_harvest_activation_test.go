package world

import (
	"context"
	"testing"
	"time"

	constt "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/eventbus"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCropHarvestCommittedDropsBecomeVisibleToStationaryObservers(t *testing.T) {
	installCapacityBacklogItemDefinitions(t)
	cm, _ := newCapacityChunkManager(t, 64)
	w := cm.world
	state := ecs.GetResource[ecs.TimeState](w)
	state.Now = time.Unix(100, 0)
	state.RuntimeSecondsTotal = 100
	chunk := core.NewChunk(types.ChunkCoord{}, 1, 0, constt.ChunkSize)
	chunk.SetState(types.ChunkStateActive)
	cm.chunks[chunk.Coord] = chunk
	observer := w.Spawn(50, nil)
	ecs.AddComponent(w, observer, components.Transform{X: 200, Y: 200})
	ecs.AddComponent(w, observer, components.Vision{Radius: 100, Power: 100})
	ecs.AddComponent(w, observer, components.ChunkRef{})
	ecs.AddComponent(w, observer, components.EntityInfo{Region: 1})
	chunk.Spatial().AddDynamic(observer, 200, 200)
	visibility := ecs.GetResource[ecs.VisibilityState](w)
	visibility.VisibleByObserver[observer] = ecs.ObserverVisibility{Known: map[types.Handle]types.EntityID{observer: 50}}
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1, Logger: zap.NewNop()})
	t.Cleanup(func() { require.NoError(t, bus.Shutdown(context.Background())) })
	vision := systems.NewVisionSystem(w, cm, bus, false, zap.NewNop())
	vision.Update(w, 0)
	require.NotEmpty(t, visibility.VisibleByObserver[observer].LastChunkGens)
	oldGeneration := chunk.Spatial().Generation()
	raw, roots := liveBacklogDrop(cm, 1000)
	raw.X, raw.Y = 210, 200
	require.NoError(t, cm.InsertCommittedDropped(raw, &roots[0]))
	cm.remainingDroppedActivations = committedDroppedActivationBudget
	before := w.EntityCount()
	require.NoError(t, cm.activateChunkInternal(chunk.Coord, chunk))
	require.Equal(t, committedDroppedActivationBudget-1, cm.remainingDroppedActivations)
	require.Equal(t, before+2, w.EntityCount(), "one ordinary drop reserves object and inventory handles")
	require.Greater(t, chunk.Spatial().Generation(), oldGeneration)
	handle := w.GetHandleByEntityID(1000)
	require.True(t, w.Alive(handle))
	state.Now = visibility.VisibleByObserver[observer].NextUpdateTime
	vision.Update(w, 0)
	require.Equal(t, types.EntityID(1000), visibility.VisibleByObserver[observer].Known[handle])
	position, _ := ecs.GetComponent[components.Transform](w, observer)
	require.Equal(t, float64(200), position.X)
	// A committed cache entry has been fully consumed into the normal ECS
	// dropped-item lifetime and remains clean for later chunk saves.
	require.Empty(t, chunk.GetRawObjects())
	refs := ecs.GetResource[ecs.InventoryRefIndex](w)
	root, found := refs.Lookup(constt.InventoryDroppedItem, 1000, 0)
	require.True(t, found)
	_, hasContainer := ecs.GetComponent[components.InventoryContainer](w, root)
	require.True(t, hasContainer)
}
