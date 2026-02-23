# Items Catalog (`data/items`)

Items define inventory content: resources, tools, food, containers, and stack rules.

Files in this folder are loaded by `internal/itemdefs`.

## JSONC File Shape

```json
{
  "v": 1,
  "source": "tools",
  "items": [
    {
      "defId": 1002,
      "key": "stone_axe",
      "name": "Stone Axe",
      "resource": "items/stone_axe.png",
      "tags": ["axe"],
      "size": { "w": 1, "h": 1 }
    }
  ]
}
```

## Required Fields Per Item

- `defId` (int, `> 0`)
- `key` (string, non-empty)
- `name` (string, non-empty)
- `size.w` (`>= 1`)
- `size.h` (`>= 1`)

Recommended:
- `tags` (array; use `[]` when no tags)

## Useful Defaults (Applied by Loader)

If omitted, loader fills:
- `allowed.hand = true`
- `allowed.grid = true`
- `allowed.equipmentSlots = []`
- `resource = key`
- `discoveryLP = 50`

## Optional Fields

- `stack`
  - `"mode": "none"` or `"stack"`
  - if `"stack"`, then `max >= 2`
- `allowed`
  - placement permissions in hand/grid/equipment slots
- `container`
  - nested inventory size and content rules
- `visual`
  - dynamic resource selection (currently used for nested container visuals)
- `discoveryLP`
  - override LP granted on discovery

## Container Items (Nested Inventory)

Use `container` to make an item hold other items:

```json
{
  "defId": 2000,
  "key": "seed_bag",
  "name": "Seed Bag",
  "tags": ["container"],
  "size": { "w": 1, "h": 2 },
  "container": {
    "size": { "w": 4, "h": 4 },
    "rules": {
      "allowTags": ["seed"]
    }
  }
}
```

Validation rules:
- `container.size.w >= 1`
- `container.size.h >= 1`

## Tagging Tips (Important)

Tags are used by:
- crafting recipes (`itemTag`)
- build recipes (`itemTag`)
- container content rules (`allowTags`, `denyTags`)

Be consistent with tag vocabulary (`ore`, `seed`, `axe`, etc.).

## Content Creator Checklist

- `defId` unique across all files in `data/items`
- `key` unique across all files in `data/items`
- `name` is readable for UI
- `tags` reflect recipe/build usage
- `resource` path exists (or intentionally omitted to default to `key`)
- no trailing commas / no unknown fields
