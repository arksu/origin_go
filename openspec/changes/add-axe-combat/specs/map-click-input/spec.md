# Spec Delta

## MODIFIED Requirements

### Requirement: Ordinary map clicks preserve movement, pickup, and object linking

The existing behavior and scenarios below apply to noncombat actions unless explicitly identified as combat. Committed combat follows the exception stated in this requirement.
Without a pending administrator action and without an armed gameplay action, a primary map click on empty ground, a stale target, or a target without a collider SHALL move the player toward the clicked coordinates, subject to existing movement rules. A primary map click reporting a live non-dropped object with a collider SHALL set an explicit link intent, move the player to that object, and the link SHALL be established on confirmed collision without executing any context action; a primary click on empty ground SHALL cancel any outstanding link intent. A live dropped-item target SHALL retain normal primary-click pickup behavior. Explicit placement, context interaction, and UI-consumed input SHALL retain their ordinary routing and SHALL NOT emit duplicate gameplay actions. Item-in-hand drop SHALL retain its ordinary client routing when no gameplay target action is selecting, approaching, or executing.

An active gameplay action in target-selection phase SHALL take precedence over this ordinary primary-click behavior. An object-target action SHALL consume a primary click on a live object, including one without a collider; a primary click on empty ground or a stale target SHALL fall through to ordinary movement while the action remains armed. A tile-target action SHALL consume every primary map click and pass its coordinates to its handler. A consumed click SHALL NOT also trigger ordinary movement, linking, or pickup, though an accepted action may initiate its own approach to the target.

While any gameplay action is selecting a target, a primary click that reaches map input SHALL send MapClick instead of item-in-hand drop, regardless of target kind or whether the server will consume the click. If an object-target action lets an empty-ground or stale-target click fall through, the server SHALL perform ordinary map-click movement and SHALL NOT drop the held item. If a primary MapClick reaches ordinary routing while an action is approaching, the server SHALL cancel that action, its pending effect, and its approach movement without a stamina charge before routing the new click. The new primary click SHALL then run its ordinary pickup, link, or movement behavior exactly once. Administrator-consumed clicks SHALL leave the gameplay action unchanged.

Direction-target selection SHALL consume primary input as a direction request before ordinary routing, after any pending primary administrator command. Target entity ID SHALL NOT lock a victim. A rejected or stale tagged direction attempt SHALL NOT fall through. During committed combat, primary clicks SHALL request only coordinate movement under current combat limits; they SHALL NOT link, pick up, drop a held item, or queue an attack. Administrator-consumed clicks SHALL retain their existing precedence.

#### Scenario: Object click requests a link
- **WHEN** a player primary-clicks a live non-dropped object with a collider and no pending administrator action and no armed gameplay action
- **THEN** the server SHALL set a link intent for that object, the player SHALL move to and link with it on confirmed collision, and no context action SHALL execute from the click alone

#### Scenario: Ground click uses coordinates and clears link intent
- **WHEN** a player primary-clicks empty ground, a stale target, or a target without a collider with no pending administrator action and no armed gameplay action
- **THEN** movement SHALL target the supplied coordinates and any outstanding link intent SHALL be cleared

#### Scenario: Stale ordinary target
- **WHEN** a normal primary map click reports a target that no longer exists
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

#### Scenario: Held item does not override armed action
- **WHEN** a player holds an item and primary-clicks the map while selecting any object- or tile-target action
- **THEN** the client SHALL send MapClick and SHALL NOT send item-in-hand drop; an unconsumed object-target click on empty ground SHALL move normally without dropping the item

#### Scenario: Ordinary click replaces an approach
- **WHEN** a player primary-clicks another object or empty ground while an action is approaching its previous target
- **THEN** the server SHALL cancel the old action and its pending effect before starting the new ordinary link or movement, and a late old-target callback SHALL NOT apply the canceled effect

