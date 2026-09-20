# Spec Delta

## Purpose

Allow a constructed campfire to wait safely for a player to ignite it before its fuel and cooking state become active.

## ADDED Requirements

### Requirement: New campfires wait unlit for player ignition
The system SHALL create each newly completed campfire in the `unlit` station state with its configured initial fuel reserve intact and without active fuel consumption. While it remains this initial unlit campfire, its only context action SHALL be `Light my fire`.

#### Scenario: Built campfire offers only ignition
- **WHEN** a player targets a newly completed unlit campfire
- **THEN** the available context-action list SHALL contain exactly `Light my fire`

#### Scenario: Campfire appearance follows station state
- **WHEN** a campfire is created unlit or successfully ignited
- **THEN** visible clients SHALL receive the existing appearance update using `campfire/unlit` or `campfire/burning`, respectively

#### Scenario: Unlit campfire does not cook
- **WHEN** a campfire has been completed but has not been successfully ignited
- **THEN** a craft requiring a burning cooking station SHALL not be eligible at that campfire

### Requirement: Ignition uses one normal cyclic action
The system SHALL start exactly one target-linked cyclic action when a player selects `Light my fire` on an eligible unlit campfire. The action SHALL use the existing context-action movement, active-action, progress, cancellation, and completion flow; it SHALL not consume fuel, inventory items, or stamina before its cycle completes.

#### Scenario: Selecting ignition starts one cycle
- **WHEN** a player selects `Light my fire` and reaches the campfire through the normal context-action flow
- **THEN** the player SHALL enter one cyclic action targeting that campfire

#### Scenario: Interrupted ignition leaves the campfire unlit
- **WHEN** the ignition action is canceled before completing its cycle
- **THEN** the campfire SHALL remain unlit and its initial fuel reserve and player stamina SHALL remain unchanged

### Requirement: Successful ignition spends stamina and starts burning atomically
At successful completion of the one ignition cycle, the system SHALL verify that the player can pay 50 stamina. If payment succeeds, it SHALL deduct exactly 50 stamina, change the campfire to `burning`, and activate its initial fuel reserve for normal burner consumption as one successful ignition outcome. If the player cannot pay the stamina at completion, the system SHALL cancel the action without changing the campfire or stamina.

#### Scenario: Ignition succeeds with sufficient stamina
- **WHEN** an eligible player's ignition cycle completes and the player has at least 50 stamina available
- **THEN** exactly 50 stamina SHALL be deducted, the campfire SHALL become burning, and its fuel schedule SHALL begin

#### Scenario: Ignition fails with insufficient stamina
- **WHEN** an ignition cycle completes and the player cannot pay 50 stamina
- **THEN** the campfire SHALL remain unlit, its initial fuel reserve SHALL remain unchanged, and no stamina SHALL be deducted
