# Design

## Context

See proposal.md for motivation. The current generator precomputes the entire world in `TerrainPrecompute`; chunk workers copy final tiles and scatter objects. The biome layer uses macro families, hash variants, and four cleanup passes. Its cleanup lock predicate currently protects water and sand, but not mountain or stone paving. Water/river resolution and shoreline sand run after the land layer.

The reference is the repository copy `docs/features/patch.py` (loftar, forum attachment t=11222). It exposes tree/outline/drawing code but imports unavailable `utils` helpers. We preserve its two independent scales and branching structure; spline and raster behavior are explicitly defined adaptations, not a claim of byte-for-byte equivalence to H&H.

## Goals / Non-Goals

**Goals:**
- Branching, overlapping terrain masses with clearings, forest undergrowth, wet ground, and surviving satellite patches.
- Finite, deterministic generation independent of worker count.
- Structural terrain protected throughout land painting and cleanup.
- Complete YAML semantics and testable placement/overlap rules.

**Non-Goals:**
- Changing elevation, river layout, water resolution, shoreline sand, object scatter, DB format, protocol, server, or client.
- Resource quality fields, guaranteed occurrence of every biome, or runtime generation.
- Additional dependencies or the unavailable H&H helper library.

## Decisions

### D1: Preserve the original scales: branch size sz and outline width wd

For each patch type `t`, sample `sz` uniformly from `blob_<t>_size_min` to `blob_<t>_size_max` (constant when equal). Set `wd = blob_<t>_width` when positive; zero means `wd = sz / 2`, exactly the original `randpatch` default.

Both values are in tiles. `sz` controls each branch length, not distance from the seed. `wd` controls thickness around the skeleton. Do not normalize or clip the shape to a sampled radius. Actual extent follows the tree, width, smoothing, and raggedness; satellites extend it further.

- `genBlobTree`: BFS from the selected seed. Continue unconditionally while fewer than four nodes exist, then with probability 0.5. Continuing nodes produce one child with probability 0.8 or two with probability 0.2, adopting the documented weighted-index interpretation of `wrandoom([0,4,1])`. Use the original angular sectors and uniformly sampled branch lengths in `[0.9*sz, 1.1*sz]`.
- Stop adding nodes at `blob_max_nodes` or `blob_max_depth` (root depth zero); process remaining queued nodes as leaves. These limits guarantee finite work instead of relying on probabilistic termination.
- `genBlobOutline`: preserve the original ordered walk through parent/child adjacency, leaf caps, and perpendicular offsets using sampled widths in `[0.8*wd, 1.2*wd]`. Do not assert that the raw contour is simple or contains all skeleton points.
- Keep geometric coordinates as floats until rasterization; this avoids collapsed branches at small sizes. This rounding behavior is an intentional adaptation.

### D2: Define smoothing and a robust connected main mask

Use a closed cubic Hermite spline through the outline. At vertex `p_i`, tangent direction is `p_(i+1) - p_(i-1)`, with magnitude `wd` (zero for a zero direction). Use the usual cubic Hermite basis; sample each segment with `max(1, ceil((segment chord length + 2*wd) / 3))` subdivisions. Remove consecutive duplicate outline points first. This specifies a replacement for the missing `utils.spline` without claiming it was Catmull-Rom.

Displace each distinct sampled vertex independently in each coordinate by a uniform value in `[-blob_raggedness, +blob_raggedness]`; reuse the first displaced point when closing the contour. Zero disables displacement.

Construct a scratch mask within the bounding box of the displaced contour plus skeleton capsules/supercovers, intersected with the world:
1. Even-odd scanline fill sampled at tile centers, with half-open edge intersections; include a 4-connected supercover of the contour boundary.
2. Union the filled mask with rasterized capsules around the skeleton edges, radius `0.8*wd`, and a 4-connected supercover of each skeleton edge, including the root. These corridors preserve organic branches through self-intersections and spline pinches; the supercover also keeps sub-tile diagonal branches connected.
3. Retain only the 4-connected component containing the seed; include the seed tile explicitly for sub-tile degeneracies.

The result, after world-edge cropping but **before terrain restrictions**, is the connected main mask. Detached raster lobes are discarded. This is an explicit topology repair, not equivalence to the original flood fills. Restricting that mask to usable terrain may produce multiple fragments or an empty result; no reconnecting across water or mountains.

