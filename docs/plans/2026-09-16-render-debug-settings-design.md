# Render debug settings design

## Goal

Provide one saved Settings-window switch that enables the renderer's existing
debug facilities together: runtime metrics, object collision bounds, and
entity/type labels.

## Data flow

`useRenderDebugSettings` owns a strictly parsed boolean preference in
`localStorage`, defaulting to the existing build-time debug setting when no
preference exists. `GameView` persists a changed value and sends it through
`GameFacade` to `Render`. The facade retains the value before renderer
initialization so the first frame also has the intended state.

`Render` forwards the value to `DebugOverlay`. The overlay controls its metrics
container and asks `ObjectManager` to update both object bounds and object
labels. `ObjectManager` retains the state and applies it to later spawns.

## UI and verification

Settings gains a single labelled checkbox, "Render debug", below Renderer.
It immediately changes the live renderer and is keyboard accessible through
the native control.

Tests cover strict load and persistence behavior. The web-client production
build validates the Vue and renderer integration.
