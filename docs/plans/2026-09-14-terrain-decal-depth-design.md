# Terrain decal depth design

## Goal

Render generated terrain decals behind or in front of actors according to each
decal's ground-contact point, including deterministic placement jitter.

## Current issue

Terrain sprites receive a visual Y position that includes `jitterY` and the
configured layer Y offset. Their z-index is instead calculated from the source
tile's fixed lower edge. Consequently the visual location and the sorting
location can differ, allowing a decal to render over an actor incorrectly.

## Decision

Each terrain draw command will carry an explicit `depthY`. The generator will
derive it from the same jittered tile anchor used for placement. That anchor
is the tile centre and matches the actor's world-position depth. The renderer
will derive z-index from this value and force terrain decals below an actor at
the same depth, even for legacy configs with a positive per-layer `z` offset.

`depthY` deliberately does not use the sprite's top-left Y or bitmap height:
large sprites may extend above their foot point, and their visual bounds are
not a reliable depth anchor. Layer-specific depth adjustments remain possible
through the existing `z` field.

## Data flow

```
tile anchor + jitterY -> TerrainDrawCmd.depthY -> sprite.zIndex
tile anchor + jitterY + visual offsets -> sprite.y
```

The visual and depth positions now share the random placement displacement,
while their distinct purposes remain explicit.

## Validation

Add focused generator coverage for deterministic jittered depth values and
renderer coverage that verifies `depthY` is used instead of the unjittered
context anchor. Run the relevant web-client tests and TypeScript build.