Reuse scratch storage per patch. Retain only small seed/type/size records, not a full-world bitmap for every patch. Satellite generation can reconstruct a parent's geometric mask from its independent deterministic RNG stream.

Alternative: direct flood fill against drawn borders. Rejected because border leakage and clipped/self-intersecting contours make behavior harder to bound. The tree corridors preserve branches while scanline filling gives bounded raster work.

### D3: Structural mask and pipeline order

Ground classification retains the climate sand rule and ruggedness mountain/stone threshold, with grass elsewhere. Mountain ruggedness is a separate domain-warped, normalized Perlin field with tile-space lattice scale `mountain_massif_scale` (default 512). Sample at `sampleX / (CoordPerTile * scale) + 6000` and likewise Y. This scale is independent of the other climate fields and replaces the former fine erosion/weirdness mixture. Inside its threshold mask, coherent smooth coordinate-hash noise at `mountain_stone_scale` (default 32 tiles) selects stone below 0.2 and mountain otherwise; both types belong to one structural massif. Final water/river clipping may split a massif. Delete macro families and land hash speckles.

Create an immutable structural lock mask from elevation, `RiverClass`, and ground classification. A tile is locked if final water/river resolution would make it water, or its base type is mountain, stone paving, or sand. This does not require early mutation of the base tile buffer.

Pipeline:
1. Elevation and river precompute.
2. Signal-only ground classification and structural lock mask.
3. Main forest/heath/moor/swamp patches.
4. Main-layer cleanup, with the structural mask.
5. Secondary thicket/dirt/clay patches.
6. Intentional satellites of visible main patches.
7. Existing `resolveTileType` and `applyShorelineSand`.

Every land painter and cleanup pass skips locked cells. Cleanup also excludes locked neighbors from votes and component traversal, so hidden ground under water cannot influence visible biomes. Final hydrology always wins; shoreline sand may replace eligible land as before.

### D4: Main seeds, climate bias, and overlap precedence

Place one candidate at the center of each `blob_seed_spacing` grid cell intersecting the world. For partial edge cells use their actual center. Add independent uniform X/Y offsets bounded by `blob_seed_jitter * half the actual cell dimension`; clamp inside that cell and map to a tile. Jitter is a fraction in `[0,1)`. Visit cells in row-major order.

A main seed is suitable only when unlocked grass. If unsuitable, skip the cell without relocation or retry. Main selections use the unpainted ground snapshot so overlap does not change seed eligibility. Geometry uses tile-space coordinates with the seed at that tile's center.

At the seed, eligible candidate weights are:
- forest: `blob_forest_weight * (0.5 + Moisture)`;
- moor when `Moisture < blob_moor_moisture_max OR Temperature < blob_moor_temperature_max`, otherwise heath: the selected type's weight multiplied by `(1.5 - Moisture)`;
- swamp only when `Wetness > blob_swamp_wetness_min OR Moisture > blob_swamp_moisture_min`: `blob_swamp_weight * (0.5 + Wetness)`;
- skip: `blob_skip_weight`.

Zero weights remove candidates. Normalize eligible weights and perform one categorical draw in the fixed order forest, heath, moor, swamp, skip; if their sum is zero, skip. Forest uses pine when `Temperature < blob_forest_cold_threshold`, leaf otherwise. Seed signals select the whole patch; there is no per-tile climate clipping beyond structural restrictions.

Paint selected main patches in increasing priority: forest, heath/moor, swamp. A higher priority wins intersections; at equal priority the later row-major seed wins. Heath and moor share one priority. This lets open heath/moor cut clearings into forest and wet patches interrupt both. Precedence describes the main painting stage; cleanup may subsequently adjust its borders.

Skip is a no-op: it never erases a neighboring patch. Grass is the unpainted background, not a separately painted biome. Climate bias promotes regional tendencies without promising identical neighboring choices or a minimum biome share.

### D5: Secondary density and terrain-limited painting

Use a separate grid with `blob_secondary_spacing` and the same jitter definition. Visit it once in row-major order after main cleanup. Evaluate the current seed tile; there are no relocation retries.
- Pine/leaf seed: select thicket with probability `blob_thicket_density`, otherwise skip.
- Grass seed: dirt has probability `blob_dirt_density`; clay has probability `blob_clay_density` only if `Wetness > blob_clay_wetness_min OR Moisture > blob_clay_moisture_min`; remaining probability skips. Use fixed dirt, clay, skip intervals without renormalizing after a clay gate fails.
- Other tile types or locked seeds: skip.

