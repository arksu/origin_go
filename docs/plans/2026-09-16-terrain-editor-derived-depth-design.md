# Terrain editor derived depth design

## Goal

Keep each terrain layer's runtime `z` value synchronized with its visual
placement so a terrain sprite cannot be positioned correctly in the editor
but draw at an obsolete depth in the game.

## Problem

Terrain layers currently store both their visual `offset` and their runtime
`z` offset. The terrain editor changes only `offset` and previews layers by
their list index. It neither recalculates `z` nor renders against an actor.
Consequently a moved sprite can retain stale depth; `terrain/wald/image_8.png`
showed the entire grass sprite over an actor.

## Decision

Treat terrain-layer `z` as derived editor data. The editor will recalculate it
whenever a root or layer offset changes and write the calculated value when the
configuration is saved. Terrain decals never advance beyond their own tile's
ground plane, so a tall grass bitmap cannot draw over an actor occupying that
tile:

```
layerZ = min(0, visualBottomY - TILE_HEIGHT_HALF)
visualBottomY = -variant.offsetY + layer.offsetY + textureHeight
```

The texture height comes from the loaded atlas frame. This preserves the
runtime's ground-contact sorting model while making the visual position the
only editable depth input.

## Editor behavior

- Recalculate every layer after a root-offset change because the root offset
  changes every layer's visual position.
- Recalculate the affected layer after a layer-offset change or drag.
- Show a non-editable depth readout and a ground-contact reference in the
  preview so the calculated value is visible while positioning assets.
- On loading an existing configuration, calculate depth from the current
  offsets and texture dimensions; saving repairs any stale stored `z` values.
- If an atlas texture cannot be loaded, retain the existing `z`, show a clear
  warning, and do not silently replace it with a guessed value.

## Synchronization

The terrain editor remains the source config. Its existing export/sync flow
writes the same calculated `z` values to the game's terrain config. Runtime
code remains unchanged: it continues to combine tile depth with `layer.z`.

## Validation

- Unit-test the derived-depth formula for `image_8` and a negative-offset
  asset.
- Test root and layer offset edits update the intended `z` values.
- Test an unloaded texture leaves a stored value intact and reports the
  failure.
- Run the terrain-editor test suite and its production TypeScript build.
