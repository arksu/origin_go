# Spec Delta

## ADDED Requirements

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

## MODIFIED Requirements

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
