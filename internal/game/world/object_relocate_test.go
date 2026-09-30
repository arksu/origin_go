package world

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	_const "origin/internal/const"
	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/eventbus"
	"origin/internal/types"
)

func newRelocationTest(t *testing.T, isStatic bool) (*ecs.World, *ChunkManager, types.Handle) {
	t.Helper()
	w := ecs.NewWorldForTesting()
	ecs.GetResource[ecs.TimeState](w).UnixMs = 123456
	chunks := make(map[types.ChunkCoord]*core.Chunk)
	for _, coord := range []types.ChunkCoord{{}, {X: 1}} {
		chunk := core.NewChunk(coord, 0, 0, _const.ChunkSize)
		chunk.SetState(types.ChunkStateActive)
		chunks[coord] = chunk
	}
	manager := &ChunkManager{cfg: newTestConfig(), chunks: chunks}
	handle := w.Spawn(11, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.Transform{X: 20.5, Y: 30.25, Direction: 1.5})
		ecs.AddComponent(w, h, components.ChunkRef{})
		ecs.AddComponent(w, h, components.EntityInfo{IsStatic: isStatic})
		ecs.AddComponent(w, h, components.ObjectInternalState{})
	})
	if isStatic {
		chunks[types.ChunkCoord{}].Spatial().AddStatic(handle, 20, 30)
	} else {
		chunks[types.ChunkCoord{}].Spatial().AddDynamic(handle, 20, 30)
	}
	return w, manager, handle
}

func TestRelocateWorldObjectNoopAndForcedReindex(t *testing.T) {
	w, manager, handle := newRelocationTest(t, true)
	opts := RelocateWorldObjectImmediateOptions{IsTeleport: true, CarriedByEntityID: 7}
	if success, entry := RelocateWorldObject(w, manager, handle, opts, 20.5, 30.25, nil); !success || entry != nil {
		t.Fatalf("unchanged relocation = %v, %+v; want successful no-op", success, entry)
	}
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if state.IsDirty {
		t.Fatal("no-op dirtied object")
	}

	// Pickup changes indexing and carry relation even at identical coordinates.
	ecs.WithComponent(w, handle, func(info *components.EntityInfo) { info.IsStatic = false })
	opts.ForceReindex = true
	success, entry := RelocateWorldObject(w, manager, handle, opts, 20.5, 30.25, nil)
	want := &ecs.MoveBatchEntry{
		EntityID: 11, Handle: handle, CarriedByEntityID: 7, X: 20, Y: 30,
		Heading: 1.5, ServerTimeMs: 123456, IsTeleport: true,
	}
	if !success || !reflect.DeepEqual(entry, want) {
		t.Fatalf("forced relocation = %v, %+v; want %+v", success, entry, want)
	}
	spatial := manager.chunks[types.ChunkCoord{}].Spatial()
	if spatial.StaticCount() != 0 || !slices.Contains(spatial.GetDynamicHandles(), handle) {
		t.Fatal("forced reindex did not change static object to dynamic")
	}
	state, _ = ecs.GetComponent[components.ObjectInternalState](w, handle)
	if !state.IsDirty {
		t.Fatal("forced relocation did not dirty object")
	}
}

