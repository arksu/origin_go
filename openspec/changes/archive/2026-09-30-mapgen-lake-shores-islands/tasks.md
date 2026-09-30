# Tasks

## 1. Baseline and configuration

- [x] 1.1 Capture fixed-seed current terrain/network golden fixtures and the current H&H values before runtime edits; verify fixtures pass on the existing working tree, including variable river widths, and record that the paused drainage proposal is not part of this change.
- [x] 1.2 Add the twelve lake controls and defaults from `design.md` to river options, YAML loading, and focused validation helpers; verify omitted/false mode preserves legacy output and table tests cover explicit zero, non-finite values, reversed bounds, and incompatible active layout/clearance.
- [x] 1.3 Add checked memory accounting for peak local lake scratch, retained reservations/paths, and bounded searches to terrain and preview estimators; verify overflow/over-budget tests reject before allocation without changing the existing memory limit.

## 2. Option C shore geometry

- [x] 2.1 Add deterministic lake-local shape construction from existing basins with padded two-scale shore displacement; verify zero roughness and same-seed repeatability, outward as well as inward detail, and unchanged lake centers/size classes/main-link selection.
- [x] 2.2 Add shore-connected peninsula candidates with bounded count/depth/attempts and reserved river entry footprints; verify fixed fixtures produce connected peninsulas and never erase or narrow incoming rivers.
- [x] 2.3 Add pre-commit water/land topology validation, incidental-hole removal, clean candidate rejection, and undetailed per-lake fallback; verify detached-pool, unrequested-island, thin-neck, and blocked-inlet fixtures leave no partial edits and report fallbacks.

## 3. Controlled islands and lake banks

- [x] 3.1 Implement separate deterministic class-selection and large-lake count decisions; verify probabilities zero/one, the conditional second-island decision, unchanged selection under contour-detail changes, and multi-seed pre-fit selection statistics.
- [x] 3.2 Implement compact irregular island candidates with dry-radius bounds, an 8% total area cap, bounded attempts, and protected incoming-river exclusion; verify minimum-size rejection, one-of-two acceptance, island separation, and no placement residue.
- [x] 3.3 Generate coherent shallow bands from final mainland and island shores using independent lake limits; verify constant/zero bands, 1-5 tile default bounds in isolated fixtures, and that changing lake-bank settings does not retune river-bank profiles.

## 4. Navigation and final terrain integration

- [x] 4.1 Add new-mode footprint-aware lake routing and valid water hub selection within bounded local search areas; verify peninsula detours, diagonal pinches, all intended inlet joins, and failure diagnostics without allowing land shortcuts or changing legacy routing.
- [x] 4.2 Validate deep circumnavigation and mainland/inter-island gaps after shallow-band resolution, retaining accepted clearance paths; verify three-tile H&H and wider configurable footprints and reject optional features instead of weakening passages.
- [x] 4.3 Carry explicit feature-land reservations through carving, bank dilation, fairway protection, and later tributary placement; verify existing river water cannot be erased by an island and later water paint cannot cut through accepted land.
- [x] 4.4 Integrate reserved land with ground classification, structural biome eligibility, final water resolution, and shoreline sand; verify accepted features stay dry with Perlin water on/off and biomes on/off, surrounding water keeps existing behavior, and elevation/climate buffers remain unchanged.
- [x] 4.5 Extend final-tile validation and per-class selection/request/placement/rejection reporting; verify deliberately flooded islands and broken inlet/circumnavigation paths fail with resolved seed and lake/route/location context.

## 5. Preview and preset documentation

- [x] 5.1 Expose all lake controls in the existing river preview schema with Russian labels/descriptions; verify schema coverage, accepted ranges, and parity between direct network generation, river-only preview, and full terrain preview for fixed seeds.
- [x] 5.2 Cover preset load/edit/save/reload with the new fields, explicit false/zero, comments, and unrelated values; verify round-trip regression tests without adding a preset-file selector or a new preview layer.
- [x] 5.3 Add only the new H&H lake controls and Russian comments, and document probability semantics, dry-radius units, full-feature rollback, island-only disable, and the difference from biome satellites in README; verify no current world/river/biome tuning is overwritten and preset loading succeeds.

## 6. Regression and visual acceptance

- [x] 6.1 Run focused lake, fairway, tile-pipeline, and preset tests, then `go test ./cmd/mapgen/... -count=1`, `node --test cmd/mapgen/preview/index.test.mjs`, `go vet ./cmd/mapgen/...`, a mapgen build to a temporary output, and `git diff --check`; record results and distinguish unrelated pre-existing failures.
- [x] 6.2 Add or extend opt-in lake visual review tooling and generate before/after masks and final-tile close-ups for seeds 12345, 67890, 314159, and the current H&H seed, including forced peninsula and island fixtures; verify deterministic output across worker counts and report intended links, feature counts, rejections, passage validation, timings, and memory.
- [x] 6.3 Run one full current-size H&H preview-only review with no database writes; deliver an overview plus lake/island close-ups and measured cost, obtain user visual acceptance, and do not mark appearance tuning complete merely because geometric tests pass.

Implementation and technical verification are complete. After receiving the revised island images and full-map verification, the user requested sync and archive on 2026-09-30; this request is treated as acceptance for change closure. Task 6.3 is complete. See `verification.md` for results and artifact paths.

## 7. Island silhouette refinement

- [x] 7.1 Replace near-circular island outlines with deterministic rotation, elongation, asymmetric lobes and bays; preserve probability, nominal-size, area and navigation constraints; add shape/topology regressions, render a multi-size gallery, and repeat mapgen tests and the full H&H review after the user's visual feedback.
