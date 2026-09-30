# Spec Delta

## Purpose

Define configurable lake-to-river Y confluences for the drawn game map, including natural approach shapes, deep boat passages, controlled density, and deterministic fallback.

## ADDED Requirements

### Requirement: Probability-controlled river destinations
The drawn generator SHALL support supplementary connections from a source lake to the interior of an already accepted ordinary main river. Each eligible remaining-major or short-link opportunity SHALL receive one deterministic selection decision using `river.junction_chance`. The regional backbone and explicit major border stage SHALL retain their existing destination policy. Existing inland tributaries SHALL remain separate from these connections. Y branches and inland tributaries SHALL NOT be target parents for a new Y branch in this version.

The probability SHALL select an attempt, not an exact fraction of placed junctions. Zero SHALL disable Y destination selection; one SHALL select every eligible opportunity without forcing unsuitable geometry. Decisions SHALL use stable opportunity identities and SHALL NOT multiply with candidate retries or curve subdivision.

#### Scenario: A lake joins a river between two other lakes
- **WHEN** an eligible connection is selected and a navigable river-interior target fits
- **THEN** the source lake connects to that target, creating three river arms with no additional lake at the junction

#### Scenario: All eligible opportunities select attempts
- **WHEN** `junction_chance` is one
- **THEN** every eligible supplementary opportunity attempts a Y connection, while backbone links and unsuitable opportunities retain their documented behavior

#### Scenario: Tributaries are disabled
- **WHEN** `tributary_ratio` is zero and `junction_chance` is positive
- **THEN** lake-to-river Y connections remain available independently of inland tributaries

### Requirement: Layout and budget bookkeeping
Y destination choice SHALL preserve lake placement and size sampling and SHALL consume the existing budget slot for its generation stage. An accepted Y branch SHALL count as a main connection and increment only its actual source lake's incident-link count. It SHALL NOT register a nonexistent destination-lake inlet or mark the unused ordinary fallback destination as connected. Source-lake degree limits SHALL still apply. The target river SHALL remain intact, and a branch SHALL NOT join a parent directly incident to its own source lake.

#### Scenario: Ordinary candidate is replaced by a Y connection
- **WHEN** a candidate lake-to-lake connection becomes an accepted lake-to-river branch
- **THEN** only the actual source inlet is added, the unused lake endpoint receives no bookkeeping changes, and only one connection budget slot is consumed

#### Scenario: Source is at its degree limit
- **WHEN** the source lake already has the configured maximum number of incident links
- **THEN** no Y branch is accepted from that lake

#### Scenario: Backbone already connected the region
- **WHEN** additional Y connections are enabled
- **THEN** accepted regional backbone paths remain present and are not replaced or deleted to fit the new branches

### Requirement: Natural three-arm confluence geometry
An accepted Y branch SHALL meet the parent's interior obliquely with a smooth local approach and a compact, continuous water union. The junction SHALL have three arms rather than a branch continuing across the parent to form a fourth arm. New geometry SHALL retain the configured variable river widths and shallow-bank behavior. Outside the intended source entrance and bounded target-parent overlap, the candidate SHALL NOT contact unrelated rivers, lakes, or another part of its own path. The target parent's path SHALL NOT be rerouted to accept the branch.

#### Scenario: Candidate crosses the parent
- **WHEN** an approach would continue beyond the intended target and create an X crossing
- **THEN** the candidate is rejected before any carving

#### Scenario: Nearby river lies inside the endpoint overlap region
- **WHEN** a third river would be touched near the intended parent join
- **THEN** the candidate is rejected despite being near the designated endpoint

#### Scenario: Smooth approach into a bend
- **WHEN** a candidate joins a curved parent segment
- **THEN** its final approach follows the local parent geometry and has an oblique confluence instead of an abrupt perpendicular intersection

### Requirement: Spaced confluences and lake exclusions
Y junctions SHALL satisfy `junction_spacing_tiles` separation from other Y junctions and lake entrances and SHALL leave enough physical space for the complete local river corridors. Junction positions SHALL lie outside lake footprints and their protected entrance regions. Later inland tributaries SHALL respect existing Y positions using at least the larger of the configured Y and tributary spacing. Multiple Y branches on a long ordinary parent SHALL be allowed when all separation rules hold.

#### Scenario: Two candidates target a short parent segment
- **WHEN** the second junction cannot satisfy spacing and corridor separation from the first
- **THEN** the second placement is rejected without changing the accepted first junction

#### Scenario: Inland tributary is proposed near a Y junction
- **WHEN** that candidate falls within the effective cross-type junction spacing
- **THEN** the inland tributary is skipped or placed elsewhere without deleting the Y connection

