# game-actions Specification

## Purpose

Provide server-owned, data-defined gameplay actions accessible from an Actions menu and hotbar. An action may need no target, an object, or a tile; may have skill and equipped-item requirements; and may execute immediately or over game ticks. Lift and put-down are the first player-facing actions.

## Requirements

### Requirement: Action definitions separate target, requirements, and execution

The server SHALL load action definitions from data files at startup. Each definition SHALL have an ID, label, local menu icon asset path under /assets/, and separate target, requirements, and execution sections. Icon paths SHALL NOT reference external URLs or traverse outside /assets/. Target kind SHALL be none, object, or tile. An object/tile action MAY specify a cursor and MAY set isRepeatable (default false); a none action SHALL NOT declare a target cursor or isRepeatable and SHALL NOT repeat automatically. A tile action MAY opt in to approaching the center of its target tile; the default SHALL leave existing tile actions unchanged. A none or object action SHALL NOT declare tile-center approach. Execution duration in ticks and stamina cost SHALL be independent optional non-negative values. Missing duration SHALL execute without a timed cycle; missing stamina cost SHALL charge none.

Requirements SHALL support a list of skill IDs, all of which must be present, and a list of equipped-item requirements. Each equipped-item requirement SHALL name exactly one item key or item tag and one or more equipment slots. Every requirement entry SHALL be satisfied; an item matching the key or tag in any listed slot satisfies that entry. The loader SHALL reject malformed or duplicate definitions, including a cursor or isRepeatable on a none action or tile-center approach on a none or object action, with an error naming the file. Every loaded definition SHALL have a registered handler, and every registered handler SHALL have a definition. A new action using an existing target kind SHALL NOT require an action-specific map-click or protocol branch.

#### Scenario: Valid definitions load
- **WHEN** the server starts with well-formed lift and lift_down definitions and matching handlers
- **THEN** both SHALL be registered with their target, requirement, execution, cursor, and repeatability values

#### Scenario: Tile-center approach is opt-in
- **WHEN** a tile action declares tile-center approach and another tile action omits it
- **THEN** the first action SHALL approach its tile center and the second SHALL retain its existing targeting behavior

#### Scenario: Invalid definition fails startup
- **WHEN** a definition has a duplicate ID, unknown equipment slot, malformed equipment selector, invalid numeric cost, isRepeatable or a cursor on a none action, tile-center approach on a non-tile action, or no registered handler
- **THEN** startup SHALL fail with an error that identifies the offending definition file

#### Scenario: Multiple equipment entries
- **WHEN** an action requires a tagged tool in either hand and an exact item key on the back
- **THEN** the requirement SHALL pass only when both entries are satisfied, with either hand accepted for the first entry

### Requirement: The server provides an action catalog

During enter-world bootstrap the server SHALL send every loaded action definition to the client in server-list order, followed by or alongside the authoritative action state. The action list SHALL contain no per-player availability flag or unavailable reason. Changes to skills, equipment, stamina, carry state, or other action requirements SHALL NOT cause the server to recalculate or resend the action list. A later enter-world bootstrap SHALL send the catalog again.

#### Scenario: Actions arrive on enter world
- **WHEN** a player enters the world
- **THEN** the client SHALL receive lift and lift_down without availability or unavailable-reason fields, followed by or alongside the authoritative action state

#### Scenario: Requirement changes do not refresh the catalog
- **WHEN** a player's equipment, stamina, skill, or carry state changes after the catalog arrives
- **THEN** the server SHALL NOT send an updated action list solely because of that change

#### Scenario: Re-entering the world receives the catalog
- **WHEN** a player enters the world again after leaving
- **THEN** the server SHALL send the current loaded action definitions again

### Requirement: Activation and active state are server-owned

The client SHALL request activation by action ID. The server SHALL validate that the action exists, its skills, equipment, and handler state condition hold, and one execution is affordable before starting it. A rejected activation SHALL produce a mini-alert, SHALL NOT start the requested action, and SHALL NOT refresh the action list. A target action SHALL enter a selecting phase with its action ID and optional cursor; a none action SHALL start one execution immediately. The client SHALL receive authoritative action ID, phase, and cursor updates and SHALL NOT infer armed state from the cursor ID.

For an untimed action whose handler transitions synchronously, the server SHALL send the resulting stable phase without first sending a transient executing state. A timed action SHALL send executing when its cycle starts.

Activating the same action while selecting SHALL toggle it off. Activating a different action while selecting, approaching, or executing SHALL cancel the old action without charging stamina, stop movement initiated for its target, and then attempt the new action. If the new action cannot start, the server SHALL report the reason and remain idle. Once a repeatable target action is armed, temporary lack of stamina SHALL NOT itself disarm it; target attempts SHALL still require enough stamina.

#### Scenario: Arm lift
- **WHEN** a player activates lift while eligible
- **THEN** the server SHALL enter selecting for lift and send the lift cursor

#### Scenario: Action without target
- **WHEN** a player activates an eligible none action
- **THEN** the server SHALL start exactly one execution without entering target selection or showing a target cursor

