# Design

> PAUSED / SUPERSEDED (2026-09-29): The user rejected continent/coastal
> generation and physical drainage for this task. The historical content below
> is NOT approved for implementation or spec sync. Current shape-only exploration:
> `docs/plans/river-shape-variants.txt`. Every river, including tributaries and
> narrow connectors, requires a boat-passable deep fairway. No variant is selected.

## Context

`cmd/mapgen` builds the world in `BuildTerrainPrecompute` (tile_pipeline.go): elevation (single-octave Perlin, `noise_fields.go:24-29`) → drawn river/lake network (`buildDrawLayoutRiverFlow`, which discards elevation: `_ = elevation`, river.go:280) → biome ground + blob patches → `resolveTileType` (elevation thresholds 0.25/0.35 as constants) → shoreline sand. Lake candidates are placed by blue-noise distance only (`canPlaceDrawLake`, river.go:809), carved unconditionally (`carveDrawLakeFootprint`), and every link gets a random width in `[river_width_min, river_width_max]` (`riverWidthForLink`). Measured on the current `hnh.yaml` world: ~6000 water components, largest < 7000 tiles.

The chosen style is **continents-bold** from the prototype (`/tmp/living-water/`): fBm detail + warped low-frequency continent field + percentile stretch, deep 0.20 / shallow 0.30, 27.9% water in 6 coherent bodies. The prototype's Perlin is byte-identical to the generator's, so its parameters transfer directly. Config loading uses strict YAML (`KnownFields(true)`, version 1); presets are in-repo.

## Goals / Non-Goals

**Goals:**
- Replace speckle water with a small number of coherent seas/lakes (continents-bold), deterministically.
- Make drawn lakes respect terrain (lowlands, off massifs) and let drawn rivers reach structural water.
- Give rivers a width hierarchy tied to connected volume.
- Keep the legacy single-octave path available and deterministic; keep biome blob layer and object scatter untouched.

**Non-Goals:**
- No changes to blob biomes, object scatter, DB format, protocol, server, client.
- No runtime/incremental generation; mapgen stays a one-shot offline tool.
- No new dependencies; no elevation use beyond terrain (no 3D, no movement changes).

## Decisions

### D1: Elevation composition — continents-bold (prototype v2)
`elevation = stretch(continent * wC + detail * (1-wC))`, where `continent` is a domain-warped single Perlin at feature scale ~5200 tiles (freq 1.6e-5 per world unit), weight 0.62, warp 100 tiles at 4e-5; `detail` is 4-octave fBm (gain 0.5) at base feature ~1190 tiles (freq 7e-5). Stretch maps the 2nd/98th percentiles to [0,1]. Deep/shallow thresholds become config (defaults 0.20/0.30). All fields derive from the existing `PerlinNoise` instance with distinct offsets (same pattern as `BiomeSignals`), so determinism is preserved and no new noise code is needed beyond the fBm helper.
- Config keys use **tile feature sizes** (readable), converted internally: `continent_feature_tiles: 5200`, `detail_feature_tiles: 1190`, `warp_strength_tiles: 100`, `detail_octaves: 4`, `continent_weight: 0.62`, `stretch_low_percentile: 2`, `stretch_high_percentile: 98`, `deep_threshold: 0.20`, `shallow_threshold: 0.30`.
- Alternative considered: keeping absolute frequencies in YAML — rejected (unreadable, error-prone).

### D2: Stretch is a pipeline pass, not a per-tile call
Percentile stretch needs a global histogram, so `NoiseFields` gains a composed raw elevation and `BuildTerrainPrecompute` applies `stretchElevation` to the precomputed array (same two-pass shape as the prototype; one extra pass over the existing `[]float32`). Everything downstream (river network, `resolveTileType`, shoreline) consumes the stretched field unchanged. `elevation.enabled: false` keeps today's exact per-tile composition and thresholds — byte-identical to the current generator for the same seed.

