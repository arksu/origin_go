# Hybrid character renderer implementation plan

The user approved the associated design. The brainstorming skill's referenced
writing-plans skill is not installed; this plan is maintained directly.

1. Export the approved Blender character and clips reproducibly. Inspect actual
   scene objects, materials, weights and actions. Reduce geometry with silhouette
   preservation, transfer weights and color, validate and export glTF assets.
2. Add Three.js and a small isolated WebGL2-to-Pixi GPU texture bridge. Validate
   GL state restoration, transparent pixels, orientation and target ownership.
3. Implement the shared actor asset cache, instance skeletons, distance-driven
   clips, carry pose and equipment attachment. Bound allocations and stale loads.
4. Add orthographic fixed-light rendering and pixel palette/outline processing,
   using pooled targets, nearest display and stable anchors.
5. Integrate through ObjectView/ObjectManager/Render with existing movement,
   carry, culling, KO, selection and disposal. Keep baked comparison available.
6. Add an instrumented browser review scene with actual world art and up to 30
   actors, equipment controls, direction/movement/carry controls and pixel-scale
   comparisons. Exercise meaningful lifecycle and interaction cases.
7. Inspect browser renders, correct visual/deformation defects, run type-check,
   object schema validation, production build and existing movement regressions.
   Document measured results and any remaining real-device verification.

Track completion and evidence below as work proceeds. Do not represent a desktop
benchmark as a mobile result or an unavailable test as a pass.

## Verification checkpoint — 2026-09-12

Steps 1–5 are implemented in the client. Step 6 has a browser review with real
game barrel art, eight directions, four animation states, separate wrap/belt,
1/8/30 actors and context-loss controls. Step 7 passed production build, forced
TypeScript check, object schema validation, all nine hybrid integration groups
and all eight existing movement regression groups in the desktop in-app browser.
Context restoration now explicitly restores a transparent offscreen clear;
the regression verifies byte-identical actor pixels after actual context loss.

Visual review covered eight directions and carry walking. A 30-actor desktop
sample reached 29.5 FPS under a 30 FPS cap (p95 interval 41.6 ms), with no GL
errors. This does not certify the mobile target: physical-phone performance,
thermal behavior and full live-world load remain open validation items.

Export instructions, budgets, benchmark scope and limitations are documented in
`web_new/src/game/actors/README.md`. Additional clothing content and full inventory
mapping are not part of the two-piece equipment implementation.