Each density is a probability per suitable secondary cell, not a tile coverage target. Approximate attempts per unit suitable area are `density / blob_secondary_spacing^2`; actual painted coverage also depends on shape and overlaps.

Thicket paints only currently pine/leaf tiles. Dirt and clay paint only currently grass tiles. Earlier secondary patches are preserved; a seed whose type was already replaced is skipped. The connected raw mask may fragment under these eligibility restrictions. Main and secondary patches share sz/wd geometry but have separate size ranges. No satellites for secondary types.

### D6: Cleanup preserves structures; intentional detail follows it

Retain the four existing cleanup routines with an explicit structural-mask argument. For the main layer run `smoothBiomeTiles`, `smoothBiomeEdges`, and `cleanBiomeBorderArtifacts` with exactly `hnh_smoothing_passes`, then `removeTinyBiomePatches` with `hnh_min_patch_tiles`. Zero smoothing passes disables all three morphology passes; remove the existing `max(1, passes)` behavior. Tiny-component removal is disabled at a minimum of one.

Small secondary patches and intentional satellites are painted **after every cleanup routine**, so they are excluded from tiny-component removal and smoothing by construction. No morphology or tiny-patch pass runs afterward. Avoid a per-tile island metadata system when phase order provides the required protection.

Hydrology, shoreline sand, and later terrain restrictions can still cut small patches; cleanup preservation never takes precedence over structural terrain.

### D7: Satellites with explicit frequency and separation

For each visible main patch, sample its displaced contour at approximately `blob_islet_spacing` tiles of arc length. Draw one Bernoulli trial per sample with `blob_islet_chance`. Only an exposed segment beside tiles still matching the parent's type may emit a satellite.

Choose an outward candidate 5-7 tiles along a segment normal; test the two normal signs against the parent's connected main mask and reject when neither gives an outside point. Grow a unique 4-connected cluster of a uniformly sampled 1-15 tiles from a random frontier, following the spirit of `mkcpp`. Bound growth by 64 proposals; reject an incomplete cluster without retry.

Accept the entire cluster only if every tile is inside the world, outside the parent's main mask, unlocked, and currently grass, with at least one tile of separation from existing tiles of the parent's type, including its earlier satellites (8-neighbor clearance). Otherwise skip it without partial painting. Accepted satellites keep the parent's type and are separate connected components when placed. Later hydrology/shoreline can truncate them; connectivity after those restrictions is not guaranteed.

This makes islet frequency independent of spline subdivision and prevents small satellites from overwriting undergrowth or exposed dirt/clay.

### D8: Flat YAML schema, defaults, switches, and validation

All settings belong to `biomes` in version-1 presets:

| Keys | Meaning / default |
| --- | --- |
| `enabled` | Master biome switch; existing default true |
| `blob_enabled` | New layer switch; default true |
| `blob_seed_spacing`, `blob_seed_jitter` | Main grid, default 260 tiles / 0.45 fraction |
| `blob_secondary_spacing` | Secondary grid, default 48 tiles |
| `blob_skip_weight` | Main no-op weight, default 0.45 |
| `blob_<t>_size_min`, `blob_<t>_size_max`, `blob_<t>_width` | sz bounds and explicit wd; width defaults to 0 (sz/2); t is forest, heath, moor, swamp, thicket, dirt, clay |
| `blob_<t>_weight` | Main weights only: forest 1.0, heath 0.8, moor 0.8, swamp 0.7 |
| `blob_thicket_density`, `blob_dirt_density`, `blob_clay_density` | Secondary probabilities, defaults 0.35 / 0.08 / 0.05 |
| `blob_forest_cold_threshold` | Pine/leaf split, default 0.45 |
| `blob_moor_moisture_max`, `blob_moor_temperature_max` | Moor split, defaults 0.36 / 0.42 |
| `blob_swamp_wetness_min`, `blob_swamp_moisture_min` | Swamp OR gate, defaults 0.55 / 0.72 |
| `blob_clay_wetness_min`, `blob_clay_moisture_min` | Clay OR gate, defaults 0.60 / 0.64 |
| `blob_raggedness` | Per-coordinate jitter in tiles, default 6 |
| `blob_islet_chance`, `blob_islet_spacing` | Chance per perimeter sample / arc spacing, defaults 0.25 / 12 tiles |
| `blob_max_nodes`, `blob_max_depth` | Tree safety limits, defaults 64 / 16 |
| `mountain_massif_scale`, `mountain_stone_scale` | Regional mountain lattice / stone-outcrop scales, defaults 512 / 32 tiles |

