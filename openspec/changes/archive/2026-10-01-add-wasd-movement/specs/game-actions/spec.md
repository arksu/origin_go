# Spec Delta

## ADDED Requirements

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
