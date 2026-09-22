# Spec Delta

## Purpose

Define server-authoritative gameplay actions that players arm from an actions menu or hotbar: arming is validated by the server, acknowledged by switching a server-owned cursor image, and the next matching map click executes through a per-action handler. Lift and put-down are the first two actions and the reference consumers of the framework.

## ADDED Requirements

### Requirement: Action definitions are data-driven
The server SHALL load gameplay action definitions from data files at startup, each declaring an identifier, a display label, a cursor id, a target kind (`object`, `tile`, or `none`), and optional required skill ids. Definitions SHALL be validated on load, and a duplicate identifier or malformed definition SHALL fail startup with a load error naming the file. Adding a new action SHALL NOT require protocol or dispatcher changes — a definition plus an action handler are sufficient.

#### Scenario: Valid definitions load
- **WHEN** the server starts with well-formed action definition files including `lift` and `lift_down`
- **THEN** both actions SHALL be registered with their declared labels, cursor ids, target kinds, and skill requirements

#### Scenario: Duplicate identifier fails fast
- **WHEN** two action definition files declare the same identifier
- **THEN** server startup SHALL fail with a load error identifying the conflicting file

### Requirement: Action list is provided at enter world
During the enter-world snapshot the server SHALL send the client the list of all action definitions with their identifier, label, cursor id, target kind, and required skills, in the same manner as the existing craft and build list snapshots.

#### Scenario: Client receives actions at login
- **WHEN** a player enters the world
- **THEN** the client SHALL receive the action list containing `lift` and `lift_down` before or with the initial game state

#### Scenario: New definition reaches clients without client changes
- **WHEN** a future action definition is added to the server data
- **THEN** the action list delivered to clients SHALL include it with no client-side catalog change

### Requirement: Players arm actions and the server owns the armed state
The client SHALL request activation by action id. The server SHALL validate on activation that the action exists, that all required skills are satisfied, and that any state condition holds (put-down requires an active carry). On success the server SHALL set the player's armed action and send the active cursor change with the definition's cursor id. On failure the server SHALL reject activation with a mini-alert and SHALL NOT change the armed state or cursor. Activating the currently armed action again SHALL disarm it, and activating a different action SHALL replace the armed action.

#### Scenario: Arm lift
- **WHEN** a player activates `lift`
- **THEN** the server SHALL set the armed action and the client SHALL receive an active cursor change to `lift`

#### Scenario: Locked action is rejected
- **WHEN** a player activates an action whose required skills are not all satisfied by their character profile
- **THEN** the server SHALL reject activation with a mini-alert and the cursor SHALL remain unchanged

#### Scenario: Put-down requires carrying
- **WHEN** a player activates `lift_down` while not carrying an object
- **THEN** the server SHALL reject activation with a mini-alert and no armed state SHALL be set

#### Scenario: Re-activation toggles off
- **WHEN** a player activates the action they currently have armed
- **THEN** the server SHALL clear the armed action and send a cursor reset

### Requirement: Cursor state is server-owned and resettable
The client SHALL derive its game cursor exclusively from the last active cursor change received: a known cursor id SHALL render the matching image from the client cursor assets, an unknown cursor id SHALL render the standard question-mark cursor (CSS `help`), and an empty cursor id SHALL restore the default cursor. The server MAY send a cursor change or reset at any time — after execution, on cancel or invalidation of the armed action, or on loss of the state an armed action depends on — and the client SHALL always apply it.

#### Scenario: Unknown cursor id falls back
- **WHEN** the server sends an active cursor change with an id the client has no image for
- **THEN** the client SHALL render the standard question-mark cursor

#### Scenario: Server resets the cursor
- **WHEN** the server sends an active cursor change with an empty cursor id
- **THEN** the client SHALL restore the default cursor

#### Scenario: State loss disarms
- **WHEN** a player armed `lift_down` loses the carried object (for example a forced drop)
- **THEN** the server SHALL clear the armed action and send a cursor reset

