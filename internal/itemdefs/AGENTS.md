# Item Definitions Package

Centralized item definition registry for game entities. Provides type-safe access to item metadata, stacking rules, equipment slots, and nested container constraints.

## Overview

The `itemdefs` package manages all item definitions in the game. It loads definitions from JSON files and provides fast O(1) lookups by both numeric ID (`defId`) and string key.

Key responsibilities:
- **Item metadata**: name, size, resource paths, tags
- **Inventory rules**: where items can be placed (hand, grid, equipment slots)
- **Stacking behavior**: stackable vs non-stackable, max stack size
- **Container definitions**: nested inventories with content filtering rules
- **Visual state**: dynamic resource paths based on item state

## Core Types

### ItemDef

Main item definition structure:

```go
type ItemDef struct {
    DefID    int      // Unique numeric identifier
    Key      string   // Unique string identifier (e.g., "wheat_seed")
    Name     string   // Display name
    Tags     []string // Classification tags (e.g., "seed", "ore", "weapon")
    Size     Size     // Grid dimensions (W x H)
    Stack    *Stack   // Stacking rules (nil = not stackable)
    Allowed  Allowed  // Placement restrictions
    Resource string   // Base resource path for rendering
    Visual   *Visual  // Dynamic resource rules
    Container *ContainerDef // Nested inventory config (nil = not a container)
}
```

### Size

Grid dimensions in inventory cells:

```go
type Size struct {
    W int // Width in cells
    H int // Height in cells
}
```

Common sizes:
- `1x1` - small items (seeds, ores)
- `2x2` - medium items (bags, tools)
- `3x2` - large items (weapons, armor)

### Allowed

Placement restrictions:

```go
type Allowed struct {
    Hand           *bool    // Can be held in hand? (nil = inherit default)
    Grid           *bool    // Can be placed in grid? (nil = inherit default)
    EquipmentSlots []string // Allowed equipment slots (e.g., "head", "chest")
}
```

### Stack

Stacking configuration:

```go
type Stack struct {
    Mode string // "none" or "stack"
    Max  int    // Maximum stack size
}
```

Stack modes:
- `StackModeNone` ("none") - Item cannot be stacked (default)
- `StackModeStack` ("stack") - Item can be stacked up to `Max` quantity

### ContainerDef

Nested inventory configuration:

```go
type ContainerDef struct {
    Size  Size         // Grid dimensions for the nested inventory
    Rules ContentRules // Filtering rules for allowed content
}
```

### ContentRules

Content filtering for nested containers:

```go
type ContentRules struct {
    AllowTags     []string // Item must have at least one of these tags
    DenyTags      []string // Item is forbidden if it has any of these tags
    AllowItemKeys []string // Whitelist of specific item keys
    DenyItemKeys  []string // Blacklist of specific item keys
}
```

Rule evaluation order (first match wins):
1. **DenyItemKeys** - reject if item key is blacklisted
2. **AllowItemKeys** - if non-empty, item key must be in whitelist
3. **DenyTags** - reject if item has any denied tag
4. **AllowTags** - if non-empty, item must have at least one allowed tag

## Registry API

### Global Registry

Access the global registry (initialized at startup):

```go
// Get global registry
def := itemdefs.Global().GetByID(101)

// Check if item exists
if def, ok := itemdefs.Global().GetByKey("wheat_seed"); ok {
    // Use def...
}
```

### Registry Methods

```go
// GetByID retrieves item definition by numeric defId
func (r *Registry) GetByID(defID int) (*ItemDef, bool)

// GetByKey retrieves item definition by string key
func (r *Registry) GetByKey(key string) (*ItemDef, bool)
```

Both methods return `(nil, false)` if the item is not found.

## Usage Examples

### Basic Item Lookup

```go
def, ok := itemdefs.Global().GetByKey("iron_sword")
if !ok {
    return fmt.Errorf("unknown item: iron_sword")
}

fmt.Printf("Item: %s, Size: %dx%d\n", def.Name, def.Size.W, def.Size.H)
```

### Checking Placement Restrictions

```go
def, _ := itemdefs.Global().GetByID(itemTypeID)

// Check if can be held in hand
if def.Allowed.Hand != nil && *def.Allowed.Hand {
    // Item can be held in hand
}

// Check if can be equipped
canEquipHead := slices.Contains(def.Allowed.EquipmentSlots, "head")
```

### Working with Content Rules

```go
// Get container definition for a seed bag
seedBagDef, _ := itemdefs.Global().GetByKey("seed_bag")
if seedBagDef.Container != nil {
    rules := seedBagDef.Container.Rules
    
    // Check if item can be placed inside
    itemDef, _ := itemdefs.Global().GetByKey("wheat_seed")
    
    // Check AllowTags
    for _, allowed := range rules.AllowTags {
        if slices.Contains(itemDef.Tags, allowed) {
            // Item is allowed
        }
    }
}
```

