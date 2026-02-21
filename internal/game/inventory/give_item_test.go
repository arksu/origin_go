package inventory

import (
	"strings"
	"testing"

	constt "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/ecs/components"
	"origin/internal/itemdefs"
	"origin/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type sequentialIDAllocator struct {
	next  types.EntityID
	calls int
}

func (a *sequentialIDAllocator) GetFreeID() types.EntityID {
	a.calls++
	a.next++
	return a.next
}

func createGiveItemRegistry() *itemdefs.Registry {
	items := []itemdefs.ItemDef{
		{
			DefID:    200,
			Key:      "seed_bag_mini",
			Name:     "Seed Bag Mini",
			Tags:     []string{"container"},
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "items/bag_seed_empty.png",
			Visual: &itemdefs.Visual{
				NestedInventory: &itemdefs.NestedInventoryVisual{
					HasItems: "items/bag_seed_full.png",
					Empty:    "items/bag_seed_empty.png",
				},
			},
			Allowed: itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
			Container: &itemdefs.ContainerDef{
				Size: itemdefs.Size{W: 1, H: 1},
				Rules: itemdefs.ContentRules{
					AllowTags: []string{"seed"},
				},
			},
		},
		{
			DefID:    201,
			Key:      "wheat_seed_mini",
			Name:     "Wheat Seed Mini",
			Tags:     []string{"seed"},
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "wheat_seed_mini.png",
			DiscoveryLP: 77,
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
		},
		{
			DefID:    202,
			Key:      "iron_ore_mini",
			Name:     "Iron Ore Mini",
			Tags:     []string{"ore"},
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "iron_ore_mini.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(true), Grid: boolPtr(true)},
		},
		{
			DefID:    203,
			Key:      "grid_only_mini",
			Name:     "Grid Only Mini",
			Size:     itemdefs.Size{W: 1, H: 1},
			Resource: "grid_only_mini.png",
			Allowed:  itemdefs.Allowed{Hand: boolPtr(false), Grid: boolPtr(true)},
		},
	}
	return itemdefs.NewRegistry(items)
}

func setupGiveItemWorld(
	t *testing.T,
) (*ecs.World, types.EntityID, types.Handle, types.Handle, types.Handle, types.Handle, types.EntityID) {
	world, playerID, playerHandle := setupTestWorld(t)
	rootHandle := createGridContainer(world, playerID, 0, 2, 1)
	handHandle := createHandContainer(world, playerID, 0)

	bagItemID := types.EntityID(5000)
	addItemToContainer(world, rootHandle, components.InvItem{
		ItemID:   bagItemID,
		TypeID:   200,
		Resource: "items/bag_seed_empty.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        0,
		Y:        0,
	})

	nestedHandle := createGridContainer(world, bagItemID, 0, 1, 1)

	ecs.AddComponent(world, playerHandle, components.InventoryOwner{
		Inventories: []components.InventoryLink{
			{Kind: constt.InventoryGrid, Key: 0, OwnerID: playerID, Handle: rootHandle},
			{Kind: constt.InventoryHand, Key: 0, OwnerID: playerID, Handle: handHandle},
			{Kind: constt.InventoryGrid, Key: 0, OwnerID: bagItemID, Handle: nestedHandle},
		},
	})

	refIndex := ecs.GetResource[ecs.InventoryRefIndex](world)
	refIndex.Add(constt.InventoryGrid, playerID, 0, rootHandle)
	refIndex.Add(constt.InventoryHand, playerID, 0, handHandle)
	refIndex.Add(constt.InventoryGrid, bagItemID, 0, nestedHandle)

	return world, playerID, playerHandle, rootHandle, nestedHandle, handHandle, bagItemID
}

func TestGiveItem_UsesRootThenNestedThenHand(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, rootHandle, nestedHandle, handHandle, _ := setupGiveItemWorld(t)
	allocator := &sequentialIDAllocator{next: 6000}
	service := NewInventoryOperationService(zap.NewNop(), allocator, nil)

	result := service.GiveItem(world, playerID, playerHandle, "wheat_seed_mini", 3, 10)
	require.NotNil(t, result)
	require.True(t, result.Success, result.Message)
	assert.Equal(t, uint32(3), result.GrantedCount)
	assert.True(t, result.PlacedInHand)
	assert.Nil(t, result.SpawnedDroppedEntityID)

	rootContainer, _ := ecs.GetComponent[components.InventoryContainer](world, rootHandle)
	nestedContainer, _ := ecs.GetComponent[components.InventoryContainer](world, nestedHandle)
	handContainer, _ := ecs.GetComponent[components.InventoryContainer](world, handHandle)

	assert.Len(t, rootContainer.Items, 2, "root should contain bag + first granted seed")
	assert.Len(t, nestedContainer.Items, 1, "nested bag should contain second granted seed")
	assert.Len(t, handContainer.Items, 1, "hand should receive final fallback seed")
	assert.Equal(t, 3, allocator.calls, "allocator should be called only for placed items")
}

