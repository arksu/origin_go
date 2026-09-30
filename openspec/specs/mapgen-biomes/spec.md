# mapgen-biomes Specification

## Purpose

Defines deterministic organic land biomes for the offline map generator, including branch size and thickness, climate-driven placement, overlapping terrain masses, small patches, and preservation of structural terrain and intentional satellites.

## Requirements

### Requirement: Regional mountain massifs
Mountain ground SHALL use a deterministic regional noise field with configurable `mountain_massif_scale` in tiles, independent of the other climate scales. The existing mountain ruggedness threshold SHALL select the field's mountain regions. Within those regions, stone outcrops SHALL use coherent noise at configurable `mountain_stone_scale`, rather than independent tile hashes. Mountain and stone SHALL share structural protection; final hydrology SHALL be allowed to fragment their union.

#### Scenario: Large regional scale
- **WHEN** the H&H preset uses a mountain massif scale of 512 tiles
- **THEN** mountain/stone ground before water clipping forms broad connected regions, and stone outcrops do not become independent single-tile speckles

#### Scenario: Invalid mountain scales
- **WHEN** the massif scale is outside [1,4096] or the stone scale is outside [1,mountain_massif_scale]
- **THEN** configuration validation fails with the offending setting named

### Requirement: H&H clay coverage tuning
The H&H preset SHALL target approximately 1.5% clay coverage measured over all final world tiles. Calibration SHALL retain wet seed gating and open-grass painting eligibility and SHALL measure seeds 12345, 67890, and 314159. Density SHALL remain a per-cell probability; this target SHALL NOT imply an exact quota for every seed or world size.

#### Scenario: Coverage is measured after terrain restrictions
- **WHEN** the revised H&H preset is rendered for the three calibration seeds
- **THEN** coverage reporting counts final clay tiles after structural clipping and shoreline processing, divided by the entire world's tile count

### Requirement: Organic shape with independent size and thickness
Each biome patch SHALL use a randomly branching skeleton and a smoothed, optionally ragged outline. Configuration SHALL provide per-type branch-size bounds and an independent outline width. The sampled size sz SHALL control branch lengths, not the maximum distance from the seed. A configured width of zero SHALL select wd = sz / 2. Generation SHALL have finite node/depth limits and SHALL NOT normalize the patch to a radius.

#### Scenario: Original width default
- **WHEN** a patch samples branch size sz and its configured outline width is zero
- **THEN** its outline width is sz / 2 and successive branches may extend farther than sz from the seed

#### Scenario: Independent thickness
- **WHEN** an explicit positive outline width is configured
- **THEN** that width controls thickness independently of the sampled branch size

#### Scenario: Long branching sequence is bounded
- **WHEN** random branching continues to a configured node or depth limit
- **THEN** no nodes beyond that limit are created and mask generation terminates

### Requirement: Connected main mask before terrain restrictions
The main mask of each generated patch SHALL be one 4-connected component containing its seed, before applying structural terrain restrictions or type-specific painting eligibility. Self-intersections, thin features, and world-edge cropping SHALL be handled deterministically. Terrain restrictions SHALL be allowed to split that mask into multiple painted fragments. Satellites SHALL be separate masks and SHALL be excluded from the main-mask connectivity guarantee.

#### Scenario: Crossed or thin outline
- **WHEN** a patch contour self-intersects or narrows during smoothing/raggedness
- **THEN** the resulting unrestricted main mask remains 4-connected and contains its seed

#### Scenario: River splits a patch
- **WHEN** locked river tiles cross a connected main mask
- **THEN** the river remains intact and painted land fragments on its sides are permitted to be disconnected

#### Scenario: World edge cuts a branch
- **WHEN** a generated outline extends outside the world
- **THEN** no out-of-bounds writes occur and the main mask retained inside the world is connected to its seed before terrain restrictions

