# Design

## Context

See `proposal.md` for motivation and `specs/client-minimap/spec.md` for behavior. This design covers the HUD/rendering boundary, borrowed chunk data, asynchronous asset loading, and reset behavior across several existing client modules.

Observed integration points:

- `network/handlers.ts` applies `ChunkStreamGuard` before changing `gameStore` and forwarding accepted loads/unloads to `GameFacade`.
- `ChunkManager.activeChunks` holds accepted tile payloads even before their graphics are ready. `ChunkCache` retains fully built unloaded chunks. Its current limits are 64 hidden entries and 180 seconds, checked every 10 seconds; these settings remain owned by the existing cache.
- `ChunkCache.peek()` reads without changing retention or metrics. `getTileTypeAtWorld()` deliberately uses active data only, including for shallow-water effects; that contract must remain intact.
- `MoveController` already calculates the rendered player pose and initializes world position/heading on spawn. `gameStore.playerPosition` alone is insufficient before the first movement message.
- `GameView.vue` owns the Vue HUD. The upper left, top center, and bottom corners already contain controls. Vue accesses game rendering through `GameFacade` and does not import Pixi.
- Existing world-entry/reset handling clears active chunks, retained graphics, and movement state. There is no client world/layer identifier suitable for keeping terrain across transitions, and this feature does not need one.
- `web_new/tests/chunks.test.ts` already exercises ordering, active-only lookup, cache eviction, and fresh-world behavior with the real chunk manager. `scripts/test-chunks.mjs` provides its Node/esbuild runner.

## Goals / Non-Goals

**Goals:**

- Borrow available terrain synchronously for drawing without extending its ownership or lifetime.
- Reuse the main rendering loop and existing movement pose so the minimap does not introduce a second interpolation clock.
- Bound minimap working storage by viewport size and static atlas size, independently of the player's route.
- Treat every world entry as a new presentation generation, including future mine transitions.

**Non-Goals:**

- Altering chunk admission, TTL/LRU policy, active-world tile lookup, or server behavior.
- Introducing a terrain store, chunk raster cache, worker pipeline, exploration persistence, or a separate map protocol.
- Implementing mines or changing their gameplay. Future layer transitions must use the existing world-entry/reset lifecycle.

## Decisions

### 1. Vue owns the widget; the game facade owns access and rendering

Add `MinimapWindow.vue` as a compact HUD component in `GameView.vue`. It owns the Canvas element, accessible zoom buttons, and responsive placement. Attach/detach the Canvas and set its zoom through `GameFacade`; the facade forwards to an optional Canvas 2D minimap renderer owned by `Render`. Put sampling and asset-loading helpers under `game/minimap/`.

Update the attached minimap from the existing render loop after movement interpolation. Do not call `MoveController.update()` a second time. Detach on component unmount and destroy the presentation resources with their owner. No Pinia terrain store or Pixi objects enter the component.

This keeps one route from UI to game internals. A Pixi HUD texture would require additional GPU ownership, while a Vue component reading `ChunkCache` directly would bypass the existing boundary.

### 2. Read active and retained tiles only for the current frame

Add a narrow read-only chunk accessor/visitor to `ChunkManager`, reachable through the rendering bridge. Resolve only coordinates intersecting the current viewport. For each coordinate, use the active payload first and otherwise `chunkCache.peek()`.

Pass coordinates, tile bytes, and version as borrowed plain values. Do not expose `Chunk`, meshes, or mutable cache operations. Tile arrays are not cloned, stored in component state, or captured by asynchronous callbacks. Borrowed references end when the synchronous draw returns.

Keep `getTileTypeAtWorld()` active-only. Reusing it with broader semantics would change shallow-water behavior and contradict its existing test. A separate minimap read path makes the permitted use of retained terrain explicit.

Read live availability on each minimap frame and clear the full working raster before filling it. Eviction therefore disappears without adding cache listeners, retaining the last chunk image, or requiring player movement. Repeated reads never call `retain()`, `markActive()`, or change cache configuration.

### 3. Use fixed top-down coordinates and the existing player pose

The user confirmed square top-down tiles and fixed orientation. Use world positive X to the right and positive Y downward. Convert world units to tile coordinates with the supplied `coordPerTile`; derive chunk/local coordinates using floor division and the supplied `chunkSize`.

Center the viewport on the player's fractional tile position, not on the main camera. Draw a fixed-size arrow at the center using world heading rather than a projected sprite-direction index. Expose the already initialized spawn pose if the movement controller has not yet produced its first rendered sample; once it has, use its smoothed pose. No additional player-position history is needed. Until a valid current-world pose exists, keep the map empty.

### 4. Generate static base textures during implementation

The implementing agent must invoke the `imagegen` skill and built-in image generator to create the terrain materials, using the game's existing art as reference. This is part of implementation, not a prerequisite for the user to supply. The final asset contract is:

- `/assets/game/minimap_base.png`: one atlas containing an opaque 16-by-16 base material for each ID in `RENDERABLE_TILE_IDS`, currently 17; no isometric corners, labels, or baked directional shadows.
- `/assets/game/minimap_base.json`: a simple object keyed by decimal tile ID, each value containing integer `x`, `y`, `width`, and `height`. Each region is 16-by-16, in bounds, distinct, and corresponds to the intended material. `TILE_VOID` has no region.

