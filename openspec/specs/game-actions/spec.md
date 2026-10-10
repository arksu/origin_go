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

For activation through the Actions menu or hotbar, the client SHALL request activation by action ID. Secondary map-click put-down SHALL use the separate carry shortcut rather than requiring a client activation request. The server SHALL validate that the action exists, its skills, equipment, and handler state condition hold, and one execution is affordable before starting it. A rejected activation SHALL produce a mini-alert, SHALL NOT start the requested action, and SHALL NOT refresh the action list. A target action activated through the Actions menu or hotbar SHALL enter a selecting phase with its action ID and optional cursor; a none action SHALL start one execution immediately. The client SHALL receive authoritative action ID, phase, and cursor updates and SHALL NOT infer armed state from the cursor ID.

For an untimed action whose handler transitions synchronously, the server SHALL send the resulting stable phase without first sending a transient executing state. A timed action SHALL send executing when its cycle starts.

Activating the same action while selecting SHALL toggle it off. Unless the requested action is still cooling down, activating a different action while selecting, approaching, executing, or waiting for cooldown SHALL cancel the old action without charging stamina, stop movement initiated for its target, and then attempt the new action. If the new action cannot start, the server SHALL report the reason and remain idle. Once a repeatable target action is armed, temporary lack of stamina SHALL NOT itself disarm it; target attempts SHALL still require enough stamina.

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

After a pending primary administrator click and before ordinary primary click behavior, the server SHALL offer only primary MapClick to the active target action. Secondary MapClick SHALL cancel the active action before secondary routing and SHALL NOT be offered as a target or retarget for that action. The dispatcher SHALL use target kind and SHALL NOT have branches for individual action IDs. An object-target action SHALL consume a primary click on a live object; an invalid object SHALL produce a mini-alert without movement and leave the action selecting. An object-target action SHALL let a primary click on empty ground or a stale target use ordinary movement while staying selecting. A tile-target action SHALL consume every primary map click while selecting and pass its coordinates to its handler. A tile action using tile-center approach SHALL also consume primary map clicks while approaching: an accepted click SHALL retarget the approach to the newly clicked tile, and a rejected click SHALL alert while preserving the original approach. A handler rejection while selecting SHALL leave the action selecting while its non-stamina requirements hold.

#### Scenario: Non-liftable object
- **WHEN** a player selecting lift primary-clicks a live object that cannot be lifted
- **THEN** the server SHALL alert, SHALL NOT move or pick up anything from that click, and SHALL leave lift selecting

#### Scenario: Empty ground with object action
- **WHEN** a player selecting lift primary-clicks empty ground
- **THEN** ordinary movement SHALL occur and lift SHALL remain selecting

#### Scenario: Tile target
- **WHEN** a player selecting a test tile action primary-clicks an object or empty ground
- **THEN** the handler SHALL receive the clicked coordinates and ordinary pickup/movement SHALL NOT also run

#### Scenario: Tile click while approaching retargets
- **WHEN** a tile action using tile-center approach is approaching tile A and the player primary-clicks eligible tile B
- **THEN** the action SHALL target tile B instead of tile A without canceling

#### Scenario: Rejected retarget keeps the original tile
- **WHEN** a tile action using tile-center approach is approaching tile A and the player primary-clicks an ineligible tile
- **THEN** the server SHALL alert, the approach SHALL continue to tile A, and the action SHALL remain approaching

#### Scenario: Secondary click does not retarget an armed tile action
- **WHEN** a player selecting or approaching a tile-target action secondary-clicks another tile
- **THEN** the current action SHALL cancel and secondary routing SHALL run without selecting or retargeting that action

### Requirement: Timed target actions may repeat execution on one accepted target

An object- or tile-target action with a positive `execution.ticks` MAY declare `execution.repeat: true`. If omitted or false, one accepted target click SHALL start at most one execution cycle. If true, each successful nonterminal cycle SHALL start another cycle on the same target without another click, until the handler reports a successful terminal cycle, a cycle fails, the target or requirements become invalid, or the player cancels the action. Each cycle SHALL use the declared tick duration and SHALL apply its effect and stamina cost at most once. The server SHALL revalidate the target, actual arrival position for tile-center actions, and requirements before each effect and before beginning another cycle. A stopped or canceled unfinished cycle SHALL produce no effect or action stamina charge. A new cycle SHALL reset progress and advance its cycle identity; the repeated attempt SHALL send a terminal finish when it ends.

