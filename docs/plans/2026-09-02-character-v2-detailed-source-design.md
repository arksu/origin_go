# Character v2: detailed stylised male source model

## Decision

Replace the v1 primitive blockout as the visual source with an original,
smooth, stylised, heroic male character. The supplied pixel turnaround and the
generated v2 turnaround are visual references only. The new mesh, texture,
face, hair, and clothing must remain original project assets.

The character remains intentionally bare except for hair and a linen
loincloth. Equipment is out of scope for this model, but its body, hair, and
loincloth remain separate collections so the bake pipeline can add it later.

## Modelling approach

Create the model directly in Blender through a versioned Python generator.
The body is a smooth mid-poly mesh built from joined anatomical surface forms,
not voxels and not an imported/generated 3D mesh. It prioritises silhouette,
muscle landmarks, and deformation around shoulders, elbows, hips, knees, and
hands over micro-detail invisible in the final isometric bake.

The target body has exaggerated but internally coherent heroic proportions:
wide shoulders, large deltoids and chest, clear abdominal planes, powerful
thighs and calves, and a compact stylised face. Hair is a layered sculpted mesh
and the loincloth is a separate draped mesh with low-frequency folds.

## Rig and actions

`male_commoner_v2_rig` contains a stable root; pelvis/spine/neck/head chain;
clavicle, arm, forearm, and hand chains; and thigh, shin, and foot chains.
The body uses an Armature modifier and explicit vertex groups rather than
object-to-bone parenting. This is required for smooth deformation.

Two in-place loop actions are provided:

- `idle`: 24 frames at 12 FPS, subtle breathing and posture shift.
- `walk`: eight render frames at 12 FPS, with an additional matching endpoint
  in Blender to close the loop.

The simulation continues to own movement; root motion is not exported.

## Render and validation

Five fixed orthographic isometric cameras (`ne`, `e`, `se`, `s`, `sw`) and an
additional neutral review camera provide consistent bakes and inspection.
The generator renders a neutral `idle` review image and a mid-stride `walk`
review image.

Generation fails if a required collection, armature, action, vertex group, or
bake camera is missing. The generated `.blend`, generator, reference image,
and previews live under `art_source/characters/male_commoner_v2/`.

## Scope boundary

This is a fully editable production-oriented base character, not a promise of
film-quality sculpting. The first pass validates anatomy, topology strategy,
rigging, and the 2D bake silhouette. After review, facial detail, muscle
definition, and garment folds are refined before adding equipment, female
bodies, animals, or additional actions.