Initial sz bounds: forest 45-110, heath 25-65, moor 25-60, swamp 35-85, thicket 4-10, dirt 3-8, clay 3-7. These are branch lengths, and deliberately smaller than the retired proposed radius values.

Switch semantics:
- `enabled: false`: retain existing `classifyBaseTile` fallback, hydrology, and shoreline; do not run blobs or biome cleanup.
- `enabled: true, blob_enabled: false`: signal-only mountain/stone/sand/grass base; no blobs, secondary patches, satellites, or biome cleanup.
- Both true: full pipeline above.

Retire `hnh_enabled` along with `hnh_region_count`, `hnh_region_jitter`, `hnh_blend_width`, `hnh_variant_density`, and all five `hnh_*_share` keys. The new layer has no separate H&H variant mode. Keep `hnh_smoothing_passes`, `hnh_min_patch_tiles`, `hnh_swamp_clump_scale`, `hnh_mountain_rugged_threshold`, the existing signal scales, and `domain_warp_strength`. No `blob_*_radius_*` keys or secondary `*_weight` keys are introduced.

Initialize the biome section with `DefaultMapgenOptions().Biome` before strict YAML decoding, so missing biome fields inherit defaults and explicit false/zero remains effective. An omitted or null biome section keeps defaults. Preserve the current loading behavior of other sections. Unknown/retired keys remain errors naming the key; `version: 1` remains required.

Validation applies even when a layer is disabled:
- All biome floats, including retained fields, must be finite.
- Grid/perimeter spacings are positive integers; jitter is in `[0,1)`.
- Each sz bound is in `[1,4096]` with min <= max; width is in `[0,4096]`.
- Weights are nonnegative; their sum and that sum multiplied by the maximum climate multiplier (1.5) must be finite; all-zero selection is a valid no-op.
- Gates and densities are in `[0,1]`; dirt density + clay density <= 1.
- Raggedness is in `[0,64]`; max nodes in `[4,256]`, max depth in `[3,32]`.
- Retained smoothing/minimum-patch/signal constraints still apply.
- Mountain massif scale is in `[1,4096]` tiles; stone scale is in `[1,mountain_massif_scale]` tiles. The retained ruggedness threshold now applies to the regional field.
- Check arithmetic and the existing precompute memory cap for structural locks, seed records, and peak reusable raster/queue scratch as well as existing buffers, before allocation.

### D9: Determinism and tuning acceptance

Use independent RNG streams derived by a stable seed mixer from the effective world seed, pass salt, grid cell coordinates, and purpose (selection, main geometry, secondary geometry, satellites). A geometry reconstruction uses the same geometry stream. No stream is shared with chunk object scatter; no map iteration order or worker scheduling affects generation. Paint each phase single-threaded in its specified order.

`seed: 0` retains the CLI's time-based seed selection. Determinism is defined for the resolved nonzero seed and terrain output; this change does not promise deterministic DB entity IDs. All presets explicitly set the new switches; `hnh.yaml` uses seed 12345 for comparisons.

Use the actual CLI flags:
`go run ./cmd/mapgen -gen-config etc/mapgen/presets/hnh.yaml -seed 12345 -png-overview-only -png-dir /tmp/mapgen-blob-biomes/after`
Overview-only implies PNG export and skips DB access. Render the baseline from an isolated checkout with its existing preset and the same seed/dimensions; do not stash unrelated edits or use `map_gen.sh` for previews.

Validate the live-world aesthetic with overviews and close crops for seeds 12345, 67890, and 314159: branching coherent masses, clear grass spaces, exposed wet regions, forest-contained undergrowth, and visible surviving satellites where accepted. Record per-type coverage and component-size summaries alongside visual results; do not claim weights are area shares. Log main-mask, cleanup, secondary, and satellite timings against elevation/river passes.

## Risks / Trade-offs

