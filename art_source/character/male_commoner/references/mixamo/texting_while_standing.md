# Craft motion

`texting_while_standing.fbx` preserves the user-provided Mixamo **Texting While
Standing** file, supplied on 2026-09-27 as `Texting While Standing.fbx`.
SHA-256: `a795f757feb5e5ed96b01f9dcc5c261b73db5e70d686e3f5af15b27db5a29411`.

The source contains frames 1–708 at 30 FPS. The requested inclusive range
**70–600** is retargeted onto the existing commoner rig and saved as `craft`,
frames **1–531**. Its first-to-last-sample duration is 530/30 = 17.666667 seconds.
No endpoint closure, extra frames, or further trimming is applied. The donor's
finger channels have no counterparts on the existing 20-bone game skeleton;
the retained arm and hand motion is visible at native game resolution.

The import recipe contains the range and bone mapping. To repeat the import:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background \
  art_source/character/male_commoner/source.blend --python-exit-code 1 \
  --python tools/blender/import_mixamo_clip.py -- \
  --recipe art_source/character/male_commoner/references/mixamo/texting_while_standing.import.json
tools/assets build character/male_commoner --animations --clip craft
```

`data/action_animations/craft.json` binds the clip to the four current craft
recipe keys. Each binding uses the common tick-based timeline, unrestricted
equipment selection, preserved facing and ordinary pose eligibility. The
complete cropped clip fits each server cycle; its original duration never sets
gameplay timing. Future recipe keys need an explicit def binding.

Restart the server after changing defs and reload clients after publication.
Preview: `/tests/hybrid-character.html?state=action/0` (choose a `Крафт` entry).
The generic integration page `/tests/hybrid-integration.html` validates all
published bindings, eight directions, 33 phases, bounds, picking, cancellation
and WebGL context restoration.