#### Scenario: Combat click movement
- **WHEN** an untagged primary MapClick arrives during axe recovery on a live collider object
- **THEN** it SHALL request coordinate movement without linking, pickup, or canceling recovery

### Requirement: Secondary map clicks route carry placement before object interaction

The existing behavior and scenarios below apply to noncombat actions unless explicitly identified as combat. Committed combat follows the exception stated in this requirement.
For each secondary MapClick, the server SHALL first cancel any active gameplay action in selecting, approaching, or executing phase and its incomplete work without an action stamina charge, then route the same click exactly once using current authoritative carry and target state. The secondary click SHALL NOT select or retarget the canceled action, consume a pending administrator command, or fall through to ordinary primary movement or hand-item dropping.

If the player carries a world object, the server SHALL request put-down at the clicked coordinates regardless of the target ID, including zero, stale targets, and dropped items. Placement SHALL use the supplied click position rather than the clicked object's position or a tile center, and SHALL retain existing deferred approach and placement validity rules. Rejection or failure SHALL preserve a valid carried object and report the existing placement reason without falling through to pickup, context interaction, or ordinary movement.

Without a carried world object, a live dropped-item target SHALL initiate the existing pickup flow exactly once. A live non-dropped collider object SHALL use existing server-computed context actions: zero actions means ignore; one action means auto-select unless the object requests a menu even for one action; multiple actions means open the menu. An unavailable target, empty ground, or a non-dropped object without a collider SHALL start no interaction or movement. Existing unrelated movement SHALL NOT be stopped merely by a secondary ground click; canceling action-owned movement remains required. Inventory items in hand SHALL remain in hand.

The normal cancellation/routing rules SHALL continue for noncombat actions and uncommitted combat selection. A committed combat execution, including recovery, SHALL instead consume secondary clicks without cancellation, placement, pickup, context action, or queued work. The directional-release handoff remains a movement-only operation and SHALL NOT cancel combat.

#### Scenario: Carry placement outranks a dropped item
- **WHEN** a player carrying a world object secondary-clicks a dropped item
- **THEN** the server SHALL request put-down at the clicked coordinates without picking up the dropped item or opening a context menu

#### Scenario: Carry placement uses the click on an object
- **WHEN** a carrying player secondary-clicks a point on an object's rendered area that differs from the object's world position
- **THEN** the placement request SHALL use the clicked coordinates rather than the target object's position

#### Scenario: Carry placement on ground or stale target
- **WHEN** a carrying player secondary-clicks ground or a target that no longer exists
- **THEN** the server SHALL request put-down at the clicked coordinates using the existing placement flow

#### Scenario: Invalid placement has no fallback
- **WHEN** a carrying player's secondary placement request is rejected or fails
- **THEN** the server SHALL preserve valid carry state, report the placement reason, and SHALL NOT run pickup, context interaction, or ordinary movement as a fallback

#### Scenario: Ground cancels an armed action without moving
- **WHEN** a player without a carried world object secondary-clicks empty ground with an active gameplay action
- **THEN** the action and its incomplete work SHALL cancel, action-owned approach movement SHALL stop, and no new movement SHALL start

#### Scenario: Stale secondary target does not become movement
- **WHEN** a player without a carried world object secondary-clicks an unavailable target
- **THEN** the server SHALL cancel any active gameplay action and SHALL NOT start movement or substitute another interaction target

#### Scenario: Ground does not interrupt unrelated movement
- **WHEN** a player without a carried world object secondary-clicks ground while moving through an ordinary primary-click intent and without an active gameplay action
- **THEN** the secondary click SHALL start no new movement and SHALL leave the existing ordinary movement unchanged

#### Scenario: Context menu keeps server action rules
- **WHEN** a non-carrying player secondary-clicks a live collider object
- **THEN** the server SHALL ignore zero available actions, auto-select one action unless a single-action menu is required, or open a menu for multiple actions, using the same server-authoritative context rules as before

