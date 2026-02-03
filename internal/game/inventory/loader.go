package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	_const "origin/internal/const"
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

type LoadResult struct {
	ContainerHandles []types.Handle
	Warnings         []string
	LostAndFoundUsed bool
}

func (il *InventoryLoader) LoadPlayerInventories(
	ctx context.Context,
	queries *repository.Queries,
	world *ecs.World,
	playerHandle types.Handle,
	characterID int64,
) (*LoadResult, error) {
	dbInventories, err := queries.GetInventoriesByOwner(ctx, characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to load inventories from DB: %w", err)
	}

	result := &LoadResult{
		ContainerHandles: make([]types.Handle, 0, len(dbInventories)+3),
		Warnings:         make([]string, 0),
	}

	inventoryLinks := make([]components.InventoryLink, 0, len(dbInventories)+3)
	existingContainers := make(map[containerKey]bool)

	for _, dbInv := range dbInventories {
		key := containerKey{kind: _const.InventoryKind(dbInv.Kind), key: uint32(dbInv.InventoryKey)}
		existingContainers[key] = true

		containerHandle, warnings := il.buildContainer(world, playerHandle, dbInv)
		if containerHandle != types.InvalidHandle {
			inventoryLinks = append(inventoryLinks, components.InventoryLink{
				Kind:   _const.InventoryKind(dbInv.Kind),
				Key:    uint32(dbInv.InventoryKey),
				Handle: containerHandle,
			})
			result.ContainerHandles = append(result.ContainerHandles, containerHandle)
			result.Warnings = append(result.Warnings, warnings...)
		}
	}

	defaultContainers := []struct {
		kind   _const.InventoryKind
		key    uint32
		width  uint8
		height uint8
	}{
		{kind: _const.InventoryGrid, key: 0, width: DefaultBackpackWidth, height: DefaultBackpackHeight},
		{kind: _const.InventoryHand, key: 0, width: 0, height: 0},
		{kind: _const.InventoryEquipment, key: 0, width: 0, height: 0},
	}

	for _, def := range defaultContainers {
		key := containerKey{kind: def.kind, key: def.key}
		if !existingContainers[key] {
			containerHandle := il.createDefaultContainer(world, playerHandle, def.kind, def.key, def.width, def.height)
			inventoryLinks = append(inventoryLinks, components.InventoryLink{
				Kind:   def.kind,
				Key:    def.key,
				Handle: containerHandle,
			})
			result.ContainerHandles = append(result.ContainerHandles, containerHandle)
			result.Warnings = append(result.Warnings, fmt.Sprintf("created default container: kind=%d key=%d", def.kind, def.key))
		}
	}

	ecs.AddComponent(world, playerHandle, components.InventoryOwner{
		Inventories: inventoryLinks,
	})

	return result, nil
}

type containerKey struct {
	kind _const.InventoryKind
	key  uint32
}

func (il *InventoryLoader) buildContainer(
	world *ecs.World,
	playerHandle types.Handle,
	dbInv repository.Inventory,
) (types.Handle, []string) {
	warnings := make([]string, 0)

	var data InventoryDataV1
	if err := json.Unmarshal(dbInv.Data, &data); err != nil {
		il.logger.Error("Failed to parse inventory data",
			zap.Int64("owner_id", dbInv.OwnerID),
			zap.Int16("kind", dbInv.Kind),
			zap.Int16("key", dbInv.InventoryKey),
			zap.Error(err),
		)
		warnings = append(warnings, fmt.Sprintf("corrupted inventory data: kind=%d key=%d", dbInv.Kind, dbInv.InventoryKey))
		return types.InvalidHandle, warnings
	}

	if data.Version != 1 {
		warnings = append(warnings, fmt.Sprintf("unsupported inventory version %d, expected 1", data.Version))
		return types.InvalidHandle, warnings
	}

	kind := _const.InventoryKind(dbInv.Kind)
	width := uint8(0)
	height := uint8(0)
	if kind == _const.InventoryGrid {
		if dbInv.Width.Valid {
			width = uint8(dbInv.Width.Int16)
		}
		if dbInv.Height.Valid {
			height = uint8(dbInv.Height.Int16)
		}
		if width == 0 || height == 0 {
			warnings = append(warnings, fmt.Sprintf("invalid grid dimensions: %dx%d", width, height))
			return types.InvalidHandle, warnings
		}
	}

	sanitizedItems, itemWarnings := il.sanitizeItems(data.Items, kind, width, height)
	warnings = append(warnings, itemWarnings...)

	playerEntityID, _ := ecs.GetComponent[ecs.ExternalID](world, playerHandle)

	container := components.InventoryContainer{
		OwnerEntityID: playerEntityID.ID,
		Kind:          kind,
		Key:           uint32(dbInv.InventoryKey),
		Version:       uint64(dbInv.Version),
		Width:         width,
		Height:        height,
		Items:         sanitizedItems,
	}

	if kind == _const.InventoryGrid && width > 0 && height > 0 {
		container.Occupied = make([]uint8, int(width)*int(height))
		for _, item := range sanitizedItems {
			il.markOccupied(container.Occupied, width, height, item.X, item.Y, item.W, item.H)
		}
	}

	containerHandle := world.Spawn(types.EntityID(0), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, container)
	})

	return containerHandle, warnings
}

