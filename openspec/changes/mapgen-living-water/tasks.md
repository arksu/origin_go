# Tasks

> PAUSED / SUPERSEDED (2026-09-29): The user rejected continent/coastal
> generation and physical drainage for this task. The historical content below
> is NOT approved for implementation or spec sync. Current shape-only exploration:
> `docs/plans/river-shape-variants.txt`. Every river, including tributaries and
> narrow connectors, requires a boat-passable deep fairway. No variant is selected.

## 1. Configuration (`cmd/mapgen/options.go`, `gen_config.go`)

- [ ] 1.1 Add `ElevationOptions` (enabled, continent/detail feature tiles, continent weight, detail octaves, warp strength tiles, stretch percentiles, deep/shallow thresholds, min water body tiles, anchor min tiles) with continents-bold defaults, wire into `mapgenConfigFile` as a new `elevation:` section; add `river:` keys `lake_max_elevation`, `lake_max_massif`, `width_by_volume` — verify with `go test ./cmd/mapgen/ -run TestLoadMapgenOptions`
- [ ] 1.2 Add `Validate()` range checks (feature tiles ≥ 64, weight ∈ [0,1], percentiles ∈ [0,100] with low < high, deep < shallow, min sizes ≥ 0, gates ∈ [0,1]); extend `options_test.go`/`gen_config_test.go`: valid preset loads, unknown elevation/river key fails naming the key, omitted section inherits continents-bold defaults — verify with `go test ./cmd/mapgen/ -run 'TestMapgenOptions|TestLoadMapgenOptions'`

## 2. Structural elevation (`cmd/mapgen/noise_fields.go`, `tile_pipeline.go`)

- [ ] 2.1 Add fBm helper and composed raw elevation (warped continent + fBm detail, weights/feature sizes from config) behind `elevation.enabled`; keep the legacy single-octave path byte-identical when disabled — verify with a determinism test asserting legacy equality on a fixed seed
- [ ] 2.2 Implement `stretchElevation` (percentile sample → lo/hi → clamp map) as a pipeline pass over the precomputed elevation array, applied only when enabled; deep/shallow thresholds now come from config — verify unit tests: stretch maps p2/p98 to 0/1 on synthetic arrays, thresholds respected
- [ ] 2.3 Extract the mountain-massif signal into a shared `NoiseFields` helper used by both `BiomeSignals` and the lake gate (no formula drift); verify `BiomeSignals` outputs unchanged on a fixed seed

## 3. Water body filter (`cmd/mapgen/tile_pipeline.go`)

- [ ] 3.1 Implement the min-water-body pass between `resolveTileType` and `applyShorelineSand`: flood-fill water components, convert those below `min_water_body_tiles` to grass in both `tiles` and `baseTiles` (no sand rings on removed ponds); 0 disables — verify unit tests: tiny pond removed without sand ring, large body untouched, legacy mode with filter loses speckle
- [ ] 3.2 Log filter stats (bodies before/after, removed count) alongside existing precompute logs — verify via `go run ./cmd/mapgen -gen-config ... -png-overview-only` log output

## 4. Terrain-gated lakes and anchors (`cmd/mapgen/river.go`)

- [ ] 4.1 Thread elevation into `buildDrawLayoutRiverFlow` (remove `_ = elevation`); gate blue-noise lake candidates: skip when stretched elevation exceeds `lake_max_elevation`, massif signal exceeds `lake_max_massif`, or center is already structural water; preserve candidate order, spacing and size-class demotion — verify unit tests: highland candidate skipped, lowland placed, demotion still fires
- [ ] 4.2 Derive structural anchors (flood-fill components ≥ `anchor_min_tiles` on stretched elevation) as synthetic structural lake nodes; include them in link selection; skip footprint carving for structural nodes; shore points and inlets work unchanged — verify unit tests: drawn corridor reaches an anchor's shore, small components are not anchors, no double-carve
- [ ] 4.3 Implement volume-based width hierarchy behind `width_by_volume`: endpoint weights by size class / anchor area, deterministic pair-keyed ±1 jitter, clamped to `[river_width_min, river_width_max]`; `false` keeps legacy random — verify unit tests: large-large link wider than small-small link, deterministic across runs

## 5. Presets and logging (`etc/mapgen/presets/`, `cmd/mapgen/main.go`)

- [ ] 5.1 Update `hnh.yaml` (continents-bold values, gated `lake_count`, width hierarchy on, explicit seed), `default.yaml` and `minecraft_like.yaml` (conservative continent weight) with comments in the existing annotated style — verify with `go run ./cmd/mapgen -gen-config etc/mapgen/presets/hnh.yaml -png-overview-only` completing without config errors
- [ ] 5.2 Update `main.go` biome/river logging for the new options — verify `go build ./...`

## 6. Verification

- [ ] 6.1 Full mapgen suite green: `go test ./cmd/mapgen/...`, `go vet ./cmd/mapgen/...`, `go build ./...`
- [ ] 6.2 Determinism: same seed twice (Threads 1 vs N) → byte-identical `TerrainPrecompute.Tiles`; legacy mode (`elevation.enabled: false`) byte-identical to pre-change output on the same seed
- [ ] 6.3 Render before/after overview PNGs at the preset seed; record water share, body count, largest body, lake/anchor counts, and stage timings (elevation+stretch vs river pass) — attach to the change for visual acceptance before world regeneration
