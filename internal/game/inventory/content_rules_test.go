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

// createContentRulesRegistry creates a registry with items for content rules testing:
//
//	defID=100 "seed_bag"   — container with AllowTags: ["seed"], size 4x4
//	defID=101 "wheat_seed" — tagged ["seed"], 1x1
//	defID=102 "iron_ore"   — tagged ["ore"], 1x1
//	defID=103 "magic_seed" — tagged ["seed","magic"], 1x1
//	defID=104 "banned_ore" — tagged ["ore"], key in DenyItemKeys
//	defID=105 "special_bag"— container with DenyTags: ["contraband"]
//	defID=106 "contraband"  — tagged ["contraband"], 1x1
//	defID=107 "normal_item" — no tags, 1x1
//	defID=108 "key_bag"    — container with AllowItemKeys: ["wheat_seed","magic_seed"]
func createContentRulesRegistry() *itemdefs.Registry {
	items := []itemdefs.ItemDef{
		{
			DefID:    100,
			Key:      "seed_bag",
			Name:     "Seed Bag",
			Tags:     []string{"container"},
			Size:     itemdefs.Size{W: 2, H: 2},
			Resource: "seed_bag.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
			Container: &itemdefs.ContainerDef{
				Size: itemdefs.Size{W: 4, H: 4},
				Rules: itemdefs.ContentRules{
					AllowTags: []string{"seed"},
				},
			},
		},
		{
			DefID:    101,
			Key:      "wheat_seed",
			Name:     "Wheat Seed",
			Tags:     []string{"seed"},
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "wheat_seed.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
		},
		{
			DefID:    102,
			Key:      "iron_ore",
			Name:     "Iron Ore",
			Tags:     []string{"ore"},
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "iron_ore.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
		},
		{
			DefID:    103,
			Key:      "magic_seed",
			Name:     "Magic Seed",
			Tags:     []string{"seed", "magic"},
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "magic_seed.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
		},
		{
			DefID:    105,
			Key:      "special_bag",
			Name:     "Special Bag",
			Tags:     []string{"container"},
			Size:     itemdefs.Size{W: 2, H: 2},
			Resource: "special_bag.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
			Container: &itemdefs.ContainerDef{
				Size: itemdefs.Size{W: 4, H: 4},
				Rules: itemdefs.ContentRules{
					DenyTags: []string{"contraband"},
				},
			},
		},
		{
			DefID:    106,
			Key:      "contraband_item",
			Name:     "Contraband",
			Tags:     []string{"contraband"},
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "contraband.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
		},
		{
			DefID:    107,
			Key:      "normal_item",
			Name:     "Normal Item",
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "normal.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
		},
		{
			DefID:    108,
			Key:      "key_bag",
			Name:     "Key Bag",
			Tags:     []string{"container"},
			Size:     itemdefs.Size{W: 2, H: 2},
			Resource: "key_bag.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
			Container: &itemdefs.ContainerDef{
				Size: itemdefs.Size{W: 4, H: 4},
				Rules: itemdefs.ContentRules{
					AllowItemKeys: []string{"wheat_seed", "magic_seed"},
				},
			},
		},
		{
			DefID:    109,
			Key:      "deny_key_bag",
			Name:     "Deny Key Bag",
			Tags:     []string{"container"},
			Size:     itemdefs.Size{W: 2, H: 2},
			Resource: "deny_key_bag.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
			Container: &itemdefs.ContainerDef{
				Size: itemdefs.Size{W: 4, H: 4},
				Rules: itemdefs.ContentRules{
					DenyItemKeys: []string{"iron_ore"},
				},
			},
		},
	}
	return itemdefs.NewRegistry(items)
}

