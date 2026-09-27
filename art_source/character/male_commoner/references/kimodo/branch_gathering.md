# Kimodo branch gathering motion

`branch_gathering.npz` preserves the user-provided `output (1).npz`, supplied on
2026-09-27. SHA-256:
`9231df53868cf29279c95864ba4f8f03866ce6b26bbd92308ed95b1f8888b902`.

All **150 frames**, inclusive NPZ indices **0–149**, are retargeted onto the
existing commoner rig as `pick_up`, Blender frames **1–150**. No trimming,
endpoint closure or extra frames are applied. The source rate is **30 FPS**, the
[documented Kimodo NPZ rate](https://research.nvidia.com/labs/sil/projects/kimodo/docs/user_guide/motion_convert.html).
The archive has no FPS metadata; the import recipe explicitly records that rate.
The duration between first and last samples is **149/30 = 4.966667 seconds**.

The file has `posed_joints` and `global_rot_mats` in the **77-joint SOMA layout**,
using a standard T-pose, Y up and +Z forward. The recipe declares the complete
20-bone mapping and rotates the coordinates into Blender Z up / -Y forward.
Joint indices and neutral directions were read from NVIDIA's
[SOMA definition](https://github.com/nv-tlabs/kimodo/blob/58e781898b3d7e328a676a75d3e338c45dce3ad9/kimodo/skeleton/definitions.py)
and [standard T-pose](https://github.com/nv-tlabs/kimodo/blob/58e781898b3d7e328a676a75d3e338c45dce3ad9/kimodo/assets/skeletons/somaskel77/somaskel77_standard_tpose.bvh).
The importer verifies these directions against every supplied frame, aligns the
donor T-pose with the target A-pose, and bakes editable keys. Unmapped finger and
face joints have no counterparts in the game rig.

As for the previous clip, horizontal root travel is omitted and the supporting
foot stays grounded after retargeting. Joint rotations, including body turns,
come from the supplied motion. Meshes, rest pose, sockets and other actions stay
unchanged. The previous Mixamo source and recipe remain available under
`../mixamo/` as historical references.

Repeat the import and publish only the changed clip:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  art_source/character/male_commoner/source.blend --python-exit-code 1 \
  --python tools/blender/import_kimodo_clip.py -- \
  --recipe art_source/character/male_commoner/references/kimodo/branch_gathering.import.json
tools/assets build character/male_commoner --animations --clip pick_up
```

The `tree/take_branch` and `take/chip_stone` defs select the same `pick_up` clip.
The shared client
timeline fits the whole clip to the actual server action duration; the source
rate does not set gameplay duration. Both hand equipment models stay visually
unbound during playback and return afterwards. No runtime, server or protocol
change is needed. Reload clients after publication to read the new catalog.

Preview: `/tests/hybrid-character.html`, select `Сбор ветки` or
`Откалывание камня`.
Equipment checks: `/tests/equipment-unbind.html?binding=tree_take_branch` or
`/tests/equipment-unbind.html?binding=take_chip_stone`.
Adding a binding requires restarting the server with the updated defs.
Rendering checks: `/tests/hybrid-integration.html`.
