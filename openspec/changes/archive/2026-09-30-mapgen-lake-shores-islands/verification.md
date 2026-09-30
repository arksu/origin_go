# Implementation verification

Implementation verified on 2026-09-29. The user requested sync and archive on 2026-09-30 after delivery of revised island images; this request is treated as acceptance for change closure.
The paused drainage/continent proposal is not included. Existing variable-width river work and unrelated working-tree changes are preserved. No database or live world writes were performed.

## Automated checks

All checks passed:

- `go test ./cmd/mapgen/... -count=1` (5.868 seconds).
- `node --test cmd/mapgen/preview/index.test.mjs` (9 tests).
- `go vet ./cmd/mapgen/...`.
- `go build -o /private/tmp/origin-mapgen-lake-preview ./cmd/mapgen`.
- `git diff --check`.

Coverage includes legacy fixed-seed terrain/network hashes, deterministic output across worker counts, zero-chance island exclusion across 16 seeds, probabilistic selection, atomic feature rejection, dry-land preservation, 3-tile and wider deep-water clearance, shallow bands including zero, memory bounds, preview parity, and preset round trips with comments and explicit false/zero values.

## Visual review artifacts

The opt-in review renders before/after final terrain and close-ups without writing to the database:

```sh
go test ./cmd/mapgen -run '^TestLakeGeometryReview$' -count=1 -timeout 15m -v -args -lake-review-dir /private/tmp/origin-lake-review
go test ./cmd/mapgen -run '^TestLakeGeometryReview$' -count=1 -timeout 15m -v -args -lake-review-dir /private/tmp/origin-lake-review-full -lake-review-full
```

- Small review: 1536×1536, seeds 12345, 67890, 314159, and 378605981; passed in 9.41 seconds. Report: `/private/tmp/origin-lake-review/README.md`.
- Full current H&H review: 6400×6400, seed 378605981; passed in 25.38 seconds. Report: `/private/tmp/origin-lake-review-full/README.md`.
- Full overview: `/private/tmp/origin-lake-review-full/378605981-after-map.png`.
- Lake close-up pair: `/private/tmp/origin-lake-review-full/378605981-lake-8-before.png` and `/private/tmp/origin-lake-review-full/378605981-lake-8-after.png`.
- Forced two-island/peninsula fixture: `/private/tmp/origin-lake-review-full/forced-islands.png`.

## Full-map result

- 200 lakes: 132 small, 52 medium, 16 large.
- Medium lakes: 13 selected/requested islands, 9 placed, 4 rejected for geometry.
- Large lakes: 5 selected lakes, 6 requested/placed islands.
- Total: 15 islands in 14 lakes, 402 accepted peninsulas, 3 undetailed fallbacks.
- Main routes: 149, unchanged. Tributaries: 89, unchanged.
- All final deep-water route and reserved dry-land checks passed.
- Generation time: legacy 11.897 seconds; detailed lakes 13.022 seconds in the same review run.
- Go `HeapSys` after the detailed generation: 2015 MiB. The review retains both maps; this is not peak RSS or standalone generator memory usage.

These results establish geometry and gameplay invariants. Appearance was subsequently refined in response to user feedback, as recorded below.

## Island silhouette refinement after visual feedback

The user rejected near-circular islands. The updated contour adds seeded rotation, elongation, asymmetric lobes, a recessed bay, and bounded fine detail. Both dry land and surrounding water are checked for four-connectivity in a padded local mask; invalid shapes are rejected, not fragmented or shrunk. Selection/count randomness, placement limits, area cap, configured radii, and deep-clearance checks remain unchanged. No preset values were edited during this refinement.

Latest checks passed:

- `go test ./cmd/mapgen/... -count=1` (6.048 seconds), including equal-radius asymmetry/bay regressions and connected, hole-free shapes at nominal radii 2, 4, 8, 12, 32, and 64.
- `node --test cmd/mapgen/preview/index.test.mjs` (9 tests).
- `go vet ./cmd/mapgen/...` (rerun with approved cache access after a sandbox cache-permission failure).
- `go build -o /private/tmp/origin-mapgen-lake-preview ./cmd/mapgen`.
- `openspec validate mapgen-lake-shores-islands --strict` and `git diff --check`.

```sh
go test ./cmd/mapgen -run '^TestLake(Geometry|IslandOutline)Review$' -count=1 -timeout 15m -v -args -lake-review-dir /private/tmp/origin-island-shapes-v2-full -lake-review-full
```

The current user-edited H&H preset differs from the initial review recorded above: nominal island radii are 14-55, lake banks are 5 tiles, and island/river probabilities have changed. The new review uses those current values unchanged; island totals across the two reviews are therefore not a contour-only comparison.

Latest full review: 6400×6400, seed 378605981, 200 lakes, 13 placed islands (4 medium, 9 large). All final reserved-land and deep-water route checks pass. Main routes remain 56 and tributaries 33 between the legacy/detail variants. Detailed generation takes 12.560 seconds versus 11.220 seconds for the legacy mode; Go `HeapSys` is 1938 MiB while retaining both maps, not standalone peak RSS. The full review test completes in 24.23 seconds.

New artifacts:

- `/private/tmp/origin-island-shapes-v2-full/island-shapes.png`: 12 contour examples; top/middle/bottom rows have nominal radii 4/8/12. The gallery uses a fixed two-tile bank to expose the outline, not the current preset's bank width.
- `/private/tmp/origin-island-shapes-v2-full/378605981-lake-2-after.png`: actual final lake with an irregular island using the current H&H preset.
- `/private/tmp/origin-island-shapes-v2-full/378605981-after-map.png`: full overview.
- `/private/tmp/origin-island-shapes-v2-full/README.md`: complete full-map review report.

Tasks 7.1 and 6.3 are complete. The user's 2026-09-30 sync-and-archive request closes the revised-appearance review.
