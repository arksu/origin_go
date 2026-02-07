package inventory

import (
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	netproto "origin/internal/network/proto"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func boolPtr(b bool) *bool {
	return &b
}

func createTestRegistry() *itemdefs.Registry {
	items := []itemdefs.ItemDef{
		{
			DefID: 1,
			Key:   "test_item",
			Name:  "Test Item",
			Size:  itemdefs.Size{W: 1, H: 1},
			Allowed: itemdefs.Allowed{
				Hand: boolPtr(true),
				Grid: boolPtr(true),
			},
		},
		{
			DefID: 2,
			Key:   "large_item",
			Name:  "Large Item",
			Size:  itemdefs.Size{W: 2, H: 2},
			Allowed: itemdefs.Allowed{
				Hand: boolPtr(true),
				Grid: boolPtr(true),
			},
		},
		{
			DefID: 3,
			Key:   "stackable_item",
			Name:  "Stackable Item",
			Size:  itemdefs.Size{W: 1, H: 1},
			Stack: &itemdefs.Stack{Mode: itemdefs.StackModeStack, Max: 10},
			Allowed: itemdefs.Allowed{
				Hand: boolPtr(true),
				Grid: boolPtr(true),
			},
		},
		{
			DefID: 4,
			Key:   "hand_only_item",
			Name:  "Hand Only Item",
			Size:  itemdefs.Size{W: 1, H: 1},
			Allowed: itemdefs.Allowed{
				Hand: boolPtr(true),
				Grid: boolPtr(false),
			},
		},
		{
			DefID: 5,
			Key:   "equipment_item",
			Name:  "Equipment Item",
			Size:  itemdefs.Size{W: 1, H: 1},
			Allowed: itemdefs.Allowed{
				Hand:           boolPtr(true),
				Grid:           boolPtr(true),
				EquipmentSlots: []string{"head"},
			},
		},
	}
	return itemdefs.NewRegistry(items)
}

func setupTestWorld(t *testing.T) (*ecs.World, types.EntityID, types.Handle) {
	world := ecs.NewWorldForTesting()
	playerID := types.EntityID(1000)
	playerHandle := world.Spawn(playerID, nil)

	return world, playerID, playerHandle
}

func createGridContainer(world *ecs.World, ownerID types.EntityID, key uint32, width, height uint8) types.Handle {
	handle := world.SpawnWithoutExternalID()
	container := components.InventoryContainer{
		OwnerID: ownerID,
		Kind:    constt.InventoryGrid,
		Key:     key,
		Version: 1,
		Width:   width,
		Height:  height,
		Items:   []components.InvItem{},
	}
	ecs.AddComponent(world, handle, container)
	return handle
}

func createHandContainer(world *ecs.World, ownerID types.EntityID, key uint32) types.Handle {
	handle := world.SpawnWithoutExternalID()
	container := components.InventoryContainer{
		OwnerID: ownerID,
		Kind:    constt.InventoryHand,
		Key:     key,
		Version: 1,
		Items:   []components.InvItem{},
	}
	ecs.AddComponent(world, handle, container)
	return handle
}

func addItemToContainer(world *ecs.World, containerHandle types.Handle, item components.InvItem) {
	ecs.MutateComponent[components.InventoryContainer](world, containerHandle, func(c *components.InventoryContainer) bool {
		c.Items = append(c.Items, item)
		return true
	})
}

func setupPlayerWithInventories(world *ecs.World, playerID types.EntityID, playerHandle types.Handle) (types.Handle, types.Handle) {
	gridHandle := createGridContainer(world, playerID, 0, 5, 5)
	handHandle := createHandContainer(world, playerID, 0)

	owner := components.InventoryOwner{
		Inventories: []components.InventoryLink{
			{Kind: constt.InventoryGrid, Key: 0, OwnerID: playerID, Handle: gridHandle},
			{Kind: constt.InventoryHand, Key: 0, OwnerID: playerID, Handle: handHandle},
		},
	}
	ecs.AddComponent(world, playerHandle, owner)

	// Populate InventoryRefIndex for O(1) lookup
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	refIndex.Add(constt.InventoryGrid, playerID, 0, gridHandle)
	refIndex.Add(constt.InventoryHand, playerID, 0, handHandle)

	return gridHandle, handHandle
}

func TestExecuteMove_SimpleMove(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	gridHandle, handHandle := setupPlayerWithInventories(world, playerID, playerHandle)

	// Add item to grid
	item := components.InvItem{
		ItemID:   types.EntityID(100),
		TypeID:   1,
		Resource: "test",
		Quality:  100,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, gridHandle, item)

	itemdefs.SetGlobalForTesting(createTestRegistry())
	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move item from grid to hand
	moveSpec := &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		Dst: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_HAND,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		ItemId: uint64(item.ItemID),
	}

	result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, nil)

	require.True(t, result.Success, "Move should succeed: %s", result.Message)
	assert.Len(t, result.UpdatedContainers, 2, "Should have 2 updated containers")

	// Verify grid is empty
	gridContainer, _ := ecs.GetComponent[components.InventoryContainer](world, gridHandle)
	assert.Len(t, gridContainer.Items, 0, "Grid should be empty")

	// Verify hand has the item
	handContainer, _ := ecs.GetComponent[components.InventoryContainer](world, handHandle)
	assert.Len(t, handContainer.Items, 1, "Hand should have 1 item")
	assert.Equal(t, item.ItemID, handContainer.Items[0].ItemID)
}