`execution.repeat` SHALL be independent of `isRepeatable`: the former repeats cycles on the selected target, while the latter determines whether an ended target attempt returns to target selection. The loader SHALL reject `execution.repeat: true` on a targetless action or an action without a positive tick duration, identifying the definition file. Existing definitions without the field SHALL retain their behavior.

#### Scenario: Automatic next cycle
- **WHEN** a timed tile action with `execution.repeat: true` completes a successful cycle and its target and requirements remain valid
- **THEN** a fresh timed cycle SHALL begin on the same tile without another click

#### Scenario: Existing repeatable action keeps click-per-attempt behavior
- **WHEN** `plow_tile` completes a cycle without `execution.repeat: true`
- **THEN** it SHALL return to target selection and SHALL NOT start another cycle until a new target click

#### Scenario: Invalid automatic-repeat definition
- **WHEN** an action with no target or no positive tick duration declares `execution.repeat: true`
- **THEN** action definitions SHALL fail to load with an error identifying the file

#### Scenario: Cancellation does not start another cycle
- **WHEN** a player cancels an automatically repeating action during a cycle
- **THEN** that cycle SHALL end without an effect or stamina charge and no successor cycle SHALL start

#### Scenario: Successful terminal cycle
- **WHEN** a repeating handler reports that its current successful cycle is terminal
- **THEN** that cycle SHALL retain its effect and stamina charge while no successor cycle starts

### Requirement: Completion and repeatability follow the definition

On success, a non-repeatable action without `execution.repeat: true` SHALL end and reset its cursor. A non-repeatable target action with `execution.repeat: true` SHALL remain executing after each successful cycle and SHALL end and reset its cursor when its repeated attempt stops. A repeatable object/tile action SHALL return to target selection for another click after its attempt ends, even if stamina is temporarily insufficient. For an automatically repeating action, an attempt includes all consecutive cycles on the accepted target. Non-stamina requirement loss and explicit cancellation SHALL still end the action. A none action SHALL end after one execution. Both lift and lift_down SHALL be non-repeatable and SHALL have no action-specific tick duration or stamina cost.

#### Scenario: Repeatable target action
- **WHEN** a repeatable tile action without automatic execution repeat completes successfully
- **THEN** it SHALL return to selecting with its cursor and accept a later target click

#### Scenario: Repeatable action remains armed after spending its stamina
- **WHEN** a repeatable tile action without automatic execution repeat succeeds and leaves too little stamina for another attempt
- **THEN** it SHALL remain selecting with its cursor, and a later click SHALL be rejected until stamina recovers

#### Scenario: Non-repeatable action
- **WHEN** lift or lift_down completes successfully
- **THEN** it SHALL end and the server SHALL send an idle action state with an empty cursor

#### Scenario: Non-repeatable automatically repeating action ends after the sequence
- **WHEN** a non-repeatable targeted action with `execution.repeat: true` completes one cycle and then its repeated attempt stops
- **THEN** it SHALL not require a click between successful cycles and SHALL become idle with an empty cursor after the stop

### Requirement: Escape cancels the action before closing windows

The first Escape while an action is selecting, approaching, or executing SHALL send a cancellation request. The server SHALL clear the action and its action-owned pending work, stop its approach movement, charge no stamina for incomplete work, and send an idle action state with an empty cursor. An open window SHALL remain open on that Escape; a later Escape MAY close it through existing window handling. A server-initiated cancellation or reset SHALL be applied by the client at any time. Known cursor IDs SHALL use client cursor assets, unknown IDs SHALL use CSS help, and an empty ID SHALL restore the default cursor.

#### Scenario: Escape during approach
- **WHEN** a player presses Escape while moving toward an object selected for an action
- **THEN** the approach and action SHALL cancel, action-owned movement SHALL stop, and the cursor SHALL reset

#### Scenario: Unknown cursor ID
- **WHEN** the server sends an active action state with an unknown cursor ID
- **THEN** the client SHALL render the standard CSS help cursor

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

