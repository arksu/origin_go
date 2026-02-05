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
	itemRegistry *itemdefs.Registry
	logger       *zap.Logger
}

func NewInventoryLoader(itemRegistry *itemdefs.Registry, logger *zap.Logger) *InventoryLoader {
	return &InventoryLoader{
		itemRegistry: itemRegistry,
		logger:       logger,
	}
}

func (il *InventoryLoader) ItemRegistry() *itemdefs.Registry {
	return il.itemRegistry
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

	itemIDToHandle := make(map[types.EntityID]types.Handle)
	allHandles := make([]types.Handle, 0)

	for _, dbInv := range dbInventories {
		containerHandle, warnings := il.loadInventoryRecursive(
			world,
			characterID,
			dbInv,
			itemIDToHandle,
			&allHandles,
		)
		if containerHandle != 0 {
			result.ContainerHandles = append(result.ContainerHandles, containerHandle)
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
	itemIDToHandle map[types.EntityID]types.Handle,
	allHandles *[]types.Handle,
) (types.Handle, []string) {
	warnings := make([]string, 0)

	containerHandle := world.SpawnWithoutExternalID()

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
		itemDef, ok := il.itemRegistry.GetByID(int(dbItem.TypeID))
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
			nestedHandle, nestedWarnings := il.loadInventoryRecursive(
				world,
				types.EntityID(dbItem.ItemID),
				*dbItem.NestedInventory,
				itemIDToHandle,
				allHandles,
			)
			if nestedHandle != 0 {
				itemIDToHandle[types.EntityID(dbItem.ItemID)] = nestedHandle
			}
			warnings = append(warnings, nestedWarnings...)
		}
	}

	ecs.AddComponent(world, containerHandle, container)

	// Add this handle to the list of all handles
	*allHandles = append(*allHandles, containerHandle)

	return containerHandle, warnings
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
	switch slot {
	case "head":
		return netproto.EquipSlot_EQUIP_SLOT_HEAD
	case "chest":
		return netproto.EquipSlot_EQUIP_SLOT_CHEST
	case "legs":
		return netproto.EquipSlot_EQUIP_SLOT_LEGS
	case "feet":
		return netproto.EquipSlot_EQUIP_SLOT_FEET
	case "hands":
		return netproto.EquipSlot_EQUIP_SLOT_HANDS
	case "back":
		return netproto.EquipSlot_EQUIP_SLOT_BACK
	case "neck":
		return netproto.EquipSlot_EQUIP_SLOT_NECK
	case "ring1":
		return netproto.EquipSlot_EQUIP_SLOT_RING_1
	case "ring2":
		return netproto.EquipSlot_EQUIP_SLOT_RING_2
	default:
		return netproto.EquipSlot_EQUIP_SLOT_NONE
	}
}
