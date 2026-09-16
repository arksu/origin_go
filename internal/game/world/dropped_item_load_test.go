package world

import (
	"encoding/json"
	"errors"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"github.com/sqlc-dev/pqtype"
)

type recordingObjectDeleter struct {
	region   int
	entityID types.EntityID
	calls    int
}

type recordingObjectDataUpdater struct {
	region   int
	entityID types.EntityID
	data     json.RawMessage
	calls    int
	err      error
}

func (u *recordingObjectDataUpdater) UpdateObjectData(region int, entityID types.EntityID, data json.RawMessage) error {
	u.region = region
	u.entityID = entityID
	u.data = append([]byte(nil), data...)
	u.calls++
	return u.err
}

func (d *recordingObjectDeleter) DeleteObject(region int, entityID types.EntityID) error {
	d.region = region
	d.entityID = entityID
	d.calls++
	return nil
}

func TestObjectFactoryDeletesExpiredDroppedItemDuringLoad(t *testing.T) {
	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 110})

	entityID := types.EntityID(701)
	metadata, err := json.Marshal(DroppedItemData{
		HasInventory:    true,
		ContainedItemID: uint64(entityID),
		DropTime:        100,
		DropperID:       7,
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	deleter := &recordingObjectDeleter{}
	factory := NewObjectFactory(nil)
	factory.SetObjectDeleter(deleter)
	_, err = factory.Build(w, &repository.Object{
		ID:     int64(entityID),
		TypeID: constt.DroppedItemTypeID,
		Region: 3,
		Data: pqtype.NullRawMessage{
			RawMessage: metadata,
			Valid:      true,
		},
	}, nil)
	if !errors.Is(err, ErrDroppedItemExpired) {
		t.Fatalf("Build() error = %v, want ErrDroppedItemExpired", err)
	}
	if deleter.calls != 1 || deleter.region != 3 || deleter.entityID != entityID {
		t.Fatalf("unexpected delete request: %+v", deleter)
	}
	if handle := w.GetHandleByEntityID(entityID); handle != types.InvalidHandle {
		t.Fatalf("expired object must not enter ECS, got handle %d", handle)
	}
}

func TestParseDroppedItemDataRejectsMismatchedContainedItemID(t *testing.T) {
	_, err := parseDroppedItemData([]byte(`{
        "has_inventory": true,
        "contained_item_id": 702,
        "drop_time": 1,
        "dropper_id": 7
    }`), 701)
	if err == nil {
		t.Fatal("expected mismatched contained_item_id to be rejected")
	}
}

func TestObjectFactoryLoadsDroppedItemAsStaticSingleItemEntity(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{
		DefID:    9,
		Key:      "stone",
		Resource: "items/stone.png",
		Size:     itemdefs.Size{W: 1, H: 1},
	}}))
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 105})
	entityID := types.EntityID(703)
	metadata, err := json.Marshal(DroppedItemData{
		HasInventory:    true,
		ContainedItemID: uint64(entityID),
		DropTime:        100,
		DropperID:       7,
	})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	inventoryData, err := json.Marshal(objectInventoryDataV1{
		Kind:    uint8(constt.InventoryDroppedItem),
		Key:     0,
		Version: 1,
		Items: []objectInventoryItem{{
			ItemID:   uint64(entityID),
			TypeID:   9,
			Quantity: 1,
		}},
	})
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}

	factory := NewObjectFactory(nil)
	handle, err := factory.Build(w, &repository.Object{
		ID:     int64(entityID),
		TypeID: constt.DroppedItemTypeID,
		Region: 3,
		X:      10,
		Y:      20,
		Data: pqtype.NullRawMessage{
			RawMessage: metadata,
			Valid:      true,
		},
	}, []repository.Inventory{{
		OwnerID:      int64(entityID),
		Kind:         int16(constt.InventoryDroppedItem),
		InventoryKey: 0,
		Data:         inventoryData,
		Version:      1,
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	info, hasInfo := ecs.GetComponent[components.EntityInfo](w, handle)
	if !hasInfo || !info.IsStatic || info.TypeID != constt.DroppedItemTypeID {
		t.Fatalf("unexpected static dropped item info: %+v", info)
	}
	if _, hasCollider := ecs.GetComponent[components.Collider](w, handle); hasCollider {
		t.Fatal("loaded dropped item must not have a collider")
	}
	appearance, hasAppearance := ecs.GetComponent[components.Appearance](w, handle)
	if !hasAppearance || appearance.Name != nil || appearance.Resource != "items/stone.png" {
		t.Fatalf("unexpected computed appearance: %+v", appearance)
	}
	dropped, hasDropped := ecs.GetComponent[components.DroppedItem](w, handle)
	if !hasDropped || dropped.ContainedItemID != entityID || dropped.DropTime != 100 {
		t.Fatalf("unexpected dropped metadata: %+v", dropped)
	}

	rootHandle, found := ecs.GetResource[ecs.InventoryRefIndex](w).Lookup(constt.InventoryDroppedItem, entityID, 0)
	if !found {
		t.Fatal("loaded dropped root inventory was not indexed")
	}
	root, hasRoot := ecs.GetComponent[components.InventoryContainer](w, rootHandle)
	if !hasRoot || root.OwnerID != entityID || len(root.Items) != 1 || root.Items[0].ItemID != entityID || root.Items[0].Quantity != 1 {
		t.Fatalf("unexpected loaded dropped root inventory: %+v", root)
	}
}

func TestObjectFactoryMigratesLegacyUnixDropTimeOnLoad(t *testing.T) {
	previousRegistry := itemdefs.Global()
	itemdefs.SetGlobalForTesting(itemdefs.NewRegistry([]itemdefs.ItemDef{{
		DefID:    9,
		Key:      "stone",
		Resource: "items/stone.png",
		Size:     itemdefs.Size{W: 1, H: 1},
	}}))
	t.Cleanup(func() {
		itemdefs.SetGlobalForTesting(previousRegistry)
	})

	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 500})
	entityID := types.EntityID(704)
	legacyMetadata := []byte(`{
        "has_inventory": true,
        "contained_item_id": 704,
        "drop_time": 1700000000,
        "dropper_id": 7
    }`)
	inventoryData, err := json.Marshal(objectInventoryDataV1{
		Kind:    uint8(constt.InventoryDroppedItem),
		Key:     0,
		Version: 1,
		Items: []objectInventoryItem{{
			ItemID:   uint64(entityID),
			TypeID:   9,
			Quantity: 1,
		}},
	})
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}

	updater := &recordingObjectDataUpdater{}
	factory := NewObjectFactory(nil)
	factory.SetObjectDataUpdater(updater)
	handle, err := factory.Build(w, &repository.Object{
		ID:     int64(entityID),
		TypeID: constt.DroppedItemTypeID,
		Region: 3,
		Data: pqtype.NullRawMessage{
			RawMessage: legacyMetadata,
			Valid:      true,
		},
	}, []repository.Inventory{{
		OwnerID:      int64(entityID),
		Kind:         int16(constt.InventoryDroppedItem),
		InventoryKey: 0,
		Data:         inventoryData,
		Version:      1,
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if handle == types.InvalidHandle {
		t.Fatal("legacy dropped item was not spawned")
	}
	if updater.calls != 1 || updater.region != 3 || updater.entityID != entityID {
		t.Fatalf("unexpected migration request: %+v", updater)
	}
	migrated, err := parseDroppedItemData(updater.data, entityID)
	if err != nil {
		t.Fatalf("parse migrated metadata: %v", err)
	}
	if migrated.DropTime != 500 || migrated.TimeBasis != constt.DroppedItemTimeBasisRuntimeSecondsV1 {
		t.Fatalf("unexpected migrated metadata: %+v", migrated)
	}
	dropped, ok := ecs.GetComponent[components.DroppedItem](w, handle)
	if !ok || dropped.DropTime != 500 {
		t.Fatalf("unexpected migrated ECS component: %+v", dropped)
	}
}

func TestObjectFactoryDoesNotLoadLegacyDroppedItemWhenMigrationFails(t *testing.T) {
	w := ecs.NewWorldForTesting()
	ecs.SetResource(w, ecs.TimeState{RuntimeSecondsTotal: 500})
	entityID := types.EntityID(705)
	updater := &recordingObjectDataUpdater{err: errors.New("database unavailable")}
	factory := NewObjectFactory(nil)
	factory.SetObjectDataUpdater(updater)

	_, err := factory.Build(w, &repository.Object{
		ID:     int64(entityID),
		TypeID: constt.DroppedItemTypeID,
		Region: 3,
		Data: pqtype.NullRawMessage{
			RawMessage: []byte(`{
                "has_inventory": true,
                "contained_item_id": 705,
                "drop_time": 1700000000,
                "dropper_id": 7
            }`),
			Valid: true,
		},
	}, nil)
	if err == nil {
		t.Fatal("legacy object must not load when its migration cannot be persisted")
	}
	if updater.calls != 1 {
		t.Fatalf("migration calls = %d, want 1", updater.calls)
	}
	if handle := w.GetHandleByEntityID(entityID); handle != types.InvalidHandle {
		t.Fatalf("unmigrated object must not enter ECS, got handle %d", handle)
	}
}
