# Shallow-water ripples

The runtime is a five-frame loop at 5 FPS. The user selected review variants 1
and 4; they are animation frames 1 and 3. Three generated intermediate frames
complete the sequence. All five RGBA sources and their ordered list are in
`animation/`; exact built-in image_gen prompts are in `animation/prompts.md`.
The original v1, denser v2, and the four review candidates remain available.
Light comes from the upper left; the background and gaps remain transparent.

From the repository root, with the existing asset-pipeline dependencies installed:

```sh
node art_source/fx/shallow_water/export.mjs
```

The exporter applies the same crop and nearest sampling to every frame and writes
`ripples-strip.png` (400 × 40) and `ripples.json` to
`web_new/public/assets/game/fx/shallow_water/`. All actors share the atlas source.
`ripples.png` is the first frame for static reviews. Placement, opacity, immersion,
walking pulse and `framesPerSecond: 5` live in
`web_new/src/game/actors/shallowWaterConfig.ts`.

Run the client dev server and open `/tests/shallow-water.html` for the actual
actor preview over green and blue, including all eight directions and carry/walk.

The animation uses subtle blue crests and broken outer arcs. Idle opacity is 0.9
and moving opacity is 1. Time controls the frame sequence; walking distance controls
only the additional scale pulse. The effect draws under the visible body.
