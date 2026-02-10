package inventory

import (
	"encoding/json"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/types"
)

// SpawnDroppedEntityParams contains all data needed to spawn a dropped item entity.
type SpawnDroppedEntityParams struct {
	DroppedEntityID types.EntityID
	ItemID          types.EntityID
	TypeID          uint32
	Resource        string
	Quality         uint32
	Quantity        uint32
	W, H            uint8
	DropX, DropY    int
	Region          int
	Layer           int
	ChunkX, ChunkY  int
	DropperID       types.EntityID
	NowUnix         int64
}

// SpawnDroppedEntityResult holds the output of a successful spawn.
type SpawnDroppedEntityResult struct {
	DroppedHandle    types.Handle
	ContainerHandle  types.Handle
}

// SpawnDroppedEntity creates a dropped item ECS entity with all required components
// and its inventory container. Both operations.go (ExecuteDropToWorld) and
// give_item.go (dropNewItem) share this logic.
func SpawnDroppedEntity(w *ecs.World, p SpawnDroppedEntityParams) (SpawnDroppedEntityResult, bool) {
	var containerHandle types.Handle

	droppedHandle := w.Spawn(p.DroppedEntityID, func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, components.CreateTransform(p.DropX, p.DropY, 0))

		ecs.AddComponent(w, h, components.EntityInfo{
			TypeID:   constt.DroppedItemTypeID,
			IsStatic: true,
			Region:   p.Region,
			Layer:    p.Layer,
		})

		ecs.AddComponent(w, h, components.ChunkRef{
			CurrentChunkX: p.ChunkX,
			CurrentChunkY: p.ChunkY,
		})

		ecs.AddComponent(w, h, components.Appearance{
			Resource: p.Resource,
		})

		ecs.AddComponent(w, h, components.DroppedItem{
			DropTime:        p.NowUnix,
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
		return SpawnDroppedEntityResult{}, false
	}

	return SpawnDroppedEntityResult{
		DroppedHandle:   droppedHandle,
		ContainerHandle: containerHandle,
	}, true
}

// PersistDroppedEntity serializes and persists a dropped item to the database.
func PersistDroppedEntity(
	persister DroppedItemPersister,
	p SpawnDroppedEntityParams,
	nestedInvData *InventoryDataV1,
) error {
	droppedData := droppedItemData{
		HasInventory:    true,
		ContainedItemID: uint64(p.ItemID),
		DropTime:        p.NowUnix,
		DropperID:       uint64(p.DropperID),
	}
	objectJSON, _ := json.Marshal(droppedData)

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
	inventoryJSON, _ := json.Marshal(invData)

	return persister.PersistDroppedObject(
		p.DroppedEntityID, constt.DroppedItemTypeID,
		p.Region, p.DropX, p.DropY, p.Layer,
		p.ChunkX, p.ChunkY,
		objectJSON, inventoryJSON,
	)
}
