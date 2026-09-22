# Spec Delta

## MODIFIED Requirements

### Requirement: Ordinary map clicks preserve movement, pickup, and object linking
Without a pending administrator action and without an armed gameplay action, a map click on empty ground, a stale target, or a target without a collider SHALL move the player toward the clicked coordinates, subject to existing movement rules. A map click reporting a live non-dropped object with a collider SHALL set an explicit link intent, move the player to that object, and the link SHALL be established on confirmed collision without executing any context action; a click on empty ground SHALL cancel any outstanding link intent. A live dropped-item target SHALL retain normal primary-click pickup behavior. Explicit placement, item-in-hand drop, context interaction, and UI-consumed input SHALL retain their ordinary routing and SHALL NOT emit duplicate gameplay actions.

An active gameplay action in target-selection phase SHALL take precedence over this ordinary behavior. An object-target action SHALL consume a click on a live object, including one without a collider; a click on empty ground or a stale target SHALL fall through to ordinary movement while the action remains armed. A tile-target action SHALL consume every map click and pass its coordinates to its handler. A consumed click SHALL NOT also trigger ordinary movement, linking, or pickup, though an accepted action may initiate its own approach to the target.

#### Scenario: Object click requests a link
- **WHEN** a player primary-clicks a live non-dropped object with a collider and no pending administrator action and no armed gameplay action
- **THEN** the server SHALL set a link intent for that object, the player SHALL move to and link with it on confirmed collision, and no context action SHALL execute from the click alone

#### Scenario: Ground click uses coordinates and clears link intent
- **WHEN** a player primary-clicks empty ground, a stale target, or a target without a collider with no pending administrator action and no armed gameplay action
- **THEN** movement SHALL target the supplied coordinates and any outstanding link intent SHALL be cleared

#### Scenario: Stale ordinary target
- **WHEN** a normal map click reports a target that no longer exists
- **THEN** movement SHALL still use the supplied coordinates

#### Scenario: Dropped item pickup
- **WHEN** a player primary-clicks a live dropped item with no pending administrator action and no armed gameplay action
- **THEN** the server SHALL initiate the existing pickup flow exactly once

#### Scenario: Armed action outranks ordinary routing
- **WHEN** a player with an armed gameplay action primary-clicks a target that the armed action consumes
- **THEN** the click SHALL be handled by the armed action and SHALL NOT also initiate movement, link intent, or pickup

#### Scenario: Armed object action handles an object without a collider
- **WHEN** a player selecting an object-target action primary-clicks a live object without a collider
- **THEN** the action SHALL receive that object, and ordinary coordinate movement SHALL NOT also run

#### Scenario: Armed object action permits ground movement
- **WHEN** a player selecting an object-target action primary-clicks empty ground
- **THEN** ordinary movement SHALL use the clicked coordinates and the action SHALL remain selecting

#### Scenario: Armed tile action consumes a click on an object
- **WHEN** a player selecting a tile-target action primary-clicks a live dropped item
- **THEN** the action SHALL receive the click coordinates and ordinary pickup SHALL NOT run

### Requirement: Administrator click actions are interpreted only by the server
The server SHALL give one pending administrator click action the highest precedence over both armed gameplay actions and ordinary map-click behavior. `/spawn` and coordinate-less `/tp` SHALL consume click coordinates; `/info` and `/destroy` SHALL consume the target ID. The consumed click SHALL NOT also initiate armed action execution, movement, pickup, or context interaction. The server SHALL preserve current command availability, resolve object targets against the current live world, and reject unavailable or ineligible targets without substituting another object. `/destroy` SHALL preserve deletion of the selected non-player object and all inventory contents through its existing deletion operation.

#### Scenario: Spawn or teleport on an object
- **WHEN** an administrator awaiting `/spawn` or `/tp` sends a map click on an object
- **THEN** the command SHALL use the clicked coordinates rather than the object's center

#### Scenario: Destroy a dropped item
- **WHEN** an administrator awaiting `/destroy` primary-clicks a dropped item
- **THEN** the server SHALL execute destruction and SHALL NOT pick up the item

#### Scenario: Invalid object selection
- **WHEN** an administrator awaiting `/info` or `/destroy` sends a map click with zero or an unavailable target ID
- **THEN** the server SHALL report the appropriate target error, consume the pending attempt, and SHALL NOT execute ordinary movement

#### Scenario: Player destruction is rejected
- **WHEN** an administrator awaiting `/destroy` selects a player
- **THEN** the server SHALL reject destruction and consume the attempt

#### Scenario: Pending selection is isolated and transient
- **WHEN** a pending action is consumed, replaced by another click-target administrator command, or its player leaves the active world or dies
- **THEN** the server SHALL clear that player's obsolete pending state without altering other players' pending state

#### Scenario: Context interaction is not selection
- **WHEN** a player with a pending object-selection command sends an explicit `Interact`
- **THEN** the server SHALL process normal interaction without consuming that command's pending selection

#### Scenario: Administrator command outranks armed action
- **WHEN** an administrator awaiting `/destroy` primary-clicks an object while having an armed gameplay action
- **THEN** the administrator command SHALL consume the click and the armed action SHALL NOT execute or change
