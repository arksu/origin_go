package game

import (
	"encoding/json"
	"errors"
	"testing"

	"origin/internal/core"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/game/inventory"
	"origin/internal/itemdefs"
	"origin/internal/objectdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/require"
)

type burnerOutcomeIDAllocator struct{ next types.EntityID }

func (a *burnerOutcomeIDAllocator) GetFreeID() types.EntityID {
	a.next++
	return a.next
}

type burnerOutcomePersister struct {
	records         []inventory.DroppedItemPersistenceRecord
	persistErr      error
	despawned       []types.EntityID
	replacedSources []types.EntityID
}

func (p *burnerOutcomePersister) PersistDroppedObject(
	entityID types.EntityID,
	typeID int,
	region, x, y, layer, chunkX, chunkY int,
	objectData, inventoryData json.RawMessage,
) error {
	if p.persistErr != nil {
		return p.persistErr
	}
	p.records = append(p.records, inventory.DroppedItemPersistenceRecord{
		EntityID: entityID, TypeID: typeID, Region: region, X: x, Y: y, Layer: layer,
		ChunkX: chunkX, ChunkY: chunkY, ObjectData: objectData, InventoryData: inventoryData,
	})
	return nil
}

func (p *burnerOutcomePersister) DeleteObject(_ int, _ types.EntityID) error { return nil }

func (p *burnerOutcomePersister) ReplaceObjectWithDroppedItem(record inventory.DroppedItemPersistenceRecord, _ int, sourceID types.EntityID) error {
	if p.persistErr != nil {
		return p.persistErr
	}
	p.records = append(p.records, record)
	p.replacedSources = append(p.replacedSources, sourceID)
	return nil
}

func (p *burnerOutcomePersister) RecordChunkObjectDespawn(_ *core.Chunk, entityID types.EntityID) {
	p.despawned = append(p.despawned, entityID)
}

type burnerTestChunkManager struct {
	chunk *core.Chunk
	added []types.Handle
}

func (m *burnerTestChunkManager) GetChunkFast(_ types.ChunkCoord) *core.Chunk { return m.chunk }

func (m *burnerTestChunkManager) AddStaticToChunkSpatial(handle types.Handle, _, _, _, _ int) {
	m.added = append(m.added, handle)
}

func TestBurnerExhaustionCreatesDurableAshAtSourceLocation(t *testing.T) {
	previousItems := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{
		DefID: 91, Key: "ash", Resource: "items/ash", Size: itemdefs.Size{W: 1, H: 1},
	}}))

	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 42})
	source := w.Spawn(500, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{IsStatic: true, Quality: 7, Region: 3, Layer: 2})
		ecs.AddComponent(w, h, components.CreateTransform(101, 202, 0))
		ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: 4, CurrentChunkY: 5})
	})

	persister := &burnerOutcomePersister{}
	handler := &burnerExhaustionHandler{idAllocator: &burnerOutcomeIDAllocator{next: 900}}
	params, err := handler.outcomeParams(w, source, "ash")
	require.NoError(t, err)
	require.NoError(t, inventory.PersistDroppedEntity(persister, params, nil))
	result, err := inventory.SpawnDroppedEntity(w, params)
	require.NoError(t, err)

	require.Len(t, persister.records, 1)
	require.Equal(t, types.EntityID(901), persister.records[0].EntityID)
	require.Equal(t, 101, persister.records[0].X)
	require.Equal(t, 202, persister.records[0].Y)
	require.Equal(t, 3, persister.records[0].Region)
	require.Equal(t, 2, persister.records[0].Layer)
	dropped, ok := ecs.GetComponent[components.DroppedItem](w, result.DroppedHandle)
	require.True(t, ok)
	require.Equal(t, types.EntityID(901), dropped.ContainedItemID)
	transform, ok := ecs.GetComponent[components.Transform](w, result.DroppedHandle)
	require.True(t, ok)
	require.Equal(t, float64(101), transform.X)
	require.Equal(t, float64(202), transform.Y)
}

