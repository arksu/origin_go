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
One cycle covers 0.765424409 tiles. Time alone does not advance a stopped actor.
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
- `/tests/player-walk.html`: existing movement and terminal-deceleration checks.

The game uses hybrid rendering by default. Append `?characters=baked` to the game
URL for comparison. Unsupported WebGL2, load failures and rendering failures
fall back to the existing baked character. WebGL restore explicitly reapplies
transparent clears because Three derives its reset default from Pixi's opaque
canvas context.

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
