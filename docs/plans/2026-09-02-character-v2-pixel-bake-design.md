# Character v2: deterministic pixel-art bake

## Goal

Convert the rigged Blender source character into stable, transparent pixel-art
frames for Pixi. The output must preserve a shared grid, ground anchor,
palette, frame count, and draw order across every direction, action, and
equipment layer.

## Chosen pipeline

1. Blender renders transparent, flat-lit source PNGs at 384x576 (four times
   the logical resolution) from the five fixed orthographic cameras.
2. A dedicated pixel baker downsamples every raw render to 96x144, applies one
   project-owned palette, thresholds alpha, adds a one-pixel silhouette
   outline, and produces a nearest-neighbour 4x review copy.
3. The baker writes a manifest containing the fixed grid, display scale,
   palette, actions, directions, frame counts, and layer order.

The baker does not call the existing image-pixel-processor watcher. That tool
removes backgrounds and detects arbitrary sprite grids, both of which are
actively harmful when a rigged animation needs identical bounds and anchors.

## Render passes

For every frame, Blender emits a composited reference render and independent
passes for `BODY`, `HAIR`, `BASE_LOINCLOTH`, and `FACE_DETAILS`. The source
layers use the same camera and timeline position. Pixi can therefore stack
them without phase or position drift.

`--preview` renders only idle frame 1 and walk frame 3 in all five directions.
A normal run renders all 24 idle and eight walk frames.

## Output contract

```text
art_source/characters/male_commoner_v2/bake/
  raw/{pass}/{action}/{direction}/frame_000.png
  pixel/{pass}/{action}/{direction}/frame_000.png
  preview_4x/{pass}/{action}/{direction}/frame_000.png
  manifest.json
```

Generated bake output is ignored by Git. A later promotion step will pack
approved pixel frames into game atlases under `web_new/public/assets`; this is
not part of the current source-art task.

## Palette and outline

The committed palette is shared by every pass and frame. It contains skin,
hair, linen, leather, highlight, and outline colours. Dithering is disabled in
the first version because deterministic solid clusters are easier to inspect
and prevent animation shimmer. The outline is formed by one deterministic
4-connected alpha dilation on the 96x144 grid.

## Validation

The bake renderer fails on missing collections, cameras, actions, or bad frame
counts. The pixel baker fails on wrong raw dimensions, missing alpha, or an
empty result. The manifest is written only after every requested pixel file
has been generated.
