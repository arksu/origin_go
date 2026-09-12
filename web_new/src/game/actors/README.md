# Hybrid pixel characters

The client renders the approved commoner with Three.js in Pixi's WebGL2 context.
An orthographic camera and fixed world light produce a 256 × 256 scratch image;
the GPU pass reduces it to a 128 × 128 transparent frame with palette ramps and
one-native-pixel outlines. The body is approximately 96 pixels tall. Pixi keeps
the existing world sorting, culling and object ownership. Frame transport uses
GPU copies; only interaction picking reads a single alpha pixel.

## Content and animation

`config.ts` contains URLs, equipment slots, palette, distance per cycle and budgets.
The initial equipment catalog contains the source linen wrap and belt. Call
`GameFacade.setCharacterEquipment(entityId, items)` to replace these pieces.
Inventory-to-catalog mapping and additional authored clothing are future work.
Assets are loaded on demand and shared between actors, with reference-counted
leases and eviction of unused assets when the 128 MiB estimated asset budget is
exceeded. This estimate is not total browser or GPU memory.

Clips: `idle`, `walk`, `carry_idle`, `carry_walk`. Eight sampled poses per walking
cycle advance with actual client displacement, including final movement damping.
One cycle covers 0.965424409 tiles. Time alone does not advance a stopped actor.
Skinning uses dual quaternions to preserve the source Blender shoulder/arm volume.
The universal carry pose does not depend on prop size. Carried world props remain
2D and use the existing sorting; interleaved 3D hand/prop depth is not implemented.

High detail: 17,659 triangles including both garments; low detail: 6,478.
Normal-scale actors use high detail; projected scale below 0.8 selects low detail.
Secondary actors update at most 15 times per second; unchanged poses are reused.
The player has priority. Offscreen actors return their texture to the pool.
The pool is capped at 128 outputs (8 MiB RGBA8); 30 outputs use 1.875 MiB.
Shared scratch color/depth and processed output add approximately 0.563 MiB,
excluding driver overhead, asset buffers and per-instance skeleton resources.

## Reproduce and inspect

From the repository root:

```sh
/Applications/Blender.app/Contents/MacOS/Blender --background --python-exit-code 1 --python tools/blender/export_commoner_realtime.py
```

This reads the approved idle/walk Blender sources and writes the runtime Blender
scene, `art_source/characters/male_commoner_v3/realtime/export-report.json`, and
three GLBs under `web_new/public/assets/game/characters/male_commoner/realtime`.
The source scenes are preserved. The current GLBs total 1,458,608 bytes.
The anatomical source is MakeHuman; its asset license is CC0 1.0, preserved in
`art_source/characters/male_commoner_v3/source/makehuman/LICENSE.ASSETS.md`.

Run `npm run dev` from `web_new`, then open:

- `/tests/hybrid-character.html`: eight directions, idle/walk/carry, equipment,
  real game barrel for comparison, 1/8/30 actors, context loss and live metrics.
- `/tests/hybrid-integration.html`: actual ObjectManager/ObjectView integration,
  64 skeletal poses, distance invariance, picking, gear races, carry relation,
  culling, LOD, context restoration and baked fallback.
- Movement and terminal-deceleration regressions now run against live actors in the integration page.

The game requires hybrid rendering and WebGL2. Baked character atlases and the
comparison/fallback path have been removed. Initialization errors propagate to
the game initialization handler; runtime rendering or model-loading failures stop
the render loop and show an explicit reload message. Context restoration reapplies
transparent clears because Three derives its reset default from Pixi's opaque canvas.

Facing uses actual displayed world displacement projected into screen coordinates,
then eight equal screen sectors with a three-degree boundary dead band. Adjacent
sector changes must persist for 120 ms and the current facing is held for at least
250 ms. Sharp turns of two or more sectors and movement starts respond immediately. Zero
animation-distance corrections do not turn the actor. Model yaw is computed by
inverting the orthographic camera elevation, so the forward vector projects onto
the selected screen ray. Camera pan/zoom do not change facing. The review page
shows these rays and actors moving along them.

## Verification, 2026-09-12

Production build, TypeScript check and object schema validation passed. All nine
hybrid browser test groups and all eight existing movement groups passed in the
desktop in-app browser, including actual context loss and exact restored pixels.
The source material was visually inspected in eight directions alongside the
game barrel, including raised-arm walking.

Desktop review sample: 30 high-detail actors, both garments, carry walk, speed 1,
30 FPS cap, rolling 300-frame window: 29.5 FPS, p95 frame interval 41.6 ms,
GL error 0. One sampled frame updated eight poses: 2.4 ms CPU submission,
141,288 triangles including fullscreen passes, 80 draw calls. Asset estimate
2,428,220 bytes; output textures 1,966,080 bytes. Poses are staggered and cached;
these are not the costs of redrawing all 30 actors simultaneously.

Phone performance, thermal throttling and a full populated live server scene
remain unverified. The review scene measures character work, not the complete
game workload. CPU submission time is not GPU execution time. Current build
warnings include large JavaScript chunks and dependency eval/mixed imports.

## Screen-facing revision, 2026-09-13

Removed the two published baked atlases and 357 generated bake images, keeping
Blender sources, concept and material maps. Legacy sprite-only review files were
removed; terminal-stop tests now inspect the live actor animation. The integration
suite also projects the model forward vector through a real Three camera, checks
ObjectManager displacement at three zoom levels, zero-distance corrections and
adjacent-sector noise at 30/60/144 FPS. The user's cycle distance remains
0.965424409 tiles. The design commit is 9fd00b7; implementation is uncommitted.
