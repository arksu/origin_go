## MODIFIED Requirements

### Requirement: Activation and active state are server-owned

For activation through the Actions menu or hotbar, the client SHALL request activation by action ID. Secondary map-click put-down SHALL use the separate carry shortcut rather than requiring a client activation request. The server SHALL validate that the action exists, its skills, equipment, and handler state condition hold, and one execution is affordable before starting it. A rejected activation SHALL produce a mini-alert, SHALL NOT start the requested action, and SHALL NOT refresh the action list. A target action activated through the Actions menu or hotbar SHALL enter a selecting phase with its action ID and optional cursor; a none action SHALL start one execution immediately. The client SHALL receive authoritative action ID, phase, and cursor updates and SHALL NOT infer armed state from the cursor ID.

For an untimed action whose handler transitions synchronously, the server SHALL send the resulting stable phase without first sending a transient executing state. A timed action SHALL send executing when its cycle starts.

Activating the same action while selecting SHALL toggle it off. Except for active Hearth return as specified below, unless the requested action is still cooling down, activating a different action while selecting, approaching, executing, or waiting for cooldown SHALL cancel the old action without charging stamina, stop movement initiated for its target, and then attempt the new action. If the new action cannot start, the server SHALL report the reason and remain idle. Once a repeatable target action is armed, temporary lack of stamina SHALL NOT itself disarm it; target attempts SHALL still require enough stamina.

While `Travel to Hearth Fire` progress is active, a valid attempt to activate another gameplay action SHALL cancel return before checking the requested action's cooldown, requirements, affordability, or handler eligibility. A subsequent rejection SHALL NOT restore the return progress, perform its transfer, or start its cooldown. Existing activation and rejection behavior for other active actions SHALL remain unchanged.

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

#### Scenario: Cooling-down other action interrupts Hearth return
- **WHEN** a valid activation request for another gameplay action reaches the server during Hearth return and that requested action is cooling down
- **THEN** Hearth return SHALL cancel before the cooldown rejection, the requested action SHALL NOT start, and return SHALL neither resume nor start its cooldown

#### Scenario: Unavailable other action interrupts Hearth return
- **WHEN** a player progressing Hearth return attempts another gameplay action whose skills, equipment, stamina, carry state, or handler conditions are not satisfied
- **THEN** return SHALL cancel before eligibility rejection, no return transfer SHALL occur, and the server SHALL retain the requested action's ordinary rejection response

#### Scenario: Cooling-down request preserves an unrelated active action
- **WHEN** a player with an active gameplay action other than Hearth return requests another action that is cooling down
- **THEN** the requested activation SHALL be rejected without replacing the existing active action

### Requirement: Manual directional movement preserves selection and interrupts execution
Fresh valid nonzero directional input SHALL preserve an armed gameplay action in target-selection phase without selecting a target. When an action is approaching or executing, fresh directional input accepted under movement restrictions SHALL cancel that attempt through its normal cancellation lifecycle before manual movement starts. Cancellation SHALL discard pending effects and unfinished cycles, stop action-owned approach, charge no action stamina for unfinished work, and prevent late callbacks from affecting the canceled or a newer attempt. This rule SHALL also end an active context/crafting cycle through its existing cancellation and link lifecycle. Movement stamina already spent SHALL not be refunded.

Directional refreshes and releases SHALL NOT repeatedly cancel actions. Invalid, expired, obsolete-session, or movement-rejected directional input SHALL NOT cancel an action, except that fresh valid nonzero current-session input SHALL cancel active Hearth return before gameplay movement restrictions are checked. This Hearth-only exception SHALL include restrictions that reject movement while the character remains lying after knockout, is stunned, or cannot afford movement; it SHALL NOT permit the rejected movement. Malformed, stale, expired, obsolete-session, release, and unchanged-refresh input SHALL still have no Hearth cancellation effect. Merely pushing against a wall SHALL not make a canceled action eligible to finish. Directional interruption rules for other actions SHALL remain unchanged.

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

#### Scenario: Lying movement attempt interrupts Hearth return
- **WHEN** a character progressing Hearth return while lying after knockout sends fresh valid nonzero directional input for its current session
- **THEN** return SHALL cancel before the lying-state movement rejection, the character SHALL remain at its original position, and no return cooldown SHALL start

#### Scenario: Restricted movement interrupts only Hearth return
- **WHEN** fresh valid nonzero directional input is rejected for stun, stamina, energy, or carry-related movement restrictions during Hearth return
- **THEN** return SHALL cancel without granting the rejected movement, while the same restrictions SHALL retain existing interruption behavior for other gameplay actions

#### Scenario: Invalid or retired direction preserves Hearth return
- **WHEN** malformed, stale, expired, wrong-session, release, or unchanged-refresh directional input reaches a character progressing Hearth return
- **THEN** that input SHALL NOT cancel return or create movement

### Requirement: Independent persisted action cooldowns

Each menu-action definition MAY declare top-level `cooldown` as a non-negative integer in milliseconds; zero or omission disables cooldown. The server SHALL enforce a separate deadline for each character and action, starting only on successful completion at the same commit point where action costs such as stamina are charged. Selection, approach, execution progress, rejection, failure, and cancellation before completion SHALL NOT start cooldown. Cooldown SHALL block new activation, armed target attempts and direct server-routed attempts without replacing current state, except that an attempt to start another gameplay action SHALL first interrupt active Hearth return even when the requested action is cooling down.

