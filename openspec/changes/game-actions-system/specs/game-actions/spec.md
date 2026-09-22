# Spec Delta

## Purpose

Provide server-owned, data-defined gameplay actions accessible from an Actions menu and hotbar. An action may need no target, an object, or a tile; may have skill and equipped-item requirements; and may execute immediately or over game ticks. Lift and put-down are the first player-facing actions.

## ADDED Requirements

### Requirement: Action definitions separate target, requirements, and execution

The server SHALL load action definitions from data files at startup. Each definition SHALL have an ID, label, local menu icon asset path under /assets/, and separate target, requirements, and execution sections. Icon paths SHALL NOT reference external URLs or traverse outside /assets/. Target kind SHALL be none, object, or tile. An object/tile action MAY specify a cursor and MAY set isRepeatable (default false); a none action SHALL NOT repeat automatically or require a target cursor. Execution duration in ticks and stamina cost SHALL be independent optional non-negative values. Missing duration SHALL execute without a timed cycle; missing stamina cost SHALL charge none.

Requirements SHALL support a list of skill IDs, all of which must be present, and a list of equipped-item requirements. Each equipped-item requirement SHALL name exactly one item key or item tag and one or more equipment slots. Every requirement entry SHALL be satisfied; an item matching the key or tag in any listed slot satisfies that entry. The loader SHALL reject malformed or duplicate definitions with an error naming the file. Every loaded definition SHALL have a registered handler, and every registered handler SHALL have a definition. A new action using an existing target kind SHALL NOT require an action-specific map-click or protocol branch.

#### Scenario: Valid definitions load
- **WHEN** the server starts with well-formed lift and lift_down definitions and matching handlers
- **THEN** both SHALL be registered with their target, requirement, execution, cursor, and repeatability values

#### Scenario: Invalid definition fails startup
- **WHEN** a definition has a duplicate ID, unknown equipment slot, malformed equipment selector, invalid numeric cost, or no registered handler
- **THEN** startup SHALL fail with an error that identifies the offending definition file

#### Scenario: Multiple equipment entries
- **WHEN** an action requires a tagged tool in either hand and an exact item key on the back
- **THEN** the requirement SHALL pass only when both entries are satisfied, with either hand accepted for the first entry

### Requirement: The server provides action definitions and availability

During enter-world bootstrap the server SHALL send every action definition to the client with its current availability and an unavailable reason when applicable. The server SHALL refresh availability when skills, equipment, carry state, or another supported requirement changes. Availability shown in the client SHALL NOT replace validation by the server at activation or execution.

#### Scenario: Actions arrive on enter world
- **WHEN** a player enters the world
- **THEN** the client SHALL receive lift and lift_down with their current availability, followed by or alongside the authoritative action state

#### Scenario: Equipment change updates menu
- **WHEN** a player's equipment change makes an action unavailable
- **THEN** the server SHALL update its availability and reason in the client's action list

### Requirement: Activation and active state are server-owned

The client SHALL request activation by action ID. The server SHALL validate that the action exists, its skills, equipment, and handler state condition hold, and one execution is affordable before starting it. An unavailable action SHALL produce a mini-alert and SHALL NOT start. A target action SHALL enter a selecting phase with its action ID and optional cursor; a none action SHALL start one execution immediately. The client SHALL receive authoritative action ID, phase, and cursor updates and SHALL NOT infer armed state from the cursor ID.

Activating the same action while selecting SHALL toggle it off. Activating a different action while selecting, approaching, or executing SHALL cancel the old action without charging stamina, stop movement initiated for its target, and then attempt the new action. If the new action cannot start, the server SHALL report the reason and remain idle.

#### Scenario: Arm lift
- **WHEN** a player activates lift while eligible
- **THEN** the server SHALL enter selecting for lift and send the lift cursor

#### Scenario: Action without target
- **WHEN** a player activates an available none action
- **THEN** the server SHALL start exactly one execution without entering target selection or showing a target cursor

#### Scenario: Switch during a cycle
- **WHEN** a player activates another action during an active timed cycle or action-owned approach
- **THEN** the server SHALL cancel the old work without a stamina charge, stop its approach movement, and attempt the new action

### Requirement: Requirements remain valid throughout execution