func (il *InventoryLoader) sanitizeItems(
	items []InventoryItemV1,
	kind _const.InventoryKind,
	width, height uint8,
) ([]components.InvItem, []string) {
	warnings := make([]string, 0)
	result := make([]components.InvItem, 0, len(items))
	usedSlots := make(map[netproto.EquipSlot]bool)
	occupiedGrid := make([][]bool, height)
	for i := range occupiedGrid {
		occupiedGrid[i] = make([]bool, width)
	}

	for _, item := range items {
		itemDef, exists := il.itemRegistry.GetByID(int(item.TypeID))
		if !exists {
			warnings = append(warnings, fmt.Sprintf("unknown item type_id=%d, skipping", item.TypeID))
			continue
		}

		if item.Quantity == 0 {
			warnings = append(warnings, fmt.Sprintf("item_id=%d has quantity=0, skipping", item.ItemID))
			continue
		}

		maxStack := 1
		if itemDef.Stack != nil && itemDef.Stack.Mode == itemdefs.StackModeStack {
			maxStack = itemDef.Stack.Max
		}
		if int(item.Quantity) > maxStack {
			warnings = append(warnings, fmt.Sprintf("item_id=%d quantity=%d exceeds max=%d, clamping", item.ItemID, item.Quantity, maxStack))
			item.Quantity = uint32(maxStack)
		}

		invItem := components.InvItem{
			ItemID:   item.ItemID,
			TypeID:   item.TypeID,
			Resource: itemDef.Key,
			Quality:  item.Quality,
			Quantity: item.Quantity,
			W:        uint8(itemDef.Size.W),
			H:        uint8(itemDef.Size.H),
			X:        item.X,
			Y:        item.Y,
		}

		switch kind {
		case _const.InventoryGrid:
			if itemDef.Allowed.Grid == nil || !*itemDef.Allowed.Grid {
				warnings = append(warnings, fmt.Sprintf("item_id=%d not allowed in grid, skipping", item.ItemID))
				continue
			}

			if !il.canPlaceInGrid(occupiedGrid, width, height, item.X, item.Y, invItem.W, invItem.H) {
				warnings = append(warnings, fmt.Sprintf("item_id=%d collision at (%d,%d), skipping", item.ItemID, item.X, item.Y))
				continue
			}

			il.markGridOccupied(occupiedGrid, item.X, item.Y, invItem.W, invItem.H)
			result = append(result, invItem)

		case _const.InventoryHand:
			if itemDef.Allowed.Hand == nil || !*itemDef.Allowed.Hand {
				warnings = append(warnings, fmt.Sprintf("item_id=%d not allowed in hand, skipping", item.ItemID))
				continue
			}

			if len(result) > 0 {
				warnings = append(warnings, fmt.Sprintf("hand already has item, skipping item_id=%d", item.ItemID))
				continue
			}

			result = append(result, invItem)

		case _const.InventoryEquipment:
			slot := il.parseEquipSlot(item.EquipSlot)
			if slot == netproto.EquipSlot_EQUIP_SLOT_NONE {
				warnings = append(warnings, fmt.Sprintf("item_id=%d has invalid equip_slot=%s, skipping", item.ItemID, item.EquipSlot))
				continue
			}

			allowed := false
			for _, allowedSlot := range itemDef.Allowed.EquipmentSlots {
				if allowedSlot == item.EquipSlot {
					allowed = true
					break
				}
			}
			if !allowed {
				warnings = append(warnings, fmt.Sprintf("item_id=%d not allowed in slot=%s, skipping", item.ItemID, item.EquipSlot))
				continue
			}

			if usedSlots[slot] {
				warnings = append(warnings, fmt.Sprintf("slot=%s already occupied, skipping item_id=%d", item.EquipSlot, item.ItemID))
				continue
			}

			invItem.EquipSlot = slot
			usedSlots[slot] = true
			result = append(result, invItem)
		}
	}

	return result, warnings
}

func (il *InventoryLoader) canPlaceInGrid(grid [][]bool, width, height, x, y, w, h uint8) bool {
	if x+w > width || y+h > height {
		return false
	}

	for dy := uint8(0); dy < h; dy++ {
		for dx := uint8(0); dx < w; dx++ {
			if grid[y+dy][x+dx] {
				return false
			}
		}
	}

	return true
}

func (il *InventoryLoader) markGridOccupied(grid [][]bool, x, y, w, h uint8) {
	for dy := uint8(0); dy < h; dy++ {
		for dx := uint8(0); dx < w; dx++ {
			grid[y+dy][x+dx] = true
		}
	}
}

func (il *InventoryLoader) markOccupied(occupied []uint8, width, height, x, y, w, h uint8) {
	for dy := uint8(0); dy < h; dy++ {
		for dx := uint8(0); dx < w; dx++ {
			idx := int(y+dy)*int(width) + int(x+dx)
			if idx < len(occupied) {
				occupied[idx] = 1
			}
		}
	}
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

func (il *InventoryLoader) createDefaultContainer(
	world *ecs.World,
	playerHandle types.Handle,
	kind _const.InventoryKind,
	key uint32,
	width, height uint8,
) types.Handle {
	playerEntityID, _ := ecs.GetComponent[ecs.ExternalID](world, playerHandle)

	container := components.InventoryContainer{
		OwnerEntityID: playerEntityID.ID,
		Kind:          kind,
		Key:           key,
		Version:       0,
		Width:         width,
		Height:        height,
		Items:         make([]components.InvItem, 0),
	}

	if kind == _const.InventoryGrid && width > 0 && height > 0 {
		container.Occupied = make([]uint8, int(width)*int(height))
	}

	containerHandle := world.Spawn(types.EntityID(0), func(w *ecs.World, h types.Handle) {
		ecs.AddComponent(w, h, container)
	})

	return containerHandle
}
