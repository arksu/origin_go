# Male commoner v2

`male_commoner_v2.blend` is the editable, smooth, heroic male base character
for the Blender-to-2D bake pipeline. It supersedes the primitive `v1` blockout
as the intended art source; v1 remains useful only as a small pipeline test.

## Contents

- `male_commoner_v2.blend` — original Blender mesh, rig, actions, cameras, and lighting.
- `reference/male_commoner_v2_turnaround_reference.png` — original concept
  reference generated for this project. It guides proportions only; it is not
  geometry and is not copied verbatim.
- `previews/` — neutral review renders from the five bake cameras.
- `../../../tools/blender/generate_male_commoner_v2.py` — deterministic asset generator.

## Asset layout

- `BODY` — one remeshed smooth body with an Armature modifier and vertex weights.
- `HAIR`, `BASE_LOINCLOTH`, `FACE_DETAILS` — independent meshes, also weighted
  to the same rig, for future bake-layer rules.
- `RIG` — `male_commoner_v2_rig`.
- `CAMERAS` — five orthographic isometric cameras: `ne`, `e`, `se`, `s`, `sw`.

The rest pose is T-pose for equipment authoring. `idle` and `walk` are normal
in-place standing actions rather than T-pose renders.

## Actions

- `idle`: looped 24-frame breathing/posture cycle at 12 FPS.
- `walk`: looped eight-frame walk at 12 FPS; the ninth Blender keyframe closes
  the interpolation loop and is not a baked frame.

## Regenerate

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  --python tools/blender/generate_male_commoner_v2.py
```

The generator recreates the `.blend` and previews. Approved changes should be
made in the generator so they remain reproducible.