### Requirement: Suitable seeds and configurable climate selection
Main seeds SHALL be selected only on unlocked grass in the signal-only ground snapshot. Unsuitable candidates SHALL be skipped without relocation. Main selection SHALL use nonnegative forest/heath/moor/swamp weights, an explicit skip weight, and seed climate signals. Forest weight SHALL be biased toward moisture; heath/moor weight toward drier ground; eligible swamp weight toward wetness. Zero total eligible weight SHALL produce a no-op.

The forest cold threshold, moor moisture/temperature thresholds, swamp wetness/moisture thresholds, and clay wetness/moisture thresholds SHALL be configurable through YAML. Forest SHALL be pine below the cold threshold and leaf otherwise. Moor SHALL be eligible below either moor threshold, otherwise heath. Swamp SHALL require wetness or moisture strictly above its respective threshold. Gates SHALL apply at the seed; structural restrictions, rather than per-tile climate gates, SHALL clip a selected patch.

#### Scenario: Candidate lies on protected terrain
- **WHEN** a main grid candidate falls on water, river, mountain, stone paving, or sand
- **THEN** that cell creates no patch and does not move the seed elsewhere

#### Scenario: Swamp gate excludes a dry seed
- **WHEN** seed wetness and moisture are both at or below their configured swamp thresholds
- **THEN** swamp is excluded from its type selection regardless of swamp weight

#### Scenario: Forest temperature threshold
- **WHEN** a selected forest seed has temperature below blob_forest_cold_threshold
- **THEN** its patch type is pine; temperature equal to or above the threshold selects leaf

#### Scenario: Zero candidate weights
- **WHEN** all eligible main candidate weights including skip are zero
- **THEN** the seed leaves existing terrain unchanged

### Requirement: Main overlap precedence and no-op skip
During main painting, swamp SHALL take precedence over heath/moor, and heath/moor SHALL take precedence over forest. Equal-priority overlaps SHALL be resolved by later row-major seed order. A skipped seed SHALL never erase another patch. Tiles not covered by a painted patch SHALL retain their ground classification. Subsequent cleanup and secondary detail SHALL follow their own requirements.

#### Scenario: Wet patch crosses forest
- **WHEN** a swamp main mask overlaps a forest main mask on replaceable ground
- **THEN** the intersection is swamp at the end of main painting, independent of their seed enumeration order

#### Scenario: Equal-priority intersection
- **WHEN** two equal-priority main masks overlap
- **THEN** the later row-major seed supplies the intersection's type

#### Scenario: Skip beside a painted patch
- **WHEN** a main seed selects skip inside an area covered by a neighboring patch
- **THEN** that neighboring patch is not erased

### Requirement: Secondary density and painting eligibility
Secondary patches SHALL use their own configurable seed spacing and per-cell thicket/dirt/clay probabilities. Density SHALL mean selection probability at a suitable secondary seed, not target tile coverage. Thicket SHALL be seeded and painted only on currently pine/leaf tiles. Dirt and clay SHALL be seeded and painted only on currently grass tiles. Clay SHALL additionally require seed wetness or moisture strictly above its configured clay threshold. A failed clay gate SHALL increase the no-op probability without renormalizing dirt probability.

Secondary painting SHALL preserve earlier secondary patches and locked terrain. Suitability SHALL be evaluated in deterministic row-major order without relocation retries. Secondary masks SHALL use the same size/thickness semantics as main patches and SHALL be allowed to fragment after eligibility clipping.

#### Scenario: Thicket crosses a forest boundary
- **WHEN** a thicket mask extends beyond forest onto grass or swamp
- **THEN** only its currently pine/leaf tiles become thicket

#### Scenario: Open-ground details overlap
- **WHEN** a later dirt or clay mask covers grass, forest, and an earlier secondary patch
- **THEN** only currently grass tiles may change

#### Scenario: Density is disabled
- **WHEN** a secondary type's density is zero
- **THEN** no patches of that secondary type are selected

#### Scenario: Clay gate fails
- **WHEN** an open-ground seed fails both clay thresholds
- **THEN** its clay probability becomes a no-op and its dirt probability stays configured

