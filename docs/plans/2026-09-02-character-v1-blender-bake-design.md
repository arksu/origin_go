# Character v1: Blender-to-2D bake

## Goal

Create a maintainable source asset for the first playable male commoner. The
game continues to render 2D assets in Pixi; Blender is an offline authoring
and baking tool, not a runtime dependency.

The first vertical slice contains a stylised low-poly male body, a loincloth,
simple hair, an armature, and `idle` and `walk` actions. It establishes a
repeatable process before clothes, weapons, women, and animals are added.

## Chosen approach

Use one rigged Blender character as the source of truth and bake it through
five unique orthographic isometric views. The remaining directions can be
mirrored only for visual assets explicitly marked symmetric. Every render uses
the same canvas dimensions, pixel scale, ground anchor, action timing, and
camera transform.

The exporter can render either a flattened character or named layer passes.
The production path is named passes so the Pixi client can compose compatible
body and equipment layers without creating an atlas for every outfit
combination.

## Asset contract

The Blender scene contains these collections:

- `BODY`: skin mesh split only where draw ordering needs it.
- `HAIR`: removable hair mesh.
- `BASE_LOINCLOTH`: removable base clothing.
- `RIG`: the armature and animation actions.
- `CAMERAS`: five fixed orthographic isometric cameras.
- `LIGHTING`: locked baked-light setup and transparent render world.

The initial armature has root, pelvis, torso, head, upper/lower arm, hand,
upper/lower leg, and foot bones. It deliberately omits facial and cloth
simulation bones. The initial clips are loopable `idle` and `walk`, authored
at 12 FPS with 8 walk frames and a shared root/ground anchor.

An item later declares its visual slot, compatible body archetypes, attachment
bone, layer/order rule, and whether it hides a conflicting layer (for example,
a helmet hiding hair).

## Bake output

Each bake job produces:

```text
assets/generated/characters/male_commoner_v1/
  manifest.json
  body/{idle,walk}/{ne,e,se,s,sw}/atlas.png
  hair/{idle,walk}/{ne,e,se,s,sw}/atlas.png
  base_loincloth/{idle,walk}/{ne,e,se,s,sw}/atlas.png
```

`manifest.json` records canvas size, frames per second, frame count, ground
anchor, directions, layer order, and source model version. An atlas and its
metadata always contain matching frame counts so runtime composition cannot
drift.

## Automation

A versioned Blender Python script creates the starter scene, constructs the
low-poly meshes, adds the armature and weights, creates the two actions, and
sets up cameras/materials. A second script validates and exports frames into
the output contract. Both run headlessly with Blender so recurring bakes do
not require manual UI steps.

Generation fails when a required collection/action/camera is absent, an
animation has inconsistent timing, or the root anchor changes between frames.

## Validation

1. Open the generated `.blend` and inspect the mesh, named collections,
   armature, and action list.
2. Render a contact sheet of five directions for `idle` and `walk`.
3. Confirm the feet share the same baseline and the walk loop has no jump.
4. Load the baked manifest and matching atlas layers in an isolated Pixi test
   before connecting it to inventory/equipment networking.

## Out of scope

Equipment, weapons, female body, animals, combat/work actions, automatic
retopology, and a runtime 3D renderer are intentionally outside this first
asset. They will reuse this contract after the visual direction is approved.
