# Builds Catalog (`data/builds`)

Build recipes consume item inputs and create a world object (`objectKey`).

Files in this folder are loaded by `internal/builddefs`.

## JSONC File Shape

```json
{
  "v": 1,
  "source": "basic",
  "builds": [
    {
      "defId": 10001,
      "key": "box",
      "name": "Box",
      "inputs": [
        { "itemKey": "branch", "count": 5, "qualityWeight": 1 }
      ],
      "staminaCost": 8,
      "ticksRequired": 40,
      "requiredSkills": [],
      "requiredDiscovery": [],
      "allowedTiles": [],
      "objectKey": "box"
    }
  ]
}
```

## Required Fields Per Build

- `defId` (int, `> 0`)
- `key` (string, non-empty)
- `inputs` (non-empty array)
- `staminaCost` (`>= 0`)
- `ticksRequired` (`> 0`)
- `objectKey` (must exist in `data/objects`)

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

## Requirements

Optional fields:
- `requiredSkills` (`[]string`)
- `requiredDiscovery` (`[]string`)

Loader normalization:
- trims values
- removes duplicates/empties
- sorts values

## Placement Rules (`allowedTiles` / `disallowedTiles`)

You may use:
- `allowedTiles`
- or `disallowedTiles`
- or neither

But not both with values at the same time.

Validation rules:
- lists are deduplicated and sorted by loader
- only known tile IDs are allowed (from `internal/types/tile.go`)

### Known Tile IDs

- `1` = `TileWaterDeep`
- `3` = `TileWater`
- `10` = `TileStone`
- `11` = `TilePlowed`
- `13` = `TileForestPine`
- `15` = `TileForestLeaf`
- `17` = `TileGrass`
- `23` = `TileSwamp`
- `29` = `TileClay`
- `30` = `TileDirt`
- `32` = `TileSand`
- `42` = `TileCave`

### Examples

Allow only land-like tiles:

```json
"allowedTiles": [10, 11, 17, 30]
```

Disallow swamp/water:

```json
"disallowedTiles": [1, 3, 23]
```

## `objectKey` (Build Result)

`objectKey` is the world object key created by the build action.

It must match an object in `data/objects`, for example:
- `"box"`
- `"crate"`

## Common Validation Failures

- unknown `objectKey`
- both `allowedTiles` and `disallowedTiles` populated
- tile ID not in the known tile list
- input uses both `itemKey` and `itemTag`
- input references unknown item key
- total `qualityWeight == 0`