#### Scenario: Untimed approach does not send a transient state
- **WHEN** an untimed targeted action accepts a click and immediately starts an approach
- **THEN** the server SHALL send approaching without first sending executing for that click

#### Scenario: Switch during a cycle
- **WHEN** a player activates another action during an active timed cycle or action-owned approach
- **THEN** the server SHALL cancel the old work without a stamina charge, stop its approach movement, and attempt the new action

#### Scenario: Rejected activation does not change the catalog
- **WHEN** a player activates lift_down without carrying an object
- **THEN** the server SHALL send a mini-alert, SHALL leave the player without a newly armed action, and SHALL NOT send an action-list refresh

#### Scenario: Repeatable action waits for stamina
- **WHEN** a repeatable target action is armed and stamina becomes insufficient for another attempt
- **THEN** the action SHALL stay selecting with its cursor, and the next target click SHALL alert without an effect or action stamina charge

### Requirement: Requirements remain valid throughout execution

The server SHALL recheck requirements and handler state at target acceptance, while an action is active when relevant state changes, and immediately before successful completion. If a required skill, equipped item, or state condition is lost, the server SHALL cancel the action, stop its action-owned movement, reset its cursor, and charge no stamina. Insufficient stamina SHALL prevent starting or completing an attempt without producing its effect. While a repeatable target action is selecting, insufficient stamina SHALL leave it armed so another click can be attempted after recovery.

#### Scenario: Equipped item removed during a cycle
- **WHEN** a player unequips an item required by an active action
- **THEN** that action SHALL cancel with no effect or stamina cost and the client SHALL receive an idle action state

#### Scenario: Carry loss while put-down is armed
- **WHEN** a player selecting lift_down loses the carried object
- **THEN** the server SHALL cancel lift_down and reset its cursor

#### Scenario: Stamina becomes insufficient during a cycle
- **WHEN** a timed action loses the stamina needed for its cost before completion
- **THEN** that attempt SHALL end without effect or action stamina charge; a repeatable target action SHALL return to selection

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

After a pending administrator click and before ordinary click behavior, the server SHALL offer MapClick to the active target action. The dispatcher SHALL use target kind and SHALL NOT have branches for individual action IDs. An object-target action SHALL consume a click on a live object; an invalid object SHALL produce a mini-alert without movement and leave the action selecting. An object-target action SHALL let a click on empty ground or a stale target use ordinary movement while staying selecting. A tile-target action SHALL consume every map click while selecting and pass its coordinates to its handler. A tile action using tile-center approach SHALL also consume map clicks while approaching: an accepted click SHALL retarget the approach to the newly clicked tile, and a rejected click SHALL alert while preserving the original approach. A handler rejection while selecting SHALL leave the action selecting while its non-stamina requirements hold.

#### Scenario: Non-liftable object
- **WHEN** a player selecting lift clicks a live object that cannot be lifted
- **THEN** the server SHALL alert, SHALL NOT move or pick up anything from that click, and SHALL leave lift selecting

#### Scenario: Empty ground with object action
- **WHEN** a player selecting lift clicks empty ground
- **THEN** ordinary movement SHALL occur and lift SHALL remain selecting

#### Scenario: Tile target
- **WHEN** a player selecting a test tile action clicks an object or empty ground
- **THEN** the handler SHALL receive the clicked coordinates and ordinary pickup/movement SHALL NOT also run

#### Scenario: Tile click while approaching retargets
- **WHEN** a tile action using tile-center approach is approaching tile A and the player clicks eligible tile B
- **THEN** the action SHALL target tile B instead of tile A without canceling

#### Scenario: Rejected retarget keeps the original tile
- **WHEN** a tile action using tile-center approach is approaching tile A and the player clicks an ineligible tile
- **THEN** the server SHALL alert, the approach SHALL continue to tile A, and the action SHALL remain approaching

### Requirement: Completion and repeatability follow the definition

On success, a non-repeatable action SHALL end and reset its cursor. A repeatable object/tile action SHALL return to target selection for another click after an attempt completes or fails, even if stamina is temporarily insufficient. Non-stamina requirement loss and explicit cancellation SHALL still end it. A none action SHALL end after one execution. Both lift and lift_down SHALL be non-repeatable and SHALL have no action-specific tick duration or stamina cost.

#### Scenario: Repeatable target action
- **WHEN** a repeatable tile action completes successfully
- **THEN** it SHALL return to selecting with its cursor and accept a later target click

#### Scenario: Repeatable action remains armed after spending its stamina
- **WHEN** a repeatable tile action succeeds and leaves too little stamina for another attempt
- **THEN** it SHALL remain selecting with its cursor, and a later click SHALL be rejected until stamina recovers

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

### Requirement: Actions menu and hotbar expose selectable server actions

Clicking the Actions button in the left HUD rail SHALL toggle a compact, non-modal Actions panel beside that button. The panel SHALL show one horizontal row of action icons, in server-list order, without wrapping; it MAY scroll horizontally when needed. Each icon SHALL expose the action label accessibly and show the label on hover or keyboard focus. Every listed action SHALL remain selectable and pinnable regardless of the player's current requirements, without an unavailable style, disabled state, or precomputed reason. The icon for the active action SHALL have a visible and accessible selected state, including when the panel opens after the action was armed.