For automatic execution repeat, every successfully completed cycle SHALL start its own cooldown when its costs are charged. The next cycle SHALL wait for that full cooldown to expire, retaining its target and generation. During `cooldown_wait`, execution animation/progress SHALL be hidden and no stamina SHALL be charged. Resume SHALL revalidate requirements, target and position; cancellation SHALL end the sequence without resumption and retain the completed cycle's cooldown.

Cooldown start and expiry times SHALL be included in ordinary character saves and restored across sessions and orderly restarts. Offline time SHALL count toward expiry. Reattachment SHALL preserve runtime deadlines, and transfer/rollback SHALL carry the current snapshot. Abrupt crashes retain the existing character-save durability guarantees.

#### Scenario: Successful completion starts the action cooldown
- **WHEN** a character successfully completes an action with a positive cooldown
- **THEN** the server SHALL start that character's cooldown for that action at the same commit point as its action costs
- **AND** new attempts of that action SHALL remain blocked until the deadline expires

#### Scenario: Interrupting return does not charge a cooldown
- **WHEN** an attempt to start a different cooling-down gameplay action cancels Hearth return before successful transfer
- **THEN** no Hearth-return cooldown SHALL start, and the requested action's existing cooldown deadline SHALL remain unchanged

### Requirement: Universal action icon presentation

All client action icon locations SHALL use `ActionIcon` and the shared action presentation interface. A single session clock and server-owned timestamps SHALL determine the clockwise dark overlay, clearing from 12 o'clock to reveal the icon. The Actions dropdown, duplicate hotbar assignments and icons mounted during cooldown SHALL show the same proportional progress. No per-icon countdown timer or click-derived cooldown SHALL exist. Cooldown SHALL block mouse, touch and keyboard activation while preserving dragging, tooltips, cancellation and hotbar clearing.

While Hearth return progresses, a user's attempt to activate a different gameplay action through any icon or hotbar shortcut SHALL convey return-cancellation intent even if the client blocks that action for cooldown. The cooling-down action SHALL remain blocked; dragging, hovering, focusing, or clearing a slot SHALL NOT count as an activation attempt. Presentation and activation behavior outside active Hearth return SHALL remain unchanged.

#### Scenario: Duplicate icons share cooldown progress
- **WHEN** an action is cooling down and its icon appears in the Actions dropdown and multiple hotbar slots
- **THEN** every icon SHALL show the same proportional cooldown overlay from the shared clock and server-owned timestamps
- **AND** activation SHALL remain blocked while dragging, tooltips, cancellation and hotbar clearing remain available

#### Scenario: Locally blocked activation still interrupts return
- **WHEN** a user progressing Hearth return attempts mouse, touch, or keyboard activation of another gameplay action whose icon is cooling down
- **THEN** the client SHALL convey cancellation so the server cancels Hearth return, SHALL NOT start the cooling-down action, and SHALL NOT infer a new return cooldown

#### Scenario: Non-activation icon interaction preserves return
- **WHEN** a user progressing Hearth return drags an action icon, views its tooltip, focuses it without activation, or clears a hotbar slot
- **THEN** that interaction SHALL NOT cancel return or activate the action

## ADDED Requirements

### Requirement: Hearth-return interruption applies to shared gameplay ingress
While Hearth return progresses, valid attempts to start another gameplay action or fresh manual movement SHALL cancel it before eligibility checks. This SHALL include context execution, crafting, building, carry attempts, and movement clicks. Chat and personal-window opening SHALL not cancel it. Cancellation SHALL discard unfinished return work without transfer or cooldown. Malformed, stale, expired, or wrong-session input SHALL have no cancellation effect.

#### Scenario: Rejected execution requests interrupt return
- **WHEN** a valid context-action selection, crafting execution, or building execution request is made during Hearth return and ordinary gameplay validation rejects it
- **THEN** return SHALL already be canceled, the request SHALL retain its ordinary rejection behavior, and no old return completion SHALL transfer the player or start its cooldown

#### Scenario: Rejected carry attempt interrupts return
- **WHEN** a valid direct put-down attempt is made during Hearth return and ordinary placement or carry eligibility rejects it
- **THEN** return SHALL cancel before rejection while valid carry state and the normal placement error SHALL be preserved

#### Scenario: Ground movement without displacement interrupts return
- **WHEN** a valid ordinary primary ground click requests movement during Hearth return but collision, lying, stun, movement affordability, or an unchanged destination prevents displacement
- **THEN** return SHALL cancel before movement eligibility or displacement and the same click SHALL retain ordinary routing without granting forbidden movement

#### Scenario: Invalid input cannot interrupt return
- **WHEN** malformed, stale, expired, wrong-session, or unsupported-button input reaches a character progressing Hearth return
- **THEN** that input SHALL NOT cancel return, move the character, or consume pending administrator state

#### Scenario: Chat and personal windows preserve return
- **WHEN** a character progressing Hearth return sends chat or opens a personal window without requesting another gameplay action
- **THEN** those operations SHALL NOT cancel return or restart its progress

#### Scenario: Administrator-consumed clicks remain isolated
- **WHEN** a pending administrator command consumes a valid primary map click during Hearth return without requesting ordinary gameplay movement or another gameplay action
- **THEN** the click SHALL retain its existing administrator routing and SHALL NOT additionally cancel return through the ordinary movement rule

#### Scenario: Unrelated gameplay ingress retains its behavior
- **WHEN** the same gameplay or movement request reaches a player without active Hearth return
- **THEN** the existing action interruption, rejection, movement, and interaction rules SHALL remain unchanged