### Requirement: Structural terrain survives all land operations
The biome layer SHALL protect water as determined by enabled elevation-water classification, river water as determined by river classification, and base mountain, stone paving, and sand throughout all land painting and cleanup. Protected cells SHALL be excluded from cleanup neighbor voting and component traversal. Final water/river resolution and shoreline sand SHALL retain their existing precedence and behavior outside explicit drawn-lake feature land reservations and explicitly protected deep river fairways.

When fairway protection is enabled, protected fairway cells SHALL resolve to deep water even where elevation would otherwise select shallow water, and SHALL remain deep through shoreline processing. This exception SHALL NOT alter elevation generation or the precedence of cells outside the protected fairway. With fairway protection disabled, existing resolution behavior SHALL remain unchanged.

Accepted island and peninsula land reservations from the enabled irregular drawn-lake mode SHALL be treated as land for ground classification and biome eligibility even when their underlying elevation would otherwise produce water. Those reservations SHALL remain dry through final water resolution and shoreline processing without changing the underlying elevation or climate fields. Existing mountain, stone, and sand protection SHALL continue to apply within reserved land. Reservations SHALL NOT overlap protected river or lake-navigation footprints; such a conflict SHALL fail validation rather than erasing either feature. Without the lake feature enabled, structural terrain behavior SHALL remain unchanged.

#### Scenario: Mountain and paving meet forest
- **WHEN** main patches and every cleanup pass operate beside or over mountain and stone paving
- **THEN** those protected base tiles retain their original type until final hydrology is resolved

#### Scenario: Water has an unresolved land base
- **WHEN** a tile is destined to become water but its base buffer still contains a land type
- **THEN** no biome painting modifies it and it does not influence cleanup votes as land

#### Scenario: Protected fairway overlaps shallow base water
- **WHEN** an explicitly protected deep river footprint crosses tiles classified as shallow by elevation
- **THEN** the protected footprint resolves to deep water and stays deep after shoreline processing

#### Scenario: Legacy water precedence remains available
- **WHEN** fairway protection is disabled
- **THEN** shallow elevation water retains its existing precedence over river classification

#### Scenario: Reserved island over a Perlin lowland
- **WHEN** an accepted lake island occupies low-elevation tiles with elevation water enabled
- **THEN** its tiles are classified as land, participate in land-biome eligibility subject to other structural locks, and remain dry after final shoreline processing

#### Scenario: Nearby water is not reserved land
- **WHEN** elevation water lies outside an explicit accepted island or peninsula reservation
- **THEN** the new lake-land exception does not change its existing water classification or protection

### Requirement: Cleanup preserves intentional small detail
Biome smoothing, border repair, and tiny-component removal SHALL operate on the main layer before secondary patches and satellites are added. No biome cleanup SHALL run after intentional small detail is painted. Configured zero smoothing passes SHALL disable every biome morphology pass; a minimum-patch size of one SHALL disable tiny-component removal. Structural constraints and final hydrology/shoreline SHALL remain authoritative.

#### Scenario: Satellite is smaller than the cleanup threshold
- **WHEN** an accepted 1-15 tile satellite is added with hnh_min_patch_tiles configured to 24
- **THEN** it is not removed or reshaped by biome cleanup

#### Scenario: Small undergrowth survives
- **WHEN** an accepted secondary patch has fewer tiles than the main-layer cleanup minimum
- **THEN** it is not deleted by that cleanup minimum

#### Scenario: Zero smoothing means no morphology
- **WHEN** hnh_smoothing_passes is zero
- **THEN** no biome smoothing, edge smoothing, or border morphology is applied

### Requirement: Raggedness and separate satellites
Raggedness SHALL bound independent displacement in each coordinate and SHALL preserve a closed contour. Zero SHALL disable displacement. Satellite chance SHALL be interpreted per configured perimeter sampling interval, independently of spline subdivision. Only visible main patches SHALL emit satellites.

An accepted satellite SHALL contain 1-15 unique 4-connected tiles of its parent's type outside the parent's main mask. It SHALL be painted only on unlocked grass inside the world, separated by at least one tile from existing tiles of its parent's type. Invalid or incomplete candidates SHALL be skipped entirely without unbounded retries. Secondary patches SHALL not emit satellites.

