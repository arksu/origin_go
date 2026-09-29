# Proposal

## Why

The user selected Option B, "bends within bends": natural-looking river paths for player travel, not physically simulated drainage. The restored generator suppresses small bends, has no sparse river-to-river tributary pass, and can turn an already carved deep fairway back into shallow water during final terrain resolution.

## What Changes

- Add a world-distance-based, multiscale path-shaping mode to the existing drawn-river generator. Retain broad meanders, smaller irregular bends, and quieter stretches without adding terrain simulation.
- Add occasional, non-recursive tributaries using a separate low-density budget and junction spacing. Every accepted tributary joins an existing river through a continuous deep fairway; increasing curve detail does not increase the branch budget.
- Make minimum deep-fairway clearance explicit and configurable. Protect it through main rivers, tributaries, narrow links, lake entrances, final tile resolution, and shoreline processing.
- Validate final deep-water clearance and intended connections, not only blue-water connectivity in the preview.
- Expose the controls in the existing river preview, enable the selected style in the H&H preset, and retain legacy behavior when the new controls are disabled.
- Keep elevation generation, landmass shape, lake-placement policy, and existing main-link selection policy. Do not add an ocean frame, world-center drainage, catchments, or erosion.

## Capabilities

### New Capabilities

- `mapgen-rivers`: deterministic drawn river geometry, sparse navigable tributaries, protected deep fairways, validation, and preview controls.

### Modified Capabilities

- `mapgen-biomes`: narrow exception to final water precedence for explicitly protected river fairways. Land painting and shoreline behavior outside those fairways stay unchanged.

## Impact

- `cmd/mapgen/river.go` and focused geometry/tributary/fairway helpers; `options.go`, `tile_pipeline.go`, preview schema, adjacent tests, and `cmd/mapgen/README.md`.
- `etc/mapgen/presets/hnh.yaml`: selected geometry, sparse tributary tuning, and explicit clearance. Existing version-1 files retain their old behavior when the new fields are absent/zero.
- No new dependencies, server/client boat mechanics, village placement, portage mechanics, database schema changes, or automatic world regeneration.
- The existing `mapgen-living-water` change remains paused and rejected; none of its continent/terrain tasks apply here. Selection references: `docs/plans/river-shape-variants.txt` and `docs/plans/river-fairways-and-connections.txt`.
- Boat sprite definitions exist, but inspection found no server boat-specific collision footprint. The user selected a minimum of 3 deep tiles. This is an explicit generator clearance contract, not a claim of completed boat-runtime testing.