func TestExecuteMove_GridToGrid_SameContainer(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(world, playerID, playerHandle)

	// Add item to grid at position (0,0)
	item := components.InvItem{
		ItemID:   types.EntityID(100),
		TypeID:   1,
		Resource: "test",
		Quality:  100,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, gridHandle, item)

	itemdefs.SetGlobalForTesting(createTestRegistry())
	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move item within same grid to position (2,2)
	dstX := uint32(2)
	dstY := uint32(2)
	moveSpec := &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		Dst: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		ItemId: uint64(item.ItemID),
		DstPos: &netproto.GridPos{X: dstX, Y: dstY},
	}

	result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, nil)

	require.True(t, result.Success, "Move should succeed: %s", result.Message)

	// Verify item moved to new position
	gridContainer, _ := ecs.GetComponent[components.InventoryContainer](world, gridHandle)
	require.Len(t, gridContainer.Items, 1, "Grid should still have 1 item")
	assert.Equal(t, uint8(dstX), gridContainer.Items[0].X)
	assert.Equal(t, uint8(dstY), gridContainer.Items[0].Y)
}

func TestExecuteMove_Swap(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	gridHandle, handHandle := setupPlayerWithInventories(world, playerID, playerHandle)

	// Add item to grid
	gridItem := components.InvItem{
		ItemID:   types.EntityID(100),
		TypeID:   1,
		Resource: "test",
		Quality:  100,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, gridHandle, gridItem)

	// Add item to hand
	handItem := components.InvItem{
		ItemID:   types.EntityID(101),
		TypeID:   2,
		Resource: "test2",
		Quality:  100,
		Quantity: 1,
		W:        2,
		H:        2,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, handHandle, handItem)

	itemdefs.SetGlobalForTesting(createTestRegistry())
	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move grid item to hand (should swap)
	moveSpec := &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		Dst: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_HAND,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		ItemId:           uint64(gridItem.ItemID),
		AllowSwapOrMerge: true,
	}

	result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, nil)

	require.True(t, result.Success, "Swap should succeed: %s", result.Message)

	// Verify grid has the hand item
	gridContainer, _ := ecs.GetComponent[components.InventoryContainer](world, gridHandle)
	require.Len(t, gridContainer.Items, 1, "Grid should have 1 item")
	assert.Equal(t, handItem.ItemID, gridContainer.Items[0].ItemID)

	// Verify hand has the grid item
	handContainer, _ := ecs.GetComponent[components.InventoryContainer](world, handHandle)
	require.Len(t, handContainer.Items, 1, "Hand should have 1 item")
	assert.Equal(t, gridItem.ItemID, handContainer.Items[0].ItemID)
}

func TestExecuteMove_Merge(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	gridHandle, handHandle := setupPlayerWithInventories(world, playerID, playerHandle)

	// Add stackable item to grid (quantity 3)
	gridItem := components.InvItem{
		ItemID:   types.EntityID(100),
		TypeID:   3, // stackable_item
		Resource: "stackable",
		Quality:  100,
		Quantity: 3,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, gridHandle, gridItem)

	// Add same stackable item to hand (quantity 5)
	handItem := components.InvItem{
		ItemID:   types.EntityID(101),
		TypeID:   3, // same stackable_item
		Resource: "stackable",
		Quality:  100,
		Quantity: 5,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, handHandle, handItem)

	itemdefs.SetGlobalForTesting(createTestRegistry())
	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move hand item to grid (should merge)
	moveSpec := &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_HAND,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		Dst: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		ItemId:           uint64(handItem.ItemID),
		DstPos:           &netproto.GridPos{X: 0, Y: 0},
		AllowSwapOrMerge: true,
	}

	result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, nil)

	require.True(t, result.Success, "Merge should succeed: %s", result.Message)

	// Verify grid has merged quantity (max 10)
	gridContainer, _ := ecs.GetComponent[components.InventoryContainer](world, gridHandle)
	require.Len(t, gridContainer.Items, 1, "Grid should have 1 item")
	assert.Equal(t, uint32(8), gridContainer.Items[0].Quantity, "Grid item should have merged quantity")

	// Verify hand is empty (all merged)
	handContainer, _ := ecs.GetComponent[components.InventoryContainer](world, handHandle)
	assert.Len(t, handContainer.Items, 0, "Hand should be empty after full merge")
}

