# Proposal

## Why

Players need a compact view of nearby terrain to orient themselves while moving through the isometric world. The client already receives the required tile data, so the minimap can use the existing chunk lifecycle without server work or an exploration-memory system.

## What Changes

- Add a corner minimap to the Vue HUD with a fixed top-down projection, square tiles, a centered player direction arrow, and discrete zoom controls.
- Display active chunks and chunks still retained by the existing client cache. Discard their minimap representation when those sources disappear; do not create a separate terrain store, session history, persistent storage, or chunk-image/zoom cache.
- Follow the player's smoothed world position independently of the main camera, including correct positioning immediately after spawn.
- Clear the minimap on every world entry/reset, including teleports, reconnects, entering/leaving future mines, and transitions between underground layers; prevent deferred work from restoring the previous world.
- Make generation of `minimap_base.png` through the `imagegen` skill an implementation task owned by the implementing agent. Deliver its `minimap_base.json` mapping, world-coordinate texture sampling, and a complete fallback palette for the 17 renderable tile types.
- Keep this release entirely client-side. Large-map navigation, map-click movement, exploration/fog state, persistent markers, other entity icons, and elevation overlays are outside this change.

## Capabilities

### New Capabilities

- `client-minimap`: Display currently available client terrain and the player's position in a corner HUD map, with generated terrain textures, bounded rendering resources, zoom, and world-reset behavior.

### Modified Capabilities

None.

## Impact

- Reference: `docs/features/minimap.md`, incorporating the user's 2026-10-02 decisions and subsequent confirmation of the fixed top-down projection.
- Client HUD: a new minimap component under `web_new/src/components/ui/` and integration into `web_new/src/views/GameView.vue`.
- Client rendering bridge: read-only access through `GameFacade`, `Render`, and `ChunkManager` to current chunk data and player pose; world lifecycle integration and Canvas rendering helpers.
- Assets: new `web_new/public/assets/game/minimap_base.png` and `minimap_base.json`, generated during implementation and committed as static assets. No runtime image-generation dependency.
- Validation: extend existing chunk lifecycle coverage, add minimap sampling/lifecycle checks, verify atlas coverage, and inspect the HUD in the browser.
- Existing `plow-tile` stream ordering, active-only world lookup, cache retention, and eviction requirements remain unchanged. No Go server, protobuf, network endpoint, storage migration, or new runtime package is required.
