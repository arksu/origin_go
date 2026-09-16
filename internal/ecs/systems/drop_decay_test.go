package systems

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"

	"go.uber.org/zap"
)

type recordingDropObjectDeleter struct {
	calls    int
	region   int
	entityID types.EntityID
}

func (d *recordingDropObjectDeleter) DeleteObject(region int, entityID types.EntityID) error {
	d.calls++
	d.region = region
	d.entityID = entityID
	return nil
}

type recordingDroppedSpatialRemover struct {
	calls  int
	handle types.Handle
	chunkX int
	chunkY int
	x      int
	y      int
}

func (r *recordingDroppedSpatialRemover) RemoveStaticFromChunkSpatial(handle types.Handle, chunkX, chunkY, x, y int) {
	r.calls++
	r.handle = handle
	r.chunkX = chunkX
	r.chunkY = chunkY
	r.x = x
	r.y = y
}

func TestDropDecayDeletesExpiredDroppedItemFromPersistenceAndECS(t *testing.T) {
	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 110})
	entityID := types.EntityID(801)
	droppedHandle := w.Spawn(entityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.DroppedItem{
			DropTime:        100,
			ContainedItemID: entityID,
		})
		ecs.AddComponent(w, h, components.EntityInfo{Region: 2, IsStatic: true})
		ecs.AddComponent(w, h, components.ChunkRef{CurrentChunkX: 3, CurrentChunkY: 4})
		ecs.AddComponent(w, h, components.Transform{X: 10, Y: 20})
	})
	rootHandle := w.SpawnWithoutExternalID()
	nestedHandle := w.SpawnWithoutExternalID()
	ecs.AddComponent(w, rootHandle, components.InventoryContainer{
		OwnerID: entityID,
		Kind:    constt.InventoryDroppedItem,
		Items:   []components.InvItem{{ItemID: entityID, TypeID: 1, Quantity: 1}},
	})
	ecs.AddComponent(w, nestedHandle, components.InventoryContainer{
		OwnerID: entityID,
		Kind:    constt.InventoryGrid,
	})
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	refIndex.Add(constt.InventoryDroppedItem, entityID, 0, rootHandle)
	refIndex.Add(constt.InventoryGrid, entityID, 0, nestedHandle)

	deleter := &recordingDropObjectDeleter{}
	spatial := &recordingDroppedSpatialRemover{}
	system := NewDropDecaySystem(deleter, spatial, zap.NewNop())
	if got := system.UpdateEveryNTicks(); got != dropDecaySweepIntervalTicks {
		t.Fatalf("drop decay interval = %d, want %d", got, dropDecaySweepIntervalTicks)
	}
	system.Update(w, 0)

	if deleter.calls != 1 || deleter.region != 2 || deleter.entityID != entityID {
		t.Fatalf("unexpected persistence deletion: %+v", deleter)
	}
	if spatial.calls != 1 || spatial.handle != droppedHandle || spatial.chunkX != 3 || spatial.chunkY != 4 || spatial.x != 10 || spatial.y != 20 {
		t.Fatalf("unexpected spatial removal: %+v", spatial)
	}
	if w.Alive(droppedHandle) || w.Alive(rootHandle) || w.Alive(nestedHandle) {
		t.Fatal("expired drop and all of its inventory containers must be despawned")
	}
	if _, found := refIndex.Lookup(constt.InventoryDroppedItem, entityID, 0); found {
		t.Fatal("dropped root ref was not removed")
	}
	if _, found := refIndex.Lookup(constt.InventoryGrid, entityID, 0); found {
		t.Fatal("nested ref was not removed")
	}
}