#### Scenario: Secondary pickup without carry
- **WHEN** a player without a carried world object secondary-clicks a live dropped item
- **THEN** the server SHALL run the existing pickup flow once after canceling any active gameplay action

#### Scenario: Secondary input retains a hand item
- **WHEN** a player with an inventory item in hand secondary-clicks an object or ground
- **THEN** the client SHALL send secondary MapClick without dropToWorld and SHALL retain the inventory item in hand

#### Scenario: Committed attack consumes secondary input
- **WHEN** a player secondary-clicks ground or an object during axe windup or recovery
- **THEN** the action and its cost/timers SHALL remain intact, no secondary gameplay work SHALL begin, and pending administrator state SHALL remain pending

### Requirement: Administrator click actions are interpreted only by the server

The existing behavior and scenarios below apply to noncombat actions unless explicitly identified as combat. Committed combat follows the exception stated in this requirement.
For primary MapClick only, the server SHALL give one pending administrator click action the highest precedence over both armed gameplay actions and ordinary primary map-click behavior. Secondary MapClick SHALL use secondary routing and SHALL NOT consume or clear any pending administrator click command. `/spawn` and coordinate-less `/tp` SHALL consume click coordinates; `/info` and `/destroy` SHALL consume the target ID. The consumed click SHALL NOT also initiate armed action execution, movement, pickup, or context interaction. The server SHALL preserve current command availability, resolve object targets against the current live world, and reject unavailable or ineligible targets without substituting another object. `/destroy` SHALL preserve deletion of the selected non-player object and all inventory contents through its existing deletion operation.

Secondary input during committed combat SHALL follow the combat busy exception: it SHALL preserve both the attack and the pending administrator selection without ordinary secondary work. The primary administrator precedence and selected-object deletion contract SHALL remain unchanged.

#### Scenario: Spawn or teleport on an object
- **WHEN** an administrator awaiting `/spawn` or `/tp` sends a primary map click on an object
- **THEN** the command SHALL use the clicked coordinates rather than the object's center

#### Scenario: Destroy a dropped item
- **WHEN** an administrator awaiting `/destroy` primary-clicks a dropped item
- **THEN** the server SHALL execute destruction and SHALL NOT pick up the item

#### Scenario: Invalid object selection
- **WHEN** an administrator awaiting `/info` or `/destroy` sends a primary map click with zero or an unavailable target ID
- **THEN** the server SHALL report the appropriate target error, consume the pending attempt, and SHALL NOT execute ordinary movement

#### Scenario: Player destruction is rejected
- **WHEN** an administrator awaiting `/destroy` selects a player
- **THEN** the server SHALL reject destruction and consume the attempt

#### Scenario: Pending selection is isolated and transient
- **WHEN** a pending action is consumed, replaced by another click-target administrator command, or its player leaves the active world or dies
- **THEN** the server SHALL clear that player's obsolete pending state without altering other players' pending state

#### Scenario: Context interaction is not selection
- **WHEN** a player with a pending object-selection command sends a secondary MapClick
- **THEN** the server SHALL cancel any active gameplay action and process secondary routing without consuming that command's pending selection

#### Scenario: Administrator command outranks armed action
- **WHEN** an administrator awaiting `/destroy` primary-clicks an object while having an armed gameplay action
- **THEN** the administrator command SHALL consume the click and the armed action SHALL NOT execute or change

#### Scenario: Secondary carry placement leaves a command pending
- **WHEN** an administrator carrying a world object secondary-clicks an object or ground while awaiting a click-target administrator command
- **THEN** the server SHALL process carry placement using the clicked coordinates and SHALL leave the administrator command pending

#### Scenario: Pending command during committed attack
- **WHEN** an administrator with a pending info selection secondary-clicks while an axe attack is committed
- **THEN** neither the attack nor the administrator selection SHALL be canceled and no secondary gameplay work SHALL start
