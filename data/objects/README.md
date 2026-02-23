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
      "resource": "box/empty"
    }
  ]
}
```

## Required Fields Per Object

- `defId` (int, `> 0`)
- `key` (string, non-empty)
- `name` (string, non-empty)

## Common Optional Fields

- `static` (defaults to `true`)
- `hp`
- `resource`
- `appearance` (conditional visual variants)
- `components`
  - `collider`
  - `inventory`
- `behaviors` (server behavior config map)

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

## Cross-References

Objects are referenced by:
- `builds.objectKey`
- `crafts.requiredLinkedObjectKey`
- world spawning/admin tools/runtime systems

Changing an object key can break builds/crafts and code paths. Prefer adding new objects instead of renaming existing keys.

## Content Creator Checklist

- `defId` unique across all files in `data/objects`
- `key` unique across all files in `data/objects`
- `name` is present and user-friendly
- collider/inventory dimensions are positive when used
- behavior keys are valid (copy from known working examples)
- no trailing commas / no unknown fields

