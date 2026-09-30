# Design

## Context

See `proposal.md` for the approved visual direction and probabilities. The user authorized implementation on 2026-09-29 after reviewing the plan.

Observed implementation:
- `river.go` builds lakes from positive elliptical basin fields. `lakeContourValue` adds one weak coarse noise term and returns early outside all basins, limiting outward shoreline detail.
- Main river routes and inlet anchors are selected before lake carving. `carveDrawLakeFootprint` only raises the shared flow mask, so simply skipping island tiles would not remove water already written there by a river.
- `routeLakeFairway` currently permits crossing land at a higher cost; `protectRiverRoute` then paints a full square footprint deep. Checking only route centers would still cut the edges of islands or peninsulas.
- `buildRiverClassMask` can dilate banks into dry tiles. `tile_pipeline.go` later combines river water with Perlin water, biome painting, and shoreline sand. Clearing lake flow alone cannot guarantee dry islands in the final result.
- The preview has a fast river-only path and a full terrain path. Both must share the new geometry and final classification semantics.
- `mapgen-living-water` is explicitly paused. Existing river-bend artifacts contain historical H&H values, while the current preset and uncommitted variable-width work have newer values. Preserve the current working tree; do not restore historical tuning.

## Goals / Non-Goals

**Goals:** Local, deterministic lake geometry; recognizable bays and peninsulas; controlled islands; true three-tile H&H boat-clearance checks; no loss of intended river connections; adjustable preview controls.

**Non-Goals:** Drainage simulation, a continent or ocean border, moving lake centers, rewriting the main-link policy, changing river bends or tributary density, archipelagos, changing biome probabilities, adding settlements/resources to islands, or testing runtime boat physics.

## Decisions

### 1. Isolate Option C behind an explicit switch

Add a focused `lake_geometry.go` helper and a `lake_options.go` helper rather than extending the already large `river.go` with another monolithic algorithm. The new switch applies only to drawn layout with positive fairway clearance. Feature-off execution retains the existing carving/routing path and allocations.

Keep lake IDs, centers, size classes, size sampling, and accepted main-link endpoints/order independent of lake-detail random streams. Build local shape candidates around these existing lakes and preserve incoming river footprints as water reservations. Do not globally reroute or delete main links to fit a decorative peninsula.

Alternative rejected: replacing the generator with thresholded world noise. That changes placement, creates uncontrolled fragments, and couples island counts to contour resolution.

### 2. Construct local water and land masks before committing them

Use the current basin union as the broad silhouette. Add bounded two-scale shoreline displacement, evaluated over padded bounds so detail can move the shoreline outward as well as inward. Cut a small number of broad, irregular, shore-connected land intrusions into that silhouette. Those intrusions create peninsulas and neighboring bays rather than independent holes.

Each candidate is checked within the lake's bounded tile rectangle. Preserve one substantial four-connected water body and all inlet attachment reservations. Reject a cut that disconnects open water, leaves a new detached pool, or destroys boat access; do not keep only the largest fragment and silently lose an inlet. Fill incidental enclosed land holes before intentional island placement. Preserve a nontrivial land attachment for peninsulas rather than one-tile necks.

Use stable per-lake/per-purpose seeds, not a shared sequential RNG. Candidate rejection does not reshuffle later lakes, islands, or river routes. Commit masks only after validation. If an optional detail does not fit after bounded attempts, omit it; if the complete candidate remains unusable, fall back to that lake's undetailed geometry and report the fallback.

### 3. Select islands independently of geometric fit

Draw one island-selection roll per lake using its existing size class. For selected large lakes, draw one separate count roll: 20% request two islands, otherwise one. Selected small/medium lakes request one. Failed placement never triggers another selection roll or a compensating quota elsewhere. For a two-island request, accept one if only the first fits.

Construct compact, noncircular, four-connected dry shapes. Placement is offset from the lake center and can be near a peninsula when clearance allows; no central-island rule. Start with a dry island radius range of 4-12 tiles, excluding shallow banks. Cap the combined island dry area at 8% of the pre-island lake water area. These dimensions and the area cap are initial engineering tuning, not a previously approved visual result. Skip candidates that cannot meet the minimum radius rather than shrinking them into tile specks.

