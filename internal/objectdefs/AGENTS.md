# Object Definitions Package

## Overview

The `objectdefs` package provides a data-driven definition system for game objects, analogous to the `itemdefs` package for items. It loads object definitions from JSONC files (JSON with comments) and provides a registry for runtime lookup by `defId` or `key`.

## Architecture

```
objectdefs/
├── types.go      # Definition structs (ObjectDef, Components, etc.)
├── loader.go     # JSONC loading and validation
├── registry.go   # Thread-safe in-memory registry
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
    DefID                     int
    Key                       string
    Static                    *bool // default true -> resolved into IsStatic
    ContextMenuEvenForOneItem *bool // default true -> resolved runtime value
    HP                        int
    Components                *Components
    Resource                  string
    Appearance                []Appearance
    Behaviors                 map[string]json.RawMessage
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
7. **Behaviors**: Keys must exist in unified runtime behavior registry (`contracts.BehaviorRegistry` from `internal/game/behaviors/contracts`); each behavior config is validated strictly
8. **ContextMenuEvenForOneItem**: Optional; defaults to `true`

## Usage

### Loading Definitions

```go
behaviorRegistry, err := behaviors.DefaultRegistry()
if err != nil {
    log.Fatal(err)
}
registry, err := objectdefs.LoadFromDirectory("./data/objects", behaviorRegistry, logger)
if err != nil {
    log.Fatal(err)
}
objectdefs.SetGlobal(registry)
```

### Accessing Definitions

```go
// By defId
def, ok := registry.GetByID(1)

// By key
def, ok := registry.GetByKey("tree")

// Global access (convenience)
def, ok := objectdefs.Global().GetByID(1)
```

### Creating Entities from Definitions

```go
factory := world.NewObjectFactory(registry)
handle := factory.Build(world, rawObject)  // rawObject from DB
```

## File Format (JSONC)

```jsonc
{
  "v": 1,
  "objects": [
    {
      "defId": 1,
      "key": "tree",
      "static": true,
      "contextMenuEvenForOneItem": true,
      "hp": 100,
      "components": {
        "collider": { "w": 10, "h": 10, "layer": 1, "mask": 1 }
      },
      "resource": "trees/birch/6",
      "behaviors": {
        "tree": {
          "priority": 20,
          "chopPointsTotal": 6,
          "chopCycleDurationTicks": 20,
          "logsSpawnDefKey": "log_y",
          "logsSpawnCount": 3,
          "logsSpawnInitialOffset": 12,
          "logsSpawnStepOffset": 10,
          "transformToDefKey": "stump_birch"
        }
      }
    }
  ]
}
```

## Built-in Behaviors

The unified runtime registry (`internal/game/behaviors.DefaultRegistry()`) registers:
- `tree` — Tree behavior (chopping, wood resource)
- `container` — Container behavior (storage)
- `player` — Player behavior (character control)

## Integration Points

| Package | Integration |
|---------|-------------|
| `game/world` | `ObjectFactory` uses registry to build entities |
| `ecs/components` | `EntityInfo` stores `TypeID` (defId) and `Behaviors` |
| `persistence` | DB stores `type_id` referencing defId |
| `game/events` | Network spawn uses `EntityInfo.TypeID` → proto `type_id` |

## Behavior Config Contract

- Canonical behavior config location is `ObjectDef.Behaviors` (`map[string]json.RawMessage`), keyed by string behavior key (e.g. `"tree"`).
- Behavior config payloads are validated by runtime behavior implementations (`ValidateAndApplyDefConfig`), not by ad-hoc loader code.
- Keep object def focused on numeric/static tuning values; behavior algorithms stay in code.
- `BehaviorOrder` is derived from validated behavior priorities; use `CopyBehaviorOrder()` where a defensive copy is needed.
- `contextMenuEvenForOneItem` defaults to `true` when omitted.

## Tree Behavior Def Fields

Tree config is numeric-only and includes:
- chop loop: `chopPointsTotal`, `chopCycleDurationTicks`
- sound keys: `action_sound`, `finish_sound`
- chop outcome: `logsSpawnDefKey`, `logsSpawnCount`, `logsSpawnInitialOffset`, `logsSpawnStepOffset`, `transformToDefKey`
- growth runtime: `growthStageMax`, `growthStartStage`, `growthStageDurationsTicks`, `allowedChopStages`

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
