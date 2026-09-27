# Tree chopping motion

`standing_melee_attack_downward.fbx` is the user-provided Mixamo animation
**Standing Melee Attack Downward**, supplied on 2026-09-27. Original filename:
`Standing Melee Attack Downward.fbx`.

SHA-256: `38bb876ade8ed5ca096a13ab6aa3a605655dba02f9896977990649f446f51333`.
This reference retains its original bytes and provenance; it is not game geometry.

The motion is baked onto the existing 20-bone commoner skeleton as `chop_r`
and a reflected `chop_l`, 69 frames at 30 FPS. Retargeting accounts for the
donor's T-pose and the commoner's A-pose, keeps the supporting foot grounded,
and closes the last seven frames onto the first pose for repeated cycles.
The current hand sockets and approved axe grips remain authoritative.

To repeat the import into the canonical source:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  art_source/character/male_commoner/source.blend \
  --python tools/blender/import_tree_chop.py
tools/assets build character/male_commoner --animations
```

The import saves the source and replaces only `chop_l` and `chop_r`. Production
exports read the saved actions, not this FBX. Existing model, rig and animation
artifacts were checked to remain unchanged after publication.