While an object/tile action is selecting, approaching, or executing, the client SHALL send a primary map click as MapClick even when an inventory item is in hand, and SHALL NOT send dropToWorld for that click. The client SHALL determine this priority from the authoritative action ID and phase together with the catalog target kind, rather than from a non-empty visual cursor. An empty visual cursor during approach or execution SHALL NOT restore hand-item dropping. The server SHALL retain control of target acceptance, approach retargeting, and ordinary map-click routing; an executing click SHALL NOT be queued as a later action attempt. With no active target action, existing primary-click hand-item dropping SHALL remain available. Secondary map clicks SHALL always send MapClick without hand-item dropping, regardless of active action state.

#### Scenario: Select a target with an item in hand
- **WHEN** a player holding an inventory item primary-clicks the map while a target action is selecting
- **THEN** the client SHALL send MapClick without dropToWorld and the item SHALL remain in hand

#### Scenario: Retarget an approach with an item in hand
- **WHEN** a player holding an inventory item primary-clicks another tile while a tile-center action is approaching
- **THEN** the client SHALL send MapClick without dropToWorld so the server can accept or reject the retarget, and the item SHALL remain in hand

#### Scenario: Execution does not drop the held item
- **WHEN** a player holding an inventory item primary-clicks the map while a target action is executing
- **THEN** the client SHALL send MapClick without dropToWorld, SHALL leave the item in hand, and SHALL not queue another action attempt

#### Scenario: No target action is active
- **WHEN** a player holding an inventory item primary-clicks the map without an active target action
- **THEN** the existing dropToWorld behavior SHALL remain available

#### Scenario: Secondary click never drops the inventory hand item
- **WHEN** a player holding an inventory item secondary-clicks the map in idle, selecting, approaching, or executing phase
- **THEN** the client SHALL send secondary MapClick without dropToWorld, and the server SHALL cancel any active gameplay action before secondary routing

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

### Requirement: Secondary map clicks cancel active gameplay actions before routing
A secondary map click SHALL cancel an active gameplay action in selecting, approaching, or executing phase before the same click is processed through ordinary secondary routing. Cancellation SHALL clear the action, its pending effects, its cycle and action-owned approach movement, and reset its cursor without an action stamina charge for incomplete work. A late callback from the canceled attempt SHALL NOT apply its effect or alter a newer attempt. The canceled action SHALL NOT consume the secondary click as a target, retarget, or queued execution. Administrator pending state SHALL remain pending. Existing Escape cancellation SHALL remain available.

#### Scenario: Cancel selecting action and interact
- **WHEN** a non-carrying player selecting lift secondary-clicks a live object with context actions
- **THEN** lift SHALL cancel and the same click SHALL run normal secondary object interaction without attempting lift

#### Scenario: Cancel an approach
- **WHEN** a non-carrying player secondary-clicks ground while a gameplay action approaches its target
- **THEN** the action, pending effect, and action-owned movement SHALL cancel, the cursor SHALL reset, and no new movement SHALL start

#### Scenario: Cancel an executing cycle
- **WHEN** a player secondary-clicks the map before an executing gameplay cycle completes
- **THEN** the cycle SHALL cancel without its effect or action stamina charge, secondary routing SHALL run once, and the old execution SHALL NOT resume

#### Scenario: Pending administrator selection does not intercept cancellation
- **WHEN** a player with both a pending administrator click command and an active gameplay action secondary-clicks the map
- **THEN** the gameplay action SHALL cancel and secondary routing SHALL proceed while the administrator command remains pending

### Requirement: Lift uses Actions and put-down also supports secondary map clicks

Lift SHALL not appear in an object context menu or start from an ordinary secondary map click on an object without a collider. The player entry points for arming lift and lift_down SHALL remain the Actions menu and hotbar. Armed lift SHALL reuse the existing collider move-to-link path or the no-collider approach path and SHALL complete only when the object is actually carried. Armed lift_down SHALL show the placement ghost while selecting and use the existing placement validation and deferred transition. An invalid primary target or rejected primary placement SHALL alert and leave the explicit action selecting while its requirements hold. A successful lift SHALL NOT auto-arm lift_down.

