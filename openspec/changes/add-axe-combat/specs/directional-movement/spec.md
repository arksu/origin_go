# Spec Delta

## MODIFIED Requirements

### Requirement: Directional movement obeys existing world rules
Directional motion SHALL use existing server speed modes, movement restrictions, collision resolution, carry restrictions, stamina accounting, world boundaries, and chunk transitions. Directional input SHALL NOT create a click destination, pickup request, link intent, or point-arrival event. An existing active link SHALL be broken through its normal lifecycle before manual movement starts. Stationary collision contact SHALL NOT spend movement stamina or trigger an interaction.

During axe windup and the strike step, the effective movement mode SHALL be capped at Crawl after ordinary restrictions; a prohibition on movement SHALL still win. Recovery SHALL remove only that cap and SHALL NOT overwrite the selected movement mode or revive expired/rejected held input. Movement SHALL NOT cancel a committed axe cycle or rotate its locked attack direction. Input validation, revisions, expiry, collision and lifecycle rules SHALL remain unchanged.

#### Scenario: Modes and diagonal speed
- **WHEN** the player moves in any WASD combination while walking, running, crawling, swimming, or carrying an object
- **THEN** allowed speed and movement cost SHALL follow the same server rules as ordinary movement

#### Scenario: Held direction reaches a wall
- **WHEN** collision fully blocks movement while fresh input remains held
- **THEN** the authoritative position SHALL stop, the movement broadcast SHALL report no actual movement, and the held direction SHALL remain available for subsequent collision checks

#### Scenario: Obstacle disappears
- **WHEN** an obstacle blocking a still-valid held direction disappears
- **THEN** movement SHALL resume through normal collision resolution without requiring key repeat or a new key press

#### Scenario: Restricted player
- **WHEN** a stun, knockout, death, or inability to move prevents or ends manual movement
- **THEN** the player SHALL not move, the old directional revision SHALL be retired, and recovery SHALL require fresh input

#### Scenario: No implicit world interaction
- **WHEN** WASD movement contacts an object or passes over a dropped item
- **THEN** it SHALL NOT automatically link, pick up, place, or activate that object

#### Scenario: Combat movement cap follows phase
- **WHEN** a player with sufficient stamina and Run selected moves through windup, impact, and recovery
- **THEN** movement SHALL use Crawl through impact and normal allowed Run in recovery while the attack direction remains locked
