# Spec Delta

## MODIFIED Requirements

### Requirement: Structural terrain survives all land operations
The biome layer SHALL protect water as determined by elevation, river water as determined by river classification, and base mountain, stone paving, and sand throughout all land painting and cleanup. Protected cells SHALL be excluded from cleanup neighbor voting and component traversal. Final water/river resolution and shoreline sand SHALL retain their existing precedence and behavior except for explicitly protected deep river fairways: when fairway protection is enabled, protected cells SHALL resolve to deep water even where elevation would otherwise select shallow water, and SHALL remain deep through shoreline processing. This exception SHALL NOT alter elevation generation or the precedence of cells outside the protected fairway. With fairway protection disabled, existing resolution behavior SHALL remain unchanged.

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
