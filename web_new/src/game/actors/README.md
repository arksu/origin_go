# Hybrid pixel characters

The client renders the approved commoner with Three.js in Pixi's WebGL2 context.
An orthographic camera and fixed world light produce a supersampled scratch image;
the GPU pass reduces it to a normally 128 × 128 transparent frame with palette ramps and
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

Locomotion clips: `idle`, `crawl`, `walk`, `run`, `fast_run`, `carry_idle`,
`carry_walk`. Server movement modes select the corresponding gait; `crawl` is
tired walking upright. Carrying retains its raised-arm `carry_walk` pose.
Each gait advances with actual client displacement, including final movement
damping, and reads its own cycle distance from metadata: crawl 1.206404019601927,
walk/carry walk 1.677975879375, run 2.340608494800207, fast run 3.0774975003271843
tiles. Hybrid rendering blends gait changes over the existing locomotion blend
duration; the discrete review mode samples eight poses per cycle. Time alone
does not advance a stopped actor. Footstep contacts select the same gait.
Skinning follows each mesh’s `skinning` extra: the Meshy model uses linear
skinning, matching Blender; the old dual-quaternion path remains supported.
The original idle/walk/carry clips use a 0.21 m ankle-center width. Walking retains donor foot timing
with 25% longer forward/backward travel; cycle distance scales with that travel.
The universal carry pose does not depend on prop size. Carried world props remain
2D and use the existing sorting; interleaved 3D hand/prop depth is not implemented.

High detail: 16,000 triangles including the integrated garment; low detail: 5,500.
Normal-scale actors use high detail; projected scale below 0.8 selects low detail.
Equipment switches to low detail only when that item has a low-detail mesh;
items with only a high-detail model remain visible at every camera zoom.
Secondary actors update at most 20 times per second by default; unchanged poses are reused.
The player has priority. Offscreen actors return their texture to the pool.
The pool is capped at 128 outputs and 128 MiB of RGBA8 pixels. Ordinary 128²
frames use 8 MiB for 128 outputs; action frames use their actual dimensions.
Shared scratch color/depth and processed output add approximately 0.563 MiB,
excluding driver overhead, asset buffers and per-instance skeleton resources.

## Timed actions

`ActorAssetCatalog` loads the optional immutable action definition projection.
`ActorInstance.setActionAnimation({key, phase, facingAngle})` accepts a normalized
phase from a network controller or local preview. Ordered clip/equipment variants,
eligibility, target-facing policy, blend time and output frame all come from defs.
An isolated full-body sampler blends over ordinary locomotion/equipment layers.
The actual loaded clip duration scales to the server's tick duration, and phase
one holds until the next confirmed cycle. There are no per-action runtime APIs.

`outputFrame` owns width, height and ground origin. Rendering, culling, picking,
pose invalidation and GPU accounting use those metrics. Blending out retains the
outgoing bounds. Enlarging a frame extends the camera without scaling the body;
context restoration recreates the scratch targets before further resizes.

The preview enumerates every binding/variant in the same catalog, with a local
loop/scrub controller and optional def-supplied label/equipment/duration. Sources
remain intact. Visible impact, sound and gameplay result timing are independent.
See [the synchronization contract](../../../../docs/features/action-animation-sync.md).

## Reproduce and inspect

Knocked out characters use a separate runtime 3D state: the idle skeleton lies
face up with the head to screen left, centered at the entity's ground position.
This state immediately suppresses gait blending, carry poses and equipment arm
layers. Its ground-centered camera frame is shared by rendering, picking and
culling; ObjectView supplies a low horizontal contact shadow. Recovery restores
ordinary facing, equipment poses and the standing frame. This is a static lying
pose; no fall or get-up transition clip is authored yet.

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

- `/tests/hybrid-character.html`: eight directions, idle/crawl/walk/run/fast run/carry,
  real game barrel for comparison, 1/8/30 actors, context loss and live metrics.
- `/tests/hybrid-integration.html`: actual ObjectManager/ObjectView integration,
  64 skeletal poses, distance invariance, picking, rejected legacy gear, carry relation,
  culling, LOD and context restoration.
- Movement and terminal-deceleration regressions now run against live actors in the integration page.
- `/tests/axe-review.html`: ordinary grip binding in both hands, movement and carry.
- `/tests/shallow-water.html`: eight dry/submerged directions, walk/carry,
  1/30 actor scenes and waterline GPU/picking/context-restoration checks.

Shallow-water tiles lower the body sprite according to `shallowWaterConfig.ts` and cut the
finished silhouette at the ground anchor in `PixelActorPass`. Five 80 × 40 ripple
frames share a 400 × 40 atlas and loop at 5 FPS below the body, so visible legs
occlude the crests. Frame changes also run while idle, without invalidating the
cached body render. The scale pulse follows walking distance and settles when
stopped. `shallowWaterConfig.ts` owns the frame rate and other tuning. The world container
and sorting anchor stay fixed, ground shadows disappear, and carried props follow
the lowered hands. Unknown terrain, carried characters and the KO pose immediately
clear the standing water effect. The approved source and reproducible PNG export
are in `art_source/fx/shallow_water/`.

The game requires hybrid rendering and WebGL2. Baked character atlases and the
comparison/fallback path have been removed. Initialization errors propagate to
the game initialization handler; runtime rendering or model-loading failures stop
the render loop and show an explicit reload message. Context restoration reapplies
transparent clears because Three derives its reset default from Pixi's opaque canvas.

Normal facing comes from server `Position.heading` in world radians, including
spawn and stationary movement updates. The client projects this heading into
screen coordinates; visual displacement does not replace it. New models start
in the supplied direction, and subsequent turns use the existing turn smoothing.
Target-facing actions temporarily override this base facing on the client. When
the action finishes or is canceled, the actor turns back to the latest server
heading, including updates received during the action.

Views without a supplied heading, such as local review fixtures, retain the
displacement fallback: eight screen sectors with a three-degree boundary dead
band, 120 ms adjacent-sector persistence and a 250 ms facing hold. Sharp turns
and movement starts respond immediately. Model yaw inverts the orthographic
camera elevation so its forward vector projects onto the selected screen ray.
Camera pan/zoom do not change facing.

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
