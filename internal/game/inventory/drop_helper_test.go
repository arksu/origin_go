package inventory

import (
	"encoding/json"
	"errors"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/ecs/systems"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"go.uber.org/zap"
)

type failingDroppedPersister struct {
	persistErr   error
	deleteErr    error
	persisted    int
	deleted      int
	persistedIDs []types.EntityID
	records      []DroppedItemPersistenceRecord
	inventories  []systems.InventorySnapshot
}

type fixedDroppedIDAllocator struct{}

func (fixedDroppedIDAllocator) GetFreeID() types.EntityID {
	return 9999
}

type dropTestIDAllocator struct {
	next types.EntityID
}

func (a *dropTestIDAllocator) GetFreeID() types.EntityID {
	a.next++
	return a.next
}

func (p *failingDroppedPersister) PersistDroppedObject(
	entityID types.EntityID,
	_ int,
	_ int,
	_ int,
	_ int,
	_ int,
	_ int,
	_ int,
	_ json.RawMessage,
	_ json.RawMessage,
) error {
	p.persisted++
	p.persistedIDs = append(p.persistedIDs, entityID)
	return p.persistErr
}

func (p *failingDroppedPersister) DeleteObject(_ int, _ types.EntityID) error {
	p.deleted++
	return p.deleteErr
}

func (p *failingDroppedPersister) PersistDroppedObjectBatchWithPlayerInventories(
	records []DroppedItemPersistenceRecord,
	inventories []systems.InventorySnapshot,
) error {
	p.persisted += len(records)
	p.records = append([]DroppedItemPersistenceRecord(nil), records...)
	for _, record := range records {
		p.persistedIDs = append(p.persistedIDs, record.EntityID)
	}
	p.inventories = append([]systems.InventorySnapshot(nil), inventories...)
	return p.persistErr
}

func (p *failingDroppedPersister) DeleteDroppedObjectWithPlayerInventories(
	_ int,
	_ types.EntityID,
	inventories []systems.InventorySnapshot,
) error {
	p.deleted++
	p.inventories = append([]systems.InventorySnapshot(nil), inventories...)
	return p.deleteErr
}

func inventorySnapshotData(t *testing.T, snapshots []systems.InventorySnapshot, kind constt.InventoryKind) InventoryDataV1 {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.Kind != int16(kind) {
			continue
		}
		var data InventoryDataV1
		if err := json.Unmarshal(snapshot.Data, &data); err != nil {
			t.Fatalf("unmarshal inventory snapshot: %v", err)
		}
		return data
	}
	t.Fatalf("missing inventory snapshot for kind %d", kind)
	return InventoryDataV1{}
}

func TestSpawnDroppedEntityRequiresObjectIDToMatchItemID(t *testing.T) {
	w := ecs.NewWorldForTesting()
	_, err := SpawnDroppedEntity(w, SpawnDroppedEntityParams{
		DroppedEntityID:   101,
		ItemID:            102,
		TypeID:            1,
		Quantity:          1,
		NowRuntimeSeconds: 5,
	})
	if err == nil {
		t.Fatal("expected mismatched object and item IDs to be rejected")
	}
}

func TestSpawnDroppedEntityRequiresExactlyOneItem(t *testing.T) {
	w := ecs.NewWorldForTesting()
	_, err := SpawnDroppedEntity(w, SpawnDroppedEntityParams{
		DroppedEntityID:   101,
		ItemID:            101,
		TypeID:            1,
		Quantity:          2,
		NowRuntimeSeconds: 5,
	})
	if err == nil {
		t.Fatal("expected a stack to be rejected for one dropped entity")
	}
}

