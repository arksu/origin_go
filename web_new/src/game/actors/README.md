# Hybrid pixel characters

The client renders the approved commoner with Three.js in Pixi's WebGL2 context.
An orthographic camera and fixed world light produce a 256 × 256 scratch image;
the GPU pass reduces it to a 128 × 128 transparent frame with palette ramps and
one-native-pixel outlines. The body is approximately 96 pixels tall. Pixi keeps
the existing world sorting, culling and object ownership. Frame transport uses
GPU copies; only interaction picking reads a single alpha pixel.

## Content and animation

`config.ts` contains the character asset ID, palette and render settings.
`AssetCatalog` resolves the published catalog into immutable model, standalone
animation and KTX2 texture URLs. Recipes and generated metadata own equipment
bindings, locomotion distance and asset budgets.
The current Meshy base contains a welded wrap and belt, so default equipment
is empty. The stone axe is available through catalog equipment binding; old linen pieces have incompatible
bind matrices. `GameFacade.setCharacterEquipment(entityId, items)` remains the
entry point for future garments authored against the current rig.
Inventory-to-catalog mapping and additional authored clothing are future work.
Assets are loaded on demand and shared between actors, with reference-counted
leases and eviction of unused assets when the 128 MiB estimated asset budget is
exceeded. This estimate is not total browser or GPU memory.

Clips: `idle`, `walk`, `carry_idle`, `carry_walk`. Eight sampled poses per walking
cycle advance with actual client displacement, including final movement damping.
One cycle currently covers 1.677975879375 tiles, read from the loaded walk
metadata. Time alone does not advance a stopped actor.
Skinning follows each mesh’s `skinning` extra: the Meshy model uses linear
skinning, matching Blender; the old dual-quaternion path remains supported.
All four clips use a 0.21 m ankle-center width. Walking retains donor foot timing
with 25% longer forward/backward travel; cycle distance scales with that travel.
The universal carry pose does not depend on prop size. Carried world props remain
2D and use the existing sorting; interleaved 3D hand/prop depth is not implemented.

High detail: 16,000 triangles including the integrated garment; low detail: 5,500.
Normal-scale actors use high detail; projected scale below 0.8 selects low detail.
Secondary actors update at most 15 times per second; unchanged poses are reused.
The player has priority. Offscreen actors return their texture to the pool.
The pool is capped at 128 outputs (8 MiB RGBA8); 30 outputs use 1.875 MiB.
Shared scratch color/depth and processed output add approximately 0.563 MiB,
excluding driver overhead, asset buffers and per-instance skeleton resources.

## Reproduce and inspect

The maintained workflow is [Blender asset workflow](../../../../docs/assets/README.md).
Edit the canonical `source.blend`, save, then run `tools/assets build` from the
repository root. Production builds run isolated headless Blender exports and
publish through the asset catalog; Blender MCP is an authoring/inspection helper.
Models use meshopt compression and external KTX2 textures, with one GLB per clip.
Shared model/texture leases survive animation revision changes and are released
when their final owner is evicted or the cache is destroyed. Reload the page to
load a newly published catalog snapshot.

The source GLB preserves the original 2048² PBR maps; the runtime uses a 1024²
base-color atlas and the existing fixed light and pixel outline pass. Painted
materials compare against the combined game palette so skin, hair and leather
can share a texture. The v3 exporter and MakeHuman attribution apply only to the
retained legacy source/assets, not the new Meshy geometry.

Run `npm run dev` from `web_new`, then open:

- `/tests/hybrid-character.html`: eight directions, idle/walk/carry,
  real game barrel for comparison, 1/8/30 actors, context loss and live metrics.
- `/tests/hybrid-integration.html`: actual ObjectManager/ObjectView integration,
  64 skeletal poses, distance invariance, picking, rejected legacy gear, carry relation,
  culling, LOD and context restoration.
- Movement and terminal-deceleration regressions now run against live actors in the integration page.
- `/tests/axe-review.html`: ordinary grip binding in both hands, movement and carry.

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

## Historical verification, 2026-09-12

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

## Historical screen-facing revision, 2026-09-13

Removed the two published baked atlases and 357 generated bake images, keeping
Blender sources, concept and material maps. Legacy sprite-only review files were
removed; terminal-stop tests now inspect the live actor animation. The integration
suite also projects the model forward vector through a real Three camera, checks
ObjectManager displacement at three zoom levels, zero-distance corrections and
adjacent-sector noise at 30/60/144 FPS. The user's cycle distance remains
1.20678051125 tiles. The design commit is 9fd00b7; implementation is uncommitted.
