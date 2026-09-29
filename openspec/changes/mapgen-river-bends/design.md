# Design

## Context

See `proposal.md` for the selected scope. This design is necessary because path geometry, river rasterization, lake entrances, and final terrain resolution must agree about clearance; changing noise settings alone cannot provide that guarantee.

Observed implementation seams:

- `buildDrawLayoutRiverFlow` in `cmd/mapgen/river.go` ignores elevation and builds a regional lake backbone, other lake links, optional border links, and short links. Preserve these policies rather than replacing them with hydrology.
- `buildDrawPath` caps controls at 32 segments and derives wavelength from endpoint distance. The H&H preset uses octave gain 0.06, so small-scale detail is weak. Its current coherent-noise and spline helpers are reusable.
- `carveRiverCorridor` derives its deep radius from 45% of the outer radius. This is a visual ratio, not a footprint-clearance contract. Inlet carving uses another radius rule and skips dry tiles, which can leave narrow entrances.
- `resolveTileType` selects elevation-based shallow water before `riverDeep`. `TestResolveTileTypeOceanPrecedenceOverRiver` explicitly expects that behavior. The `mapgen-biomes` structural-protection requirement also preserves the old precedence; this change includes the scoped spec delta rather than silently contradicting it.
- Preview draws `RiverNetwork.Class` directly with zero elevation, so a correct-looking preview alone cannot establish final-map clearance.
- YAML replaces the river section rather than prepopulating its fields. New zero-valued controls must therefore mean legacy/off; avoid an unrelated configuration-default migration.
- Boat graphics exist in `web_new/src/game/objects/vehicles.json`, but no server boat-specific movement footprint was found. Generator clearance is configurable and testable independently; actual boat-runtime verification remains a later integration check.

## Goals / Non-Goals

**Goals:** Bound geometry work by path length, keep new controls separate from topology controls, and guarantee protected clearance on final tiles. Keep old presets reproducible with the new feature controls off.

**Non-Goals:** Revisit continent generation, replace the existing lake graph, implement boat physics or settlements, add portage mechanics, retune biomes, or reproduce the reference's dense hierarchy of tiny streams. Optional border links remain cropped continuations, not mandatory drainage outlets or a coastal frame.

## Decisions

### 1. Opt into distance-based shaping inside the drawn path

Add `river.shape_wavelength_tiles`. Zero calls the current path builder unchanged; a positive value selects Option B and requires `layout_draw: true` plus positive fairway width. Keep existing octave, gain, amplitude, frequency, and maximum control-spacing settings. In Option B, frequency scales the tile wavelength; `shape_waves_per_link` remains a legacy-only control and is labeled accordingly.

Build a broad curve, resample it by arc length, then apply bounded smaller-scale coherent displacement along its local normals. Subdivision follows the shortest usable wavelength and clearance scale, not the old 32-segment cap. Preserve exact endpoints and blend detail over a short approach distance rather than suppressing it across the whole link. Calm reaches arise from smoothly varying displacement strength, not repeated sine-wave patterns.

Check candidate geometry for self-contact and unintended contact with existing water. A spatial grid can bound local-neighbor checks; avoid all-pairs path comparisons. Keep bounded deterministic retry seeds. Invalid optional candidates leave no painted fragments; accepted required connections cannot later be silently removed. Keep an ordered, connected raster path rather than allowing global deduplication of a self-crossing path to hide jumps.

Alternative rejected: only increasing octave gain. It does not remove the long-link sampling cap, endpoint bias, or clearance defects. Terrain routing and recursive erosion are explicitly rejected.

### 2. Retain accepted route records, then add sparse tributaries

Retain each accepted main route's ordered centerline, stable ID, width, and endpoint/inlet references. This metadata serves both branch placement and clearance validation; do not build a second global navigation simulator.

Generate tributaries after the main routes with independent random salts. The branch budget is `floor(acceptedMainRoutes * tributary_ratio)`, not path-point count or curve length. Permit at most one branch per main route and do not recurse. Enforce configurable distance between new junctions, and keep junctions away from main-route endpoints/inlets and existing junctions. Use a stable bounded candidate order and existing overlap/crowding checks, allowing contact only at the intended parent junction.

Each branch uses Option B, joins the parent's deep corridor, and ends in a clearance-sized deep cap rather than a shallow taper. It need not create a new lake. Branch endpoint distances are separately bounded in tiles (the curved path can be longer); they do not consume the main-route or narrow-connector budget. Branch width can use the configured minimum river width, but the same minimum deep clearance applies everywhere.

Alternative rejected: a probability per spline sample. It changes branch density when detail increases and tends to produce the reference's unplayable fine network.

### 3. Make the deep core an explicit protected footprint

Add `river.fairway_width_tiles`. Zero retains legacy behavior. A positive odd width defines a square tile footprint; this is a conservative generator clearance envelope, not an invented boat model. Require it to fit inside the configured minimum river width. The user selected a minimum of 3 deep tiles on 2026-09-29.

For accepted paths, rasterize a connected sequence including intermediate cardinal placements at diagonal steps. Sweep the full footprint into deep water and a dedicated protection mask. Add shallow banks outside that mask, never through it. Wider main rivers can keep a larger deep core; this setting is a floor, not a uniform width for all rivers.

