# Spec Delta

## MODIFIED Requirements

### Requirement: Structural terrain survives all land operations
The biome layer SHALL protect water as determined by enabled elevation-water classification, river water as determined by river classification, and base mountain, stone paving, and sand throughout all land painting and cleanup. Protected cells SHALL be excluded from cleanup neighbor voting and component traversal. Final water/river resolution and shoreline sand SHALL retain their existing precedence and behavior outside explicit drawn-lake feature land reservations.

Accepted island and peninsula land reservations from the enabled irregular drawn-lake mode SHALL be treated as land for ground classification and biome eligibility even when their underlying elevation would otherwise produce water. Those reservations SHALL remain dry through final water resolution and shoreline processing without changing the underlying elevation or climate fields. Existing mountain, stone, and sand protection SHALL continue to apply within reserved land. Reservations SHALL NOT overlap protected river or lake-navigation footprints; such a conflict SHALL fail validation rather than erasing either feature. Without the lake feature enabled, structural terrain behavior SHALL remain unchanged.

#### Scenario: Mountain and paving meet forest
- **WHEN** main patches and every cleanup pass operate beside or over mountain and stone paving
- **THEN** those protected base tiles retain their original type until final hydrology is resolved

#### Scenario: Water has an unresolved land base
- **WHEN** a tile is destined to become water but its base buffer still contains a land type
- **THEN** no biome painting modifies it and it does not influence cleanup votes as land

#### Scenario: Reserved island over a Perlin lowland
- **WHEN** an accepted lake island occupies low-elevation tiles with elevation water enabled
- **THEN** its tiles are classified as land, participate in land-biome eligibility subject to other structural locks, and remain dry after final shoreline processing

#### Scenario: Nearby water is not reserved land
- **WHEN** elevation water lies outside an explicit accepted island or peninsula reservation
- **THEN** the new lake-land exception does not change its existing water classification or protection