Visual feedback on 2026-09-29 rejected the first near-circular island outlines. Refine only the island contour using independent seeded rotation, elongation, asymmetric broad lobes, a recessed bay, and restrained fine detail. Keep the existing nominal-radius settings, selection/count rolls, bounded placement attempts, area cap, and clearance checks. Verify silhouettes across seeds at equal radii so size variation alone cannot hide a circular-shape regression.

Do not overwrite existing river footprints. Require sufficient room for both shores' shallow bands plus the configured deep footprint between island and mainland, and between two islands. Confirm a connected deep-water circumnavigation path using full-footprint placements, not center-cell distance alone. Retain these paths for final validation.

### 4. Plan around land, never through accepted features

Carry explicit lake-feature land reservations for accepted peninsulas and islands. Lake-interior routing in the new mode checks the entire configured square boat footprint against these reservations and the local water mask. Ordinary dry lake shore is not a cheap shortcut either; inlet attachment corridors are established explicitly before accepting decorative land.

Choose lake hubs from valid water footprint placements, not just the old analytic basin score near the nominal center. Route all intended inlets into the same navigable component. Keep searches lake-local with a checked bound based on the local search area rather than the current `8 * (width + height)` global heuristic cap.

When a proposed feature prevents a required connection, reject that feature before committing it. If the fallback geometry still cannot support an accepted link, fail with seed/lake/inlet context; do not silently delete the link. Optional tributaries created later must also respect reserved lake land and island-clearance paths.

Alternative rejected: carving islands after fairway protection. That would require repairing broken routes after the fact and could erase accepted passages. Alternative rejected: allowing expensive land traversal; it would split the very features this change introduces.

### 5. Resolve lake banks and reserved land consistently

For the new mode, derive shallow water from the final shoreline, including island shores, using bounded coherent width variation. Use separate lake-bank settings so river width/shallow controls and current river appearance do not change. The initial lake-bank range is 1-5 tiles; it remains adjustable and is subject to visual review.

Apply banks before candidate clearance checks. Deep navigation reservations take precedence over shallow-water paint, but never over accepted dry land. Prevent the legacy `bank_radius` dilation from expanding into reserved lake land or silently overriding the configured lake band. A dry-land/deep-reservation overlap is a validation error, not a last-writer-wins choice.

Carry a compact representation of reserved feature land through the network and terrain result. Treat it as ordinary land for ground classification/biome eligibility and suppress Perlin water only on those explicit reservations. Do not alter the elevation or climate arrays. Outside those tiles, retain existing water precedence and structural protection. This limited exception is why `mapgen-biomes` has a delta; its current unconditional elevation-water rule would otherwise flood islands with `perlin_water_enabled: true`.

Final validation after shoreline sand verifies dry feature preservation, inlet connectivity, island clearance paths, and the existing river fairways. The generator's configured square envelope is the tested contract, not a claim about runtime boat collision behavior.

### 6. Configuration, preview, and compatibility

Proposed controls in the existing `river:` section:

| Key | General default | Initial H&H value | Meaning |
| --- | --- | --- | --- |
| `lake_irregular_enabled` | false | true | Enable Option C; false preserves legacy lake geometry |
| `lake_peninsula_count_max` | 3 | 3 | Maximum candidate peninsulas per lake; 0 disables them |
| `lake_peninsula_depth_ratio` | 0.35 | 0.35 | Maximum intrusion depth relative to the local lake radius; 0 disables intrusions |
| `lake_shore_variation_tiles` | 3 | 3 | Maximum shoreline displacement in tiles; 0 disables fine roughness |
| `lake_island_small_chance` | 0 | 0 | Selection probability per small lake |
| `lake_island_medium_chance` | 0 | 0.15 | Selection probability per medium lake |
| `lake_island_large_chance` | 0 | 0.35 | Selection probability per large lake |
| `lake_island_second_chance` | 0.20 | 0.20 | Conditional probability of requesting two islands in a selected large lake |
| `lake_island_radius_min` | 4 | 4 | Minimum dry nominal radius in tiles |
| `lake_island_radius_max` | 12 | 12 | Maximum dry nominal radius in tiles |
| `lake_shallow_width_min` | 1 | 1 | Minimum lake shallow-band width in tiles |
| `lake_shallow_width_max` | 5 | 5 | Maximum lake shallow-band width in tiles |

