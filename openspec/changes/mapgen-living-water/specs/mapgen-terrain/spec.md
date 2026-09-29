# Spec Delta

> PAUSED / SUPERSEDED (2026-09-29): The user rejected continent/coastal
> generation and physical drainage for this task. The historical content below
> is NOT approved for implementation or spec sync. Current shape-only exploration:
> `docs/plans/river-shape-variants.txt`. Every river, including tributaries and
> narrow connectors, requires a boat-passable deep fairway. No variant is selected.

## Purpose

Defines the terrain structure of the offline map generator: how elevation is composed (continents-bold style), how water bodies form and are filtered, how drawn lakes respect terrain, how structural water joins the drawn river network, and how river widths derive from connected volume — all deterministic per seed.

## ADDED Requirements

### Requirement: Structural elevation field
When the elevation layer is enabled, elevation SHALL combine a multi-octave fBm detail field with a domain-warped low-frequency continent field (configurable continent weight, detail frequency, octaves and warp strength), then SHALL be percentile-stretched (configurable low/high percentiles) before water thresholds are applied. The legacy single-octave composition SHALL remain available by disabling the layer and SHALL produce the same output as the current generator.

#### Scenario: Same seed is identical
- **WHEN** the generator runs twice with the same seed and elevation configuration
- **THEN** the elevation fields are identical

#### Scenario: Thresholds are stable across presets
- **WHEN** two presets use different continent weights but the same deep/shallow thresholds
- **THEN** both produce water in genuine basins rather than threshold speckle, because the percentile stretch normalizes the elevation distribution

### Requirement: Coherent water bodies
With structural elevation enabled, the map SHALL contain a small number of large water bodies (seas and major lakes) instead of uniformly scattered speckle: the count of distinct water components SHALL be bounded by the configured geography (tens, not thousands), and the largest component SHALL dominate the water share.

#### Scenario: No threshold speckle
- **WHEN** a world is generated with structural elevation enabled and default thresholds
- **THEN** the count of distinct water bodies is at most a configured small bound (prototype: single digits), not thousands

### Requirement: Minimum water body filter
After tile resolution, every connected water component smaller than the configured minimum size SHALL be converted to land. A minimum of zero SHALL disable the filter. The filter SHALL apply in both structural and legacy elevation modes, and shoreline sand SHALL NOT be placed around removed bodies.

#### Scenario: Speckle is removed
- **WHEN** the legacy elevation mode runs with a non-zero minimum water body size
- **THEN** isolated ponds smaller than the minimum become land and no sand ring surrounds them

#### Scenario: Real lakes survive
- **WHEN** a water component is larger than the configured minimum
- **THEN** it is kept unchanged by the filter

### Requirement: Terrain-gated lake placement
Drawn-lake candidates SHALL be accepted only when the terrain gate passes: the stretched elevation at the candidate must not exceed the configured lowland maximum, and the mountain-massif signal must be below its gate. Unsuitable candidates SHALL be skipped without relocation; blue-noise ordering and spacing SHALL be preserved; size-class demotion SHALL continue to work for crowded classes.

#### Scenario: No lakes on highlands
- **WHEN** a lake candidate falls on terrain above the configured lowland elevation maximum
- **THEN** the candidate is skipped and no lake is carved there

#### Scenario: No lakes through mountain massifs
- **WHEN** a lake candidate falls where the mountain-massif signal exceeds its gate
- **THEN** the candidate is skipped and the mountain terrain is not carved

#### Scenario: Spacing is preserved
- **WHEN** candidates are gated out
- **THEN** remaining lakes keep at least the configured minimum distance from each other

### Requirement: Structural water anchors
Large structural water components (at or above the configured anchor size) SHALL participate in the drawn river network as nodes: drawn rivers SHALL be able to connect to their shores alongside drawn lakes. Components below the anchor size SHALL remain scenery and SHALL NOT be used as network nodes.

#### Scenario: River reaches the sea
- **WHEN** the drawn network creates a border or lake-to-lake link involving a structural anchor
- **THEN** the drawn corridor terminates at the anchor's shore and the water is connected

#### Scenario: Small structural lakes stay scenery
- **WHEN** a structural water component is below the anchor size threshold
- **THEN** no drawn link targets it

### Requirement: River width hierarchy
When width hierarchy is enabled, each drawn river corridor's width SHALL be derived deterministically from the water volume it connects (endpoint body size classes and link length) within the existing `[river_width_min, river_width_max]` bounds, replacing uniform random selection. Equal inputs SHALL produce equal widths.

#### Scenario: Trunk is wider than creek
- **WHEN** a link connects two large bodies and another connects two small bodies
- **THEN** the first link's corridor width is greater than the second's

#### Scenario: Deterministic widths
- **WHEN** the same seed and configuration are regenerated
- **THEN** every link receives the same width as before

### Requirement: Terrain configuration and determinism
The elevation layer, water body filter, lake gates, anchors and width hierarchy SHALL be configurable via the generator YAML (new `elevation:` section and new `river:` keys, flat style consistent with existing sections). Unknown or retired keys SHALL be rejected with an error naming the key. Terrain output SHALL be a pure function of the resolved seed and configuration: identical inputs SHALL produce byte-identical tile buffers across runs and worker thread counts.

#### Scenario: Defaults match the chosen style
- **WHEN** a preset omits the elevation section
- **THEN** the layer is enabled with the continents-bold default parameters

#### Scenario: Unknown key is rejected
- **WHEN** a preset contains an unknown elevation or river key
- **THEN** configuration loading fails with an error naming that key

#### Scenario: Repeated run is identical
- **WHEN** the generator runs twice with the same seed and configuration (different thread counts allowed)
- **THEN** the terrain tile buffers are byte-identical