// setupNestedContainer creates a player with:
//   - backpack grid (ownerID=playerID)
//   - hand container (ownerID=playerID)
//   - a container item (bagTypeID) placed in the backpack at (0,0)
//   - a nested grid container (ownerID=bagItemID) for the bag
//
// Returns: backpackHandle, handHandle, nestedGridHandle, bagItemID
func setupNestedContainer(
	t *testing.T,
	world *ecs.World,
	playerID types.EntityID,
	playerHandle types.Handle,
	bagItemID types.EntityID,
	bagTypeID uint32,
) (types.Handle, types.Handle, types.Handle) {
	backpackHandle := createGridContainer(world, playerID, 0, 10, 10)
	handHandle := createHandContainer(world, playerID, 0)

	// Place the bag item in the backpack
	bagDef, ok := itemdefs.Global().GetByID(int(bagTypeID))
	require.True(t, ok)
	addItemToContainer(world, backpackHandle, components.InvItem{
		ItemID:   bagItemID,
		TypeID:   bagTypeID,
		Resource: bagDef.Resource,
		Quality:  100,
		Quantity: 1,
		W:        uint8(bagDef.Size.W),
		H:        uint8(bagDef.Size.H),
		X:        0,
		Y:        0,
	})

	// Create nested grid container for the bag
	nestedHandle := createGridContainer(world, bagItemID, 0,
		uint8(bagDef.Container.Size.W), uint8(bagDef.Container.Size.H))

	// Set up InventoryOwner with all links
	owner := components.InventoryOwner{
		Inventories: []components.InventoryLink{
			{Kind: constt.InventoryGrid, Key: 0, OwnerID: playerID, Handle: backpackHandle},
			{Kind: constt.InventoryHand, Key: 0, OwnerID: playerID, Handle: handHandle},
			{Kind: constt.InventoryGrid, Key: 0, OwnerID: bagItemID, Handle: nestedHandle},
		},
	}
	ecs.AddComponent(world, playerHandle, owner)

	// Populate InventoryRefIndex
	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	refIndex.Add(constt.InventoryGrid, playerID, 0, backpackHandle)
	refIndex.Add(constt.InventoryHand, playerID, 0, handHandle)
	refIndex.Add(constt.InventoryGrid, bagItemID, 0, nestedHandle)

	return backpackHandle, handHandle, nestedHandle
}

func TestContentRules_AllowTags_Allowed(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, nestedHandle := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 100) // seed_bag

	// Put wheat_seed in hand
	seedItemID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: seedItemID, TypeID: 101, Resource: "wheat_seed.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move seed from hand → nested seed bag
	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(seedItemID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	require.True(t, result.Success, "Seed should be allowed in seed bag: %s", result.Message)

	// Verify item is in nested container
	nested, _ := ecs.GetComponent[components.InventoryContainer](world, nestedHandle)
	assert.Len(t, nested.Items, 1)
	assert.Equal(t, seedItemID, nested.Items[0].ItemID)
}

func TestContentRules_AllowTags_Denied(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, _ := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 100) // seed_bag

	// Put iron_ore in hand
	oreItemID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: oreItemID, TypeID: 102, Resource: "iron_ore.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move ore from hand → nested seed bag — should fail
	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(oreItemID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	assert.False(t, result.Success, "Ore should NOT be allowed in seed bag")
	assert.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
}

func TestContentRules_AllowTags_MultipleTagsOnItem(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, _ := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 100) // seed_bag

	// Put magic_seed (tags: ["seed","magic"]) in hand
	magicSeedID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: magicSeedID, TypeID: 103, Resource: "magic_seed.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move magic seed from hand → seed bag — should succeed (has "seed" tag)
	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(magicSeedID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	require.True(t, result.Success, "Magic seed should be allowed in seed bag (has 'seed' tag): %s", result.Message)
}

func TestContentRules_DenyTags(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, _ := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 105) // special_bag (DenyTags: ["contraband"])

	// Put contraband in hand
	contrabandID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: contrabandID, TypeID: 106, Resource: "contraband.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move contraband from hand → special bag — should fail
	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(contrabandID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	assert.False(t, result.Success, "Contraband should NOT be allowed in special bag")
	assert.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
}

