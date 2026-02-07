# Object Definitions Package

## Overview

The `objectdefs` package provides a data-driven definition system for game objects, analogous to the `itemdefs` package for items. It loads object definitions from JSONC files (JSON with comments) and provides a registry for runtime lookup by `defId` or `key`.

## Architecture

```
objectdefs/
├── types.go      # Definition structs (ObjectDef, Components, etc.)
├── loader.go     # JSONC loading and validation
├── registry.go   # Thread-safe in-memory registry
├── behavior.go   # BehaviorRegistry for validating behavior keys
└── AGENTS.md     # This file
```

## Data Flow

```
JSONC Files (data/objects/*.jsonc)
    ↓
LoadFromDirectory()
    ├─ stripJSONCComments()  # Remove // and /* */ comments
    ├─ json.Decoder with DisallowUnknownFields
    ├─ applyDefaults()       # Set defaults for optional fields
    └─ validateObject()      # Validate constraints
        ↓
Registry (indexed by defId and key)
    ↓
ObjectFactory.Build()   # Create entities from definitions
```

## Key Types

### ObjectDef

```go
type ObjectDef struct {
    DefID      int          // Unique numeric ID (1, 10, 11, ...)
    Key        string       // Unique string key ("tree", "box", "player")
    Static     bool         // Is this object static (immovable)?
    HP         int          // Hit points (0 if not damageable)
    Components Components   // Component definitions
    Resource   string       // Default resource path
    Appearance []Appearance // Appearance variations
    Behavior   []string     // Behavior keys ("tree", "container", "player")
}
```

### Components

| Component | Purpose |
|-----------|---------|
| `Collider` | Collision bounds (width, height, layer, mask) |
| `Inventory` | Container inventory (width, height, owner bool) |

## Validation Rules

1. **DefID**: Must be > 0, unique across all files
2. **Key**: Non-empty, unique across all files
3. **Collider**: Width and height must be > 0 (if present)
4. **Inventory**: Width and height must be > 0 (if present)
5. **Resource**: Required if no appearance defined
6. **Appearance IDs**: Must be unique within the def (if present)
7. **Behaviors**: No duplicates, must exist in BehaviorRegistry

## Usage

### Loading Definitions

```go
behaviors := objectdefs.DefaultBehaviorRegistry()
registry, err := objectdefs.LoadFromDirectory("./data/objects", behaviors, logger)
if err != nil {
    log.Fatal(err)
}
objectdefs.SetGlobal(registry)
```

### Accessing Definitions

```go
// By defId
def := registry.Get(1)

// By key
def, ok := registry.GetByKey("tree")

// Global access (convenience)
def := objectdefs.G().Get(1)
```

### Creating Entities from Definitions

```go
factory := world.NewObjectFactory(registry)
handle := factory.Build(world, rawObject)  // rawObject from DB
```

## File Format (JSONC)

```jsonc
{
  "version": 1,
  "objects": [
    {
      "defId": 1,
      "key": "tree",
      "static": true,  // default: true
      "hp": 100,
      "components": {
        "collider": { "width": 10, "height": 10, "layer": 1, "mask": 1 }
      },
      "resource": "tree",
      "behavior": ["tree"]
    }
  ]
}
```

## Default Behaviors

The `DefaultBehaviorRegistry()` registers:
- `tree` — Tree behavior (chopping, wood resource)
- `container` — Container behavior (storage)
- `player` — Player behavior (character control)

Register additional behaviors:

```go
behaviors := objectdefs.DefaultBehaviorRegistry()
behaviors.Register("custom", func() { ... })
```

## Integration Points

| Package | Integration |
|---------|-------------|
| `game/world` | `ObjectFactory` uses registry to build entities |
| `ecs/components` | `EntityInfo` stores `TypeID` (defId) and `Behaviors` |
| `persistence` | DB stores `type_id` referencing defId |
| `game/events` | Network spawn uses `EntityInfo.TypeID` → proto `type_id` |

## Error Handling

`LoadFromDirectory` returns detailed errors with file context:

```
object validation failed: tree (defId=1): collider width must be positive
```

## Future Extensions

Potential additions:
- Hot-reloading of definitions in development
- Validation for component dependencies
- Support for scripted behaviors (Lua/JS)