Validate the actual image and manifest together. Generate the manifest from the final layout rather than trusting image labels or an intended layout. Visually inspect the small final textures and the resulting minimap, especially water, forests, plowed ground, and the three swamp variants. Static assets are committed and loaded locally; image generation is not a build or runtime dependency.

Use existing stdlib PNG tooling for verification/packaging when needed; do not assume Pillow or ImageMagick is installed. The existing game atlas and its Pixi trim/rotation contract are independent of this plain minimap atlas.

Load/decode the atlas once per attached renderer lifecycle and reuse its static pixel buffer. Validate mappings before sampling. Missing or invalid regions use the complete fallback palette; image/manifest failure falls back for all terrain and emits one meaningful diagnostic per failure, without preventing world startup. Build the palette from `tileColor()` and supply the missing plowed and swamp2/swamp3 colors. Unknown non-void IDs use a diagnostic color and a bounded warning; void remains transparent.

### 5. Rasterize the current viewport with bounded working buffers

Use one reusable terrain raster and the displayed Canvas, sized for the current viewport and supported zoom. Sample each available world tile at `mod(tileX, textureWidth)` and `mod(tileY, textureHeight)` with nonnegative modulo. Tile phase is independent of chunk boundaries and player movement.

Clear before drawing so missing chunks, void tiles, and newly evicted terrain cannot leave stale pixels. Draw only the current viewport. Do not maintain per-chunk images, a pyramid of zoom textures, or a trail of previous viewports. Atlas pixels are static art, not retained world data.

Initial UI defaults, chosen for this proposal rather than previously confirmed by the user: top-right placement, approximately 200-by-200 CSS pixels, and zoom scales `0.5`, `1`, `2`, and `4` with default `1`. Keep these in client configuration/constants. Magnification uses nearest-neighbor presentation. For `0.5`, use nearest-neighbor reduction of the current raster without a persistent zoom cache; visually check narrow water/road features. Canvas backing dimensions account for device pixel ratio, and the widget respects safe areas on small screens.

Buttons and wheel input change only minimap zoom; suppress propagation to world input. The first release always follows the player and offers no free pan, expanded map, or click-to-move. These defaults resolve the optional UI ideas in the source plan without adding another gameplay mode.

Redrawing a small bounded raster trades some CPU for a simple, verifiable lifetime model. Measure while walking and while cache eviction occurs; tune bounded redraw work if needed without introducing terrain retention.

### 6. Reset on world lifecycle, not inferred layer identity

Add an unconditional minimap world-entry/reset hook to the existing client lifecycle. Clear the visible and working rasters, invalidate the player pose, and advance a presentation-generation token for every entry/reset, including repeated entries without a preceding leave event. On world teardown or component detach, release transient draw resources and references.

Texture-load completion may publish validated static art, but must not hold or redraw a captured chunk set or player pose. Any deferred presentation callback must check attachment/world generation and obtain fresh current data before drawing. A completed load from an old attachment must not revive that attachment.

Future mine entry, return to the surface, and underground-to-underground travel must go through the same existing world-entry/reset path. A new layer with identical chunk coordinates starts empty and is populated only by the new stream. This change adds no layer ID, retained layer map, or server requirement.

### 7. Verify lifecycle and pixels with focused existing tooling

Extend the chunk tests to cover borrowed minimap reads, active precedence, cache retention without renewal, TTL/LRU eviction while stationary, and preservation of active-only world lookup. Add focused tests for world-coordinate sampling, negative coordinates, runtime chunk dimensions, transparency/fallback, spawn pose, and reset while assets or draws are pending. Reuse the existing Node/esbuild testing pattern instead of adding a test framework.

Browser inspection covers the actual atlas, smooth following, arrow direction, zoom, device pixel ratio, narrow layouts, and input isolation. Simulate a layer change through consecutive world entries at identical coordinates; no implemented mine is required to verify the contract. Run the required atlas regression commands after adding the atlas or touching tooling.

## Risks / Trade-offs

- Retained chunks show their last received terrain until refreshed or evicted → Accept this as existing cache behavior; active payloads always win, and the minimap makes no live-world guarantee outside active chunks.
- Continuous rasterization may cost frame time on slower clients → Bound work to the small viewport, reuse buffers, cap zoom-out, and inspect performance before considering more complex scheduling.
- Generated materials can lose distinction at 16-by-16 or at zoom-out → Inspect final-size samples and in-game results, with complete fallback colors for failures.
- A displayed canvas can accidentally become history after eviction/reset → Clear the entire raster each draw and clear both buffers synchronously on world transitions.
- Async texture completion can race with teleport/unmount → Never capture terrain across awaits; gate completion by lifecycle and draw only current state.
- Main-game direction indices may not match top-down bearings → Use world heading and verify cardinal/diagonal movement explicitly.

## Migration Plan

Deploy the static atlas/manifest and client changes together through the existing web build. There are no server upgrades, protocol migrations, or stored minimap data to migrate. Rollback consists of reverting the client widget/rendering integration and its two assets. Existing chunk-cache settings and other gameplay state remain compatible.
