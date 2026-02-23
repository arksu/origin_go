# Crafts Catalog (`data/crafts`)

Craft recipes convert item inputs into item outputs.

Files in this folder are loaded by `internal/craftdefs`.

## JSONC File Shape

```json
{
  "v": 1,
  "source": "starter crafts",
  "crafts": [
    {
      "defId": 1,
      "key": "stone_axe",
      "name": "Stone Axe",
      "inputs": [
        { "itemKey": "branch", "count": 1, "qualityWeight": 1 },
        { "itemKey": "stone", "count": 1, "qualityWeight": 1 }
      ],
      "outputs": [
        { "itemKey": "stone_axe", "count": 1 }
      ],
      "staminaCost": 100,
      "ticksRequired": 10
    }
  ]
}
```

## Required Fields Per Craft

- `defId` (int, `> 0`)
- `key` (string, non-empty)
- `inputs` (non-empty array)
- `outputs` (non-empty array)
- `staminaCost` (`>= 0`)
- `ticksRequired` (`> 0`)

`name` is optional in practice:
- if blank/missing, loader defaults it to `key`

## Input Rules (`inputs[]`)

Each input must:
- define exactly one of:
  - `itemKey`
  - `itemTag`
- have `count > 0`

Validation:
- `itemKey` must exist in `data/items`
- total sum of all `qualityWeight` values must be `> 0`

`itemTag` references tags from `data/items[].tags`.

## Output Rules (`outputs[]`)

Each output must:
- define `itemKey` (required)
- have `count > 0`

Validation:
- `outputs[].itemKey` must exist in `data/items`

## Optional Requirement Fields

- `requiredSkills` (`[]string`)
- `requiredDiscovery` (`[]string`)
- `requiredLinkedObjectKey` (object key from `data/objects`)
- `qualityFormula` (defaults to `"weighted_avg_floor"`)

Loader normalizes `requiredSkills` / `requiredDiscovery`:
- trims values
- removes empty entries
- removes duplicates
- sorts values

## Content Creator Tips

- Prefer `itemKey` for exact recipes
- Use `itemTag` only when intentionally allowing substitutions
- Keep recipe names player-facing and readable
- If craft needs a station/tool object, use `requiredLinkedObjectKey`

## Common Validation Failures

- `inputs` empty / `outputs` empty
- both `itemKey` and `itemTag` set in one input
- unknown item key in input or output
- unknown `requiredLinkedObjectKey`
- `ticksRequired == 0`
- total `qualityWeight == 0`

