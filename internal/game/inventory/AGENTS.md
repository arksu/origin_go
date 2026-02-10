# Inventory System Architecture

Server-side inventory management with support for grid-based containers, equipment slots, hand inventory, nested containers, and content filtering rules.

## Overview

The inventory system manages player items across multiple container types:
- **Grid containers** - 2D spatial inventory with collision detection
- **Hand inventory** - Single item held for drag-and-drop operations
- **Equipment containers** - Slots for wearable items (head, chest, etc.)
- **Nested containers** - Items that contain their own inventories (seed bags, chests)

Key design goals:
- **Atomic operations** - All moves are validated and executed as single transactions
- **Optimistic concurrency** - Revision-based conflict detection for concurrent modifications
- **Content rules** - Restrict what items can be placed in nested containers
- **Zero-allocation hot paths** - Efficient snapshot building and validation

## Integration Contracts (Important)

- When inventory updates modify world-object root containers (`Kind=Grid`, `Key=0`, owner is object entity),
  inventory flow must mark object behavior dirty (`ecs.MarkObjectBehaviorDirty`) so runtime appearance/flags are recomputed by `ObjectBehaviorSystem`.
- Content rules for nested containers are global by parent item, not player-only:
  validation must work identically whether nested inventory is in player inventory, world container, equipment, or future station containers.

## Package Structure

```
internal/game/inventory/
├── operations.go          # Core inventory operations (ExecuteMove)
├── operations_test.go     # Unit tests for operations
├── content_rules_test.go  # Tests for container content validation
├── executor.go            # ECS integration (InventoryExecutor)
├── validation.go          # Validation logic and content rules
├── placement.go           # Grid placement and collision detection
├── loader.go              # Database loading (InventoryLoader)
├── saver.go               # Database saving (InventorySaver)
├── snapshot.go            # Client snapshot building
└── types.go               # Common types and interfaces
```

## Core Components

### InventoryOperationService

Main entry point for inventory operations. Handles the complete lifecycle of item moves:

```go
service := NewInventoryOperationService(logger)
result := service.ExecuteMove(world, playerID, playerHandle, opID, moveSpec, expected)
```

**Operation Flow:**
1. **Resolve containers** - Find source and destination via `InventoryRefIndex`
2. **Validate versions** - Check optimistic concurrency (`expected_revision`)
3. **Locate item** - Find item in source container
4. **Validate placement** - Check item constraints, container content rules
5. **Check placement/swap** - Detect collisions, support swap/merge operations
6. **Execute** - Modify ECS components atomically
7. **Build result** - Return updated containers for network sync

### Container Types

| Kind | OwnerID | Purpose |
|------|---------|---------|
| `InventoryGrid` | Player ID or Item ID | 2D spatial storage with collision |
| `InventoryHand` | Player ID | Single item held for drag operations |
| `InventoryEquipment` | Player ID | Equipment slots (head, chest, etc.) |
| `InventoryDroppedItem` | - | Ground items (not yet implemented) |

**Nested Containers:** When an item with `ContainerDef` is placed in a grid, its nested inventory has `OwnerID = item.ItemID` (not player ID). This distinguishes nested containers from player-owned containers.

### Validation Pipeline

Multi-stage validation in `ValidateItemAllowedInContainer`:

1. **Item existence** - Verify `ItemDef` exists in registry
2. **Kind restrictions** - Check `Allowed.Hand`, `Allowed.Grid`, `Allowed.EquipmentSlots`
3. **Content rules** - For grid destinations, check nested container rules:
   - `DenyItemKeys` - Reject blacklisted keys
   - `AllowItemKeys` - Whitelist-only keys (strict)
   - `DenyTags` - Reject items with forbidden tags
   - `AllowTags` - Require at least one allowed tag

### PlacementService

Grid collision detection and free space finding:

```go
ps := NewPlacementService()

// Check if item fits at position
result := ps.CheckGridPlacement(container, item, x, y, allowSwap)

// Find free space for item
found, x, y := ps.FindFreeSpace(container, itemW, itemH)
```

Collision rules:
- Items cannot overlap (2D AABB collision)
- Items must fit entirely within container bounds
- Optional swap support for hand↔grid exchanges

## Key Types

### InventoryRef

Unique container identifier:

```go
type InventoryRef struct {
    Kind    InventoryKind  // Grid, Hand, Equipment, DroppedItem
    OwnerID types.EntityID // Player ID or Item ID (for nested)
    Key     uint32         // Container index (0 for default)
}
```

