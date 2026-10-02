# Tasks

## 1. Generated terrain assets and fallback

- [x] 1.1 Invoke the `imagegen` skill and built-in image generator using existing game art as reference; deliver `web_new/public/assets/game/minimap_base.png` with 17 opaque 16-by-16 terrain regions and visually verify each material at final size, including plowed ground and all swamp variants.
- [x] 1.2 Create `minimap_base.json` from the actual final layout and add asset validation for renderable-ID coverage, integer in-bounds non-overlapping rectangles, region sizes, and opacity; verify the validator passes for the delivered PNG/JSON and rejects a deliberately invalid fixture.
- [x] 1.3 Add static atlas loading/validation and the complete fallback palette under `web_new/src/game/minimap/`; introduce `test:minimap` using the existing Node/esbuild runner pattern and verify valid textures, missing mappings, load failure, void transparency, and bounded diagnostics through focused tests.
- [x] 1.4 Document the final atlas contract and generation recipe in `docs/features/minimap.md`; verify documented IDs, region size, paths, and mapping fields match the delivered assets. Run `python3 tools/tests/roundtrip_test.py --regression`, `node tools/tests/pixi_semantics_test.mjs`, and `python3 tools/tests/render_sim.py` from the repository root and confirm all pass after the atlas work.

## 2. Borrowed terrain and player-pose access

- [x] 2.1 Add a read-only minimap chunk accessor/visitor through `ChunkManager` and the rendering bridge, preferring active payloads over existing retained entries without cloning tile arrays; extend `web_new/tests/chunks.test.ts` and verify active precedence, pending active data, and absence after both sources are removed.
- [x] 2.2 Verify repeated minimap reads leave retention timestamps, eviction order, cache limits, and active-only `getTileTypeAtWorld()` behavior unchanged; add TTL/LRU and stale-packet regression scenarios and confirm `npm run test:chunks` passes from `web_new`.
- [x] 2.3 Expose the current player's world pose through `GameFacade`, reusing movement interpolation and its initialized spawn state; add tests showing correct nonzero initial position before movement, no marker before spawn, correct heading, and no second interpolation update.

## 3. Bounded Canvas rendering

- [x] 3.1 Implement viewport coordinate conversion and world-phased texture sampling using supplied tile/chunk dimensions; verify pixel tests for negative coordinates, chunk seams, all fallback types, and transparent unavailable/void terrain with `npm run test:minimap`.
- [x] 3.2 Implement the Canvas 2D minimap renderer with reusable viewport-sized buffers, synchronous borrowed chunk reads, and a centered direction arrow; verify that removing a chunk while the player is stationary clears its pixels on the next draw and that an accepted terrain replacement updates the next draw.
- [x] 3.3 Add attach/detach and frame integration through `GameFacade`/`Render` after existing movement interpolation; verify smoothed follow, independence from main-camera pan/zoom, release on detach, and absence of terrain or per-chunk image retention between frames.

## 4. World transitions and asynchronous lifecycle

- [x] 4.1 Wire unconditional minimap clearing into every world entry/reset, including re-entry without a preceding leave packet; add lifecycle tests and verify old terrain and the player marker are cleared before any destination pose is shown.
- [x] 4.2 Guard deferred asset completion and drawing by attachment/world lifecycle without capturing terrain across awaits; verify reset during loading, detach during loading, and completion after teleport cannot revive old pixels or an old attachment.
- [x] 4.3 Add transition scenarios representing surface-to-mine, mine-to-surface, and underground-to-underground travel with identical chunk coordinates; verify each simulated world entry starts empty and only destination-stream tiles appear. Document this existing world-entry/reset integration contract for future mines in `docs/features/minimap.md`.

## 5. Corner HUD and controls

- [x] 5.1 Add `MinimapWindow.vue` to the ready game HUD with fixed top-down orientation, top-right placement, approximately 200-by-200 CSS pixels, safe-area handling, and accessible zoom buttons; verify the widget visually at desktop and narrow viewport sizes without overlapping essential controls.
- [x] 5.2 Add configured zoom levels `0.5`, `1`, `2`, and `4`, defaulting to `1`, with nearest-neighbor presentation and minimap-only wheel/button handling; verify centered player position at every level, crisp magnification, and no world command or main-camera change when interacting with the widget.
- [x] 5.3 Update the feature document to reflect the implemented controls, confirmed projection, cache lifetime semantics, and reset behavior; verify it no longer describes the chosen projection as undecided or promises exploration memory, a large map, or server changes.

## 6. Integration validation

- [x] 6.1 Using a compatible installed Node runtime, run `npm run test:minimap`, `npm run test:chunks`, `npm run type-check`, and `npm run build` from `web_new`; confirm the full client integration passes and review the change's diff for unintended server, protocol, cache-policy, or storage changes.
- [x] 6.2 Exercise the built client with the existing server: initial stationary spawn, walking across chunks, terrain edits, cache eviction while stationary, camera pan, every zoom, reconnect, and teleport; record browser evidence that the map follows current data, consumes its own input, and stays responsive without growing terrain history.
- [x] 6.3 Inspect final texture readability and arrow direction on representative water, forest, swamp, and plowed terrain at standard/high device pixel ratio and each zoom; confirm fallback behavior by failing the minimap asset load and confirm the simulated mine-transition tests cover layer resets until mine gameplay exists.

Validation record: On 2026-10-02, the user confirmed task 6.2 was manually tested and PASSED. This completes the live gameplay validation in addition to the automated tests and browser checks performed during implementation.
