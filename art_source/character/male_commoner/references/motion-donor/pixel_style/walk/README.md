# Ordinary walk / pixel-style v3

An in-place ordinary walk, baked from the approved Blender character. The rest model and its repaired linen remain in `../male_commoner_v3_pixel.blend`.

## Review

- `previews/walk_8_directions_8f.gif`: recommended eight-frame export, all directions, enlarged 3x with nearest-neighbour pixels.
- `previews/walk_8_directions_6f.gif`: six-frame alternative at exactly the same walking speed.
- `previews/walk_6_vs_8.gif`: synchronized six/eight comparison, southeast and east, enlarged 4x.
- `previews/walk_poses.png` and `walk_back_poses.png`: frame-by-frame inspection.
- `previews/walk_ink_before_after.gif`: the previous and outlined finish on unchanged grass/dirt, barrel and log textures from the game.
- `previews/walk_arms_before_after.gif`: W/E comparison before and after correcting the anatomical skin binding.
- `pixel/{6,8}/{s,se,e,ne,n,nw,w,sw}/walk.gif`: individual transparent GIFs at native resolution.

Use **8 frames at 120 ms per frame**, looping for 960 ms. Six frames at 160 ms per frame also loop for 960 ms, but leave larger jumps between the passing foot and its next contact. Eight frames retain four readable poses per step. These are exports of the same underlying motion, rather than a change in walking speed. GIF is the review format; use PNG frames or the atlas in the game.

## Asset contract

- Frame: **80 x 96 px**, binary alpha, fixed material palettes, one-pixel continuous warm dark outline, no dithering or motion blur.
- Pixel finish: `pixel_finish.json` controls the dark brown/olive ink and separate skin, hair and linen ramps. Internal lines follow actual overlap depth and material boundaries; they do not trace every lighting gradient. Tiny isolated shading speckles are merged inside broad surfaces, preserving facial features. All PNGs and GIFs use this finish.
- Camera: fixed orthographic view at 30 degrees elevation; fixed world light. Only `Walk • MODEL DIRECTION` rotates about world Z for direction bakes. E faces screen-right; W faces screen-left. Directions are rendered individually, never mirrored.
- Atlas: columns = animation phases; rows = S, SE, E, NE, N, NW, W, SW. All frames use the same camera and origin; do not trim or centre each frame independently.
- Ground origin: approximately `(40, 84)` in a sprite cell. One world metre is approximately 49.48 pixels horizontally in this camera. Forward/back ground displacement is foreshortened by the elevated view.
- The action is in place. A compatible forward travel speed during the flat support phase is approximately **0.729 model metres/second**, or a stride of 0.70 m per full cycle. Match the game's movement to the projected stride to avoid sliding.

## Blender source

`male_commoner_walk.blend` contains a 50-bone skeleton, anatomical skin weights, the preserved sculpt/UVs, and the action `Walk • ordinary step • 0.96 s • seamless`. It plays frames **1–48 at 50 fps**. Frame 49 repeats frame 1 for interpolation at the loop boundary; do not export it as a ninth sprite frame.

Legs use a two-segment IK solve during generation, then store editable pose keys. The walk includes double support, heel contact, ball/toe roll, bent-knee swing, pelvis movement, counter-rotation of the chest, stable head, opposite arm swing and relaxed curled fingers. The enlarged bare soles are calibrated against the deformed mesh. Linen uses baked shape-key corrections against the animated thighs, with attached hems; it does not require a cloth simulation cache.

Skin weights are transferred onto the **evaluated rest surface**, including the active stylization/sculpt shape keys, in the cage's coordinate space. The underlying `Mesh.vertices` may still contain pre-stylization coordinates and must not be used as skin-transfer positions. Facial smoothing is likewise restricted using the stylized basis. This corrects the former shoulder/head and forearm/upper-arm cross-binding in both arms. `arm_review/after_binding.json` and `after_deformation.json` record bilateral binding and deformation checks over the whole sampled cycle.

`walk_config.json` contains export dimensions, timing, stride, lift and framing. `gait_metrics.json`, `validation.json` and `manifest.json` record source phases, mesh-ground checks, loop closure, ink settings and exported pixel bounds. The export checks fail on non-finite vertices, unweighted anatomical vertices, an open loop, moving camera/lights, clipped sprites or a GIF duration other than 960 ms. `geometry/` contains deterministic depth/material buffers sampled from the posed source at twice the final pixel grid. Regenerate these whenever the model, camera, framing or animation changes.

## Rebuild from repository root

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background --python-exit-code 1 --python tools/blender/animate_male_commoner_walk.py
/Applications/Blender.app/Contents/MacOS/Blender --background --python-exit-code 1 --python tools/blender/render_male_commoner_walk_geometry.py
python3 tools/blender/bake_male_commoner_walk.py
python3 tools/blender/review_male_commoner_ink.py
/Applications/Blender.app/Contents/MacOS/Blender --background --python-exit-code 1 --python tools/blender/inspect_commoner_walk_arms.py
```

The pixel stage requires Pillow and NumPy. `--preview` on both scripts limits the bake to eight frames in three review directions. Blender `--render-only` reuses the saved animated source. Blender `--finalize` refreshes the saved render defaults and this embedded README without rebuilding the animation.