### Requirement: Deep navigation and lake-feature preservation
All three arms and the local junction SHALL admit connected placements of the configured complete square deep-water footprint after shallow banks and final terrain processing. H&H SHALL preserve at least three deep tiles of passage width. Diagonal tile touching and shallow-only seams SHALL NOT count as navigable. The source lake's entrance SHALL join its navigable interior. Accepted islands, peninsulas and their circumnavigation paths SHALL retain the existing lake land and navigation guarantees.

#### Scenario: Shallow paint would separate the arms
- **WHEN** bank resolution would place shallow water across a protected junction passage
- **THEN** the complete protected boat footprint remains deep through the junction

#### Scenario: Candidate requires cutting an island
- **WHEN** the branch or its protected footprint would erase accepted lake-feature land
- **THEN** that geometry is rejected rather than carving through the feature

#### Scenario: Final junction tiles are corrupted
- **WHEN** final terrain breaks an accepted confluence or source entrance
- **THEN** generation fails with resolved seed, branch, parent and junction-location context

### Requirement: Atomic rejection and ordinary fallback
A selected opportunity SHALL have a finite geometric candidate budget. Exhausting that budget SHALL trigger the same opportunity's ordinary lake-to-lake or existing short-stage border candidate without rerolling the Y selection. Rejected candidates SHALL leave no partial water, route records, inlets, degree increments, duplicate reservations or junction-index entries. Ordinary fallback SHALL remain subject to its existing validation and budgets; failure SHALL NOT force an unsafe connection.

#### Scenario: No suitable parent is available
- **WHEN** the Y decision succeeds but no valid parent-interior location fits
- **THEN** the ordinary candidate is attempted and the selected-but-unplaced Y is reported separately

#### Scenario: Several candidate geometries fail
- **WHEN** all permitted target/approach attempts are rejected
- **THEN** they leave no partial edits and the probability decision is not repeated to compensate for failure

### Requirement: Configuration and preview compatibility
The YAML river section and existing river preview SHALL expose `junction_chance` and `junction_spacing_tiles` with Russian descriptions of probability, units and disabling behavior. The general defaults SHALL be zero probability and spacing 180 tiles. Initial proposed H&H tuning SHALL be 0.25 probability and 180 tiles spacing, subject to visual review. No other current preset tuning SHALL be overwritten.

Probability SHALL be finite and within [0,1]; spacing SHALL be an integer within [0,8192], and SHALL be positive when the feature is active. Positive probability SHALL require drawn layout, active Option B bends, and positive valid deep-fairway clearance. Invalid inputs SHALL fail before generation with the offending field named. With the controls omitted or probability zero, generation SHALL retain the pre-change terrain for the same seed and effective existing options. Preset load/save SHALL preserve explicit zero and unrelated settings/comments. River-only preview, full terrain preview and direct generation SHALL use the same junction behavior.

#### Scenario: Existing preset omits the controls
- **WHEN** an existing version-one preset is loaded without junction settings
- **THEN** it uses zero junction probability and retains its pre-change output

#### Scenario: Save disables Y destinations
- **WHEN** zero probability is entered, saved and reloaded in preview
- **THEN** Y destination attempts stay disabled and other river/lake settings and preset comments remain intact

#### Scenario: Incompatible active mode
- **WHEN** positive probability is configured with routed layout, disabled Option B, or invalid boat clearance
- **THEN** validation rejects the configuration before allocation

### Requirement: Deterministic bounded generation and reporting
For a resolved seed and effective configuration, selection decisions, accepted geometry and final terrain SHALL be identical across repeated runs and worker counts. Parent selection and geometric searches SHALL have finite limits, and memory estimates SHALL account for additional metadata, candidate lookup storage and scratch before allocation without increasing the precompute limit. Reporting SHALL distinguish eligible opportunities, selected Y attempts, placed junctions, failed Y opportunities that invoked ordinary fallback, and geometric candidate rejection reasons. Geometric navigation verification SHALL not be described as runtime boat-physics verification.

#### Scenario: Same seed with a different worker count
- **WHEN** identical effective settings are generated with one and several workers
- **THEN** accepted junction records and final terrain buffers are identical

#### Scenario: Review compares probability with placed frequency
- **WHEN** a multi-seed review measures the new feature
- **THEN** it reports the eligible-opportunity denominator and selected/placed/fallback counts separately from backbone links and inland tributaries

#### Scenario: Search storage would exceed the budget
- **WHEN** route metadata, lookup storage and bounded search scratch exceed the precompute memory budget
- **THEN** generation rejects the request before allocating oversized buffers