func TestBurnerExhaustionPersistsAshBeforeRemovingSource(t *testing.T) {
	previousItems := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{
		DefID: 91, Key: "ash", Resource: "items/ash", Size: itemdefs.Size{W: 1, H: 1},
	}}))

	w := ecs.NewWorldForTesting()
	source := w.Spawn(500, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{IsStatic: true, Region: 3})
		ecs.AddComponent(w, h, components.CreateTransform(101, 202, 0))
		ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: 4, CurrentChunkY: 5})
	})
	chunk := core.NewChunk(types.ChunkCoord{X: 4, Y: 5}, 0, 0, 128)
	chunk.Spatial().AddStatic(source, 101, 202)
	persister := &burnerOutcomePersister{}
	chunkManager := &burnerTestChunkManager{chunk: chunk}
	handler := &burnerExhaustionHandler{
		idAllocator:          &burnerOutcomeIDAllocator{next: 900},
		replacementPersister: persister,
		despawnPersister:     persister,
		chunkManager:         chunkManager,
	}

	require.True(t, handler.handle(w, source, &objectdefs.BurnerBehaviorConfig{DropItem: "ash", Despawn: true}))
	require.False(t, w.Alive(source))
	require.Len(t, persister.records, 1)
	require.Equal(t, []types.EntityID{500}, persister.replacedSources)
	require.Equal(t, []types.EntityID{500}, persister.despawned)
	require.Len(t, chunkManager.added, 1)
	require.NotContains(t, chunk.GetHandles(), source)

	droppedHandle := chunkManager.added[0]
	dropped, ok := ecs.GetComponent[components.DroppedItem](w, droppedHandle)
	require.True(t, ok)
	require.Equal(t, types.EntityID(901), dropped.ContainedItemID)
}

func TestBurnerExhaustionLeavesSourceWhenAshPersistenceFails(t *testing.T) {
	previousItems := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousItems) })
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{
		DefID: 91, Key: "ash", Resource: "items/ash", Size: itemdefs.Size{W: 1, H: 1},
	}}))

	w := ecs.NewWorldForTesting()
	source := w.Spawn(500, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{IsStatic: true, Region: 3})
		ecs.AddComponent(w, h, components.CreateTransform(101, 202, 0))
		ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: 4, CurrentChunkY: 5})
	})
	chunk := core.NewChunk(types.ChunkCoord{X: 4, Y: 5}, 0, 0, 128)
	chunk.Spatial().AddStatic(source, 101, 202)
	persister := &burnerOutcomePersister{persistErr: errors.New("database unavailable")}
	handler := &burnerExhaustionHandler{
		idAllocator:          &burnerOutcomeIDAllocator{next: 900},
		replacementPersister: persister,
		despawnPersister:     persister,
		chunkManager:         &burnerTestChunkManager{chunk: chunk},
	}

	require.False(t, handler.handle(w, source, &objectdefs.BurnerBehaviorConfig{DropItem: "ash", Despawn: true}))
	require.True(t, w.Alive(source))
	require.Empty(t, persister.records)
	require.Empty(t, persister.despawned)
	require.Contains(t, chunk.GetHandles(), source)
}

func TestBurnerExhaustionReconcilesExpiredRestoreBeforeExposure(t *testing.T) {
	previousItems := itemdefs.Global()
	previousObjects := objectdefs.Global()
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousItems)
		objectdefs.SetGlobalForTesting(previousObjects)
	})
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{
		DefID: 91, Key: "ash", Resource: "items/ash", Size: itemdefs.Size{W: 1, H: 1},
	}}))
	objectdefs.SetGlobalForTesting(objectdefs.NewRegistry([]objectdefs.ObjectDef{{
		DefID: 72, BurnerConfig: &objectdefs.BurnerBehaviorConfig{DropItem: "ash", Despawn: true},
	}}))

	w := ecs.NewWorldForTesting()
	source := w.Spawn(500, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.EntityInfo{TypeID: 72, IsStatic: true, Region: 3})
		ecs.AddComponent(w, h, components.CreateTransform(101, 202, 0))
		ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: 4, CurrentChunkY: 5})
		ecs.AddComponent(w, h, components.ObjectInternalState{State: &components.RuntimeObjectState{Behaviors: map[string]any{
			"burner": &components.BurnerBehaviorState{Fuel: 0},
		}}})
	})
	chunk := core.NewChunk(types.ChunkCoord{X: 4, Y: 5}, 0, 0, 128)
	persister := &burnerOutcomePersister{}
	handler := &burnerExhaustionHandler{
		idAllocator:          &burnerOutcomeIDAllocator{next: 900},
		replacementPersister: persister,
		despawnPersister:     persister,
		chunkManager:         &burnerTestChunkManager{chunk: chunk},
	}

	require.False(t, handler.ReconcileRestoredObject(w, source))
	require.False(t, w.Alive(source))
	require.Len(t, persister.records, 1)
}
