# Hybrid actor update-rate design

## Goal

Render the local character's 3D pose smoothly while limiting visible remote
characters to 20 generated pixel-art frames per second. A stationary character
keeps its most recently generated GPU texture and does not incur 3D rendering.
The renderer can also be switched to an eight-direction, eight-phase visual
mode that still bakes its textures from the current Three.js character.

## Design

`ActorRenderSettings` is the non-UI settings contract for the future Settings
window. It holds render mode, local and remote target FPS, turn duration and
event-refresh policies. The
renderer accepts the settings at construction and exposes `setSettings()` for
the eventual UI; validation rejects invalid rates.

The local-player flag already flows from `ObjectManager` to the actor handle.
The renderer selects `localAnimationFps` for that handle and
`remoteAnimationFps` for all other handles. A newly visible actor, a changed
pose, a changed equipment set, a hover state change, LOD change, or restored
WebGL context forces the next texture immediately. Otherwise a changed pose is
held until its selected update interval passes.

The walk phase remains distance-driven, but no longer snaps to eight phase
positions. The Blender clip is sampled continuously; the selected renderer rate
decides how often that pose is copied into the 2D world. This preserves equal
stride phase for equal traveled distance at every client FPS.

`hybrid3d` derives a precise screen-space heading from actual movement and
turns through the shortest arc at 360 degrees per second, making a 180 degree
turn take 500 ms. `baked8` rounds the heading and gait phase to eight values.
Both modes render the current Three.js character into the GPU texture; neither
mode restores or depends on the old v3 raster atlas.

## Defaults

| Setting | Default | Purpose |
| --- | ---: | --- |
| `mode` | `hybrid3d` | Continuous 3D or discrete Three.js-baked eight-direction mode |
| `localAnimationFps` | 60 | Smooth local-player feedback |
| `remoteAnimationFps` | 20 | Pixel-art cadence and bounded GPU work |
| `turnDurationMs` | 500 | Time for a 180-degree hybrid turn |
| `renderStationaryChangesImmediately` | true | Equipment, carry and facing changes appear immediately |

The public code-level switch is `gameFacade.setActorRenderSettings({ mode: 'hybrid3d' })`
or `gameFacade.setActorRenderSettings({ mode: 'baked8' })`.
It can be called before renderer initialization; the value is retained and
used when the shared Three.js renderer is created.

## Validation

Tests cover settings validation, 20 FPS remote throttling, 60 FPS local
throttling, continuous distance-driven walk phase, stationary texture reuse and
immediate equipment/state refresh. Existing integration checks cover shared GL
state, context restoration and the 2D-world texture output.