- [Old share-based coverage and macro mountain regions disappear] -> tune weights, branch sizes, spacing, and the ruggedness threshold from multi-seed previews; no biome presence guarantee.
- [Missing original helpers and raster rounding change aesthetics] -> use the repository reference for sz/wd and branching, while testing the explicit spline/mask adaptations on crossing, thin, and edge-clipped trees.
- [Skeleton corridors can round some outline pinches] -> keep their radius at the minimum reference thickness (0.8*wd) and review detailed crops.
- [Phase precedence biases overlapping coverage] -> expose selection weights and report actual coverage; swamp takes priority during main painting.
- [Very dense or large configurations increase work and scratch memory] -> finite tree limits, checked allocations, reusable scratch, and measured timings.
- [Strict key retirement breaks old external presets] -> named errors, complete in-repo preset migration, and documented replacements.
- [Small detail can be clipped by hydrology/shoreline] -> preserve structural correctness; verify cleanup preservation separately from final shoreline effects.

## Migration Plan

1. Implement the tasks and migrate all three presets, config tests, and retired-field logging in the same change.
2. Pass mapgen tests, vet, and build; render isolated before/after comparisons and multi-seed detailed previews without DB access.
3. Regenerate a world only after visual acceptance; existing world content is unchanged until regeneration. Rollback uses the previous generator/preset and regeneration.

## Open Questions

No unresolved implementation decisions. Exact preset coverage and visual balance remain tuning work under D9; the topology, switches, priorities, gates, and density semantics above are fixed.


## Implementation verification — 2026-09-27

All 19 implementation tasks are complete. `go test ./cmd/mapgen/...`, `go vet ./cmd/mapgen/...`, `go build ./...`, and `openspec validate mapgen-blob-biomes --strict` passed. Tests cover raw-mask connectivity, terrain clipping, overlap priority, immutable cleanup locks, conditional secondary painting, satellite separation, invalid/retired YAML, allocation bounds, and byte-identical final buffers across repeat runs and 1/4 workers. Both disabled modes and survival below the cleanup minimum have final-buffer regression coverage.

### Reproducible preview and tuning

The baseline was rendered from `git archive HEAD` extracted before implementation into `/tmp/mapgen-blob-biomes/baseline-source`, using the old H&H preset at seed 12345. Source revision: `adc3f21e4be54f41b2666c0daaa1ce0245312370`. Baseline and revised worlds are both 50×50 chunks, 6400×6400 tiles. Every preview used `-png-overview-only`; the database was not accessed. Full-resolution outputs remain in `/tmp/mapgen-blob-biomes/{before,after,seed-67890,seed-314159}/overview.png`.

The revised invocation is the D9 command above (with `GOCACHE=/tmp/mapgen-blob-biomes/go-cache` for this sandbox). Seeds 67890 and 314159 use the same command with their seed and output directory substituted. Resource measurements run the compiled binary, include PNG export, and use Python `resource.getrusage(RUSAGE_CHILDREN).ru_maxrss` (bytes on macOS) for observed peak RSS. Phase values come from `TerrainPrecompute.Timings`, in seconds below. These are local observations, not performance guarantees.

The initial preset gave 58.18% grass and 8.31% pine/leaf. Visual tuning in `hnh.yaml` increases main density and branch sizes: spacing 230; forest sz 70–150 / weight 1.6; heath 35–85; moor 30–75; swamp 45–100; skip weight 0.30; raggedness 3; ruggedness threshold 0.60. Width remains 0 (sz/2) for every type. D8 program defaults and all selection/geometry semantics are unchanged. H&H exposes all new YAML controls explicitly; default and Minecraft-like presets inherit the common blob defaults.

[Before/after comparison](previews/comparison.png), baseline on the left and revised seed 12345 on the right. Overview reductions use nearest-neighbor sampling; inspect the crops for tile-scale details.

[Three-seed detail sheet](previews/details-contact.png): columns are 12345, 67890, 314159; rows are forest/clearings, swamp, thicket, dirt, clay. Individual crops retain 256×256 terrain tiles at 3× scale:

| Seed | Overview | Forest | Swamp | Thicket | Dirt | Clay | Raw statistics |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 12345 | [map](previews/12345-overview.png) | [crop](previews/12345-leaf.png) | [crop](previews/12345-swamp.png) | [crop](previews/12345-thicket.png) | [crop](previews/12345-dirt.png) | [crop](previews/12345-clay.png) | [JSON](previews/12345-stats.json) |
| 67890 | [map](previews/67890-overview.png) | [crop](previews/67890-leaf.png) | [crop](previews/67890-swamp.png) | [crop](previews/67890-thicket.png) | [crop](previews/67890-dirt.png) | [crop](previews/67890-clay.png) | [JSON](previews/67890-stats.json) |
| 314159 | [map](previews/314159-overview.png) | [crop](previews/314159-leaf.png) | [crop](previews/314159-swamp.png) | [crop](previews/314159-thicket.png) | [crop](previews/314159-dirt.png) | [crop](previews/314159-clay.png) | [JSON](previews/314159-stats.json) |

Visual acceptance: all three inspected overviews and the detail sheet show branching forest masses with open grass corridors, overlapping heath/moor clearings, wet patches, undergrowth limited to forest, and separate open-ground dirt/clay. Structural mountain/stone patches and hydrology remain visible. The existing elevation produces many small water bodies, which continue to cut land components as specified. Accepted as the implementation's multi-seed visual check; this does not authorize world regeneration.

### Coverage and components

Percentages are fractions of the entire 40,960,000-tile world, not target shares or percentages of land. Statistics are measured from the full-resolution lossless palette PNG. Land colors map one-to-one to tile types; water is combined because river highlighting overrides its display color. Raw JSON includes per-color tile counts, 4-connected component counts, minimum/median/maximum sizes, and 1–15 tile component counts. Small final components include terrain clipping as well as deliberate satellites.

| Type | 12345 | 67890 | 314159 |
| --- | ---: | ---: | ---: |
| water total | 24.652% | 23.826% | 23.419% |
| sand | 0.533% | 0.494% | 0.450% |
| pine | 7.096% | 9.527% | 9.455% |
| leaf | 14.235% | 13.328% | 15.945% |
| thicket | 1.096% | 1.200% | 1.353% |
| grass | 39.025% | 38.694% | 37.117% |
| heath | 4.079% | 2.901% | 3.066% |
| moor | 1.650% | 2.657% | 1.953% |
| swamp | 4.820% | 4.731% | 4.694% |
| clay | 0.092% | 0.087% | 0.070% |
| dirt | 0.332% | 0.329% | 0.340% |
| stone | 0.477% | 0.445% | 0.429% |
| mountain | 1.913% | 1.780% | 1.710% |

| Largest connected land component (tiles) | 12345 | 67890 | 314159 |
| --- | ---: | ---: | ---: |
| pine | 93125 | 134904 | 120411 |
| leaf | 178368 | 172143 | 249667 |
| grass | 1451859 | 1735604 | 1783272 |
| heath | 69574 | 40013 | 55971 |
| moor | 25903 | 56741 | 34444 |
| swamp | 68252 | 67909 | 74338 |
| thicket | 3000 | 1618 | 1779 |
| dirt | 1201 | 1296 | 1792 |
| clay | 954 | 772 | 561 |

At seed 12345, a second final render with only `blob_islet_chance: 0` changed was compared tile by tile. With satellites enabled, 37,753 added biome tiles survive in 5,211 separate components, each 1–15 tiles; 26 of the 37,779 initially accepted tiles are lost to shoreline. [Satellite comparison](previews/satellites-comparison.png) shows a 96×96 crop at 6×, disabled on the left and enabled on the right; the example includes a 15-tile leaf satellite at (5394,1087). [Exact counts and crop bounds](previews/satellites-stats.json). Final-buffer regression tests independently verify that cleanup minimum 24 preserves small secondary patches and satellites.

### Timing and memory

| Metric | 12345 | 67890 | 314159 |
| --- | ---: | ---: | ---: |
| Elevation (s) | 0.033 | 0.030 | 0.033 |
| Rivers (s) | 0.481 | 0.457 | 0.464 |
| Ground (s) | 0.234 | 0.237 | 0.234 |
| Main (s) | 0.698 | 0.734 | 0.764 |
| Cleanup (s) | 4.767 | 4.737 | 4.803 |
| Secondary (s) | 0.501 | 0.516 | 0.525 |
| Satellites (s) | 0.620 | 0.659 | 0.689 |
| Hydrology (s) | 0.544 | 0.554 | 0.554 |
| Wall time including PNG (s) | 9.87 | 9.93 | 10.05 |
| Peak RSS (MiB) | 1130.3 | 840.0 | 1123.4 |
| main_patches | 509 | 509 | 525 |
| secondary_patches | 2191 | 2266 | 2486 |
| islets | 5211 | 5433 | 5479 |
| islet_tiles | 37779 | 38604 | 38774 |

