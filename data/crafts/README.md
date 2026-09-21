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
- `outputByInputKey` (optional source item key to output item key map; see below)
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

A station recipe uses the player's current linked object and optionally restricts
its definition with `requiredLinkedObjectKey`. It declares the current capability, state, scalar conditions, and explicit
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
- `stationRequirements` work without `requiredLinkedObjectKey`; the player must
  still be linked to a suitable station. When an exact key is provided, it must
  name an object with a `station` section and remains an additional restriction.
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
- explicit `requiredLinkedObjectKey` naming an object without station configuration
- unknown station resource in `consume`
- unsupported station condition operator
- `ticksRequired == 0`
- total `qualityWeight == 0`

## Mapped Outputs (`outputByInputKey`)

A supplied map requires exactly one input row using `itemTag` and `count: 1`.
Ordinary input validation and quality weights still apply. Each map source and
target must be a nonblank, existing item key, and each source must have the input
tag. Keys outside the input tag are rejected when definitions load.

For example, Roasted meat in `cooking.jsonc` uses:

```jsonc
"inputs": [{ "itemTag": "raw_meat", "count": 1, "qualityWeight": 1 }],
"outputs": [{ "itemKey": "roasted_meat", "count": 1 }],
"outputByInputKey": {
  "beef": "roasted_beef",
  "raw_pork": "roast_pork"
},
"stationRequirements": [{ "capability": "cooking", "state": "burning" }]
```

This abbreviated example shows two entries; the catalog recipe contains all nine
meat species. The standard input traversal chooses the first matching source:
root grids, nested grids, then hand, preserving inventory and item order within
each group. It does not skip an unmapped source to find a mapped one.

With a supplied map, `outputs` and their counts are **UI preview metadata only**.
Every successful cycle consumes one source unit and creates one mapped target
through the standard inventory mechanisms, with standard craft quality. It does
not change the source item's type in place. The source, map entry, target item,
and target placement are resolved before consumption. Placement is checked for
`roasted_beef` when the source is `beef`, and for `roast_pork` when it is
`raw_pork`, using available space before input removal.

Missing map coverage is allowed at load time, including an empty map. At runtime
it fails without consuming input, with the distinct message:
`Roast can't be processed: no info {source_item_key} in roast map`.
An empty map never falls back to preview outputs. Omit the field entirely for
ordinary fixed-output recipes.

The Roasted meat preview retains `items/raw_meat` and `items/roasted_meat` icons.
The recipe uses 10 ticks and 10 stamina per cycle and checks cooking/burning at
start and completion. An unlit final station cancels without input consumption;
a broken or switched link also prevents completion against the previous station.