func TestSpawnDroppedEntityCreatesOneStaticItemContainer(t *testing.T) {
	w := ecs.NewWorldForTesting()
	entityID := types.EntityID(101)
	result, err := SpawnDroppedEntity(w, SpawnDroppedEntityParams{
		DroppedEntityID:   entityID,
		ItemID:            entityID,
		TypeID:            7,
		Resource:          "items/test",
		Quality:           3,
		Quantity:          1,
		W:                 1,
		H:                 1,
		DropX:             10,
		DropY:             20,
		Region:            1,
		Layer:             2,
		ChunkX:            3,
		ChunkY:            4,
		DropperID:         99,
		NowRuntimeSeconds: 42,
	})
	if err != nil {
		t.Fatalf("SpawnDroppedEntity() error = %v", err)
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, result.DroppedHandle)
	if !hasInfo || info.TypeID != constt.DroppedItemTypeID || !info.IsStatic {
		t.Fatalf("dropped entity must be static dropped-item metadata: %+v", info)
	}
	if _, hasCollider := ecs.GetComponent[components.Collider](w, result.DroppedHandle); hasCollider {
		t.Fatal("dropped entity must not have a collider")
	}
	appearance, hasAppearance := ecs.GetComponent[components.Appearance](w, result.DroppedHandle)
	if !hasAppearance || appearance.Name != nil || appearance.Resource != "items/test" {
		t.Fatalf("unexpected dropped appearance: %+v", appearance)
	}
	dropped, hasDropped := ecs.GetComponent[components.DroppedItem](w, result.DroppedHandle)
	if !hasDropped || dropped.ContainedItemID != entityID || dropped.DropTime != 42 {
		t.Fatalf("unexpected dropped metadata: %+v", dropped)
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	containerHandle, found := refIndex.Lookup(constt.InventoryDroppedItem, entityID, 0)
	if !found || containerHandle != result.ContainerHandle {
		t.Fatalf("dropped root inventory was not indexed: handle=%d found=%v", containerHandle, found)
	}
	container, hasContainer := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
	if !hasContainer || len(container.Items) != 1 || container.Items[0].ItemID != entityID {
		t.Fatalf("unexpected dropped root inventory: %+v", container)
	}
}

func TestDropToWorldKeepsSourceItemWhenPersistenceFails(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createTestRegistry())
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(w, playerID, playerHandle)
	ecs.AddComponent(w, playerHandle, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, playerHandle, components.EntityInfo{Region: 1, Layer: 0})
	ecs.AddComponent(w, playerHandle, components.ChunkRef{CurrentChunkX: 0, CurrentChunkY: 0})

	item := components.InvItem{
		ItemID:   2001,
		TypeID:   1,
		Resource: "items/test",
		Quantity: 1,
		W:        1,
		H:        1,
	}
	addItemToContainer(w, gridHandle, item)

	persister := &failingDroppedPersister{persistErr: errors.New("database unavailable")}
	service := NewInventoryOperationService(zap.NewNop(), fixedDroppedIDAllocator{}, persister)
	result := service.ExecuteDropToWorld(w, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId: uint64(playerID),
		},
		ItemId: uint64(item.ItemID),
	}, nil)

	if result.Success {
		t.Fatalf("drop should fail when persistence fails: %+v", result)
	}
	if persister.persisted != 1 {
		t.Fatalf("expected one persistence attempt, got %d", persister.persisted)
	}
	grid, _ := ecs.GetComponent[components.InventoryContainer](w, gridHandle)
	if len(grid.Items) != 1 || grid.Items[0].ItemID != item.ItemID {
		t.Fatalf("source item was changed despite persistence failure: %+v", grid.Items)
	}
	if handle := w.GetHandleByEntityID(item.ItemID); handle != types.InvalidHandle {
		t.Fatalf("non-persisted dropped entity must not spawn, got handle %d", handle)
	}
}

