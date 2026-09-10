# Hybrid character renderer

Approved on 2026-09-10. Target desktop browsers and mobile phone browsers, with
30 visible equipped characters at 30 FPS. This is a benchmark target, not a
claim of measured phone performance.

## Architecture

Keep the existing PixiJS world, server coordinates, interpolation and object
Y sorting. One Three.js renderer shares its WebGL2 context with PixiJS and
renders skeletal characters and equipment into small GPU render targets.
Pixel processing produces textures consumed by ordinary world sprites. Do not
copy frames through Canvas2D, PNG encoding or CPU pixel buffers for rendering.
Keep the GPU bridge isolated and validate GL state, alpha, orientation, resource
ownership and context restoration before extending the renderer.

This approach preserves the working world and makes clothing/animation costs
depend on models and clips rather than their Cartesian product. Alternatives
considered: implementing skeletal rendering directly in PixiJS (more engine
code to maintain), or moving the whole world to Three.js (larger integration
and regression surface). Full per-pixel depth for existing world props is a
later asset/rendering task; this stage retains current Y ordering.

## Character and animation

Use the approved muscular, stylized commoner Blender source. Produce an
optimized game mesh and inspect it alongside the source at actual game scale;
preserve silhouette, face, hands and limb volumes. Verify exported skinning
against the corrected Blender rig, especially W-facing shoulders and arms.
The initial geometry target is 20k triangles including equipment, subject to
visual comparison and measured performance. Add a reduced mesh for small
screen scale. Preserve the detailed authoring source.

Deliver idle, walk, carry idle and carry walk in eight directions. The camera
and lights remain fixed; only the actor rotates. Drive walk phase from the
existing rendered distance and calibrated cycle distance, including terminal
deceleration. Carry uses a universal raised-arm pose with locomotion below;
hand spacing does not depend on carried item size. Reuse existing server carry
relations and keep carried props in the existing 2D world for this stage.

Equipment assets load independently and bind to the same skeleton. Demonstrate
two concurrent equipment pieces and replacement during asynchronous loading.
Shared geometry/materials are cached; instance poses remain independent. The
full clothing catalog and future action clips are subsequent content work.

## Pixel treatment and interaction

Preserve approximately 96-pixel character scale, with sufficient transparent
space for hands and carry poses. Use nearest sampling, stable palette/shading
bands, a warm dark silhouette and selective internal contours. Compare against
the existing game sprites at 1:1 and check temporal stability in all directions.

Dynamic textures cannot use the existing immutable sprite alpha-mask cache.
Provide current-pose picking and hover without reading every actor image back
each frame. Keep world ordering, click selection and culling correct when
characters change pose, rotate, fall or carry an item.

## Resource budgets and failure behavior

Starting budgets: 32 MiB for actor render buffers and 128 MiB for resident actor
assets. Prefer 256–512-pixel material textures. Load only needed equipment,
deduplicate in-flight requests, track live references and evict unused assets
within a bounded cache. Do not update offscreen actors. Prefer lower update
rates for secondary actors before reducing the controlled player's cadence.

Validate resource definitions and fail explicitly on invalid skeletons or
assets. Cancel stale instance updates on removal/replacement. Restore resources
after context loss and dispose owned resources on scene shutdown. Keep the
existing baked renderer available for in-scene comparison and load failure;
do not silently present a fallback as successful 3D rendering.

## Acceptance

- Actual approved character in the game, not a primitive stand-in.
- Eight-direction comparison, idle/walk/carry transitions and slowing to stop.
- No recurrence of collapsed or stretched arms after export.
- Independent equipment loading and two simultaneously equipped pieces.
- Correct current-pose selection, hover, world occlusion and lifecycle.
- Instrumented 30-character scene, including warm-up and resource counters.
- Type check, production build and focused browser integration/regression checks.
- Phone frame-time claims require a real phone measurement; desktop emulation
  is insufficient. Record hardware and test conditions with results.

## References

- [PixiJS / Three.js shared context](https://pixijs.com/8.x/guides/third-party/mixing-three-and-pixi)
- [PZ runtime character rendering](https://projectzomboid.com/blog/news/2014/01/body-movin/)
- [PZ upper-body animation](https://projectzomboid.com/blog/news/2016/03/re-animator/)
- [PZ modular clothing](https://projectzomboid.com/blog/news/2016/06/bloody-business/)

These are architectural references. No Project Zomboid source or assets are
copied or assumed to be licensed for reuse.
