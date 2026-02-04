package inventory

import (
	"encoding/json"
	"testing"

	_const "origin/internal/const"
	"origin/internal/ecs"
	"origin/internal/eventbus"
	"origin/internal/itemdefs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadNestedInventory(t *testing.T) {
	// Create test item registry with seed_bag container
	registry := itemdefs.NewRegistry([]itemdefs.ItemDef{
		{
			DefID: 4001,
			Key:   "seed_bag",
			Name:  "Seed Bag",
			Tags:  []string{"container"},
			Size:  itemdefs.Size{W: 1, H: 1},
			Container: &itemdefs.ContainerDef{
				Size: itemdefs.Size{W: 5, H: 5},
				Rules: itemdefs.ContentRules{
					AllowTags: []string{"seed"},
				},
			},
			Allowed: itemdefs.Allowed{
				Grid: func() *bool { b := true; return &b }(),
			},
		},
		{
			DefID: 4003,
			Key:   "wheat_seed",
			Name:  "Wheat Seed",
			Tags:  []string{"seed"},
			Size:  itemdefs.Size{W: 1, H: 1},
			Allowed: itemdefs.Allowed{
				Grid: func() *bool { b := true; return &b }(),
			},
		},
	})

	// Create inventory loader
	logger, _ := zap.NewDevelopment()
	loader := NewInventoryLoader(registry, logger)

	// Create test world with event bus
	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)

	// Create test inventory data with nested inventory
	inventoryData := InventoryDataV1{
		Version: 1,
		Items: []InventoryItemV1{
			{
				ItemID:   1000002,
				TypeID:   4001, // seed_bag
				Quality:  10,
				Quantity: 1,
				X:        2,
				Y:        0,
				NestedInventory: &InventoryDataV1{
					Version: 1,
					Items: []InventoryItemV1{
						{
							ItemID:   1000003,
							TypeID:   4003, // wheat_seed
							Quality:  10,
							Quantity: 10,
							X:        0,
							Y:        0,
						},
					},
				},
			},
		},
	}

	// Test sanitizeItems with nested inventory
	sanitizedItems, warnings := loader.sanitizeItems(
		world,
		inventoryData.Items,
		_const.InventoryGrid,
		10, 10, // backpack size
	)

	// Verify results
	require.Len(t, sanitizedItems, 1)
	require.Empty(t, warnings)

	// Check main item
	mainItem := sanitizedItems[0]
	assert.Equal(t, uint64(1000002), mainItem.ItemID)
	assert.Equal(t, uint32(4001), mainItem.TypeID)
	assert.Equal(t, uint8(2), mainItem.X)
	assert.Equal(t, uint8(0), mainItem.Y)

	// Verify nested container was created (check world for additional entities)
	// The nested container should be spawned as a separate entity
	// We can verify this by checking the world's entity count
	// Note: The exact verification depends on how you want to track nested containers
}

func TestNestedInventoryValidation(t *testing.T) {
	registry := itemdefs.NewRegistry([]itemdefs.ItemDef{
		{
			DefID: 1001,
			Key:   "regular_item",
			Name:  "Regular Item",
			Size:  itemdefs.Size{W: 1, H: 1},
			Allowed: itemdefs.Allowed{
				Grid: func() *bool { b := true; return &b }(),
			},
		},
	})

	logger, _ := zap.NewDevelopment()
	loader := NewInventoryLoader(registry, logger)

	eventBus := eventbus.New(&eventbus.Config{MinWorkers: 1, MaxWorkers: 2})
	world := ecs.NewWorldWithCapacity(100, eventBus, 0)

	// Test with non-container item having nested inventory
	inventoryData := InventoryDataV1{
		Version: 1,
		Items: []InventoryItemV1{
			{
				ItemID:   1000001,
				TypeID:   1001, // regular_item (not a container)
				Quality:  10,
				Quantity: 1,
				X:        0,
				Y:        0,
				NestedInventory: &InventoryDataV1{
					Version: 1,
					Items:   []InventoryItemV1{},
				},
			},
		},
	}

	sanitizedItems, warnings := loader.sanitizeItems(
		world,
		inventoryData.Items,
		_const.InventoryGrid,
		10, 10,
	)

	// Should have warning about non-container having nested inventory
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "has nested_inventory but is not a container")
	require.Len(t, sanitizedItems, 1) // Item should still be processed
}

func TestNestedInventoryJSONSerialization(t *testing.T) {
	// Test that the JSON structure matches the expected format
	inventoryData := InventoryDataV1{
		Version: 1,
		Items: []InventoryItemV1{
			{
				ItemID:   1000001,
				TypeID:   2001,
				Quality:  10,
				Quantity: 1,
			},
			{
				ItemID:   1000002,
				TypeID:   4001,
				Quality:  10,
				Quantity: 1,
				X:        2,
				Y:        0,
				NestedInventory: &InventoryDataV1{
					Version: 1,
					Items: []InventoryItemV1{
						{
							ItemID:   1000003,
							TypeID:   4003,
							Quality:  10,
							Quantity: 10,
						},
					},
				},
			},
		},
	}

	// Serialize to JSON
	jsonData, err := json.MarshalIndent(inventoryData, "", "  ")
	require.NoError(t, err)

	// Verify JSON structure
	expectedJSON := `{
  "v": 1,
  "items": [
    {
      "item_id": 1000001,
      "type_id": 2001,
      "quality": 10,
      "quantity": 1
    },
    {
      "item_id": 1000002,
      "type_id": 4001,
      "quality": 10,
      "quantity": 1,
      "x": 2,
      "nested_inventory": {
        "v": 1,
        "items": [
          {
            "item_id": 1000003,
            "type_id": 4003,
            "quality": 10,
            "quantity": 10
          }
        ]
      }
    }
  ]
}`

	assert.JSONEq(t, expectedJSON, string(jsonData))

	// Test deserialization
	var decoded InventoryDataV1
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	// Verify decoded data matches original
	assert.Equal(t, inventoryData.Version, decoded.Version)
	require.Len(t, decoded.Items, 2)
	assert.Equal(t, inventoryData.Items[0].ItemID, decoded.Items[0].ItemID)
	assert.Equal(t, inventoryData.Items[1].ItemID, decoded.Items[1].ItemID)
	require.NotNil(t, decoded.Items[1].NestedInventory)
	require.Len(t, decoded.Items[1].NestedInventory.Items, 1)
	assert.Equal(t, inventoryData.Items[1].NestedInventory.Items[0].ItemID, decoded.Items[1].NestedInventory.Items[0].ItemID)
}