Apply the same rule to branch joins, narrow links, and lake entrances. Connect entrances to a reachable common deep interior in each linked lake; do not just deepen disconnected water cells along a straight line. Local inlet/neck widening is permitted where needed for clearance, but lake positions and unrelated lake contours stay unchanged. If bounded routing/widening cannot produce a valid accepted passage, return an error instead of keeping a shallow seam.

Keep the protection mask separate from all lake deep-water classification, so the final-priority exception affects only explicit fairways. Account for the extra mask, retained paths, and peak validation scratch in precompute memory estimates. Do not allocate these in legacy mode.

Alternative rejected: preserving only `riverDeep` center pixels. That neither proves a boat-sized footprint fits nor distinguishes protected corridors from unrelated lake depths.

### 4. Validate after final terrain resolution

Extend final tile resolution with explicit protected-fairway input. Protected cells resolve to deep water first; all other cells follow existing precedence. Keep the legacy shallow-ocean precedence test and add protected-mode regressions. Shoreline operations already target land candidates, but test preservation after the complete pass rather than assuming it.

Validate every accepted route's full-footprint placements and intended deep joins against final tiles. Checking the retained connected placement sequence avoids an expensive full-world navigation graph. Lake-interior routes are included, not inferred from two endpoints being visually blue. At a world edge, validate through the last fully in-bounds placement; out-of-bounds tiles are never counted as water.

Failure reports include seed, stable route ID, role, and first invalid location. The pipeline must not write a successful world after a failed validation. Report separate main-route, branch-budget, accepted-branch, and rejected-candidate counts. Tests that deliberately corrupt final tiles must prove failures are detectable.

Alternative rejected: validating only the preview or pre-resolution class mask. The observed elevation precedence bug passes both of those checks.

### 5. Small explicit configuration surface

New keys under the existing `river:` section:

| Key | Zero / omitted | Accepted H&H visual setting |
| --- | --- | --- |
| `shape_wavelength_tiles` | Legacy shape | 400 |
| `fairway_width_tiles` | Legacy carving/resolution | 3 (user confirmed) |
| `tributary_ratio` | No added tributaries | 0.08 |
| `tributary_spacing_tiles` | Unused with zero ratio | 300 |
| `tributary_length_min` | Unused with zero ratio | 120 |
| `tributary_length_max` | Unused with zero ratio | 360 |

Use H&H octave gain 0.5 and amplitude scale 1.5 instead of legacy 0.06 and 0.5. The first full-size review at wavelength 600, gain 0.35, and amplitude 0.5 still looked too straight; the current settings make broad and nested bends more visible. The user accepted the current appearance on 2026-09-29. Keep the existing lake counts, main-link budgets, elevation settings, and optional border-link fraction.

Validation rejects non-finite active shape controls, negative new values, ratios outside [0,1], even/nonpositive active clearance, clearance larger than minimum river width, and nonpositive/reversed active branch lengths or spacing. Positive tributary settings require Option B and fairway protection. Keep optional zero values effective; do not silently substitute a positive branch budget. Bound sampling and allocation arithmetic before work begins.

Expose the numeric controls in `riversLayerSchema` and test schema/JSON/YAML consistency. The frontend is schema-driven; avoid a preview UI rewrite. Run the same geometry/carving helpers in preview and production. Do not claim the raw rivers layer includes final terrain validation.

## Risks / Trade-offs

- More detail can cause self-contact or excessive route rejection -> bound displacement, test fixed endpoint fixtures and route counts, and review whole-network connectivity alongside close-ups.
- A coarse reference can tempt dense twig-like branches -> use an independent low branch budget and spacing; never derive branch count from subdivision.
- No current boat footprint -> label the configurable square envelope explicitly; do not claim runtime passage has been tested. Selecting another odd width changes configuration, not the architecture or tasks.
- Geometry samples and masks increase memory/time -> keep deterministic bounded retries, avoid pairwise scans or full-world pathfinding, include buffers in the existing safety budget, and measure the full H&H size.
- River carving necessarily changes tiles and nearby shores -> compare unchanged elevation buffers and no-river output rather than claiming every land tile will remain byte-identical when river paths move.
- Existing layout does not promise every independent lake connects -> validate all accepted intended connections and flag loss of main-network coverage against the baseline; do not silently introduce a different global connectivity algorithm.

## Migration Plan

1. Capture fixed-seed legacy fixtures before changing runtime code.
2. Implement and test new controls behind zero/off defaults. Leave the existing terrain plan paused.
3. Enable only H&H for review. Render rivers-only views, full maps, and deep/shallow close-ups for seeds 12345, 67890, and 314159. Use overview-only output; do not write/regenerate the live world or database.
4. Review curvature at several scales, branch sparsity, route counts/components, fairway integrity, runtime, and memory. Obtain visual acceptance before treating preset tuning as final.
5. Roll back by setting the new controls to zero and restoring the previous H&H shape settings; legacy golden fixtures must remain unchanged.

## Visual Acceptance

- On 2026-09-29 the user confirmed: "Yes, the picture is satisfactory overall." Retain the current bend and sparse-branch settings; no further visual tuning was requested.
- This closes visual acceptance only. The documented connectivity differences and absence of boat-runtime testing remain explicit limitations, not newly approved guarantees.