The server SHALL recheck requirements and handler state at target acceptance, while an action is active when relevant state changes, and immediately before successful completion. If a required skill, equipped item, or state condition is lost, the server SHALL cancel the action, stop its action-owned movement, reset its cursor, and charge no stamina. Insufficient stamina SHALL prevent starting or completing an action without producing its effect.

#### Scenario: Equipped item removed during a cycle
- **WHEN** a player unequips an item required by an active action
- **THEN** that action SHALL cancel with no effect or stamina cost and the client SHALL receive an idle action state

#### Scenario: Carry loss while put-down is armed
- **WHEN** a player selecting lift_down loses the carried object
- **THEN** the server SHALL cancel lift_down and reset its cursor

### Requirement: Optional timed execution charges stamina only on success

An action with a positive tick duration SHALL use the existing cyclic-action progress and finish flow after its target is accepted, or immediately after activation for a none action. An action without a tick duration SHALL start its handler without a timed cycle; an existing domain transition MAY complete later. A positive stamina cost SHALL be charged exactly once only after successful completion. Rejection, timeout, cancellation, requirement loss, and failed completion SHALL not charge stamina or leave a partial action effect.

#### Scenario: Timed action completes
- **WHEN** an eligible test action has positive ticks and stamina and its handler completes successfully
- **THEN** the server SHALL emit cycle progress/finish, apply the effect once, and charge the declared stamina once

#### Scenario: Escape interrupts a timed action
- **WHEN** a player cancels before the completion tick
- **THEN** the cycle SHALL end without its effect or stamina cost

#### Scenario: Instant action
- **WHEN** a test action has no tick duration
- **THEN** its handler SHALL run without cyclic progress and any declared stamina SHALL be charged only on success

### Requirement: Target clicks are dispatched by target kind

After a pending administrator click and before ordinary click behavior, the server SHALL offer MapClick to the active target action. The dispatcher SHALL use target kind and SHALL NOT have branches for individual action IDs. An object-target action SHALL consume a click on a live object; an invalid object SHALL produce a mini-alert without movement and leave the action selecting. An object-target action SHALL let a click on empty ground or a stale target use ordinary movement while staying selecting. A tile-target action SHALL consume every map click and pass its coordinates to its handler. A handler rejection SHALL leave the action selecting while its requirements hold.

#### Scenario: Non-liftable object
- **WHEN** a player selecting lift clicks a live object that cannot be lifted
- **THEN** the server SHALL alert, SHALL NOT move or pick up anything from that click, and SHALL leave lift selecting

#### Scenario: Empty ground with object action
- **WHEN** a player selecting lift clicks empty ground
- **THEN** ordinary movement SHALL occur and lift SHALL remain selecting

#### Scenario: Tile target
- **WHEN** a player selecting a test tile action clicks an object or empty ground
- **THEN** the handler SHALL receive the clicked coordinates and ordinary pickup/movement SHALL NOT also run

### Requirement: Completion and repeatability follow the definition

On success, a non-repeatable action SHALL end and reset its cursor. A repeatable object/tile action SHALL return to target selection for another click while its requirements still hold. A none action SHALL end after one execution regardless of repeatability. Both lift and lift_down SHALL be non-repeatable and SHALL have no action-specific tick duration or stamina cost.

#### Scenario: Repeatable target action
- **WHEN** a test tile action with isRepeatable true completes successfully
- **THEN** it SHALL return to selecting with its cursor and accept a later target click

#### Scenario: Non-repeatable action
- **WHEN** lift or lift_down completes successfully
- **THEN** it SHALL end and the server SHALL send an idle action state with an empty cursor

### Requirement: Escape cancels the action before closing windows

The first Escape while an action is selecting, approaching, or executing SHALL send a cancellation request. The server SHALL clear the action and its action-owned pending work, stop its approach movement, charge no stamina for incomplete work, and send an idle action state with an empty cursor. An open window SHALL remain open on that Escape; a later Escape MAY close it through existing window handling. A server-initiated cancellation or reset SHALL be applied by the client at any time. Known cursor IDs SHALL use client cursor assets, unknown IDs SHALL use CSS help, and an empty ID SHALL restore the default cursor.