A secondary map click while carrying a world object SHALL additionally request one put-down attempt without requiring explicit activation of lift_down. The server SHALL first cancel any existing gameplay action, then use the secondary click coordinates regardless of whether an object was clicked. This attempt SHALL retain the existing authoritative approach, placement, completion, and interruption rules and SHALL NOT enter an armed target-selection phase. A successful attempt SHALL end carry and return to idle. A rejected, failed, or timed-out secondary attempt SHALL preserve valid carry state, emit the existing placement error, clear the attempt and its approach movement, and return to idle with an empty cursor; it SHALL NOT arm lift_down. Escape SHALL cancel an accepted secondary approach, and another secondary click SHALL replace it with a new attempt. A late completion from a canceled or replaced attempt SHALL NOT relocate the object or alter the newer attempt.

#### Scenario: Collider object is lifted
- **WHEN** a player selects lift, primary-clicks a liftable collider object, and reaches it
- **THEN** it SHALL be carried and the non-repeatable lift action SHALL end

#### Scenario: No-collider object is lifted
- **WHEN** a player selects lift and primary-clicks a liftable object without a collider
- **THEN** its existing approach and carry rules SHALL run, and an ordinary secondary map click alone SHALL NOT lift it

#### Scenario: Rejected placement
- **WHEN** a player selecting lift_down primary-clicks an invalid placement position
- **THEN** the player SHALL receive a mini-alert, remain carrying, and remain selecting lift_down

#### Scenario: Successful placement
- **WHEN** placement completes successfully
- **THEN** the carry state and action SHALL end and the cursor SHALL reset

#### Scenario: Secondary placement succeeds without activation
- **WHEN** a carrying player secondary-clicks an object or ground without having activated lift_down and the placement succeeds
- **THEN** the server SHALL put the carried object at the clicked position through the existing deferred flow, end carry, and return to idle

#### Scenario: Secondary placement rejection does not arm an action
- **WHEN** a carrying player's secondary put-down request targets an invalid position
- **THEN** the player SHALL receive the existing placement error, remain carrying, and remain idle with no armed action or placement cursor

#### Scenario: Secondary placement timeout does not arm an action
- **WHEN** a secondary put-down approach times out or fails before successful placement
- **THEN** the attempt and its approach SHALL end, the player SHALL receive the existing placement error, and valid carry state SHALL remain with an idle action state

#### Scenario: Secondary placement replaces an armed put-down
- **WHEN** a player carrying an object with lift_down selecting secondary-clicks a different position
- **THEN** the selecting action SHALL cancel before a new single put-down attempt starts at the secondary click coordinates, without treating the click as a target of the canceled action

#### Scenario: Escape cancels secondary placement
- **WHEN** a player presses Escape during the approach initiated by a secondary put-down request
- **THEN** the attempt and its action-owned movement SHALL cancel without putting down the object or charging action stamina, valid carry state SHALL remain, and the action state SHALL become idle

#### Scenario: Replaced secondary placement cannot complete late
- **WHEN** a secondary put-down request for position A is replaced by a secondary request for position B and completion for A arrives late
- **THEN** the old completion SHALL NOT place the object at A or change the current attempt, and only the valid request for B SHALL be eligible to complete

### Requirement: Manual directional movement preserves selection and interrupts execution
Fresh valid nonzero directional input SHALL preserve an armed gameplay action in target-selection phase without selecting a target. When an action is approaching or executing, fresh directional input accepted under movement restrictions SHALL cancel that attempt through its normal cancellation lifecycle before manual movement starts. Cancellation SHALL discard pending effects and unfinished cycles, stop action-owned approach, charge no action stamina for unfinished work, and prevent late callbacks from affecting the canceled or a newer attempt. This rule SHALL also end an active context/crafting cycle through its existing cancellation and link lifecycle. Movement stamina already spent SHALL not be refunded.

Directional refreshes and releases SHALL NOT repeatedly cancel actions. Invalid, expired, obsolete-session, or movement-rejected directional input SHALL NOT cancel an action. Merely pushing against a wall SHALL not make a canceled action eligible to finish.

#### Scenario: Reposition while selecting
- **WHEN** the player holds an object- or tile-target action in selecting phase and presses WASD
- **THEN** the player SHALL move while the same action remains selecting, ready for an actual target click

#### Scenario: Interrupt target approach
- **WHEN** the player presses WASD during an action-owned approach
- **THEN** the approach and attempt SHALL end before directional movement begins, with no effect or action stamina charge for the incomplete attempt

