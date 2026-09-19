package inventory

import (
	"encoding/json"
	"errors"
	"fmt"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// SpawnDroppedEntityParams contains all data needed to spawn a dropped item entity.
type SpawnDroppedEntityParams struct {
	DroppedEntityID   types.EntityID
	ItemID            types.EntityID
	TypeID            uint32
	Resource          string
	Quality           uint32
	Quantity          uint32
	W, H              uint8
	DropX, DropY      int
	Region            int
	Layer             int
	ChunkX, ChunkY    int
	DropperID         types.EntityID
	NowRuntimeSeconds int64
}

// SpawnDroppedEntityResult holds the output of a successful spawn.
type SpawnDroppedEntityResult struct {
	DroppedHandle   types.Handle
	ContainerHandle types.Handle
}

// DroppedEntityPersistence describes one durable dropped-item record. Every
// record maps to exactly one static object and one root inventory item.
type DroppedEntityPersistence struct {
	Params          SpawnDroppedEntityParams
	NestedInventory *InventoryDataV1
}

// DroppedItemPersistenceRecord is the serialized form accepted by persistence
// implementations that can atomically save several individual dropped items.
type DroppedItemPersistenceRecord struct {
	EntityID      types.EntityID
	TypeID        int
	Region        int
	X             int
	Y             int
	Layer         int
	ChunkX        int
	ChunkY        int
	ObjectData    json.RawMessage
	InventoryData json.RawMessage
}

// BatchDroppedItemPersister is an optional stronger persistence capability. It
// is used when one inventory stack is split into several ground entities.
type BatchDroppedItemPersister interface {
	PersistDroppedObjectBatch(records []DroppedItemPersistenceRecord) error
}

// AtomicDroppedObjectReplacementPersister atomically creates a dropped item
// while removing its persistent source object.
type AtomicDroppedObjectReplacementPersister interface {
	ReplaceObjectWithDroppedItem(record DroppedItemPersistenceRecord, sourceRegion int, sourceID types.EntityID) error
}

// SpawnDroppedEntity creates a dropped item ECS entity with all required components
// and its inventory container. Player drops and craft-overflow drops share this
// logic so their identity, persistence payload, and static-object invariants stay
// identical.
func SpawnDroppedEntity(w *ecs.World, p SpawnDroppedEntityParams) (SpawnDroppedEntityResult, error) {
	if err := validateSpawnDroppedEntityParams(w, p); err != nil {
		return SpawnDroppedEntityResult{}, err
	}

	var containerHandle types.Handle

	droppedHandle := w.Spawn(p.DroppedEntityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CreateTransform(p.DropX, p.DropY, 0))

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:   constt.DroppedItemTypeID,
			IsStatic: true,
			Quality:  p.Quality,
			Region:   p.Region,
			Layer:    p.Layer,
		})

		ecs.AddComponent(w, h, components.ChunkRef{
			CurrentChunkX: p.ChunkX,
			CurrentChunkY: p.ChunkY,
		})

		ecs.AddComponent(w, h, components.Appearance{
			Name:     nil,
			Resource: p.Resource,
		})

		ecs.AddComponent(w, h, components.DroppedItem{
			DropTime:        p.NowRuntimeSeconds,
			DropperID:       p.DropperID,
			ContainedItemID: p.ItemID,
		})

		container := components.InventoryContainer{
			OwnerID: p.DroppedEntityID,
			Kind:    constt.InventoryDroppedItem,
			Key:     0,
			Version: 1,
			Items: []components.InvItem{
				{
					ItemID:   p.ItemID,
					TypeID:   p.TypeID,
					Resource: p.Resource,
					Quality:  p.Quality,
					Quantity: p.Quantity,
					W:        p.W,
					H:        p.H,
				},
			},
		}
		containerHandle = w.SpawnWithoutExternalID()
		ecs.AddComponent(w, containerHandle, container)

		refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
		refIndex.Add(constt.InventoryDroppedItem, p.DroppedEntityID, 0, containerHandle)
	})

	if droppedHandle == types.InvalidHandle {
		return SpawnDroppedEntityResult{}, fmt.Errorf("spawn dropped entity %d", p.DroppedEntityID)
	}

	return SpawnDroppedEntityResult{
		DroppedHandle:   droppedHandle,
		ContainerHandle: containerHandle,
	}, nil
}

func validateSpawnDroppedEntityParams(w *ecs.World, p SpawnDroppedEntityParams) error {
	if w == nil {
		return fmt.Errorf("spawn dropped entity: world is nil")
	}
	if p.DroppedEntityID == 0 || p.ItemID == 0 {
		return fmt.Errorf("spawn dropped entity: object and item IDs must be non-zero")
	}
	if p.DroppedEntityID != p.ItemID {
		return fmt.Errorf("spawn dropped entity: object ID %d must equal item ID %d", p.DroppedEntityID, p.ItemID)
	}
	if p.TypeID == 0 || p.Quantity != 1 {
		return fmt.Errorf("spawn dropped entity %d: invalid item", p.DroppedEntityID)
	}
	if p.NowRuntimeSeconds < 0 {
		return fmt.Errorf("spawn dropped entity %d: invalid runtime seconds %d", p.DroppedEntityID, p.NowRuntimeSeconds)
	}
	if existing := w.GetHandleByEntityID(p.DroppedEntityID); existing != types.InvalidHandle {
		return fmt.Errorf("spawn dropped entity %d: entity already exists", p.DroppedEntityID)
	}
	return nil
}

