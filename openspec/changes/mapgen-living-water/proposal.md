# Proposal

> PAUSED / SUPERSEDED (2026-09-29): The user rejected continent/coastal
> generation and physical drainage for this task. The historical content below
> is NOT approved for implementation or spec sync. Current shape-only exploration:
> `docs/plans/river-shape-variants.txt`. Every river, including tributaries and
> narrow connectors, requires a boat-passable deep fairway. No variant is selected.

## Why

The generated map reads as "not alive": the base water layer is a single-octave Perlin field thresholded per tile, producing ~6000 isolated 1–3 tile ponds scattered uniformly (the largest water body on the whole 6400² map is under 7000 tiles), while the 200 drawn lakes are placed by pure blue-noise distance with no terrain coupling and rivers get a random width. Verified by reading the applied generator and rendering the current `hnh.yaml` world. A prototype of structural elevation (style chosen: **v2 continents-bold**) fixes this in the same render budget.

## What Changes

- **Structural elevation**: replace the single-octave `Elevation()` with fBm detail (4 octaves) mixed with a domain-warped low-frequency continent field, then a 2–98 percentile stretch so water thresholds are stable. Style: continents-bold (continent weight 0.62, detail frequency 7e-5, warp 100 tiles, deep 0.20 / shallow 0.30). New `elevation:` YAML section; `enabled: false` keeps the legacy single-octave path.
- **Minimum water body filter**: after tile resolution, water components smaller than a configurable size are converted to land. Safety net for parameter drift and the legacy path.
- **Terrain-gated lakes**: drawn-lake candidates are rejected when the lowland elevation or mountain-massif gate fails (no more lakes punched through mountains or placed on plateaus); candidate order and blue-noise spacing preserved. Lake count retuned down (structural seas now carry the big water).
- **Structural water anchors**: large structural water components join the drawn network as first-class nodes, so drawn rivers connect to seas and real lakes instead of only to procedural ponds.
- **River width hierarchy**: corridor width derives from the connected water volume (endpoint body size classes and link length) within the existing `[river_width_min, river_width_max]` bounds, replacing pure random per link.
- **BREAKING** (mapgen YAML only): new `elevation:` section keys; new `river:` keys for gates, anchors and width hierarchy; all presets updated. Old presets load unchanged (new sections default on with v2-bold parameters).

## Capabilities

### New Capabilities
- `mapgen-terrain`: terrain structure of the offline map generator — elevation field composition, water body structure (seas, lakes, minimum body size), terrain gating of drawn lakes, structural water anchors in the drawn river network, and river width hierarchy; all deterministic per seed.

### Modified Capabilities
- (none — no existing spec covers the map generator; `mapgen-biomes` is a separate, unarchived change and stays untouched)

## Impact

- `cmd/mapgen/`: `noise_fields.go` (elevation composition + stretch), `tile_pipeline.go` (water body filter pass), `river.go` (lake gating, anchor nodes, width hierarchy), `options.go` (ElevationOptions + new RiverOptions keys + validation), `main.go` (logging), presets `etc/mapgen/presets/*.yaml`, tests.
- Prototype reference: `/tmp/living-water/` (v2-bold renders and stats).
- No server, client, DB schema, or protocol changes. World content changes only when the map is regenerated; biome blob layer and object scatter are untouched.