func TestGiveItem_UsesNestedBeforeHandWhenRootIsFull(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, rootHandle, nestedHandle, handHandle, _ := setupGiveItemWorld(t)
	addItemToContainer(world, rootHandle, components.InvItem{
		ItemID:   types.EntityID(7001),
		TypeID:   202,
		Resource: "iron_ore_mini.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        1,
		Y:        0,
	})

	service := NewInventoryOperationService(zap.NewNop(), &sequentialIDAllocator{next: 7100}, nil)
	result := service.GiveItem(world, playerID, playerHandle, "wheat_seed_mini", 1, 10)
	require.NotNil(t, result)
	require.True(t, result.Success, result.Message)
	assert.Equal(t, uint32(1), result.GrantedCount)
	assert.False(t, result.PlacedInHand)

	nestedContainer, _ := ecs.GetComponent[components.InventoryContainer](world, nestedHandle)
	handContainer, _ := ecs.GetComponent[components.InventoryContainer](world, handHandle)
	assert.Len(t, nestedContainer.Items, 1, "seed should be granted into nested bag")
	assert.Len(t, handContainer.Items, 0, "hand must remain empty when nested can accept item")
}

func TestGiveItem_GrantsOnlyCountThatFits(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, rootHandle, nestedHandle, handHandle, _ := setupGiveItemWorld(t)
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID:   types.EntityID(8001),
		TypeID:   202,
		Resource: "iron_ore_mini.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
	})

	allocator := &sequentialIDAllocator{next: 8100}
	service := NewInventoryOperationService(zap.NewNop(), allocator, nil)
	result := service.GiveItem(world, playerID, playerHandle, "iron_ore_mini", 5, 10)
	require.NotNil(t, result)
	require.True(t, result.Success, result.Message)
	assert.Equal(t, uint32(1), result.GrantedCount)
	assert.False(t, result.PlacedInHand)
	assert.Nil(t, result.SpawnedDroppedEntityID)
	assert.True(t, strings.Contains(result.Message, "1/5"), "message should report partial grant")

	rootContainer, _ := ecs.GetComponent[components.InventoryContainer](world, rootHandle)
	nestedContainer, _ := ecs.GetComponent[components.InventoryContainer](world, nestedHandle)
	handContainer, _ := ecs.GetComponent[components.InventoryContainer](world, handHandle)

	assert.Len(t, rootContainer.Items, 2, "root should contain bag + one granted ore")
	assert.Len(t, nestedContainer.Items, 0, "ore must not go into seed-only nested bag")
	assert.Len(t, handContainer.Items, 1, "existing hand item must remain unchanged")
	assert.Equal(t, 1, allocator.calls, "allocator should be called exactly once for one granted item")
}

func TestGiveItem_FailsWhenNoDestinationFits(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, rootHandle, _, handHandle, _ := setupGiveItemWorld(t)
	addItemToContainer(world, rootHandle, components.InvItem{
		ItemID:   types.EntityID(9001),
		TypeID:   202,
		Resource: "iron_ore_mini.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        1,
		Y:        0,
	})
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID:   types.EntityID(9002),
		TypeID:   202,
		Resource: "iron_ore_mini.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
	})

	allocator := &sequentialIDAllocator{next: 9100}
	service := NewInventoryOperationService(zap.NewNop(), allocator, nil)
	result := service.GiveItem(world, playerID, playerHandle, "grid_only_mini", 1, 10)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.Equal(t, uint32(0), result.GrantedCount)
	assert.True(t, strings.Contains(result.Message, "0/1"))
	assert.Equal(t, 0, allocator.calls, "allocator must not be called when placement fails")
}

func TestExecutorGiveItem_UpdatesParentResourceForNestedContainer(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, rootHandle, _, _, bagItemID := setupGiveItemWorld(t)
	addItemToContainer(world, rootHandle, components.InvItem{
		ItemID:   types.EntityID(9901),
		TypeID:   202,
		Resource: "iron_ore_mini.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        1,
		Y:        0,
	})

	executor := NewInventoryExecutor(zap.NewNop(), &sequentialIDAllocator{next: 12000}, nil, nil, nil)
	result := executor.GiveItem(world, playerID, playerHandle, "wheat_seed_mini", 1, 10)
	require.NotNil(t, result)
	require.True(t, result.Success, result.Message)

	rootContainer, _ := ecs.GetComponent[components.InventoryContainer](world, rootHandle)
	bagFound := false
	for _, item := range rootContainer.Items {
		if item.ItemID != bagItemID {
			continue
		}
		bagFound = true
		assert.Equal(t, "items/bag_seed_full.png", item.Resource, "bag resource should switch to full when nested contains items")
	}
	require.True(t, bagFound, "expected to find bag item in root container")
}