func TestDropToWorldSplitsStackIntoOneEntityPerItem(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createTestRegistry())
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(w, playerID, playerHandle)
	ecs.AddComponent(w, playerHandle, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, playerHandle, components.EntityInfo{Region: 1, Layer: 0})
	ecs.AddComponent(w, playerHandle, components.ChunkRef{CurrentChunkX: 0, CurrentChunkY: 0})

	const sourceItemID = types.EntityID(2001)
	addItemToContainer(w, gridHandle, components.InvItem{
		ItemID:   sourceItemID,
		TypeID:   1,
		Resource: "items/test",
		Quantity: 3,
		W:        1,
		H:        1,
	})

	persister := &failingDroppedPersister{}
	service := NewInventoryOperationService(zap.NewNop(), &dropTestIDAllocator{next: 9000}, persister)
	result := service.ExecuteDropToWorld(w, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId: uint64(playerID),
		},
		ItemId: uint64(sourceItemID),
	}, nil)

	if !result.Success {
		t.Fatalf("drop stack failed: %+v", result)
	}
	wantIDs := []types.EntityID{sourceItemID, 9001, 9002}
	if len(result.SpawnedDroppedEntityIDs) != len(wantIDs) {
		t.Fatalf("spawned IDs = %+v, want %+v", result.SpawnedDroppedEntityIDs, wantIDs)
	}
	for index, wantID := range wantIDs {
		if gotID := result.SpawnedDroppedEntityIDs[index]; gotID != wantID {
			t.Fatalf("spawned ID at %d = %d, want %d", index, gotID, wantID)
		}
	}
	if len(persister.persistedIDs) != len(wantIDs) {
		t.Fatalf("persisted IDs = %+v, want %+v", persister.persistedIDs, wantIDs)
	}
	if len(persister.inventories) != 2 {
		t.Fatalf("atomic drop must include both player root inventories, got %d", len(persister.inventories))
	}
	if gridData := inventorySnapshotData(t, persister.inventories, constt.InventoryGrid); len(gridData.Items) != 0 {
		t.Fatalf("atomic drop snapshot must remove source item: %+v", gridData.Items)
	}
	if len(persister.records) != len(wantIDs) {
		t.Fatalf("atomic drop records = %d, want %d", len(persister.records), len(wantIDs))
	}
	var objectData droppedItemData
	if err := json.Unmarshal(persister.records[0].ObjectData, &objectData); err != nil {
		t.Fatalf("unmarshal dropped object data: %v", err)
	}
	if objectData.TimeBasis != constt.DroppedItemTimeBasisRuntimeSecondsV1 {
		t.Fatalf("drop time basis = %q, want %q", objectData.TimeBasis, constt.DroppedItemTimeBasisRuntimeSecondsV1)
	}

	grid, _ := ecs.GetComponent[components.InventoryContainer](w, gridHandle)
	if len(grid.Items) != 0 {
		t.Fatalf("source stack must be removed after full drop: %+v", grid.Items)
	}

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	for _, droppedID := range wantIDs {
		droppedHandle := w.GetHandleByEntityID(droppedID)
		if droppedHandle == types.InvalidHandle {
			t.Fatalf("missing dropped entity %d", droppedID)
		}
		dropped, found := ecs.GetComponent[components.DroppedItem](w, droppedHandle)
		if !found || dropped.ContainedItemID != droppedID {
			t.Fatalf("invalid dropped metadata for %d: %+v", droppedID, dropped)
		}
		containerHandle, found := refIndex.Lookup(constt.InventoryDroppedItem, droppedID, 0)
		if !found {
			t.Fatalf("missing root inventory for dropped entity %d", droppedID)
		}
		container, found := ecs.GetComponent[components.InventoryContainer](w, containerHandle)
		if !found || container.OwnerID != droppedID || len(container.Items) != 1 ||
			container.Items[0].ItemID != droppedID || container.Items[0].Quantity != 1 {
			t.Fatalf("dropped entity %d must contain exactly one item: %+v", droppedID, container)
		}
	}
}

func TestDropToWorldPartialStackKeepsSourceIdentity(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createTestRegistry())
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(w, playerID, playerHandle)
	ecs.AddComponent(w, playerHandle, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, playerHandle, components.EntityInfo{Region: 1, Layer: 0})
	ecs.AddComponent(w, playerHandle, components.ChunkRef{CurrentChunkX: 0, CurrentChunkY: 0})

	const sourceItemID = types.EntityID(3001)
	addItemToContainer(w, gridHandle, components.InvItem{
		ItemID:   sourceItemID,
		TypeID:   1,
		Resource: "items/test",
		Quantity: 3,
		W:        1,
		H:        1,
	})

	dropQuantity := uint32(1)
	service := NewInventoryOperationService(zap.NewNop(), &dropTestIDAllocator{next: 9100}, &failingDroppedPersister{})
	result := service.ExecuteDropToWorld(w, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId: uint64(playerID),
		},
		ItemId:   uint64(sourceItemID),
		Quantity: &dropQuantity,
	}, nil)

	if !result.Success || len(result.SpawnedDroppedEntityIDs) != 1 || result.SpawnedDroppedEntityIDs[0] != 9101 {
		t.Fatalf("unexpected partial drop result: %+v", result)
	}
	grid, _ := ecs.GetComponent[components.InventoryContainer](w, gridHandle)
	if len(grid.Items) != 1 || grid.Items[0].ItemID != sourceItemID || grid.Items[0].Quantity != 2 {
		t.Fatalf("partial drop must retain source stack identity and quantity: %+v", grid.Items)
	}
}

