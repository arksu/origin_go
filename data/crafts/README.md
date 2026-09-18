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
- `stationRequirements` (requirements for the linked station; v1 station-local only)
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

## Station Requirements

A station recipe selects an object with `requiredLinkedObjectKey` and then
declares the current capability, state, scalar conditions, and explicit
station-local consumptions needed per completed cycle.

```jsonc
"requiredLinkedObjectKey": "campfire",
"stationRequirements": [
  {
    "capability": "cooking",
    "state": "burning",
    "conditions": [
      {
        "source": "station",
        "kind": "value",
        "key": "temperature",
        "operator": "gte",
        "value": 600
      }
    ],
    "consume": [
      { "resourceKey": "thread", "amount": 1 }
    ]
  }
]
```

- Every requirement must contain at least one of `capability`, `state`,
  `conditions`, or `consume`.
- In v1, `stationRequirements` require `requiredLinkedObjectKey` to name an
  object with a `station` section.
- `capability` must be exposed by the linked station; `state` must equal its
  current state.
- A v1 condition is a station scalar comparison: `source: "station"`,
  `kind: "value"`, a value `key`, and `operator` `eq`, `gte`, or `lte`.
- Each `consume` entry names a resource declared by the linked station and has
  `amount > 0`. It is consumed only after finalization validation passes, in
  the same atomic cycle completion as craft inputs, stamina, and output.

Requirements are validated read-only before a cycle begins and again when it
finishes. Fuel needed merely to keep a campfire `burning` is autonomous: it is
not a craft consumption unless explicitly listed in `consume`.

Operator, terrain/tile, and nearby-object requirements are reserved for future
providers. Do not place those condition sources in production content yet.

## Common Validation Failures

- `inputs` empty / `outputs` empty
- both `itemKey` and `itemTag` set in one input
- unknown item key in input or output
- unknown `requiredLinkedObjectKey`
- station requirement without a linked station object
- unknown station resource in `consume`
- unsupported station condition operator
- `ticksRequired == 0`
- total `qualityWeight == 0`
