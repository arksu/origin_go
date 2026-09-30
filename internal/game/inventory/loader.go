package inventory

import (
	"encoding/json"
	"fmt"
	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/persistence/repository"
	"origin/internal/types"

	"go.uber.org/zap"
)

type InventoryLoader struct {
	logger *zap.Logger
}

func NewInventoryLoader(logger *zap.Logger) *InventoryLoader {
	return &InventoryLoader{
		logger: logger,
	}
}

type LoadResult struct {
	ContainerHandles []types.Handle
	Warnings         []string
	LostAndFoundUsed bool
}

func (il *InventoryLoader) LoadPlayerInventories(
	w interface{},
	characterID types.EntityID,
	dbInventories []InventoryDataV1,
) (*LoadResult, error) {
	world := w.(*ecs.World)
	result := &LoadResult{
		ContainerHandles: make([]types.Handle, 0, len(dbInventories)),
		Warnings:         make([]string, 0),
	}

	allHandles := make([]types.Handle, 0)

	for _, dbInv := range dbInventories {
		_, warnings, err := il.loadInventoryRecursive(
			world,
			characterID,
			dbInv,
			&allHandles,
		)
		if err != nil {
			// Nothing is indexed until loading succeeds, so rollback must only
			// release entities allocated by this attempt, not existing inventories.
			for _, handle := range allHandles {
				world.Despawn(handle)
			}
			return nil, err
		}
		result.Warnings = append(result.Warnings, warnings...)
	}

	// Return all handles including nested ones
	result.ContainerHandles = allHandles

	return result, nil
}

func (il *InventoryLoader) loadInventoryRecursive(
	world *ecs.World,
	ownerID types.EntityID,
	dbInv InventoryDataV1,
	allHandles *[]types.Handle,
) (types.Handle, []string, error) {
	warnings := make([]string, 0)

	containerHandle := world.SpawnWithoutExternalID()
	if containerHandle == types.InvalidHandle {
		return types.InvalidHandle, warnings, fmt.Errorf("inventory owner %d kind %d key %d: %w", ownerID, dbInv.Kind, dbInv.Key, ecs.ErrEntityCapacityExhausted)
	}
	*allHandles = append(*allHandles, containerHandle)

	container := components.InventoryContainer{
		OwnerID: ownerID,
		Kind:    constt.InventoryKind(dbInv.Kind),
		Key:     dbInv.Key,
		Version: uint64(dbInv.Version),
		Width:   dbInv.Width,
		Height:  dbInv.Height,
		Items:   make([]components.InvItem, 0, len(dbInv.Items)),
	}

	for _, dbItem := range dbInv.Items {
		itemDef, ok := itemdefs.Global().GetByID(int(dbItem.TypeID))
		if !ok {
			warnings = append(warnings, fmt.Sprintf("item type %d not found in registry", dbItem.TypeID))
			continue
		}

		hasNestedItems := dbItem.NestedInventory != nil && len(dbItem.NestedInventory.Items) > 0

		invItem := components.InvItem{
			ItemID:    types.EntityID(dbItem.ItemID),
			TypeID:    dbItem.TypeID,
			Resource:  itemDef.ResolveResource(hasNestedItems),
			Quality:   dbItem.Quality,
			Quantity:  dbItem.Quantity,
			W:         uint8(itemDef.Size.W),
			H:         uint8(itemDef.Size.H),
			X:         dbItem.X,
			Y:         dbItem.Y,
			EquipSlot: il.parseEquipSlot(dbItem.EquipSlot),
		}

		container.Items = append(container.Items, invItem)

		if dbItem.NestedInventory != nil {
			_, nestedWarnings, err := il.loadInventoryRecursive(
				world,
				types.EntityID(dbItem.ItemID),
				*dbItem.NestedInventory,
				allHandles,
			)
			if err != nil {
				return types.InvalidHandle, warnings, err
			}
			warnings = append(warnings, nestedWarnings...)
		}
	}

	ecs.AddComponent(world, containerHandle, container)

	return containerHandle, warnings, nil
}

// ParseInventoriesFromDB converts database inventory records to InventoryDataV1 format
func (il *InventoryLoader) ParseInventoriesFromDB(dbInventories []repository.Inventory) ([]InventoryDataV1, []string) {
	inventoryDataList := make([]InventoryDataV1, 0, len(dbInventories))
	warnings := make([]string, 0)

	for _, dbInv := range dbInventories {
		var invData InventoryDataV1
		if err := json.Unmarshal(dbInv.Data, &invData); err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to unmarshal inventory data: kind=%d, key=%d, error=%v", dbInv.Kind, dbInv.InventoryKey, err))
			continue
		}
		invData.Kind = uint8(dbInv.Kind)
		invData.Key = uint32(dbInv.InventoryKey)
		invData.Version = dbInv.Version
		inventoryDataList = append(inventoryDataList, invData)
	}

	return inventoryDataList, warnings
}

func (il *InventoryLoader) parseEquipSlot(slot string) netproto.EquipSlot {
	return StringToEquipSlot(slot)
}