func TestExecuteMove_ItemNotFound(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	setupPlayerWithInventories(world, playerID, playerHandle)

	itemdefs.SetGlobalForTesting(createTestRegistry())
	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Try to move non-existent item
	moveSpec := &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		Dst: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_HAND,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		ItemId: 999, // Non-existent item
	}

	result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, nil)

	assert.False(t, result.Success, "Move should fail")
	assert.Equal(t, netproto.ErrorCode_ERROR_CODE_ENTITY_NOT_FOUND, result.ErrorCode)
}

func TestExecuteMove_VersionMismatch(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(world, playerID, playerHandle)

	// Add item to grid
	item := components.InvItem{
		ItemID:   types.EntityID(100),
		TypeID:   1,
		Resource: "test",
		Quality:  100,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, gridHandle, item)

	itemdefs.SetGlobalForTesting(createTestRegistry())
	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Try to move with wrong expected version
	moveSpec := &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		Dst: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_HAND,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		ItemId: uint64(item.ItemID),
	}

	expected := []*netproto.InventoryExpected{
		{
			Ref: &netproto.InventoryRef{
				Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
				OwnerId:      uint64(playerID),
				InventoryKey: 0,
			},
			ExpectedRevision: 999, // Wrong version
		},
	}

	result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, expected)

	assert.False(t, result.Success, "Move should fail due to version mismatch")
	assert.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
}

func TestExecuteMove_ItemNotAllowedInHand(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	gridHandle, _ := setupPlayerWithInventories(world, playerID, playerHandle)

	// Add hand-only item to grid (shouldn't be allowed, but let's test the reverse)
	// Actually, let's create an item that can't go in hand
	// We need to add a new item type for this test
	registry := itemdefs.NewRegistry([]itemdefs.ItemDef{
		{
			DefID: 10,
			Key:   "grid_only_item",
			Name:  "Grid Only Item",
			Size:  itemdefs.Size{W: 1, H: 1},
			Allowed: itemdefs.Allowed{
				Hand: boolPtr(false),
				Grid: boolPtr(true),
			},
		},
	})

	item := components.InvItem{
		ItemID:   types.EntityID(100),
		TypeID:   10, // grid_only_item
		Resource: "test",
		Quality:  100,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	}
	addItemToContainer(world, gridHandle, item)

	itemdefs.SetGlobalForTesting(registry)
	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Try to move grid-only item to hand
	moveSpec := &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_GRID,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		Dst: &netproto.InventoryRef{
			Kind:         netproto.InventoryKind_INVENTORY_KIND_HAND,
			OwnerId:      uint64(playerID),
			InventoryKey: 0,
		},
		ItemId: uint64(item.ItemID),
	}

	result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, nil)

	assert.False(t, result.Success, "Move should fail - item not allowed in hand")
	assert.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
}

func TestPlacementService_CheckGridPlacement_NoCollision(t *testing.T) {
	itemdefs.SetGlobalForTesting(createTestRegistry())
	ps := NewPlacementService()

	container := &components.InventoryContainer{
		Kind:   constt.InventoryGrid,
		Width:  5,
		Height: 5,
		Items:  []components.InvItem{},
	}

	item := &components.InvItem{
		ItemID: types.EntityID(100),
		W:      1,
		H:      1,
	}

	result := ps.CheckGridPlacement(container, item, 2, 2, false)

	assert.True(t, result.Success)
	assert.Equal(t, uint8(2), result.X)
	assert.Equal(t, uint8(2), result.Y)
	assert.Nil(t, result.SwapItem)
}

func TestPlacementService_CheckGridPlacement_OutOfBounds(t *testing.T) {
	itemdefs.SetGlobalForTesting(createTestRegistry())
	ps := NewPlacementService()

	container := &components.InventoryContainer{
		Kind:   constt.InventoryGrid,
		Width:  5,
		Height: 5,
		Items:  []components.InvItem{},
	}

	item := &components.InvItem{
		ItemID: types.EntityID(100),
		W:      2,
		H:      2,
	}

	// Try to place 2x2 item at position (4,4) - would go out of bounds
	result := ps.CheckGridPlacement(container, item, 4, 4, false)

	assert.False(t, result.Success)
}

func TestPlacementService_FindFreeSpace(t *testing.T) {
	itemdefs.SetGlobalForTesting(createTestRegistry())
	ps := NewPlacementService()

	container := &components.InventoryContainer{
		Kind:   constt.InventoryGrid,
		Width:  5,
		Height: 5,
		Items: []components.InvItem{
			{ItemID: types.EntityID(1), W: 2, H: 2, X: 0, Y: 0},
		},
	}

	// Find space for 1x1 item
	found, x, y := ps.FindFreeSpace(container, 1, 1)

	assert.True(t, found)
	// Should find space at (2,0) or (0,2) depending on algorithm
	assert.True(t, x >= 2 || y >= 2, "Should find space outside occupied area")
}