### InvItem

Item instance in a container:

```go
type InvItem struct {
    ItemID   types.EntityID // Unique instance ID
    TypeID   uint32         // ItemDef reference
    Resource string         // Override resource path
    Quality  uint32
    Quantity uint32         // Stack quantity
    W, H     uint8          // Size in grid cells
    X, Y     uint8          // Grid position (top-left)
}
```

### OperationResult

Result of inventory operation:

```go
type OperationResult struct {
    Success           bool
    ErrorCode         ErrorCode
    Message           string
    UpdatedContainers []types.Handle // ECS handles of modified containers
}
```

## Content Rules (Nested Containers)

Containers can restrict what items they accept via `ContentRules`:

```go
type ContentRules struct {
    AllowTags     []string // "seed", "ore", "weapon"
    DenyTags      []string // "contraband"
    AllowItemKeys []string // "wheat_seed", "corn_seed"
    DenyItemKeys  []string // "cursed_item"
}
```

**Example:** A seed bag that only accepts items tagged `"seed"`:

```json
{
  "key": "seed_bag",
  "container": {
    "size": {"w": 4, "h": 4},
    "rules": {
      "allowTags": ["seed"]
    }
  }
}
```

**Rule Evaluation:**
1. DenyItemKeys - immediate reject if key matches
2. AllowItemKeys - if non-empty, key must match whitelist
3. DenyTags - reject if any tag matches
4. AllowTags - if non-empty, require at least one match

## Data Flow

### Client → Server Operation

```
1. Client sends C2S_InventoryOp
   └── move: {src, dst, item_id, hand_pos}

2. Server validates
   └── ResolveContainer → ValidateItemAllowedInContainer

3. Server executes
   └── Remove from src → Add to dst → Update versions

4. Server responds
   └── S2C_InventoryOpResult: {success, updated[]}

5. Client applies updates
   └── Update inventory states from result.updated
```

### Hand Mouse Offset

When picking up items into hand, clients send click offset:

```protobuf
message InventoryMoveSpec {
  InventoryRef src = 1;
  InventoryRef dst = 2;
  uint64 item_id = 3;
  HandPos hand_pos = 4;  // Click offset within item
}
```

Server stores offset in `InventoryContainer.HandMouseOffsetX/Y` and returns it in `InventoryHandState.hand_pos` for proper client-side rendering.

### Database Persistence

**Loading:**
```
Database (JSON) → InventoryLoader → ECS Components
```

**Saving:**
```
ECS Components → InventorySaver → Database (JSON)
```

Storage format supports versioning for schema evolution:
```go
type InventoryDataV1 struct {
    Kind    uint8
    Key     uint32
    Version uint64  // Revision at save time
    Items   []InventoryItemV1
}
```

## Usage Examples

### Moving Item Grid → Hand

```go
moveSpec := &netproto.InventoryMoveSpec{
    Src: &netproto.InventoryRef{
        Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
        OwnerId: uint64(playerID),
    },
    Dst: &netproto.InventoryRef{
        Kind:    netproto.InventoryKind_INVENTORY_KIND_HAND,
        OwnerId: uint64(playerID),
    },
    ItemId: uint64(itemID),
    HandPos: &netproto.HandPos{
        MouseOffsetX: 15,
        MouseOffsetY: 15,
    },
}

result := service.ExecuteMove(world, playerID, playerHandle, opID, moveSpec, nil)
if !result.Success {
    // Handle error...
}
```

### Moving Item Hand → Nested Container

```go
// dst is a nested container (owner is the bag item, not player)
moveSpec := &netproto.InventoryMoveSpec{
    Src: &netproto.InventoryRef{
        Kind:    netproto.InventoryKind_INVENTORY_KIND_HAND,
        OwnerId: uint64(playerID),
    },
    Dst: &netproto.InventoryRef{
        Kind:    netproto.InventoryKind_INVENTORY_KIND_GRID,
        OwnerId: uint64(bagItemID),  // Nested container owner
    },
    ItemId: uint64(itemID),
    DstPos: &netproto.GridPos{X: 0, Y: 0},
}

result := service.ExecuteMove(world, playerID, playerHandle, opID, moveSpec, nil)
// Validates content rules automatically
```

### Validating Content Rules Manually

