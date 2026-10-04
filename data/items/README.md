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
- `melee`
  - shared melee weapon parameters; `baseDamage` must be positive and finite
- `armor`
  - protection parameters; `baseArmor` must be positive and finite

## Combat Parameters

Melee damage uses the same formula for axes, swords, knives, and pikes. Item type
parameters belong to item definitions; action parameters belong to action
definitions:

```json
"melee": { "baseDamage": 6 },
"armor": { "baseArmor": 4 }
```

Both blocks are optional and independent. Omission or `null` means no corresponding
capability, and an item may provide both. A present block must provide a positive,
finite number; empty blocks, zero, negative values, and unknown fields are rejected.
The armor value above is illustrative, not a value assigned to existing clothing.

The shared calculation model in `internal/combat` uses:

```text
Draw = B × (STR / S0)^0.25 × (Quality / Q0)^0.25 × multiplier
ItemArmor = BaseArmor × sqrt(Quality / Q0)
```

`B` comes from `melee.baseDamage`; `BaseArmor` comes from `armor.baseArmor`.
The action's `combat.damageMultiplier` supplies `multiplier`; its equipment
requirements, geometry, duration, stamina cost, and cooldown remain in action
definitions. Quality belongs to the item instance, and effective STR comes from
the attacker. `S0 = 1` and `Q0 = 10` are shared formula normalizations, not per-item
parameters. Bow damage uses a separate model.

These definitions currently provide validated calculation inputs. Reading equipped
items and applying calculated damage to entities will be connected separately.

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
