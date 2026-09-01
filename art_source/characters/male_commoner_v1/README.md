# Male commoner v1

Editable source character for the Blender-to-2D bake pipeline.

## Contents

- `male_commoner_v1.blend` — rigged low-poly source model.
- `previews/` — review renders from the fixed isometric cameras.
- `../../../tools/blender/generate_male_commoner_v1.py` — deterministic scene generator.

The model is a deliberately simple blockout: male body, chestnut hair, and a
base loincloth. The `BODY`, `HAIR`, and `BASE_LOINCLOTH` collections are kept
separate so later equipment can be baked as compatible visual layers.

## Actions

- `idle`: loop, 24 frames at 12 FPS.
- `walk`: loop, 8 baked frames at 12 FPS; Blender includes frame 9 only to
  close the interpolation loop.

The armature is `male_commoner_v1_rig`. It has a stable `root` bone and
separate left/right arm and leg chains. The action is in place; game movement
must continue to be controlled by the simulation rather than animation root
motion.

## Regenerate

From the repository root on macOS:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  --python tools/blender/generate_male_commoner_v1.py
```

The generator recreates the `.blend` and preview PNGs. Do not edit the
generated file and expect those edits to survive a regeneration; move approved
art-direction changes into the generator first.
