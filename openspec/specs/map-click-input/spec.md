# map-click-input Specification

## Purpose

Transmit ordinary map clicks without client knowledge of administrator commands and let the server interpret coordinates and object targets consistently.

## Requirements

### Requirement: Map clicks describe input independently of commands
The client SHALL send one `MapClick` action containing integer world coordinates `x`, `y`, and `target_entity_id` for each ordinary primary map click. The target SHALL be the object found under the pointer or zero for empty ground. Input modifiers SHALL remain on the enclosing player action. Chat text and administrator pending state MUST NOT alter client click routing or enable a special client selection mode.

#### Scenario: Same input for administrators and ordinary players
- **WHEN** either player primary-clicks the same ordinary world object
- **THEN** the client SHALL send the same map-click fields regardless of previously entered chat commands

#### Scenario: Empty ground
- **WHEN** a player primary-clicks empty ground
- **THEN** the client SHALL send the clicked coordinates with target ID zero

### Requirement: Ordinary map clicks preserve movement, pickup, and object linking
Without a pending administrator action, a map click on empty ground, a stale target, or a target without a collider SHALL move the player toward the clicked coordinates, subject to existing movement rules. A map click reporting a live non-dropped object with a collider SHALL set an explicit link intent, move the player to that object, and the link SHALL be established on confirmed collision without executing any context action; a click on empty ground SHALL cancel any outstanding link intent. A live dropped-item target SHALL retain normal primary-click pickup behavior. Explicit placement, item-in-hand drop, context interaction, and UI-consumed input SHALL retain their ordinary routing and SHALL NOT emit duplicate gameplay actions.

#### Scenario: Object click requests a link
- **WHEN** a player primary-clicks a live non-dropped object with a collider and no pending administrator action
- **THEN** the server SHALL set a link intent for that object, the player SHALL move to and link with it on confirmed collision, and no context action SHALL execute from the click alone

#### Scenario: Ground click uses coordinates and clears link intent
- **WHEN** a player primary-clicks empty ground, a stale target, or a target without a collider
- **THEN** movement SHALL target the supplied coordinates and any outstanding link intent SHALL be cleared

#### Scenario: Stale ordinary target
- **WHEN** a normal map click reports a target that no longer exists
- **THEN** movement SHALL still use the supplied coordinates

#### Scenario: Dropped item pickup
- **WHEN** a player primary-clicks a live dropped item with no pending administrator action
- **THEN** the server SHALL initiate the existing pickup flow exactly once

### Requirement: Administrator click actions are interpreted only by the server
The server SHALL give one pending administrator click action precedence over ordinary map-click behavior. `/spawn` and coordinate-less `/tp` SHALL consume click coordinates; `/info` and `/destroy` SHALL consume the target ID. The consumed click SHALL NOT also initiate movement, pickup, or context interaction. The server SHALL preserve current command availability, resolve object targets against the current live world, and reject unavailable or ineligible targets without substituting another object. `/destroy` SHALL preserve deletion of the selected non-player object and all inventory contents through its existing deletion operation.

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

### Requirement: Legacy movement actions are retired safely
The supported protocol SHALL expose `MapClick` in place of the old `MoveTo` action and SHALL remove `MoveToEntity`. Retired action field numbers and names SHALL be reserved and SHALL NOT be assigned to the new action. All maintained clients and load-test senders SHALL use the new action; an obsolete action SHALL NOT be interpreted as a map click.

#### Scenario: Legacy payload received
- **WHEN** a payload contains only a retired movement action
- **THEN** the server SHALL NOT execute movement or a pending administrator action from that payload

#### Scenario: Supported sender compatibility
- **WHEN** the updated browser or load-test client sends a map click
- **THEN** the updated server SHALL decode the same coordinates, target ID, and modifiers