func TestDropToWorldDetachesNestedContainerWhenSourceSliceShifts(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(w, playerID, playerHandle)
	ecs.AddComponent(w, playerHandle, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, playerHandle, components.EntityInfo{Region: 1, Layer: 0})
	ecs.AddComponent(w, playerHandle, components.ChunkRef{CurrentChunkX: 0, CurrentChunkY: 0})

	const bagItemID = types.EntityID(4001)
	addItemToContainer(w, gridHandle, components.InvItem{
		ItemID:   bagItemID,
		TypeID:   200,
		Resource: "items/bag_seed_empty.png",
		Quantity: 1,
		W:        1,
		H:        1,
	})
	// A second element makes removal overwrite the address returned by
	// FindItemInContainer. Its different quantity catches accidental reads from
	// that shifted pointer after the mutation.
	addItemToContainer(w, gridHandle, components.InvItem{
		ItemID:   4002,
		TypeID:   201,
		Resource: "wheat_seed_mini.png",
		Quantity: 2,
		W:        1,
		H:        1,
	})

	nestedHandle := createGridContainer(w, bagItemID, 0, 1, 1)
	addItemToContainer(w, nestedHandle, components.InvItem{
		ItemID:   4003,
		TypeID:   201,
		Resource: "wheat_seed_mini.png",
		Quantity: 1,
		W:        1,
		H:        1,
	})
	ecs.MutateComponent[components.InventoryOwner](w, playerHandle, func(owner *components.InventoryOwner) bool {
		owner.Inventories = append(owner.Inventories, components.InventoryLink{
			Kind:    constt.InventoryGrid,
			OwnerID: bagItemID,
			Handle:  nestedHandle,
		})
		return true
	})
	ecs.GetResource[ecs.InventoryRefIndex](w).Add(constt.InventoryGrid, bagItemID, 0, nestedHandle)

	persister := &failingDroppedPersister{}
	service := NewInventoryOperationService(zap.NewNop(), fixedDroppedIDAllocator{}, persister)
	result := service.ExecuteDropToWorld(w, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId: uint64(playerID),
		},
		ItemId: uint64(bagItemID),
	}, nil)
	if !result.Success {
		t.Fatalf("drop container item failed: %+v", result)
	}

	owner, _ := ecs.GetComponent[components.InventoryOwner](w, playerHandle)
	for _, link := range owner.Inventories {
		if link.Kind == constt.InventoryGrid && link.OwnerID == bagItemID {
			t.Fatal("dropped nested container must no longer be linked to the player")
		}
	}
	if len(persister.records) != 1 {
		t.Fatalf("dropped records = %d, want 1", len(persister.records))
	}
	var droppedInventory InventoryDataV1
	if err := json.Unmarshal(persister.records[0].InventoryData, &droppedInventory); err != nil {
		t.Fatalf("unmarshal dropped inventory: %v", err)
	}
	if len(droppedInventory.Items) != 1 || droppedInventory.Items[0].NestedInventory == nil || len(droppedInventory.Items[0].NestedInventory.Items) != 1 {
		t.Fatalf("dropped nested inventory was not persisted: %+v", droppedInventory)
	}
}

func TestDropFromNestedInventoryBumpsAndPersistsPlayerRoot(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w, playerID, playerHandle, rootHandle, nestedHandle, _, bagItemID := setupGiveItemWorld(t)
	ecs.AddComponent(w, playerHandle, components.Transform{X: 10, Y: 20})
	ecs.AddComponent(w, playerHandle, components.EntityInfo{Region: 1, Layer: 0})
	ecs.AddComponent(w, playerHandle, components.ChunkRef{CurrentChunkX: 0, CurrentChunkY: 0})

	const seedItemID = types.EntityID(5101)
	addItemToContainer(w, nestedHandle, components.InvItem{
		ItemID:   seedItemID,
		TypeID:   201,
		Resource: "wheat_seed_mini.png",
		Quantity: 1,
		W:        1,
		H:        1,
	})
	rootBefore, _ := ecs.GetComponent[components.InventoryContainer](w, rootHandle)

	persister := &failingDroppedPersister{}
	service := NewInventoryOperationService(zap.NewNop(), fixedDroppedIDAllocator{}, persister)
	result := service.ExecuteDropToWorld(w, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(seedItemID),
	}, nil)
	if !result.Success {
		t.Fatalf("drop from nested inventory failed: %+v", result)
	}

	rootAfter, _ := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
	if rootAfter.Version != rootBefore.Version+1 {
		t.Fatalf("player root version = %d, want %d", rootAfter.Version, rootBefore.Version+1)
	}
	rootData := inventorySnapshotData(t, persister.inventories, constt.InventoryGrid)
	if rootData.Version != int(rootAfter.Version) || len(rootData.Items) != 1 || rootData.Items[0].NestedInventory == nil || len(rootData.Items[0].NestedInventory.Items) != 0 {
		t.Fatalf("atomic player root snapshot must include emptied nested inventory: %+v", rootData)
	}
}

