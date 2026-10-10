# Spec Delta

## MODIFIED Requirements

### Requirement: Directional input has a distinct additive wire contract
Directional movement SHALL use a dedicated `MoveDirection` player action containing a world direction, a nonzero input revision, and the current world stream epoch. The client SHALL normalize every nonzero world direction to unit length before serialization, subject to the precision of the wire float fields; each serialized component SHALL remain within -1 through 1. Refreshes SHALL repeat that normalized vector. Zero direction SHALL be serialized as (0,0) without normalization and SHALL mean release of directional control. The server SHALL advertise support once in the connection's `S2C_ServerConstants`; a client SHALL send directional input only after valid constants advertise that support and an active world session is established. The support flag SHALL NOT be repeated in enter-world snapshots. Directional-action fields, retired reservations, and movement broadcasts SHALL retain their numbers and behavior. The relocation of bootstrap constants SHALL require matching server/client deployment rather than guaranteeing mixed-version bootstrap compatibility.

#### Scenario: Updated peers
- **WHEN** an updated client receives server constants advertising directional movement and enters an active world
- **THEN** it SHALL be able to start and stop WASD movement using the dedicated action without sending synthetic map clicks

#### Scenario: Diagonal direction is normalized on the wire
- **WHEN** the player holds W+D and the client sends the initial direction or a refresh
- **THEN** the decoded direction SHALL have unit length within float precision and components within -1 through 1, preserving the projected up-right direction without failing the server component-range check

#### Scenario: Mixed versions
- **WHEN** an updated client receives a legacy world bootstrap without valid server constants
- **THEN** it SHALL reject that bootstrap and SHALL NOT send directional input or invent geometry and tick-rate defaults
- **AND** this migration SHALL require matching server and client deployment rather than supporting older clients through duplicated enter-world fields

#### Scenario: Capability is false
- **WHEN** valid server constants disable directional movement and the client enters an active world
- **THEN** mouse movement SHALL continue to work and WASD SHALL remain disabled

#### Scenario: Layer transfer keeps connection capability
- **WHEN** a supported client transfers between layers on the same connection
- **THEN** directional capability SHALL remain available from the same constants snapshot
- **AND** the new active world epoch and existing fresh-input lifecycle SHALL still govern directional commands