#### Scenario: Interrupt an executing cycle
- **WHEN** the player presses WASD during a timed action, including while the requested direction is blocked by collision
- **THEN** the unfinished cycle SHALL be canceled and SHALL not grant its effect on that tick or from a late completion

#### Scenario: Context or crafting cycle
- **WHEN** accepted manual movement interrupts an active context/crafting cycle
- **THEN** its existing cancellation lifecycle SHALL end unfinished work and clear associated link state without granting a partial result

#### Scenario: Rejected movement does not cancel
- **WHEN** directional input is invalid, stale, or prohibited by current movement restrictions
- **THEN** the existing action SHALL remain governed by its own validation and cancellation rules

### Requirement: Explicit action requests relinquish an existing keyboard hold
When an explicit gameplay-action activation, context-action selection, or craft/build execution request is sent while keyboard control is active, the client SHALL first release that keyboard hold, suppress its already-held keys, and then send the existing request. The release SHALL affect only keyboard movement. An action that subsequently remains in selecting phase SHALL still allow a fresh WASD press under the selection-preservation rule. A rejected request SHALL not resume the old hold automatically. Existing mouse-only behavior and window-opening hotkeys SHALL remain unchanged.

#### Scenario: Activate from the hotbar while walking
- **WHEN** the player activates a gameplay action from the hotbar while holding W
- **THEN** the previous keyboard hold SHALL end before activation and its refreshes SHALL not cancel the new action

#### Scenario: New movement after selection
- **WHEN** activation leaves an action selecting and the player makes a fresh WASD press
- **THEN** manual movement SHALL be allowed while preserving that selection

#### Scenario: Activation rejected
- **WHEN** an explicit action request is rejected after keyboard handoff
- **THEN** the player SHALL remain stopped until a new movement input or route is issued

#### Scenario: Window hotkey
- **WHEN** the player opens a non-modal inventory or character window without focusing an editable control
- **THEN** that window hotkey alone SHALL not relinquish keyboard movement


### Requirement: Independent persisted action cooldowns

Each menu-action definition MAY declare top-level `cooldown` as a non-negative integer in milliseconds; zero or omission disables cooldown. The server SHALL enforce a separate deadline for each character and action, starting only on successful completion at the same commit point where action costs such as stamina are charged. Selection, approach, execution progress, rejection, failure, and cancellation before completion SHALL NOT start cooldown. Cooldown SHALL block new activation, armed target attempts and direct server-routed attempts without replacing current state.

For automatic execution repeat, every successfully completed cycle SHALL start its own cooldown when its costs are charged. The next cycle SHALL wait for that full cooldown to expire, retaining its target and generation. During `cooldown_wait`, execution animation/progress SHALL be hidden and no stamina SHALL be charged. Resume SHALL revalidate requirements, target and position; cancellation SHALL end the sequence without resumption and retain the completed cycle's cooldown.

Cooldown start and expiry times SHALL be included in ordinary character saves and restored across sessions and orderly restarts. Offline time SHALL count toward expiry. Reattachment SHALL preserve runtime deadlines, and transfer/rollback SHALL carry the current snapshot. Abrupt crashes retain the existing character-save durability guarantees.

#### Scenario: Successful completion starts the action cooldown
- **WHEN** a character successfully completes an action with a positive cooldown
- **THEN** the server SHALL start that character's cooldown for that action at the same commit point as its action costs
- **AND** new attempts of that action SHALL remain blocked until the deadline expires

### Requirement: Universal action icon presentation

All client action icon locations SHALL use `ActionIcon` and the shared action presentation interface. A single session clock and server-owned timestamps SHALL determine the clockwise dark overlay, clearing from 12 o'clock to reveal the icon. The Actions dropdown, duplicate hotbar assignments and icons mounted during cooldown SHALL show the same proportional progress. No per-icon countdown timer or click-derived cooldown SHALL exist. Cooldown SHALL block mouse, touch and keyboard activation while preserving dragging, tooltips, cancellation and hotbar clearing.

#### Scenario: Duplicate icons share cooldown progress
- **WHEN** an action is cooling down and its icon appears in the Actions dropdown and multiple hotbar slots
- **THEN** every icon SHALL show the same proportional cooldown overlay from the shared clock and server-owned timestamps
- **AND** activation SHALL remain blocked while dragging, tooltips, cancellation and hotbar clearing remain available
