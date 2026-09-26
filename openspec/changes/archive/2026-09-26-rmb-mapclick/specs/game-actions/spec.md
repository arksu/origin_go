# Spec Delta

## MODIFIED Requirements

### Requirement: Activation and active state are server-owned

For activation through the Actions menu or hotbar, the client SHALL request activation by action ID. Secondary map-click put-down SHALL use the separate carry shortcut rather than requiring a client activation request. The server SHALL validate that the action exists, its skills, equipment, and handler state condition hold, and one execution is affordable before starting it. A rejected activation SHALL produce a mini-alert, SHALL NOT start the requested action, and SHALL NOT refresh the action list. A target action activated through the Actions menu or hotbar SHALL enter a selecting phase with its action ID and optional cursor; a none action SHALL start one execution immediately. The client SHALL receive authoritative action ID, phase, and cursor updates and SHALL NOT infer armed state from the cursor ID.

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

## ADDED Requirements

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

## REMOVED Requirements

### Requirement: Lift and put-down use Actions exclusively
**Reason**: Put-down now also has a secondary map-click entry point while the player carries a world object.
**Migration**: Use the replacement requirement "Lift uses Actions and put-down also supports secondary map clicks". Explicit lift and lift_down activation, primary targeting, and the existing retry behavior remain available through the Actions menu and hotbar.
