# Spec Delta

## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Activation and active state are server-owned

The client SHALL request activation by action ID. The server SHALL validate that the action exists, its skills, equipment, and handler state condition hold, and one execution is affordable before starting it. A rejected activation SHALL produce a mini-alert, SHALL NOT start the requested action, and SHALL NOT refresh the action list. A target action SHALL enter a selecting phase with its action ID and optional cursor; a none action SHALL start one execution immediately. The client SHALL receive authoritative action ID, phase, and cursor updates and SHALL NOT infer armed state from the cursor ID.

For an untimed action whose handler transitions synchronously, the server SHALL send the resulting stable phase without first sending a transient executing state. A timed action SHALL send executing when its cycle starts.

Activating the same action while selecting SHALL toggle it off. Activating a different action while selecting, approaching, or executing SHALL cancel the old action without charging stamina, stop movement initiated for its target, and then attempt the new action. If the new action cannot start, the server SHALL report the reason and remain idle.

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

## REMOVED Requirements

### Requirement: The server provides action definitions and availability

**Reason**: Per-player availability and reason synchronization is removed in favor of a static action catalog and on-demand validation.
**Migration**: Use the action catalog and the mini-alert from an attempted activation. `ActionDefinition` no longer carries `available` or `unavailable_reason`.

### Requirement: Actions menu and hotbar expose the server list

**Reason**: The old requirement makes unmet actions non-activatable and exposes precomputed reasons, which the new selectable menu removes.
**Migration**: Keep all server-listed actions selectable and close the panel on selection; rely on the server's mini-alert for a rejected activation.