func TestRelocateWorldObjectUpdatesSpatialChunkTransformSynchronously(t *testing.T) {
	for _, isStatic := range []bool{false, true} {
		t.Run(map[bool]string{false: "dynamic", true: "static"}[isStatic], func(t *testing.T) {
			w, manager, handle := newRelocationTest(t, isStatic)
			x := float64(_const.ChunkSize*_const.CoordPerTile + 20)
			success, entry := RelocateWorldObject(w, manager, handle, RelocateWorldObjectImmediateOptions{}, x, 31.75, nil)
			if !success || entry == nil || entry.X != int(x) || entry.Y != 31 {
				t.Fatalf("cross-chunk relocation = %v, %+v", success, entry)
			}
			transform, _ := ecs.GetComponent[components.Transform](w, handle)
			chunkRef, _ := ecs.GetComponent[components.ChunkRef](w, handle)
			state, _ := ecs.GetComponent[components.ObjectInternalState](w, handle)
			if transform.X != x || transform.Y != 31.75 || chunkRef.CurrentChunkX != 1 || chunkRef.PrevChunkX != 0 || !state.IsDirty {
				t.Fatalf("relocation left stale components: %+v, %+v, %+v", transform, chunkRef, state)
			}
			oldSpatial := manager.chunks[types.ChunkCoord{}].Spatial()
			if oldSpatial.DynamicCount()+oldSpatial.StaticCount() != 0 {
				t.Fatal("old chunk retains object")
			}
			var handles []types.Handle
			manager.chunks[types.ChunkCoord{X: 1}].Spatial().QueryRadius(x, 31, 1, &handles)
			if !slices.Contains(handles, handle) {
				t.Fatal("new chunk does not contain relocated object")
			}
		})
	}
}

func TestRelocateWorldObjectInactiveTargetDoesNotMutate(t *testing.T) {
	w, manager, handle := newRelocationTest(t, true)
	manager.chunks[types.ChunkCoord{X: 1}].SetState(types.ChunkStateInactive)
	x := float64(_const.ChunkSize * _const.CoordPerTile)
	if success, entry := RelocateWorldObject(w, manager, handle, RelocateWorldObjectImmediateOptions{}, x, 30, nil); success || entry != nil {
		t.Fatalf("inactive target = %v, %+v; want failure without entry", success, entry)
	}
	transform, _ := ecs.GetComponent[components.Transform](w, handle)
	state, _ := ecs.GetComponent[components.ObjectInternalState](w, handle)
	if transform.X != 20.5 || transform.Y != 30.25 || state.IsDirty {
		t.Fatalf("failed relocation changed state: %+v, %+v", transform, state)
	}
}

func TestRelocateWorldObjectImmediatePreservesSingleTransitionAndNoop(t *testing.T) {
	w, manager, handle := newRelocationTest(t, true)
	bus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 1})
	t.Cleanup(func() { _ = bus.Shutdown(context.Background()) })
	received := make(chan *ecs.ObjectMoveBatchEvent, 3)
	bus.SubscribeAsync(ecs.TopicGameplayMovementMoveBatch, eventbus.PriorityMedium, func(_ context.Context, event eventbus.Event) error {
		received <- event.(*ecs.ObjectMoveBatchEvent)
		return nil
	})
	opts := RelocateWorldObjectImmediateOptions{CarriedByEntityID: 7, IsTeleport: true}
	if !RelocateWorldObjectImmediate(w, manager, bus, handle, opts, 20.5, 30.25, nil) {
		t.Fatal("immediate no-op must remain successful")
	}
	opts.ForceReindex = true
	if !RelocateWorldObjectImmediate(w, manager, bus, handle, opts, 20.5, 30.25, nil) {
		t.Fatal("immediate forced transition failed")
	}
	bus.PublishAsync(ecs.NewObjectMoveBatchEvent(-1, nil), eventbus.PriorityMedium)
	var events []*ecs.ObjectMoveBatchEvent
	drained := false
	timeout := time.After(time.Second)
	for !drained {
		select {
		case event := <-received:
			if event.Layer == -1 {
				drained = true
			} else {
				events = append(events, event)
			}
		case <-timeout:
			t.Fatal("movement events did not finish")
		}
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want only forced transition", len(events))
	}
	event := events[0]
	want := ecs.MoveBatchEntry{EntityID: 11, Handle: handle, CarriedByEntityID: 7, X: 20, Y: 30, Heading: 1.5, ServerTimeMs: 123456, IsTeleport: true}
	if len(event.Entries) != 1 || !reflect.DeepEqual(event.Entries[0], want) {
		t.Fatalf("immediate transition payload changed: %+v", event.Entries)
	}
}