func TestPickupIntoNestedInventoryBumpsAndPersistsPlayerRoot(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w, playerID, playerHandle, rootHandle, _, _, bagItemID := setupGiveItemWorld(t)
	ecs.AddComponent(w, playerHandle, components.Transform{X: 10, Y: 20})

	const droppedID = types.EntityID(5201)
	if _, err := SpawnDroppedEntity(w, SpawnDroppedEntityParams{
		DroppedEntityID:   droppedID,
		ItemID:            droppedID,
		TypeID:            201,
		Resource:          "wheat_seed_mini.png",
		Quantity:          1,
		W:                 1,
		H:                 1,
		DropX:             10,
		DropY:             20,
		Region:            1,
		NowRuntimeSeconds: 1,
	}); err != nil {
		t.Fatalf("spawn dropped seed: %v", err)
	}
	rootBefore, _ := ecs.GetComponent[components.InventoryContainer](w, rootHandle)

	persister := &failingDroppedPersister{}
	service := NewInventoryOperationService(zap.NewNop(), fixedDroppedIDAllocator{}, persister)
	result := service.ExecutePickupFromWorld(w, playerID, playerHandle, droppedID, &netproto.InventoryRef{
		Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
		OwnerId: uint64(bagItemID),
	})
	if !result.Success {
		t.Fatalf("pickup into nested inventory failed: %+v", result)
	}

	rootAfter, _ := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
	if rootAfter.Version != rootBefore.Version+1 {
		t.Fatalf("player root version = %d, want %d", rootAfter.Version, rootBefore.Version+1)
	}
	rootData := inventorySnapshotData(t, persister.inventories, constt.InventoryGrid)
	if rootData.Version != int(rootAfter.Version) || len(rootData.Items) != 1 || rootData.Items[0].NestedInventory == nil || len(rootData.Items[0].NestedInventory.Items) != 1 || rootData.Items[0].NestedInventory.Items[0].ItemID != uint64(droppedID) {
		t.Fatalf("atomic player root snapshot must include picked-up nested item: %+v", rootData)
	}
}

func TestPickupKeepsDroppedItemWhenDeletionFails(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(createTestRegistry())
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(w, playerID, playerHandle)
	ecs.AddComponent(w, playerHandle, components.Transform{X: 10, Y: 20})

	droppedID := types.EntityID(2002)
	_, err := SpawnDroppedEntity(w, SpawnDroppedEntityParams{
		DroppedEntityID:   droppedID,
		ItemID:            droppedID,
		TypeID:            1,
		Resource:          "items/test",
		Quantity:          1,
		W:                 1,
		H:                 1,
		DropX:             10,
		DropY:             20,
		Region:            1,
		NowRuntimeSeconds: 1,
	})
	if err != nil {
		t.Fatalf("spawn dropped item: %v", err)
	}

	persister := &failingDroppedPersister{deleteErr: errors.New("database unavailable")}
	service := NewInventoryOperationService(zap.NewNop(), fixedDroppedIDAllocator{}, persister)
	result := service.ExecutePickupFromWorld(w, playerID, playerHandle, droppedID, &netproto.InventoryRef{
		Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
		OwnerId: uint64(playerID),
	})

	if result.Success {
		t.Fatalf("pickup should fail when durable deletion fails: %+v", result)
	}
	if persister.deleted != 1 {
		t.Fatalf("expected one deletion attempt, got %d", persister.deleted)
	}
	if handle := w.GetHandleByEntityID(droppedID); handle == types.InvalidHandle {
		t.Fatal("dropped entity was removed despite persistence failure")
	}
	grid, _ := ecs.GetComponent[components.InventoryContainer](w, gridHandle)
	if len(grid.Items) != 0 {
		t.Fatalf("player inventory changed despite persistence failure: %+v", grid.Items)
	}
	if len(persister.inventories) != 2 {
		t.Fatalf("atomic pickup must include both player root inventories, got %d", len(persister.inventories))
	}
	gridData := inventorySnapshotData(t, persister.inventories, constt.InventoryGrid)
	if len(gridData.Items) != 1 || gridData.Items[0].ItemID != uint64(droppedID) {
		t.Fatalf("atomic pickup snapshot must contain dropped item: %+v", gridData.Items)
	}
}