### D3: Water body filter — after resolve, before shoreline
New pass between `resolveTileType` and `applyShorelineSand`: flood-fill water components (deep+shallow, 4-connected) on the final tiles; components smaller than `min_water_body_tiles` (default 250, 0 = off) become grass **and their `baseTiles` entries are cleared to grass**, so the shoreline pass cannot ring removed ponds with sand. The filter runs in both elevation modes (it is what makes the legacy path usable). Scratch memory: one visited bitmap + int32 queue, reused; O(tiles).

### D4: Lake terrain gating — evaluate at candidate centers
`buildDrawLayoutRiverFlow` stops discarding elevation. Each blue-noise candidate is checked before placement: `elevation ≤ river.lake_max_elevation` (default 0.55 of stretched range) and massif signal `≤ river.lake_max_massif` (default 0.55). The massif signal reuses the exact field `BiomeSignals` already computes (extracted into a shared `NoiseFields` helper over warped coordinates — no formula drift). Failing candidates are skipped without relocation; ordering, spacing and the existing size-class demotion logic are untouched. Candidates whose center is already structural water (`elevation < shallow_threshold`) are skipped as pointless. Lake count is retuned down in presets (structural seas now carry the big water).

### D5: Structural water anchors join the drawn network
Inside `buildDrawLayoutRiverFlow`, structural water is derived from the (stretched) elevation directly: flood-fill components with `elevation < shallow_threshold` and area ≥ `elevation.anchor_min_tiles` (default 40000) become synthetic `drawLake` nodes — centroid position, radius = `sqrt(area/π)` clamped, size class from area, flagged `structural`. They join the same lake list the existing machinery uses (regional backbone, border links, short-jitter stage), so drawn rivers can terminate at their shores. Footprint carving is skipped for structural nodes (their water already exists); everything else (inlets, widths, usedPairs) works unchanged. This is the H&H "unified waterway" property: drawn rivers reach the sea.

### D6: River width hierarchy — volume-scored, deterministic
`river.width_by_volume: true` (default) replaces the per-link random width: endpoint weights (draw-lake size class: large 1.0 / medium 0.6 / small 0.3; structural anchor: `min(1, area/anchor_ref)`), `score = clamp((wA+wB)/2)`, `width = min + (max-min)*score` clamped to `[river_width_min, river_width_max]`, plus ±1 deterministic jitter keyed by the pair. Border links use the source body's weight. `false` keeps the legacy random behavior. Same inputs → same widths (spec scenario).

### D7: Config, validation, presets
New top-level `elevation:` section + new `river:` keys (`lake_max_elevation`, `lake_max_massif`, `width_by_volume`), flat style consistent with existing sections; `Validate()` gains range checks (feature tiles ≥ 64, weights in [0,1], percentiles ordered, thresholds ordered, min sizes ≥ 0); unknown keys fail fast with the key named. Missing `elevation:` section defaults to enabled continents-bold (chosen style); old presets load unchanged. All three presets updated: `hnh.yaml` adopts v2-bold + gated lake count, `default.yaml` and `minecraft_like.yaml` get conservative v1-ish values (lower continent weight).

## Risks / Trade-offs

- [World look changes drastically on regeneration] → intended; gated by explicit regen; presets keep explicit seed for before/after comparison.
- [Anchor links can be very long (sea shore to far lake)] → existing `lake_link_max_distance` caps link length; presets retune `LakeBorderMix`.
- [Percentile stretch adds one global pass] → O(tiles) over the already-materialized elevation array; measured in tasks (expected negligible vs river pass).
- [Massif gate duplicates ruggedness math] → avoided by extracting the shared `NoiseFields` helper used by both `BiomeSignals` and the lake gate.
- [Two lake systems (structural + drawn) could double-count water] → drawn candidates inside structural water are skipped (D4) and anchors are not re-carved (D5).

## Migration Plan

1. Land code + presets + tests; `go test ./cmd/mapgen/...` green.
2. Render before/after overview PNGs at the preset's explicit seed; check body count, water share, timing.
3. Regenerate the world DB only after visual acceptance; rollback = `git revert` + regenerate.

## Open Questions

None — defaults are the prototype v2-bold parameters; preset values remain tuning surfaces.
