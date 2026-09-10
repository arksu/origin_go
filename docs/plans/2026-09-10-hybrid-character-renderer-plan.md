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
