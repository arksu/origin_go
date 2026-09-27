# Proposal

## Why

The current macro-family biome layer produces broad cell-derived regions and repairs per-tile speckles with aggressive cleanup. H&H's published patch generator offers branching organic masses; adopting its sz/wd geometry with explicit placement and cleanup rules will produce varied forest, clearings, wet ground, and surviving small terrain detail.

## What Changes

- Add an organic patch generator based on the repository reference `docs/features/patch.py`: branching skeleton, independent branch size sz and outline width wd (default sz/2), closed smoothing, raggedness, bounded rasterization, and satellites. Define robust connected masks and finite tree limits explicitly where the original helper code is unavailable.
- Replace macro families and land hash speckles with climate-biased forest/heath/moor/swamp selection plus an explicit skip probability. Paint main overlaps with swamp above heath/moor above forest.
- Add a separate secondary seed grid and configurable per-cell densities. Thicket paints only forest; dirt/clay paint only open grass, with wetness gating for clay.
- Build an immutable structural lock mask from hydrology and mountain/stone/sand ground. Apply it to every painter and cleanup routine. Guarantee main-mask connectivity before terrain restrictions; allow fragments after clipping.
- Generate mountain massifs with their own regional noise scale and coherent stone outcrops. Tune the H&H preset's clay fields toward 1.5% of all final world tiles.
- Revise main-layer cleanup ordering and zero-pass behavior. Add secondary patches and intentional satellites after all cleanup, so a large cleanup minimum cannot erase them.
- **BREAKING** (mapgen YAML only): retire `hnh_enabled`, `hnh_region_count`, `hnh_region_jitter`, `hnh_blend_width`, `hnh_variant_density`, and five `hnh_*_share` keys. Add flat `blob_*` size/width, selection, density, climate-threshold, shape, and safety settings; define switches, omitted-field defaults, strict validation, and complete preset migration.
- Preserve elevation, river generation, final hydrology/shoreline, tree/boulder scatter logic, and existing world storage. Preview with the actual `-png-overview-only` CLI flag and fixed seeds.

## Capabilities

### New Capabilities

- `mapgen-biomes`: deterministic organic land biome geometry, climate placement, overlap precedence, secondary density, protected terrain, intentional satellites, cleanup behavior, and configuration contracts.

### Modified Capabilities

- None; no existing spec covers the map generator.

## Impact

- `cmd/mapgen/`: new patch geometry/mask code; macro removal in `biome.go`; structural mask and ordered passes in `tile_pipeline.go`; mask-aware cleanup; options, biome default loading, validation/memory accounting, and retired-field logging in `main.go`; focused geometry/config/pipeline tests.
- `etc/mapgen/presets/default.yaml`, `hnh.yaml`, `minecraft_like.yaml`: migrate switches and all retired settings, tune branch sizes and densities, and set an explicit H&H comparison seed.
- No DB schema, protocol, server, client, or dependency changes. Generated content changes only on deliberate world regeneration.
