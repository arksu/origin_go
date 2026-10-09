# Mixamo motion references

## Falling and lying, 2026-10-09

`sword_and_shield_death.fbx` retains the user's original **Sword And Shield Death.fbx**
motion reference, supplied on 2026-10-09. Its SHA-256 is
`8d5a2e3784d0753e9e590948e13dcc74569ddbc0ef4d1d88cbd92c356d8e789a`.
It supplies motion only; the existing commoner model, 20-bone rig, sockets and
all 15 previous actions remain unchanged.

`fall_down` retargets all 118 source frames at 30 FPS, for a duration of 3.9 seconds.
It does not loop or close its terminal frame onto its first. The final pose is the
shared resting frame for lying characters and corpses. The importer corrects the
donor T-pose to the commoner's A-pose and scales pelvis travel by the target/donor
leg-length ratio (`1.0199211374524602`). The first supporting body geometry defines
one fixed vertical anchor. Later samples retain local horizontal travel and vertical
motion, with only upward penetration corrections against both evaluated body LODs.
This model-local movement never changes the server entity position.

Body samples use linear interpolation and a 5 mm floor clearance. The saved-source
regression samples every half frame, verifies the initial/final floor contact and
holds the exact final bone matrices beyond the action range. Its authoring evidence
is `fall_down.validation.json`, including the original action hashes and projected
conservative bounds for all eight facing directions at normal game scale.

To repeat the import and publish only this clip:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --disable-autoexec art_source/character/male_commoner/source.blend \
  --python-exit-code 1 --python tools/blender/import_mixamo_clip.py -- \
  --recipe art_source/character/male_commoner/references/mixamo/fall_down.import.json
tools/assets build character/male_commoner --animations --clip fall_down
```

Run the read-only saved-source regression after authoring or rebuilding:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background --factory-startup \
  --disable-autoexec art_source/character/male_commoner/source.blend \
  --python-exit-code 1 --python tools/blender/validate_body_clip.py -- \
  --recipe art_source/character/male_commoner/references/mixamo/fall_down.import.json
```

The animation-only publication verifies identical model/texture artifacts and rig
compatibility, and preserves every previous clip reference. It publishes no new
character geometry or equipment.

## Locomotion, 2026-10-08

The user supplied these original FBX files. They remain motion references;
the existing commoner mesh, skeleton, sockets and ordinary `walk` are retained.

| Original filename | Retained file | Action | Frames at 30 FPS | Cycle distance (tiles) |
| --- | --- | --- | --- | --- |
| Sad Walk.fbx | `sad_walk.fbx` | `crawl` | 1–45 | 1.206404019601927 |
| Slow Run.fbx | `slow_run.fbx` | `run` | 1–23 | 2.340608494800207 |
| Fast Run.fbx | `fast_run.fbx` | `fast_run` | 1–17 | 3.0774975003271843 |

`crawl` means tired walking upright. The importer maps all 20 bones, corrects
the donor T-pose for the target A-pose, removes horizontal root travel, and
retains the supporting-foot height variation, including the airborne portion
of a running step. Stride and height scale by the target/donor leg-length ratio
(`1.0199211374524602`). The last sample matches the first exactly for looping.
Playback uses interpolated movement distance and the server's movement mode;
it does not change movement speed. Carrying continues to use `carry_walk`.

SHA-256 of the original bytes:

- `sad_walk.fbx`: `2e35cf9a5365fdfecb4e3c40e0c322182d3cb2de1f55b66f5f14c1623a0ea4f3`
- `slow_run.fbx`: `3be494258578f06d530b469375e992881e379b868f759c5ff13c58d01e901b14`
- `fast_run.fbx`: `195bbdfd6c90ef58930bcbb5e4933d0f30997b7ad4701ed9d2ac1c5951dfc564`

To repeat the import, run each adjacent recipe against the saved source, then
build the animations and publish the authored foot contacts:

```sh
for clip in crawl run fast_run; do
  /Applications/Blender.app/Contents/MacOS/Blender --background \
    art_source/character/male_commoner/source.blend \
    --python tools/blender/import_mixamo_clip.py -- \
    --recipe "art_source/character/male_commoner/references/mixamo/$clip.import.json"
done
tools/assets build character/male_commoner --animations
tools/assets publish-action-animations
```

The importer prints measured `cycleDistanceTiles`; retain that value in
`asset.yaml`. Review the [audio contacts](../../../../../data/locomotion_audio/README.md)
when changing gait timing. The original 12 actions and all their published clip
references were checked to remain unchanged after this import.

## Tree chopping motion

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
