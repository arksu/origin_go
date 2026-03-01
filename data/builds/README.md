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

- `1` = `TileDeepWater`
- `3` = `TileShallowWater`
- `5` = `TileBrickRed`
- `6` = `TileBrickYellow`
- `7` = `TileBrickBlack`
- `8` = `TileBrickBlue`
- `9` = `TileBrickWhite`
- `12` = `TileStonePaving`
- `14` = `TilePlowed`
- `20` = `TileConiferousForest`
- `25` = `TileBroadleafForest`
- `30` = `TileThicket`
- `35` = `TileGrass`
- `40` = `TileHeath`
- `45` = `TileMoor`
- `50` = `TileSwamp1`
- `53` = `TileSwamp2`
- `56` = `TileSwamp3`
- `60` = `TileDirt`
- `64` = `TileClay`
- `68` = `TileSand`
- `80` = `TileHouse`
- `90` = `TileHouseCellar`
- `100` = `TileMineEntry`
- `105` = `TileMine`
- `110` = `TileCave`
- `120` = `TileMountain`
- `255` = `TileVoid`

### Examples

Allow only land-like tiles:

```json
"allowedTiles": [12, 14, 35, 60]
```

Disallow swamp/water:

```json
"disallowedTiles": [1, 3, 50, 53, 56]
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
