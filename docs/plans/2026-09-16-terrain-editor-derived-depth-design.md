# Terrain editor derived depth design

## Goal

Expose each terrain layer's explicit runtime `z` value for manual editing in
the terrain editor.

## Problem

Terrain layers store both their visual `offset` and their runtime `z` offset.
The editor must preserve that distinction: moving a texture must not silently
replace the artist-configured depth value.

## Decision

Treat terrain-layer `z` as explicit configuration. The editor shows the saved
value, previews layer order using it, and provides a numeric input for the
selected layer. Moving a layer changes only its visual offset.

The texture height comes from the loaded atlas frame. This preserves the
runtime's ground-contact sorting model while making the visual position the
only editable depth input.

## Editor behavior

- Show the saved depth value in the layer list and as a numeric input for the
  selected layer.
- Keep `z` unchanged when a root or layer offset changes.
- Save only the user-entered `z` value; a missing `z` is presented as `0`.

## Synchronization

The terrain editor remains the source config. Its existing export/sync flow
writes the manually edited `z` values to the game's terrain config. Runtime
code protects actor ordering at a shared world-depth anchor.

## Validation

- Test manually editing `z` updates only that layer and persists on save.
- Test moving a layer does not alter its saved `z`.
- Run the terrain-editor test suite and its production TypeScript build.
