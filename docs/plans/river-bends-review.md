# Option B implementation review

2026-09-29. Implementation and automated checks complete. The user accepted the
appearance: "Yes, the picture is satisfactory overall." No further visual tuning
was requested. The rejected continent/drainage model remains removed.

## Selected behavior

- Tile-distance broad bends with smaller irregular bends and calmer reaches.
- A minimum **3x3 deep-water footprint** along every accepted main river,
  tributary, junction, narrow link, and lake-inlet passage, checked on final tiles.
- Sparse, non-recursive tributaries: at most `floor(main routes * 0.08)`, one
  per parent, with junctions at least 300 tiles from other junctions/inlets.
- Existing elevation, lake placement, and link-selection policies retained;
  no continent mask, border sea, or center-to-border drainage simulation.
- H&H uses the accepted visual settings; all other shipped presets retain legacy mode.

Accepted tuning: wavelength 400 tiles, frequency scale 0.75, four octaves,
gain 0.5, amplitude scale 1.5. The first review at wavelength 600, gain 0.35,
and amplitude 0.5 looked too straight; the user accepted the current appearance.

## Full-size results

All runs use 6400x6400 tiles and do not access the database or write world chunks.
Before = original H&H shape settings with the new controls disabled.

| Seed | Main routes before / after | Linked lakes before / after | Water components before / after | Tributaries / budget | Minimum junction spacing | Build seconds before / after |
| --- | --- | --- | --- | --- | --- | --- |
| 12345 | 126 / 123 | 101 / 99 | 20 / 23 | 9 / 9 | 303.6 | 7.94 / 8.33 |
| 67890 | 129 / 132 | 104 / 109 | 24 / 29 | 10 / 10 | 308.0 | 8.11 / 8.47 |
| 314159 | 139 / 137 | 121 / 118 | 24 / 28 | 10 / 10 | 304.6 | 8.20 / 8.57 |

Every retained route passes final deep-footprint validation for every seed.
Go reserved heap snapshots reached 1690 MiB in the comparison process, which
retains both variants together; this is not a per-map OS peak-RSS measurement.

**Connectivity review needed:** components are cardinally connected drawn-water
groups touching main routes, not a boat-navigation metric. Counts increased
on all three seeds. Link-selection policy is unchanged, but geometry and
collision rejection change the accepted graph and incidental contacts. This
implementation does not promise one globally connected waterway, and individual
route clearance does not establish whole-world travel quality.

## Images

Generated locally in `map_png/river-bends-review/` (git-ignored):

- `README.md`: measured results and links to all 24 images.
- `<seed>-before-map.png` / `<seed>-after-map.png`: full-map overview.
- `<seed>-before-rivers.png` / `<seed>-after-rivers.png`: waterways only.
- `<seed>-before-bends.png` / `<seed>-after-bends.png`: identical-location crops.
- `<seed>-before-fairway.png` / `<seed>-after-fairway.png`: six-times zoom at a
  new tributary junction; dark bright blue is deep, light cyan is shallow.

Overviews preserve thin river pixels when downsampling. Close-ups use final tile
depth, not just the drawn river class. Review both scales for any future tuning.

Reproduce:

```bash
go test ./cmd/mapgen -run '^TestRiverBendsReview$' -count=1 -timeout 15m -v -args \
  -bends-review-dir "$PWD/map_png/river-bends-review" -bends-review-full
```

## Verification

- `go test ./cmd/mapgen/... -count=1`: passed, including legacy golden buffers,
  width 1/3/5 footprints, lake routing, sparse/crowded branches, preview parity,
  final-tile corruption detection, no-river output, and worker determinism.
- `go vet ./cmd/mapgen/...`: passed after granting access to Go's build cache.
- `go build -o /private/tmp/origin-mapgen-bends ./cmd/mapgen`: passed.
- `openspec validate mapgen-river-bends --strict`: passed.
- Full-size three-seed review: passed in about 53 seconds, including PNG output.
- Boat-runtime collision/movement: **not tested**; this is a generator contract.
- User visual acceptance: **recorded on 2026-09-29**, with no further visual tuning
  requested. This does not establish runtime passage or global connectivity.
  No archive or commit performed.

Rollback: set the six new river controls to zero, restore H&H octave gain 0.06
and amplitude scale 0.5. No existing live world is automatically regenerated.