### Requirement: Armed actions execute through per-action handlers on target click
A player's armed action SHALL take the map click before ordinary click behavior and SHALL dispatch by the definition's target kind to that action's registered handler; the dispatcher SHALL be generic over action identifiers. For an object-target action, a click on a live object SHALL be offered to the handler, which validates the target against the action; an object the handler rejects SHALL consume the click with a mini-alert and leave the action armed, and a click on empty ground SHALL fall through to ordinary movement while the action stays armed. For a tile-target action, any click SHALL be offered to the handler as a target position. Successful execution SHALL clear the armed action and reset the cursor; target-level validation failure SHALL keep the action armed so the player can retry.

#### Scenario: Armed lift on a liftable object
- **WHEN** a player with `lift` armed clicks a liftable object
- **THEN** the lift flow SHALL execute (moving the player to the object first as the existing lift flow requires) and on success the carried state SHALL become active and the cursor SHALL reset

#### Scenario: Armed lift on a non-liftable object
- **WHEN** a player with `lift` armed clicks a live object that does not support lifting
- **THEN** the click SHALL be consumed with a mini-alert, the player SHALL NOT move, and `lift` SHALL remain armed

#### Scenario: Armed lift on empty ground
- **WHEN** a player with `lift` armed clicks empty ground
- **THEN** the player SHALL move toward the clicked coordinates as ordinary behavior and `lift` SHALL remain armed

#### Scenario: Armed tile action consumes any click
- **WHEN** a player with a tile-target action armed clicks anywhere in the world
- **THEN** the click position SHALL be offered to that action's handler as the target position

### Requirement: Lift is armed mode-first
The lift action SHALL no longer appear in object context menus. The only entry points SHALL be the actions menu and hotbar, and execution SHALL reuse the existing lift flow including move-to-object, validation, and the carry state broadcast.

#### Scenario: Context menu has no lift entry
- **WHEN** a player opens the context menu on a liftable object
- **THEN** no lift entry SHALL be offered

#### Scenario: Armed lift results in carrying
- **WHEN** a player arms `lift`, clicks a liftable object, and the player reaches it
- **THEN** the object SHALL be lifted exactly as the previous context-menu flow did, including the carry state broadcast

### Requirement: Put-down is manually armed while carrying
The `lift_down` action SHALL be armed only by explicit player action — the server SHALL NOT arm it automatically after a lift. While armed, the client SHALL show the placement ghost as today; a click executes put-down at the clicked position through the existing placement validation. Successful placement SHALL clear the carry state, clear the armed action, and reset the cursor; a placement rejection SHALL alert the player and keep the action armed.

#### Scenario: Successful placement
- **WHEN** a player with `lift_down` armed clicks a valid placement position
- **THEN** the object SHALL be placed, the carry state SHALL end, and the cursor SHALL reset

#### Scenario: Rejected placement stays armed
- **WHEN** a player with `lift_down` armed clicks a position the placement rules reject
- **THEN** the player SHALL receive a mini-alert, remain carrying, and remain armed with the put-down cursor

#### Scenario: No auto-arm after lifting
- **WHEN** a lift succeeds
- **THEN** the server SHALL NOT arm `lift_down` and the cursor SHALL reset to default until the player arms put-down themselves

### Requirement: Hotbar pins gameplay actions
Hotbar slots SHALL accept gameplay action identifiers in addition to the existing window-opener entries. Activating a hotbar slot holding a gameplay action SHALL send the same activation request as selecting it in the actions menu. A stored assignment referencing an action the server does not provide SHALL be treated as an empty slot.

#### Scenario: Pinned action arms from hotbar
- **WHEN** a player activates a hotbar slot pinned to `lift`
- **THEN** the same activation request SHALL be sent and the action SHALL arm exactly as from the menu

#### Scenario: Stale assignment is inert
- **WHEN** a stored hotbar assignment references an action id the server no longer provides
- **THEN** the slot SHALL behave as empty and activating it SHALL send nothing

### Requirement: Actions menu presents server actions
The actions menu SHALL list the actions from the server-provided list, using each definition's label and cursor image as its icon. Actions whose state requirement the client can observe as unmet (for example put-down while not carrying) SHALL be shown as unavailable and SHALL NOT dispatch activation locally; the server remains authoritative for all activation validation.

#### Scenario: Put-down unavailable while not carrying
- **WHEN** a player who is not carrying opens the actions menu
- **THEN** `lift_down` SHALL be visible but marked unavailable and SHALL NOT be activatable until the player carries something