// PersistDroppedEntity serializes and persists a dropped item to the database.
func PersistDroppedEntity(
	persister DroppedItemPersister,
	p SpawnDroppedEntityParams,
	nestedInvData *InventoryDataV1,
) error {
	record, err := buildDroppedItemPersistenceRecord(p, nestedInvData)
	if err != nil {
		return err
	}
	if persister == nil {
		return fmt.Errorf("persist dropped entity %d: persister is not configured", p.DroppedEntityID)
	}
	return persister.PersistDroppedObject(
		record.EntityID, record.TypeID,
		record.Region, record.X, record.Y, record.Layer,
		record.ChunkX, record.ChunkY,
		record.ObjectData, record.InventoryData,
	)
}

// PersistDroppedEntityReplacement persists a dropped outcome and removes its
// source in one transaction, so a retry cannot duplicate the outcome item.
func PersistDroppedEntityReplacement(
	persister AtomicDroppedObjectReplacementPersister,
	p SpawnDroppedEntityParams,
	sourceRegion int,
	sourceID types.EntityID,
) error {
	if persister == nil {
		return fmt.Errorf("persist dropped replacement %d: persister is not configured", p.DroppedEntityID)
	}
	if sourceRegion <= 0 || sourceID == 0 {
		return fmt.Errorf("persist dropped replacement %d: invalid source", p.DroppedEntityID)
	}
	record, err := buildDroppedItemPersistenceRecord(p, nil)
	if err != nil {
		return err
	}
	return persister.ReplaceObjectWithDroppedItem(record, sourceRegion, sourceID)
}

// PersistDroppedEntities saves one or more individual dropped items before any
// of them are spawned in ECS. Production persistence uses one DB transaction;
// the fallback keeps small test doubles compatible and compensates on failure.
func PersistDroppedEntities(
	persister DroppedItemPersister,
	entries []DroppedEntityPersistence,
) error {
	if persister == nil {
		return errors.New("persist dropped entities: persister is not configured")
	}
	if len(entries) == 0 {
		return errors.New("persist dropped entities: no entries")
	}

	records, err := buildDroppedItemPersistenceRecords(entries)
	if err != nil {
		return err
	}

	if batchPersister, ok := persister.(BatchDroppedItemPersister); ok {
		return batchPersister.PersistDroppedObjectBatch(records)
	}

	persisted := records[:0]
	for _, record := range records {
		if err := persister.PersistDroppedObject(
			record.EntityID, record.TypeID,
			record.Region, record.X, record.Y, record.Layer,
			record.ChunkX, record.ChunkY,
			record.ObjectData, record.InventoryData,
		); err != nil {
			for _, saved := range persisted {
				_ = persister.DeleteObject(saved.Region, saved.EntityID)
			}
			return fmt.Errorf("persist dropped entity %d: %w", record.EntityID, err)
		}
		persisted = append(persisted, record)
	}
	return nil
}

func buildDroppedItemPersistenceRecords(entries []DroppedEntityPersistence) ([]DroppedItemPersistenceRecord, error) {
	if len(entries) == 0 {
		return nil, errors.New("persist dropped entities: no entries")
	}

	records := make([]DroppedItemPersistenceRecord, 0, len(entries))
	for _, entry := range entries {
		record, err := buildDroppedItemPersistenceRecord(entry.Params, entry.NestedInventory)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildDroppedItemPersistenceRecord(
	p SpawnDroppedEntityParams,
	nestedInvData *InventoryDataV1,
) (DroppedItemPersistenceRecord, error) {
	if p.DroppedEntityID == 0 || p.ItemID == 0 || p.DroppedEntityID != p.ItemID {
		return DroppedItemPersistenceRecord{}, fmt.Errorf("persist dropped entity: object and item IDs must match")
	}
	if p.TypeID == 0 || p.Quantity != 1 || p.NowRuntimeSeconds < 0 {
		return DroppedItemPersistenceRecord{}, fmt.Errorf("persist dropped entity %d: invalid item metadata", p.DroppedEntityID)
	}
	droppedData := droppedItemData{
		HasInventory:    true,
		ContainedItemID: uint64(p.ItemID),
		DropTime:        p.NowRuntimeSeconds,
		DropperID:       uint64(p.DropperID),
		TimeBasis:       constt.DroppedItemTimeBasisRuntimeSecondsV1,
	}
	objectJSON, err := json.Marshal(droppedData)
	if err != nil {
		return DroppedItemPersistenceRecord{}, fmt.Errorf("marshal dropped object metadata: %w", err)
	}

	droppedItem := InventoryItemV1{
		ItemID:          uint64(p.ItemID),
		TypeID:          p.TypeID,
		Quality:         p.Quality,
		Quantity:        p.Quantity,
		NestedInventory: nestedInvData,
	}
	invData := InventoryDataV1{
		Kind:    uint8(constt.InventoryDroppedItem),
		Key:     0,
		Version: 1,
		Items:   []InventoryItemV1{droppedItem},
	}
	inventoryJSON, err := json.Marshal(invData)
	if err != nil {
		return DroppedItemPersistenceRecord{}, fmt.Errorf("marshal dropped inventory: %w", err)
	}

	return DroppedItemPersistenceRecord{
		EntityID:      p.DroppedEntityID,
		TypeID:        constt.DroppedItemTypeID,
		Region:        p.Region,
		X:             p.DropX,
		Y:             p.DropY,
		Layer:         p.Layer,
		ChunkX:        p.ChunkX,
		ChunkY:        p.ChunkY,
		ObjectData:    objectJSON,
		InventoryData: inventoryJSON,
	}, nil
}