Selecting an icon SHALL send an activation request and close the panel immediately, including when the server later rejects the request. The server's mini-alert SHALL explain a rejected activation. Dragging an icon to a hotbar slot SHALL pin it. Pressing the Actions button again or clicking outside the panel SHALL close it; clicking inside the panel SHALL NOT count as an outside click. Gameplay action IDs SHALL be pinnable to hotbar slots alongside existing window openers. Menu selection and a pinned slot SHALL send the same activation request. Persisted gameplay IDs SHALL remain intact while the server action list is loading; after the list arrives, an ID absent from it SHALL behave as an empty slot and SHALL send nothing.

#### Scenario: Action without met requirements remains selectable
- **WHEN** a player without a carried object opens the Actions menu
- **THEN** lift_down SHALL appear like the other actions and SHALL remain selectable and pinnable

#### Scenario: Rejected selection closes the panel
- **WHEN** a player without a carried object selects lift_down from the Actions panel
- **THEN** the panel SHALL close immediately and the server SHALL send a mini-alert explaining why lift_down cannot start

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

#### Scenario: Icon does not precompute a rejection reason
- **WHEN** an action's requirements are unmet and its icon receives hover or keyboard focus
- **THEN** the icon SHALL show its label without an availability state or reason; an activation attempt SHALL receive the server's mini-alert

### Requirement: Active target actions take priority over hand-item dropping

While an object/tile action is selecting, approaching, or executing, the client SHALL send a primary map click as MapClick even when an inventory item is in hand, and SHALL NOT send dropToWorld for that click. The client SHALL determine this priority from the authoritative action ID and phase together with the catalog target kind, rather than from a non-empty visual cursor. An empty visual cursor during approach or execution SHALL NOT restore hand-item dropping. The server SHALL retain control of target acceptance, approach retargeting, and ordinary map-click routing; an executing click SHALL NOT be queued as a later action attempt. With no active target action, existing hand-item dropping SHALL remain available.

#### Scenario: Select a target with an item in hand
- **WHEN** a player holding an inventory item clicks the map while a target action is selecting
- **THEN** the client SHALL send MapClick without dropToWorld and the item SHALL remain in hand

#### Scenario: Retarget an approach with an item in hand
- **WHEN** a player holding an inventory item clicks another tile while a tile-center action is approaching
- **THEN** the client SHALL send MapClick without dropToWorld so the server can accept or reject the retarget, and the item SHALL remain in hand

#### Scenario: Execution does not drop the held item
- **WHEN** a player holding an inventory item clicks the map while a target action is executing
- **THEN** the client SHALL send MapClick without dropToWorld, SHALL leave the item in hand, and SHALL not queue another action attempt

#### Scenario: No target action is active
- **WHEN** a player holding an inventory item clicks the map without an active target action
- **THEN** the existing dropToWorld behavior SHALL remain available

### Requirement: Opt-in tile actions approach the tile center

For an action that opts in to tile-center approach, the server SHALL normalize each map click to its tile coordinate and use that tile's center as the movement target, regardless of the exact click position or whether the player is already inside the tile. The timed action SHALL start only when the player's actual, collision-resolved position reaches the center within the movement arrival tolerance. Arrival SHALL be reported as a discrete event rather than detected by polling the distance on every action tick. A blocked or interrupted approach SHALL end without effect or action stamina charge; a repeatable action SHALL remain armed. Immediately before completion, the server SHALL revalidate the target and confirm that the player's actual position is within movement arrival tolerance of the target center. Leaving the center and returning within tolerance before completion SHALL be allowed and SHALL NOT by itself invalidate the cycle. The approach timeout SHALL allow a normally moving player to reach a distant valid tile.

#### Scenario: Click targets the tile center
- **WHEN** a player clicks any point inside an eligible tile, including while already inside its bounds
- **THEN** the approach SHALL target that tile's center and the cycle SHALL not start before the center is reached

#### Scenario: Collision prevents false arrival
- **WHEN** a collider prevents the player from reaching the target tile's center
- **THEN** the server SHALL not start the cycle or change the tile; the approach SHALL eventually fail or time out

#### Scenario: Arrival starts the action
- **WHEN** the player's collision-resolved position reaches the remembered tile center
- **THEN** the timed cycle SHALL start for that tile

#### Scenario: Player is away from the center at completion
- **WHEN** a player is beyond movement arrival tolerance of the tile center at a timed tile action's completion check
- **THEN** completion SHALL fail without effect or action stamina charge

#### Scenario: Player returns before completion
- **WHEN** a player leaves the tile center during a timed tile action's cycle and returns within movement arrival tolerance before its completion check, with all other completion requirements satisfied
- **THEN** the attempt SHALL complete successfully

#### Scenario: Existing tile action keeps its behavior
- **WHEN** a tile action without tile-center approach, such as lift_down, receives a map click
- **THEN** its existing execution and placement behavior SHALL remain unchanged