Island probabilities/count rules and the three-tile H&H fairway are approved. Other numeric defaults are initial tuning for the required visual review. Derive roughness wavelengths smoothly from lake size with conservative limits; do not add a separate octave configuration system. Reuse the existing coherent noise utilities.

Validate numeric fields even when inactive: finite probabilities in [0,1], peninsula count in [0,8], finite depth ratio in [0,0.6], roughness in [0,16] tiles, ordered island radii in [2,64], and ordered lake-bank widths in [0,32]. Active Option C requires `layout_draw: true` and positive configured fairway width. The master false switch ignores otherwise valid stored shape/island values. Equal lake-bank limits mean a constant band; both zero disable it. All three class chances zero disable intentional islands, regardless of the second-island chance.

Expose every new field in a lake subsection of `riversLayerSchema`, preserving the current schema-driven page. Include Russian descriptions, probability semantics, units, and off behavior in H&H YAML comments and README. Preserve explicit false/zero on YAML load/save. Do not conflate lake islands with the existing `biomes.blob_islet_*` land-biome satellites.

Both preview paths reuse the same lake geometry, reservations, bank logic, and final water semantics. No new layer or UI redesign is needed. Save retains unrelated preset values and comments.

### 7. Bound cost and validate behavior, then appearance

Reuse local raster/flood-fill/distance scratch per lake; retain only accepted geometry/reservations and navigation paths needed downstream. Fix candidate attempt limits (initially eight placements per requested peninsula/island) and avoid world-sized flood fills per lake. Add conservative checked allocation accounting for retained geometry, peak local scratch, and pathfinding queues to production and preview estimators without increasing the existing memory limit.

Tests cover zero/one selection probabilities, conditional count boundaries, deterministic population statistics before fit filtering, rejected candidates leaving no partial edits, no accidental islands, narrow/diagonal gaps, protected entry points, low-elevation reserved land, final-bank interactions, and feature-off golden buffers. Repeat fixed seeds with different thread counts.

Extend the opt-in review tooling or add a focused lake review helper. Render old/new lake close-ups, full lake masks, and final deep/shallow/land tiles for seeds 12345, 67890, and 314159 plus the current H&H seed. Include both a navigable peninsula fixture and a forced-island fixture; normal low-probability maps alone may miss the feature. Report selection/request/accept/reject counts by class, shape fallback counts, build time, and memory. Check main route counts and inlet connectivity alongside images. Require visual acceptance before calling the preset appearance final.

## Risks / Trade-offs

- Feature rejection can make realized island frequency lower than configured selection probability -> separate selection and placement statistics; do not promise an exact count per world.
- Narrow peninsulas may create detached fragments after cleanup -> topology checks before commit and final dry-land checks after all tile passes.
- A single deep centerline can hide a boat-width failure -> footprint-aware routing and final-tile validation, including island loops and diagonal gaps.
- Small features can be flooded by bank dilation or Perlin water -> explicit local land reservations shared by all relevant stages; keep surrounding hydrology unchanged.
- Larger lake-local path searches cost more than the old heuristic cap -> finite local budgets, reused scratch, memory preflight, and a full-size preview-only measurement.
- Uncommitted river-width work and user preset edits coexist -> capture the actual baseline at implementation time and preserve those edits rather than resetting historical fixtures or presets.

## Migration Plan

1. Record current fixed-seed legacy buffers and working-tree baseline before runtime edits.
2. Implement opt-in geometry, clearance, tile integration, and controls with the master switch off by default.
3. Enable only the new lake controls in H&H; leave all current river, biome, world, and placement settings intact.
4. Run focused regressions, full mapgen tests, preview tests, and multi-seed visual review; include a full current-size H&H overview without DB writes and report observed cost.
5. Roll back by setting `lake_irregular_enabled: false`. Turn off only islands by setting the three class probabilities to zero while keeping Option C shores.
6. Deliver the implementation and generated review images for visual acceptance. Preview-only generation is authorized; production data is outside this change.
