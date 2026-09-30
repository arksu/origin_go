# Spec Delta

## Purpose

Define visually irregular, gameplay-oriented drawn lakes with bays, peninsulas, optional small islands, and verified deep-water boat passages without simulating physical drainage.

## ADDED Requirements

### Requirement: Option C lake outlines
The generator SHALL support opt-in lake outlines containing broad irregular bays, shore-connected peninsulas, and restrained smaller shoreline irregularities. Each accepted lake shape SHALL retain one four-connected water body and all its intended river entrances. Shore detail SHALL NOT create detached pools, incidental islands, or disconnected peninsula fragments. Invalid optional shape candidates SHALL be skipped without partially modifying the map.

#### Scenario: Peninsula with usable open water
- **WHEN** an Option C candidate peninsula fits without dividing the lake or blocking an intended entrance
- **THEN** the peninsula remains attached to surrounding land and the lake retains connected open water around it

#### Scenario: Shore detail creates a detached fragment
- **WHEN** proposed roughness or a land intrusion would leave a detached pool, unrequested island, or broken peninsula attachment
- **THEN** that defect is removed or the candidate is rejected before it becomes the accepted lake shape

### Requirement: Lake detail preserves the drawn layout
The new lake mode SHALL preserve existing lake-placement and size-class policies, resolved centers, and intended main river connections. Its feature-selection randomness SHALL NOT perturb main route selection. It SHALL NOT introduce terrain drainage, a continent, boundary oceans, or a world-center-to-border flow model. Existing river-width, shallow-bank, and tributary settings SHALL retain their meaning and SHALL NOT be retuned as part of enabling lake detail.

#### Scenario: Only lake controls change
- **WHEN** Option C is toggled for the same seed and otherwise identical configuration
- **THEN** lake placements and intended main connections remain unchanged while local lake shore and internal navigation geometry can change

#### Scenario: A decorative feature conflicts with a river
- **WHEN** a proposed island or peninsula intersects an existing incoming river footprint
- **THEN** the feature is skipped or placed elsewhere within its bounded attempts instead of deleting or narrowing the river

### Requirement: Per-lake island probability and conditional counts
Island selection SHALL use one deterministic probability decision per lake, based on its size class, independently of raster resolution and shoreline subdivision. The H&H preset SHALL use small 0%, medium 15%, and large 35%. A selected small or medium lake SHALL request one island. A selected large lake SHALL request two with conditional probability 20%, otherwise one. Actual placements SHALL be allowed to fall below requested counts because of fit constraints; rejected candidates SHALL NOT cause selection rerolls or compensating placements in other lakes.

#### Scenario: Selected large lake requests two islands
- **WHEN** the large-lake selection and second-island decisions both succeed
- **THEN** at most two islands are placed and each must independently satisfy the placement rules

#### Scenario: Only one of two requested islands fits
- **WHEN** one island fits but the second exhausts its permitted placement attempts
- **THEN** the first remains and the second is reported as unplaced without altering another lake's selection

#### Scenario: Zero and one probabilities
- **WHEN** a class probability is zero or one
- **THEN** no lakes or all lakes of that class respectively are selected for an island attempt, without implying that all requested islands fit

### Requirement: Compact islands and complete disable semantics
An accepted island SHALL be a compact four-connected dry land component with an irregular outline, wholly inside its lake and separate from mainland and other islands. It SHALL satisfy the configured minimum and maximum dry-size bounds and SHALL NOT be shrunk below the minimum to force placement. Total accepted island dry area SHALL be no more than 8% of the lake's pre-island water area. With all class probabilities zero, the new lake mode SHALL produce no islands, including incidental holes from shore generation. Lake islands SHALL remain distinct from land-biome satellite patches.

#### Scenario: Insufficient space
- **WHEN** no location can accommodate the minimum island size, shallow margins, and deep-water clearance
- **THEN** the island is skipped without a partial island, one-tile residue, or reduced navigation clearance

#### Scenario: Island silhouettes at equal size
- **WHEN** islands with the same configured nominal radius are generated across seeds
- **THEN** their outlines vary in orientation, elongation, asymmetric lobes and bays rather than only scaling near-circular spots, and each accepted island remains one dry component without enclosed water holes

#### Scenario: Islands disabled while irregular shores remain enabled
- **WHEN** all three island class probabilities are zero and Option C remains enabled
- **THEN** irregular shores and peninsulas remain active but no isolated dry components are introduced inside those lakes

### Requirement: Navigable islands and preserved lake passages
Every accepted island SHALL allow deep-water circumnavigation and separation from mainland and other islands using the configured square boat footprint after shallow bands are applied. All intended river entrances into a lake SHALL connect through navigable lake water. H&H SHALL retain a minimum footprint width of three deep tiles. Clearance SHALL be tested over complete connected footprint placements rather than centerline cells or diagonal contact alone. Routing SHALL NOT cut through an accepted island or peninsula to satisfy this requirement.