#### Scenario: Smooth closed contour
- **WHEN** raggedness is zero
- **THEN** no vertex displacement is applied and the contour remains closed

#### Scenario: Satellite accepted
- **WHEN** a complete satellite candidate satisfies terrain, extent, and separation rules
- **THEN** it is painted as one separate component of the parent's type before final hydrology/shoreline

#### Scenario: Satellite hits locked ground
- **WHEN** any tile of a satellite candidate would overwrite protected terrain or existing non-grass land
- **THEN** that candidate is discarded without partial painting

#### Scenario: Satellite generation disabled
- **WHEN** blob_islet_chance is zero
- **THEN** no satellites are generated

### Requirement: Deterministic terrain output
For a resolved world seed and effective generator configuration, biome tile output SHALL be identical across runs and worker thread counts. Seed selection, geometry, secondary detail, and satellites SHALL use stable deterministic ordering and SHALL not depend on chunk object-scatter randomness. Seed zero SHALL retain the existing time-based CLI seed selection; reproducibility SHALL refer to the resolved seed. This requirement SHALL apply to terrain buffers, not DB entity IDs.

#### Scenario: Worker count changes
- **WHEN** identical resolved seed and effective configuration are generated with one and multiple workers
- **THEN** final terrain tile buffers are byte-identical

#### Scenario: Omitted defaults equal explicit defaults
- **WHEN** one configuration omits biome fields and another explicitly supplies their default values
- **THEN** the same resolved seed produces identical biome terrain

### Requirement: Explicit switches and configuration defaults
The YAML biome section SHALL expose the master enabled switch and blob_enabled, main/secondary spacing and jitter, main/skip weights, secondary densities, per-type size bounds and outline width, climate thresholds, raggedness, satellite chance/spacing, and finite tree limits. Missing biome fields SHALL inherit biome defaults; explicit false and zero SHALL remain effective. An omitted or null biome section SHALL retain defaults.

With enabled false, the generator SHALL use its existing fallback ground classification without blobs or biome cleanup. With enabled true and blob_enabled false, it SHALL use signal-only mountain/stone/sand/grass ground without blobs or biome cleanup. Existing hydrology and shoreline SHALL remain active under both configurations.

#### Scenario: Partial biome section
- **WHEN** a version-1 preset supplies only biomes.blob_raggedness
- **THEN** other biome fields inherit defaults and the supplied raggedness is used

#### Scenario: Explicit disable
- **WHEN** enabled is true and blob_enabled is false
- **THEN** no main patches, secondary patches, satellites, or biome cleanup are applied

#### Scenario: Master switch disabled
- **WHEN** enabled is false regardless of blob_enabled
- **THEN** the existing fallback ground classifier is used and the blob layer is bypassed

### Requirement: Invalid and retired configuration fails early
Unknown or retired biome keys, including hnh_enabled, macro region/share keys, and hnh_variant_density, SHALL be rejected with the offending key named. Configuration SHALL reject non-finite biome numbers, nonpositive spacings, jitter outside [0,1), sizes outside [1,4096] or reversed bounds, widths outside [0,4096], negative or overflowing weights, thresholds/densities outside [0,1], dirt plus clay density above one, raggedness outside [0,64], node limits outside [4,256], and depth limits outside [3,32]. Retained constraints SHALL still apply even when the layer is disabled. The generator SHALL check allocation arithmetic and its precompute memory budget before allocation.

#### Scenario: Retired mode key
- **WHEN** a preset contains hnh_enabled or a retired macro/share key
- **THEN** loading fails with an error naming that key

#### Scenario: Invalid numeric input
- **WHEN** a biome setting is NaN, infinite, out of range, or violates a cross-field bound
- **THEN** generation fails before allocating terrain buffers with an error identifying the setting or bound

#### Scenario: Excessive allocation
- **WHEN** terrain buffers plus lock masks, seed records, and peak shape scratch exceed the precompute budget
- **THEN** generation fails before those allocations with a meaningful budget error
