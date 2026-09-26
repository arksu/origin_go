# Shallow-water ripples

`review/ripples-v1.png` is the current generated RGBA source, restored at the user's request. The denser v2 is retained for comparison. The exact image
generation prompts and a dark-green/blue background preview are in `review/`.
Light comes from the upper left; the center and background remain transparent.

From the repository root, with the existing asset-pipeline dependencies installed:

```sh
node art_source/fx/shallow_water/export.mjs
```

The exporter crops the approved wave ring and writes an 80 × 40 nearest-sampled
PNG to `web_new/public/assets/game/fx/shallow_water/ripples.png`. The runtime uses
one shared texture for all actors. Its placement, opacity, knee depth and walking
pulse are configured in `web_new/src/game/actors/shallowWaterConfig.ts`.

Run the client dev server and open `/tests/shallow-water.html` for the actual
actor preview over green and blue, including all eight directions and carry/walk.

The restored sprite uses subtle blue crests and broken outer arcs. Idle opacity
is 0.9 and moving opacity is 1. The effect uses one frame with a distance-driven
scale pulse; there is no frame-swapping animation or sprite sheet.