#### Scenario: Shallow bands approach each other
- **WHEN** a candidate island leaves fewer than the configured number of deep tiles between the island and nearby land after both shallow bands are resolved
- **THEN** the island is rejected rather than accepting a shallow-only or undersized passage

#### Scenario: Peninsula obstructs the shortest route
- **WHEN** a straight route between a river entrance and the lake interior crosses a peninsula
- **THEN** the accepted navigation route goes around it in water, or the optional peninsula is rejected before acceptance

#### Scenario: Diagonal centerline appears connected
- **WHEN** a diagonal gap admits connected deep center tiles but not the full boat footprint
- **THEN** the gap does not count as navigable

### Requirement: Variable lake banks and final land preservation
The new mode SHALL support independently configurable minimum and maximum lake shallow-band widths, initially 1-5 tiles in H&H, varying coherently along outer and island shores. Equal bounds SHALL produce a constant band and two zero bounds SHALL disable that band. Later bank dilation, Perlin-water resolution, biome passes, and shoreline processing SHALL preserve accepted lake-feature land and deep navigation reservations. A conflict between reserved dry land and a protected deep passage SHALL be rejected rather than resolved by paint order.

#### Scenario: Island lies over low base elevation
- **WHEN** Perlin water is enabled and the underlying elevation of an accepted island would otherwise classify as water
- **THEN** the island remains dry while water outside explicit lake-feature land reservations retains its existing classification behavior

#### Scenario: Lake banks differ from river banks
- **WHEN** only lake shallow-band bounds change
- **THEN** lake margins change but existing river-bank settings and their interpretation remain unchanged

### Requirement: Final-tile validation and reporting
Generation SHALL validate accepted lake-feature land, intended inlet connections, island clearance paths, and existing protected river routes against final terrain tiles. Failures SHALL identify the resolved seed and affected lake/feature or route location. Reporting SHALL distinguish selection decisions, requested island counts, placed islands, rejected placements, and shape fallbacks. These results SHALL NOT be described as runtime boat-physics validation.

#### Scenario: Final island or passage is corrupted
- **WHEN** a final accepted island tile becomes water or an accepted clearance path becomes shallow or dry
- **THEN** validation fails with contextual diagnostics instead of returning a successful terrain result

#### Scenario: Statistical review
- **WHEN** a multi-seed review reports island frequency
- **THEN** it distinguishes configured selection probability and selected lakes from final accepted islands, grouped by lake size class

### Requirement: Configuration preview and backward compatibility
New lake controls SHALL be available in the existing river YAML section and preview panel, including the master switch, peninsula count/depth, shore variation, class island probabilities, conditional second-island probability, island dry-size bounds, and lake shallow-band bounds. H&H comments and preview descriptions SHALL explain units, probability semantics, and disabling behavior in Russian. Preset load/save SHALL preserve explicit false/zero and unrelated settings/comments. The river-only and full-terrain previews SHALL agree with production lake geometry and water classification for the same seed and effective options.

The master switch SHALL default to false and SHALL preserve pre-change terrain output when absent or false, even if valid dormant lake controls are stored. Active Option C SHALL require drawn layout and positive fairway clearance. Non-finite values, invalid ranges, and reversed bounds SHALL fail before generation with the offending configuration field named. Probability values SHALL be within [0,1], peninsula count within [0,8], peninsula depth ratio within [0,0.6], shore variation within [0,16] tiles, island radius bounds within [2,64] tiles, and shallow-band bounds within [0,32] tiles.

#### Scenario: Existing version-one preset
- **WHEN** a previously valid preset omits the new lake fields
- **THEN** it loads without migration and produces the pre-change terrain for the same resolved seed

#### Scenario: Save disabled islands
- **WHEN** zero class probabilities are entered in preview, saved, and reloaded
- **THEN** the zeros remain effective, irregular shores remain available, and unrelated preset configuration is preserved

#### Scenario: Invalid active mode
- **WHEN** Option C is enabled with routed layout or zero fairway clearance
- **THEN** configuration validation rejects it before terrain allocation

### Requirement: Deterministic bounded generation
For the same resolved seed and effective options, accepted geometry and final tiles SHALL be identical across repeated runs and worker counts. Optional candidate attempts and local navigation searches SHALL have finite limits. Allocation arithmetic and peak terrain/preview memory estimates SHALL include additional geometry, reservation, and search storage without increasing the existing memory limit. Optional placement failure SHALL be a clean skip; inability to retain a required main connection even after the permitted shape fallback SHALL be an explicit generation failure.

#### Scenario: Thread count changes
- **WHEN** the same seed is generated with one and several workers
- **THEN** lake masks, island decisions, and final terrain buffers are identical

#### Scenario: Crowded lake and exhausted budget
- **WHEN** an optional island cannot be placed within its finite candidate budget
- **THEN** generation terminates that attempt cleanly without partial edits or unbounded retries

#### Scenario: Additional memory exceeds budget
- **WHEN** the terrain or preview memory estimate including new lake storage exceeds the configured limit
- **THEN** generation fails before the oversized allocation with a meaningful budget error