#### Scenario: Escape during approach
- **WHEN** a player presses Escape while moving toward an object selected for an action
- **THEN** the approach and action SHALL cancel, action-owned movement SHALL stop, and the cursor SHALL reset

#### Scenario: Unknown cursor ID
- **WHEN** the server sends an active action state with an unknown cursor ID
- **THEN** the client SHALL render the standard CSS help cursor

### Requirement: Lift and put-down use Actions exclusively

Lift SHALL not appear in an object context menu or start from implicit Interact on an object without a collider. The only player entry points for lift and lift_down SHALL be the Actions menu and hotbar. Armed lift SHALL reuse the existing collider move-to-link path or the no-collider approach path and SHALL complete only when the object is actually carried. Armed lift_down SHALL show the placement ghost while selecting and use the existing placement validation and deferred transition. An invalid target or rejected placement SHALL alert and leave the action selecting while its requirements hold. A successful lift SHALL NOT auto-arm lift_down.

#### Scenario: Collider object is lifted
- **WHEN** a player selects lift, clicks a liftable collider object, and reaches it
- **THEN** it SHALL be carried and the non-repeatable lift action SHALL end

#### Scenario: No-collider object is lifted
- **WHEN** a player selects lift and clicks a liftable object without a collider
- **THEN** its existing approach and carry rules SHALL run, and ordinary Interact alone SHALL NOT lift it

#### Scenario: Rejected placement
- **WHEN** a player selecting lift_down clicks an invalid placement position
- **THEN** the player SHALL receive a mini-alert, remain carrying, and remain selecting lift_down

#### Scenario: Successful placement
- **WHEN** placement completes successfully
- **THEN** the carry state and action SHALL end and the cursor SHALL reset

### Requirement: Actions menu and hotbar expose the server list

Clicking the Actions button in the left HUD rail SHALL toggle a compact, non-modal Actions panel beside that button. The panel SHALL show one horizontal row of action icons, in server-list order, without wrapping; it MAY scroll horizontally when needed. Each icon SHALL expose the action label accessibly and show the label on hover or keyboard focus. An action whose requirements are unmet SHALL remain visible as non-activatable and SHALL expose its unavailable state accessibly; it SHALL remain keyboard-focusable so hover/focus can show the reason. The icon for the active action SHALL have a visible and accessible selected state, including when the panel opens after the action was armed.

Selecting an available icon SHALL send the activation request and close the panel. Dragging an icon to a hotbar slot SHALL pin it. Pressing the Actions button again or clicking outside the panel SHALL close it; clicking inside the panel SHALL NOT count as an outside click. Gameplay action IDs SHALL be pinnable to hotbar slots alongside existing window openers. Menu selection and a pinned slot SHALL send the same activation request. Persisted gameplay IDs SHALL remain intact while the server action list is loading; after the list arrives, an ID absent from it SHALL behave as an empty slot and SHALL send nothing.

#### Scenario: Unavailable action remains visible
- **WHEN** a player without a carried object opens the Actions menu
- **THEN** lift_down SHALL appear unavailable with a carry-related reason

#### Scenario: Pinned action activates
- **WHEN** a player activates a hotbar slot pinned to lift
- **THEN** the client SHALL send the same activation request as the Actions menu

#### Scenario: Stored action before list arrives
- **WHEN** localStorage contains a valid gameplay action ID but the server list has not arrived yet
- **THEN** loading SHALL NOT delete the ID; after the list arrives it SHALL become usable if the server provides it

#### Scenario: Stale action ID
- **WHEN** the loaded server list does not provide a stored gameplay action ID
- **THEN** that hotbar slot SHALL behave as empty and activation SHALL send nothing

#### Scenario: Actions button toggles the panel
- **WHEN** the player clicks Actions in the left HUD rail
- **THEN** the one-row icon panel SHALL open, and another click on Actions or a click outside it SHALL close the panel

#### Scenario: Action icons follow server order and indicate active state
- **WHEN** the panel is open
- **THEN** icons SHALL appear left to right in server-list order, and the active action SHALL be visibly selected

#### Scenario: Unavailable icon explains its status
- **WHEN** an unavailable action icon receives hover or keyboard focus
- **THEN** its label and unavailability reason SHALL be shown