```go
validator := NewValidator()

// Find destination info
dstInfo, err := validator.ResolveContainer(world, dstRef, playerID, playerHandle)
if err != nil {
    return err
}

// Get item definition
itemDef, ok := itemdefs.Global().GetByID(int(item.TypeID))
if !ok {
    return fmt.Errorf("unknown item")
}

// Validate (includes content rules for nested containers)
if err := validator.ValidateItemAllowedInContainer(world, item, dstInfo, equipSlot); err != nil {
    return err
}
```

## Testing

### Test Setup

```go
func TestInventoryOperation(t *testing.T) {
    world := ecs.NewWorldForTesting()
    playerID := types.EntityID(1000)
    playerHandle := world.Spawn(playerID, nil)
    
    // Setup inventories
    gridHandle := createGridContainer(world, playerID, 0, 10, 10)
    handHandle := createHandContainer(world, playerID, 0)
    
    // Add InventoryOwner to player
    owner := components.InventoryOwner{
        Inventories: []components.InventoryLink{
            {Kind: constt.InventoryGrid, Key: 0, OwnerID: playerID, Handle: gridHandle},
            {Kind: constt.InventoryHand, Key: 0, OwnerID: playerID, Handle: handHandle},
        },
    }
    ecs.AddComponent(world, playerHandle, owner)
    
    // Populate ref index
    world.InventoryRefIndex().Add(constt.InventoryGrid, playerID, 0, gridHandle)
    world.InventoryRefIndex().Add(constt.InventoryHand, playerID, 0, handHandle)
    
    // Set test registry
    itemdefs.SetGlobalForTesting(createTestRegistry())
    
    // Run operation
    service := NewInventoryOperationService(zap.NewNop())
    result := service.ExecuteMove(world, playerID, playerHandle, 1, moveSpec, nil)
    
    // Assert...
}
```

### Content Rules Testing

See `content_rules_test.go` for comprehensive examples:
- `TestContentRules_AllowTags_*` - Tag-based allowlist
- `TestContentRules_DenyTags_*` - Tag-based denylist
- `TestContentRules_AllowItemKeys_*` - Key-based whitelist
- `TestContentRules_DenyItemKeys_*` - Key-based blacklist

## Performance Characteristics

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Container resolve | O(1) | Via `InventoryRefIndex` |
| Item lookup | O(n) | Linear scan (small n, typically <100) |
| Placement check | O(n) | Collision detection against all items |
| Content rules | O(tags × rules) | Typically <10 iterations |
| Snapshot build | O(containers) | One pass per container |

**Optimizations:**
- `InventoryRefIndex` provides O(1) container lookup
- `PreparedQuery` for ECS iteration (when used with executor)
- Revision-based caching for snapshot delta calculation

## Integration Points

### ECS Integration

`InventoryExecutor` bridges `InventoryOperationService` and ECS:

```go
type InventoryExecutor struct {
    service   *InventoryOperationService
    validator *Validator
}

func (e *InventoryExecutor) ExecuteOperation(...) *OperationResult {
    // Calls service.ExecuteMove
    // Sends results via InventoryResultSender interface
}
```

### Network Integration

`NetworkCommandSystem` (in `internal/ecs/systems`) routes inventory commands:

```go
case network.CmdInventoryOp:
    s.handleInventoryOp(w, handle, cmd)
```

### Database Integration

`InventoryLoader` and `InventorySaver` handle persistence:

```go
loader := NewInventoryLoader(db, logger)
loader.LoadPlayerInventories(world, playerID, playerHandle, inventoryData)

saver := NewInventorySaver(db, logger)
saver.SavePlayerInventories(ctx, world, playerHandle)
```

## Error Handling

Common error codes returned in `OperationResult`:

| ErrorCode | Cause |
|-----------|-------|
| `ERROR_CODE_ENTITY_NOT_FOUND` | Container or item doesn't exist |
| `ERROR_CODE_INVALID_REQUEST` | Version mismatch, invalid placement, content rules violation |
| `ERROR_CODE_SERVER_ERROR` | Unexpected internal error |

All validation errors include descriptive messages for client display.

## Future Extensions

Potential enhancements:
- **Crafting integration** - Consume items from inventory for recipes
- **Trading system** - Atomic exchange between players
- **Loot tables** - Weighted random item generation
- **Durability/Wear** - Item degradation over time
- **Container permissions** - Access control for shared containers
