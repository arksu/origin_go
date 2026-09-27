# Branch gathering motion

`picking_up_object.fbx` preserves the user-provided Mixamo **Picking Up Object**
file, supplied on 2026-09-27 as `Picking Up Object.fbx`.
SHA-256: `986a8d6aed4c608b29c16fd0b28443d8271463b31b64206590ac36555c026c17`.

The complete inclusive source range **1–104**, at **30 FPS**, is retargeted to
the existing 20-bone commoner rig as `pick_up`, frames **1–104**. The duration
between its first and last samples is **103/30 = 3.433333 seconds**. No trimming,
endpoint closure or extra frames are applied. The clip remains stationary and
keeps its supporting foot grounded. Finger channels have no counterparts in the
existing game skeleton.

The adjacent import recipe declares the source, rig, range and bone mapping.
To repeat the import and publication:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  art_source/character/male_commoner/source.blend --python-exit-code 1 \
  --python tools/blender/import_mixamo_clip.py -- \
  --recipe art_source/character/male_commoner/references/mixamo/picking_up_object.import.json
tools/assets build character/male_commoner --animations --clip pick_up
```

`data/action_animations/tree.json` binds `tree/take_branch` to this clip through
the common action timeline. `unbind_equipment_slots` detaches both hand models
only for visual playback, including blends and terminal holds. Actual equipment
is retained and current models return after playback. Variant selection permits
any equipment; facing follows the action target. No action-specific client,
server or protocol logic is required.

The complete clip is fitted to the actual server cycle, independent of source
FPS and duration. Preview metadata uses 1000 ms (10 ticks at the default 10 Hz)
and never sets gameplay duration. Restart the server with these defs and reload
clients with the matching catalog after deployment.

Preview: `/tests/hybrid-character.html?state=action/2`, select `Сбор ветки`.
Real equipment retention/rebind checks:
`/tests/equipment-unbind.html?binding=tree_take_branch`.
The generic `/tests/hybrid-integration.html` checks all bindings, eight headings,
33 phases, frame bounds, picking, cancellation and WebGL context restoration.
