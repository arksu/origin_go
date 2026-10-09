# Objects Catalog (`data/objects`)

Objects define world entities spawned in the game world (containers, trees, props, player object, etc.).

Files in this folder are loaded by `internal/objectdefs`.

## JSONC File Shape

```json
{
  "v": 1,
  "source": "containers",
  "objects": [
    {
      "defId": 10,
      "key": "box",
      "name": "Box",
      "hp": 100,
      "resource": "box/empty"
    }
  ]
}
```

## Required Fields Per Object

- `defId` (int, `> 0`)
- `key` (string, non-empty)
- `name` (string, non-empty)
- `hp` (positive integer for every definition except `player`, including indestructible objects)

## Common Optional Fields

- `static` (defaults to `true`)
- `indestructible` (boolean, defaults to `false`; unsupported for `player`)
- `resource`
- `appearance` (conditional visual variants)
- `station` (autonomous station configuration)
- `components`
  - `collider`
  - `inventory`
- `behaviors` (server behavior config map)

`indestructible: true` prevents object damage and excludes the object from melee
damage targets. It does not change collision, lifting, inventory interaction,
behavior-driven transforms or removal, or administrator `/destroy`. The current
catalog enables it only for `player_dead` (`Dead Player`), which retains its initial `hp: 100`.
The corpse definition keeps `defId: 15`; existing saved corpses load through this
same ID without a database migration.

## Components Rules

### Collider

If `components.collider` is present:
- `w > 0`
- `h > 0`

Loader defaults:
- `layer = 1` if omitted/zero
- `mask = 1` if omitted/zero

### Inventory

If `components.inventory[]` is present:
- each entry must have `w > 0`, `h > 0`

Loader default:
- `kind = "grid"` if omitted

## Station

`station` configures an object whose mutable state exists independently of any
craft operation. It is appropriate for a campfire burning fuel, a furnace
heating up, or a charged device discharging over time.

```jsonc
"station": {
  "capabilities": ["cooking"],
  "states": ["unlit", "burning"],
  "initialState": "unlit",
  "values": { "temperature": 20 },
  "resources": [
    { "key": "fuel", "amount": 10 },
    { "key": "thread", "amount": 3 }
  ],
  "autonomousConsumption": [
    {
      "resourceKey": "fuel",
      "amountPerTick": 1,
      "requiredState": "burning",
      "stateWhenDepleted": "unlit"
    }
  ]
}
```

- `capabilities` and `states` are non-empty, unique string lists.
- `initialState` must be one of `states`.
- `values` is an optional map of named scalar values.
- Each `resources` entry has a unique non-empty `key` and `amount > 0`.
- Each `autonomousConsumption` rule references a declared resource and listed
  states; `amountPerTick > 0`.

The server persists the station's current state, values, and remaining
resources. Capabilities and autonomous rules stay in the object definition.
Autonomous consumption is owned by the station runtime, not by crafting.

## Behaviors (Advanced / Copy Existing Examples)

`behaviors` is a map of behavior key -> config object.

Important:
- Behavior keys must exist in the server behavior registry
- Unknown behavior keys fail validation
- Behavior-specific config is validated by the behavior implementation

For new content creators:
- Copy an existing object with a similar behavior
- Change values incrementally
- Validate by running the server and fixing the first error reported

Examples in this folder:
- `containers.jsonc` for `container`
- `trees.jsonc` for `tree` / `take` patterns
- `drying.jsonc` for `container` + `drying` (currently no live processes)

### Drying

`drying` requires `container` and exactly one 2×2 root grid (key 0). Its
`processes` list may be empty. Each process uses `inputItemKey`, `outputItemKey`,
and positive integer `durationSeconds`. Both items must exist and be
nonstackable grid items without nested inventories; the output dimensions must
fit inside the input rectangle. Repeated input keys are rejected. Only declared
inputs and outputs may enter the frame, one item at a time.

Drying uses fixed recipe duration and preserves input quality. Object quality
does not affect either. See `docs/features/drying_frame.md` for timer, scheduler
and persistence semantics.

## Cross-References

Objects are referenced by:
- `builds.objectKey`
- `crafts.requiredLinkedObjectKey`
- `crafts.stationRequirements`
- world spawning/admin tools/runtime systems

Changing an object key can break builds/crafts and code paths. Prefer adding new objects instead of renaming existing keys.

## Content Creator Checklist

- `defId` unique across all files in `data/objects`
- `key` unique across all files in `data/objects`
- `name` is present and user-friendly
- `hp` is a positive integer for every definition except `player`
- collider/inventory dimensions are positive when used
- behavior keys are valid (copy from known working examples)
- no trailing commas / no unknown fields