### Dynamic Resource Resolution

```go
def, _ := itemdefs.Global().GetByKey("seed_bag")

// Get resource based on whether bag has items
resource := def.ResolveResource(hasNestedItems)
// Returns "seed_bag_full.png" if has items, "seed_bag_empty.png" if empty
```

## JSON Format

Item definitions are loaded from JSON files:

```json
{
  "v": 1,
  "source": "items/base.json",
  "items": [
    {
      "defId": 101,
      "key": "wheat_seed",
      "name": "Wheat Seed",
      "tags": ["seed"],
      "size": {"w": 1, "h": 1},
      "stack": {"mode": "stack", "max": 100},
      "allowed": {"hand": true, "grid": true},
      "resource": "wheat_seed.png"
    },
    {
      "defId": 102,
      "key": "seed_bag",
      "name": "Seed Bag",
      "tags": ["container"],
      "size": {"w": 2, "h": 2},
      "allowed": {"hand": true, "grid": true},
      "resource": "seed_bag.png",
      "container": {
        "size": {"w": 4, "h": 4},
        "rules": {
          "allowTags": ["seed"]
        }
      },
      "visual": {
        "nestedInventory": {
          "hasItems": "seed_bag_full.png",
          "empty": "seed_bag_empty.png"
        }
      }
    }
  ]
}
```

## Best Practices

### 1. Use Keys for Readability

Prefer string keys over numeric IDs in code:

```go
// Good - readable, self-documenting
def, _ := itemdefs.Global().GetByKey("wheat_seed")

// Less ideal - magic number
def, _ := itemdefs.Global().GetByID(101)
```

### 2. Defensive Checks

Always check the `ok` return value:

```go
def, ok := itemdefs.Global().GetByKey(itemKey)
if !ok {
    // Handle missing definition gracefully
    return nil, fmt.Errorf("item definition not found: %s", itemKey)
}
```

### 3. Tags for Classification

Use tags for item categories rather than hardcoding keys:

```go
// Good - works with any seed
def, _ := itemdefs.Global().GetByID(typeID)
if slices.Contains(def.Tags, "seed") {
    // Process any seed type
}

// Less ideal - specific key check
if def.Key == "wheat_seed" || def.Key == "corn_seed" {
    // Must update for each new seed
}
```

### 4. Container Rules Design

Design content rules to be permissive by default:

```go
// Good: Only deny specific problematic items
rules := ContentRules{
    DenyItemKeys: []string{"cursed_item"},
}

// Good: Allow broad category
rules := ContentRules{
    AllowTags: []string{"seed"},
}

// Avoid: Empty allow list blocks everything
rules := ContentRules{
    AllowTags: []string{}, // This blocks all items!
}
```

### 5. Stack Configuration

Always specify max stack size for stackable items:

```go
// Good
"stack": {"mode": "stack", "max": 100}

// Bad - missing max
"stack": {"mode": "stack"}
```

## Testing

### Test Registry Setup

Use `SetGlobalForTesting` to inject test definitions:

```go
func TestItemLogic(t *testing.T) {
    // Create test registry
    registry := itemdefs.NewRegistry([]itemdefs.ItemDef{
        {
            DefID: 1001,
            Key:   "test_sword",
            Name:  "Test Sword",
            Size:  itemdefs.Size{W: 1, H: 3},
            Allowed: itemdefs.Allowed{
                Hand:           boolPtr(true),
                Grid:           boolPtr(true),
                EquipmentSlots: []string{"main_hand"},
            },
        },
    })
    
    // Replace global registry for this test
    itemdefs.SetGlobalForTesting(registry)
    
    // Run test code...
}
```

### Mocking Item Definitions

Create minimal definitions for unit tests:

```go
// Minimal valid item def
minimalDef := itemdefs.ItemDef{
    DefID:    9999,
    Key:      "test_item",
    Name:     "Test Item",
    Size:     itemdefs.Size{W: 1, H: 1},
    Resource: "test.png",
}
```

## Performance Characteristics

- **Lookup by ID**: O(1) via map access
- **Lookup by Key**: O(1) via map access
- **Memory overhead**: ~200 bytes per item definition
- **Registry initialization**: One-time cost at startup
- **Thread safety**: Registry is immutable after initialization (safe for concurrent reads)

## Integration with Other Packages

### Inventory System

The inventory system uses `itemdefs` for:
- Validating item placement (hand/grid/equipment restrictions)
- Checking container content rules
- Resolving item sizes for grid placement

### ECS Components

Items in the game world reference `ItemDef` via `TypeID`:

```go
type InvItem struct {
    ItemID   types.EntityID // Unique instance ID
    TypeID   uint32         // References itemdefs.Global().GetByID(TypeID)
    Resource string         // May override def.Resource
    // ... other fields
}
```

### Network Protocol

Item instances are sent to clients with their `TypeID`, which clients use to look up local item definitions (synced from server JSON).
