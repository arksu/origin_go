package inventory

import (
	"context"
	"fmt"
	"time"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/persistence"
	"origin/internal/types"
)

// DroppedInventoryLoaderDB implements world.DroppedInventoryLoader.
// It loads a dropped item's inventory from the DB and creates the ECS container entity.
type DroppedInventoryLoaderDB struct {
	db     *persistence.Postgres
	loader *InventoryLoader
}

func NewDroppedInventoryLoaderDB(db *persistence.Postgres, loader *InventoryLoader) *DroppedInventoryLoaderDB {
	return &DroppedInventoryLoaderDB{db: db, loader: loader}
}

// LoadDroppedInventory loads inventory for a dropped item from DB,
// creates the InventoryContainer ECS entity, and returns its handle.
func (d *DroppedInventoryLoaderDB) LoadDroppedInventory(w *ecs.World, ownerID types.EntityID) (types.Handle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dbInventories, err := d.db.Queries().GetInventoriesByOwner(ctx, int64(ownerID))
	if err != nil {
		return types.InvalidHandle, fmt.Errorf("failed to load inventory: %w", err)
	}

	invDataList, _ := d.loader.ParseInventoriesFromDB(dbInventories)

	// Find the DroppedItem container
	var droppedInv *InventoryDataV1
	for i := range invDataList {
		if constt.InventoryKind(invDataList[i].Kind) == constt.InventoryDroppedItem {
			droppedInv = &invDataList[i]
			break
		}
	}
	if droppedInv == nil || len(droppedInv.Items) == 0 {
		return types.InvalidHandle, fmt.Errorf("no dropped item inventory data found")
	}

	// Use InventoryLoader to create the ECS containers (including nested)
	loadResult, err := d.loader.LoadPlayerInventories(w, ownerID, []InventoryDataV1{*droppedInv})
	if err != nil {
		return types.InvalidHandle, fmt.Errorf("failed to load dropped inventory: %w", err)
	}
	if loadResult == nil || len(loadResult.ContainerHandles) == 0 {
		return types.InvalidHandle, fmt.Errorf("no container handles created")
	}

	// Find the top-level DroppedItem container and register all containers in RefIndex
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](w)
	var topLevelHandle types.Handle
	for _, h := range loadResult.ContainerHandles {
		container, ok := ecs.GetComponent[components.InventoryContainer](w, h)
		if !ok {
			continue
		}
		if container.Kind == constt.InventoryDroppedItem {
			topLevelHandle = h
			// Top-level container is registered by the caller (buildDroppedItem)
			continue
		}
		// Register nested containers (e.g. seed_bag's grid) in RefIndex
		refIndex.Add(container.Kind, container.OwnerID, container.Key, h)
	}

	if topLevelHandle == 0 {
		return types.InvalidHandle, fmt.Errorf("no dropped item container found")
	}

	return topLevelHandle, nil
}
