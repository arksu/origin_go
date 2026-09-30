# Proposal

## Why

Drawn lakes currently have rounded basin outlines with weak coarse noise, unlike the user's references with ragged shores, substantial bays, and peninsulas. The user selected Option C and approved rare small islands, provided that the lakes remain useful for boat travel.

## What Changes

- Add opt-in Option C lake geometry: connected open water, irregular bays, shore-connected peninsulas, and restrained small-scale shoreline roughness.
- Add controlled islands with H&H selection probabilities per lake: small 0%, medium 15%, large 35%. Selected medium lakes request one island; selected large lakes request one with 80% probability or two with 20% probability. Unsuitable placements are skipped, not forced.
- Keep islands compact, slightly irregular, and separate from shore. Preserve at least the configured boat footprint of deep water around accepted islands after shallow banks are resolved; H&H uses three tiles.
- Make lake inlet routing respect island and peninsula land instead of cutting through it. Preserve existing accepted main river links and validate passages against final tiles.
- Expose the new shape, island, and lake-bank controls in YAML and the existing preview panel, with Russian H&H comments and preset load/save support. Explicit zero island probabilities disable islands, including incidental holes in the generated lake mask.
- Keep default/omitted controls on the existing geometry path; enable the feature explicitly in H&H only. Do not change current user-edited river or biome tuning.

## Capabilities

### New Capabilities

- `mapgen-lakes`: Deterministic gameplay-oriented lake shores, optional islands, navigable clearance, bounded placement, preview parity, and backward-compatible controls.

### Modified Capabilities

- `mapgen-biomes`: Recognize explicitly reserved lake-feature land during structural terrain classification, so later Perlin-water resolution and biome operations cannot flood accepted islands or peninsulas. Water outside these local reservations retains existing behavior.

## Impact

- Generation: `cmd/mapgen/river.go`, `river_fairway.go`, and focused new lake geometry/options helpers and tests.
- Tile integration: `cmd/mapgen/tile_pipeline.go`, structural biome masks, and lake-aware water/bank resolution; no global elevation or climate changes.
- Configuration/preview: `options.go`, `gen_config.go`, `preview_server.go`, preset round-trip tests, memory estimates, `etc/mapgen/presets/hnh.yaml`, and `cmd/mapgen/README.md`.
- Planning builds on the implemented `mapgen-river-bends` behavior and current variable-width work. The rejected `mapgen-living-water` proposal remains paused; no continent, boundary ocean, drainage simulation, lake-placement rewrite, or runtime boat-physics work is included.
- No new dependencies, database migration, live-world generation, or asset changes. This change is a plan only until separately authorized for implementation.