Main-mask and satellite reconstruction each take less than 0.8 s in these runs; the retained three-pass cleanup is the dominant stage at about 4.8 s. Measured peak RSS stays below the 3 GiB precompute safety cap. Existing world content was not regenerated.

## Follow-up verification: clay and mountain massifs — 2026-09-27

The user's subsequent revision targets approximately 1.5% clay over all world tiles and large mountain massifs. These results supersede the initial clay/mountain tuning above; the earlier previews remain available for comparison.

`hnh.yaml` now uses clay sz 12–26, wd=sz/2, and density 0.12 at the existing secondary spacing 48. Wet seed gates and grass-only painting remain in force. Density is still a probability, so coverage varies by seed rather than enforcing a global quota. The three-seed mean is 1.475%.

Mountain ruggedness now comes from the independent regional Perlin field specified in D3. The H&H preset uses massif scale 512 tiles, threshold 0.72, and coherent stone-outcrop scale 32 tiles. No full-world extra buffer is needed. Stone and mountain remain protected by the existing structural mask; final water/river resolution can cut them. Other presets inherit the new scale defaults and retain their configured threshold; only the H&H clay settings were tuned to 1.5%.

All previews use 50×50 chunks / 6400×6400 tiles and `-png-overview-only`, with no database access. Full-resolution PNGs and phase logs are at `/tmp/mapgen-blob-biomes/massifs-<seed>/overview.png` and `/tmp/mapgen-blob-biomes/massifs-<seed>.log`. The compiled binary was built with `GOCACHE=/tmp/mapgen-blob-biomes/go-cache go build -o /tmp/mapgen-blob-biomes/mapgen-massifs ./cmd/mapgen` and invoked with the H&H preset and each seed.

| Final terrain metric | 12345 | 67890 | 314159 |
| --- | ---: | ---: | ---: |
| Clay coverage, entire world | 1.463% | 1.569% | 1.393% |
| Largest clay component, tiles | 9,291 | 9,374 | 7,569 |
| Mountain coverage, entire world | 6.600% | 7.056% | 5.262% |
| Stone coverage, entire world | 0.834% | 0.772% | 0.582% |
| Largest mountain-only component, tiles | 200,252 | 148,787 | 92,503 |
| Largest stone component, tiles | 4,900 | 5,210 | 3,929 |
| Render wall time including PNG, seconds | 9.27 | 9.35 | 9.67 |

Statistics again count 4-connected components of each final tile type separately. Mountain/stone union components can be larger than the mountain-only values. On seed 12345 the previous largest mountain-only component was 1,318 tiles and the previous largest stone component was 18 tiles.

- [Before/after at seed 12345](previews/massifs-comparison.png): previous blob preset on the left, revised clay/massifs on the right.
- [Three-seed detail sheet](previews/massifs-details.png): columns 12345, 67890, 314159; mountain crops on top (512×512 tiles), clay crops below (256×256 tiles).
- Overviews: [12345](previews/massifs-12345-overview.png), [67890](previews/massifs-67890-overview.png), [314159](previews/massifs-314159-overview.png).
- Full palette statistics: [12345](previews/massifs-12345-stats.json), [67890](previews/massifs-67890-stats.json), [314159](previews/massifs-314159-stats.json).

Inspected all three overviews and the detailed crops: mountains form broad visible masses with coherent light stone outcrops; clay forms medium organic fields on open ground. Existing elevation water cuts remain visible within massifs. The terrain-focused regression verifies a massif of at least 50,000 tiles before hydrology at regional scale 512 and low stone-neighbor transition frequency. Existing final-buffer tests verify deterministic runs/workers and mountain/stone protection. Finite/range validation covers both new YAML scales.

`go test ./cmd/mapgen/...`, `go vet ./cmd/mapgen/...`, `go build ./...`, and strict OpenSpec validation passed. All 21 tasks are complete. The world was not regenerated and no implementation commit was created.