func TestContentRules_DenyTags_AllowNonDenied(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, nestedHandle := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 105) // special_bag

	// Put normal_item (no tags) in hand
	normalID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: normalID, TypeID: 107, Resource: "normal.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move normal item from hand → special bag — should succeed
	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(normalID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	require.True(t, result.Success, "Normal item should be allowed in special bag: %s", result.Message)

	nested, _ := ecs.GetComponent[components.InventoryContainer](world, nestedHandle)
	assert.Len(t, nested.Items, 1)
}

func TestContentRules_AllowItemKeys(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, nestedHandle := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 108) // key_bag (AllowItemKeys: ["wheat_seed","magic_seed"])

	// Put wheat_seed in hand — allowed by key
	seedID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: seedID, TypeID: 101, Resource: "wheat_seed.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(seedID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	require.True(t, result.Success, "Wheat seed should be allowed by AllowItemKeys: %s", result.Message)

	nested, _ := ecs.GetComponent[components.InventoryContainer](world, nestedHandle)
	assert.Len(t, nested.Items, 1)
}

func TestContentRules_AllowItemKeys_Denied(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, _ := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 108) // key_bag

	// Put iron_ore in hand — NOT in AllowItemKeys
	oreID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: oreID, TypeID: 102, Resource: "iron_ore.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(oreID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	assert.False(t, result.Success, "Iron ore should NOT be allowed in key bag")
	assert.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
}

func TestContentRules_DenyItemKeys(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, _ := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 109) // deny_key_bag (DenyItemKeys: ["iron_ore"])

	// Put iron_ore in hand — denied by key
	oreID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: oreID, TypeID: 102, Resource: "iron_ore.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(oreID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	assert.False(t, result.Success, "Iron ore should be denied by DenyItemKeys")
	assert.Equal(t, netproto.ErrorCode_ERROR_CODE_INVALID_REQUEST, result.ErrorCode)
}

func TestContentRules_DenyItemKeys_AllowOthers(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	_, handHandle, nestedHandle := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 109) // deny_key_bag

	// Put wheat_seed in hand — NOT in DenyItemKeys, should be allowed
	seedID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: seedID, TypeID: 101, Resource: "wheat_seed.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(bagItemID),
		},
		ItemId: uint64(seedID),
		DstPos: &netproto.GridPos{X: 0, Y: 0},
	}, nil)

	require.True(t, result.Success, "Wheat seed should be allowed in deny_key_bag: %s", result.Message)

	nested, _ := ecs.GetComponent[components.InventoryContainer](world, nestedHandle)
	assert.Len(t, nested.Items, 1)
}

func TestContentRules_RootContainer_NoRules(t *testing.T) {
	world, playerID, playerHandle := setupTestWorld(t)
	itemdefs.SetGlobalForTesting(createContentRulesRegistry())

	bagItemID := types.EntityID(5000)
	backpackHandle, handHandle, _ := setupNestedContainer(t, world, playerID, playerHandle, bagItemID, 100)

	// Put any item in hand
	oreID := types.EntityID(5001)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID: oreID, TypeID: 102, Resource: "iron_ore.png",
		Quality: 100, Quantity: 1, W: 1, H: 1,
	})

	service := NewInventoryOperationService(zap.NewNop(), nil, nil)

	// Move to root backpack (ownerID=playerID) — no rules, should always succeed
	result := service.ExecuteMove(world, playerID, playerHandle, 1, &netproto.InventoryMoveSpec{
		Src: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_HAND, OwnerId: uint64(playerID),
		},
		Dst: &netproto.InventoryRef{
			Kind: netproto.InventoryKind_INVENTORY_KIND_GRID, OwnerId: uint64(playerID),
		},
		ItemId: uint64(oreID),
		DstPos: &netproto.GridPos{X: 3, Y: 3},
	}, nil)

	require.True(t, result.Success, "Any item should be allowed in root backpack: %s", result.Message)

	backpack, _ := ecs.GetComponent[components.InventoryContainer](world, backpackHandle)
	// backpack already has the bag at (0,0), now should also have ore at (3,3)
	assert.Len(t, backpack.Items, 2)
}
