# mapgen-rivers Specification

## Purpose

Define visually natural, gameplay-oriented drawn rivers with sparse tributaries and continuous configurable deep-water clearance, without simulating drainage or imposing a continent.

## Requirements

### Requirement: Multiscale drawn river shape
The generator SHALL support Option B: broad asymmetric bends containing smaller irregular bends and quieter stretches. Bend scales SHALL be expressed in world-tile distances rather than a fixed wave count per connection. The mode SHALL keep route endpoints attached to their intended destinations and SHALL constrain detail so it cannot create an unintended crossing, shortcut, or impassable pinch.

#### Scenario: Long and short routes share a local bend scale
- **WHEN** routes with different endpoint distances use the same configured bend scales
- **THEN** a longer route accommodates more bends rather than stretching every bend to the length of the connection

#### Scenario: Detail inside a large bend
- **WHEN** smaller-scale detail is enabled on a fixed route fixture
- **THEN** local curvature changes while the broad route and exact endpoints remain recognizable and attached

#### Scenario: Curve collides with itself
- **WHEN** proposed detail would join nonadjacent arms of a river
- **THEN** that candidate is reshaped or rejected before carving, rather than creating an accidental connection

### Requirement: No new continent or drainage model
The selected mode SHALL use the existing drawn layout without deriving routes from elevation, runoff, or watersheds. It SHALL NOT impose coastal water at world boundaries or a world-center-to-border flow pattern. Elevation generation, lake-placement policy, and main-link selection policy SHALL remain unchanged. Existing optional border endpoints SHALL mean cropped river continuations, not a surrounding sea.

#### Scenario: Elevation input changes
- **WHEN** only the elevation values supplied to the drawn-network generator change
- **THEN** its route geometry and lake placement remain identical for the same seed and river configuration

#### Scenario: Land at a world boundary
- **WHEN** a boundary region would be land without a drawn river there
- **THEN** enabling the selected river shape does not replace that boundary region with a coastal ocean

### Requirement: Sparse independent tributaries
Tributaries SHALL be optional, non-recursive additions to accepted main routes. Their maximum count SHALL be a configured fraction of accepted main-route count, rounded down, with at most one added tributary per parent route and configurable separation between junctions. Zero fraction SHALL disable tributaries. Candidate attempts SHALL be bounded, and rejected candidates SHALL leave no partial water. The H&H preset SHALL use a low fraction, with 0.08 as the initial proposed tuning, rather than a branch at each bend or lake. Main links needed for travel SHALL NOT be removed to satisfy a tributary budget.

#### Scenario: Sparse budget
- **WHEN** 100 main routes are accepted and the tributary fraction is 0.08
- **THEN** at most eight tributaries are added, each on a different parent and satisfying junction spacing

#### Scenario: More bend detail
- **WHEN** curve subdivision or octave count increases for an otherwise fixed set of main routes
- **THEN** the tributary count budget does not increase

#### Scenario: Crowded candidate
- **WHEN** a tributary cannot meet its length, spacing, separation, or deep-clearance constraints
- **THEN** that optional candidate is skipped without affecting the main route or leaving shallow-only fragments

### Requirement: Continuous protected deep fairways
Every accepted river in the selected mode, including tributaries, narrow links, junctions, and lake entrances, SHALL contain continuous deep-water clearance for the configured footprint. The initial generator contract SHALL use an odd positive width in tiles and an axis-aligned square footprint of that width. A route SHALL support connected valid footprint placements without relying on diagonal corner touching. Shallow banks SHALL be outside the protected fairway and SHALL NOT sever it. A tributary's inland end SHALL retain clearance instead of tapering into shallow water.

#### Scenario: Tributary joins a main river
- **WHEN** a tributary is accepted
- **THEN** its deep corridor joins the parent's deep corridor with the configured clearance and no shallow seam

#### Scenario: Lake entrance
- **WHEN** two accepted links reach the same drawn lake
- **THEN** their protected fairways connect through the lake's navigable interior rather than ending at disconnected shallow margins

#### Scenario: Tight or diagonal bend
- **WHEN** a route turns or runs diagonally across the tile grid
- **THEN** its final deep tiles admit a continuous sequence of full-footprint placements, not merely a chain of deep center tiles

#### Scenario: World-edge continuation
- **WHEN** an existing border link reaches the map boundary
- **THEN** its fairway remains usable to the last fully in-bounds footprint placement, without wrapping or treating out-of-bounds cells as navigable

### Requirement: Final-tile validation and explicit failure
The generator SHALL validate accepted routes against final terrain tiles after land, water, and shoreline resolution. A detected clearance break in an accepted route SHALL fail generation with seed and route/location context rather than silently downgrade the passage or report success. Reporting SHALL distinguish main routes, tributary budget, accepted tributaries, and rejected candidates. Generator clearance validation SHALL NOT be presented as boat-runtime validation where no corresponding boat collision model exists.

#### Scenario: Shallow terrain crosses a deep fairway
- **WHEN** the base elevation classifies a protected fairway location as shallow water
- **THEN** final resolution keeps the protected footprint deep and validation succeeds

#### Scenario: Final mask is corrupted
- **WHEN** a protected tile is changed to shallow water or land before validation
- **THEN** validation rejects the result and identifies the affected route/location

### Requirement: Configuration preview and compatibility
The existing YAML river section and river preview SHALL expose bend wavelength, deep-fairway width, tributary fraction, junction spacing, and tributary length bounds. Existing octave, amplitude, and frequency controls SHALL have documented semantics in the selected mode. New geometry and tributaries SHALL require drawn layout and positive fairway clearance. Non-finite values, negative controls, inconsistent length bounds, and incompatible widths SHALL fail early with the offending setting named. With all new controls absent or zero, existing presets SHALL retain legacy generation behavior. The H&H preset SHALL explicitly enable the selected mode; the other presets SHALL NOT be silently migrated.

#### Scenario: Legacy version-1 preset
- **WHEN** a previously valid preset omits all new controls
- **THEN** it loads without migration and produces the same terrain buffers as the pre-change generator for the same seed

#### Scenario: Shared preview settings
- **WHEN** the same seed and effective river settings are used by the river preview and production network generation
- **THEN** they use the same geometry, tributary selection, and protected river mask

#### Scenario: Invalid clearance
- **WHEN** the selected shape is enabled with zero clearance, or clearance exceeds the configured minimum river width
- **THEN** configuration validation fails before generation rather than making shallow or undersized branches

### Requirement: Deterministic bounded generation
The selected mode SHALL produce identical river and final terrain buffers for a resolved seed and effective configuration across worker counts. Retries and shape sampling SHALL be bounded. Memory estimates SHALL include additional route records and protection/validation buffers before allocation. Optional tributary randomness SHALL not alter lake placement or main-route selection.

#### Scenario: Repeated seed and thread change
- **WHEN** the same resolved seed is generated repeatedly with one and multiple workers
- **THEN** final terrain buffers and tributary records are identical

#### Scenario: Tributaries disabled
- **WHEN** only the tributary fraction changes from a positive value to zero
- **THEN** main-route and lake generation remain unchanged, and the added branches disappear