func TestExecutorGiveItem_ReturnsLatestRootRevisionAfterCascade(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, rootHandle, _, _, _ := setupGiveItemWorld(t)
	executor := NewInventoryExecutor(zap.NewNop(), &sequentialIDAllocator{next: 13000}, nil, nil, nil)

	// First item goes to root, second goes to nested and triggers parent resource cascade.
	result := executor.GiveItem(world, playerID, playerHandle, "wheat_seed_mini", 2, 10)
	require.NotNil(t, result)
	require.True(t, result.Success, result.Message)

	rootContainer, _ := ecs.GetComponent[components.InventoryContainer](world, rootHandle)
	var rootInfo *ContainerInfo
	for _, info := range result.UpdatedContainers {
		if info == nil || info.Container == nil {
			continue
		}
		if info.Handle == rootHandle {
			rootInfo = info
			break
		}
	}
	require.NotNil(t, rootInfo, "expected updated containers to include root inventory")
	assert.Equal(t, rootContainer.Version, rootInfo.Container.Version, "returned root revision must match world revision after cascade")
}

func TestGiveItem_AddsDiscoveryKeyOnceOnSuccessfulGive(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, _, _, _, _ := setupGiveItemWorld(t)
	ecs.AddComponent(world, playerHandle, components.CharacterProfile{
		Discovery: []string{"existing_key"},
		Experience: components.CharacterExperience{
			LP: 10,
		},
	})
	service := NewInventoryOperationService(zap.NewNop(), &sequentialIDAllocator{next: 14000}, nil)

	first := service.GiveItem(world, playerID, playerHandle, "wheat_seed_mini", 1, 10)
	require.NotNil(t, first)
	require.True(t, first.Success, first.Message)
	assert.Equal(t, int64(77), first.DiscoveryLPGained, "first discovery should grant LP delta")
	second := service.GiveItem(world, playerID, playerHandle, "wheat_seed_mini", 1, 10)
	require.NotNil(t, second)
	require.True(t, second.Success, second.Message)
	assert.Equal(t, int64(0), second.DiscoveryLPGained, "repeated discovery should not grant LP delta")

	profile, hasProfile := ecs.GetComponent[components.CharacterProfile](world, playerHandle)
	require.True(t, hasProfile)
	assert.Equal(t, []string{"existing_key", "wheat_seed_mini"}, profile.Discovery)
	assert.Equal(t, int64(87), profile.Experience.LP, "discovery LP should be granted only once for a new key")
}

func TestGiveItem_DoesNotAddDiscoveryOnFailedGive(t *testing.T) {
	previousRegistry := itemdefs.Global()
	t.Cleanup(func() { itemdefs.SetGlobalForTesting(previousRegistry) })
	itemdefs.SetGlobalForTesting(createGiveItemRegistry())

	world, playerID, playerHandle, rootHandle, _, handHandle, _ := setupGiveItemWorld(t)
	ecs.AddComponent(world, playerHandle, components.CharacterProfile{
		Experience: components.CharacterExperience{
			LP: 10,
		},
	})
	addItemToContainer(world, rootHandle, components.InvItem{
		ItemID:   types.EntityID(15001),
		TypeID:   202,
		Resource: "iron_ore_mini.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
		X:        1,
		Y:        0,
	})
	addItemToContainer(world, handHandle, components.InvItem{
		ItemID:   types.EntityID(15002),
		TypeID:   202,
		Resource: "iron_ore_mini.png",
		Quality:  10,
		Quantity: 1,
		W:        1,
		H:        1,
	})
	service := NewInventoryOperationService(zap.NewNop(), &sequentialIDAllocator{next: 15000}, nil)

	result := service.GiveItem(world, playerID, playerHandle, "grid_only_mini", 1, 10)
	require.NotNil(t, result)
	require.False(t, result.Success)
	assert.Equal(t, int64(0), result.DiscoveryLPGained, "failed give should not report LP gain")

	profile, hasProfile := ecs.GetComponent[components.CharacterProfile](world, playerHandle)
	require.True(t, hasProfile)
	assert.Empty(t, profile.Discovery)
	assert.Equal(t, int64(10), profile.Experience.LP, "failed give must not grant discovery LP")
}
