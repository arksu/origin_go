# Spec Delta

## Purpose

Provide reusable, data-configured fuel burning for world objects and compatible item types.

## ADDED Requirements

### Requirement: Items declare numeric abilities
The system SHALL allow an item definition to declare zero or more named numeric abilities. Ability keys MUST be non-empty and ability values MUST be positive. The system SHALL make those values available when an item is used as fuel.

#### Scenario: Branch declares ordinary fuel
- **WHEN** the item definitions are loaded
- **THEN** `branch` SHALL expose the ability `fuel` with value `1`

#### Scenario: Invalid ability is rejected
- **WHEN** an item definition contains an empty ability key or a non-positive ability value
- **THEN** definition loading SHALL fail with an error identifying the invalid ability

### Requirement: Burner accepts configured fuel abilities
The system SHALL allow a world-object burner to declare an ordered-independent set of accepted fuel ability keys, a positive fuel capacity, a positive server-runtime duration per fuel unit, and an initial fuel amount no greater than capacity. The burner SHALL accept an item only when it has at least one accepted ability with a positive value.

#### Scenario: Campfire accepts a branch
- **WHEN** a player uses a branch in hand on a campfire configured to accept `fuel`
- **THEN** the burner SHALL accept the branch as one fuel unit

#### Scenario: Campfire rejects coal-only fuel
- **WHEN** a player uses an item that has only a `coal` ability on a campfire configured to accept only `fuel`
- **THEN** the burner SHALL not consume the item or change its fuel reserve

### Requirement: Burner sums accepted abilities and consumes the offered item
For an accepted item, the burner SHALL sum all of that item's positive ability values whose keys are in its accepted fuel-ability set. It SHALL consume the offered item exactly once and add the sum to its reserve, clamped to fuel capacity. The burner SHALL consume an accepted item even when the reserve is already full or the added amount overflows capacity.

#### Scenario: Multiple accepted abilities are summed
- **WHEN** a burner accepts `fuel` and `peat` and the offered item has `fuel: 1` and `peat: 2`
- **THEN** the burner SHALL consume the item and add `3` fuel units before applying capacity

#### Scenario: Overflow is discarded after item consumption
- **WHEN** a capacity-five burner currently has four fuel units and accepts an item contributing three units
- **THEN** the item SHALL be consumed and the burner reserve SHALL become five

#### Scenario: Full burner consumes compatible item
- **WHEN** a capacity-five burner already has five fuel units and accepts a branch
- **THEN** the branch SHALL be consumed and the burner reserve SHALL remain five

### Requirement: Burner duration uses server runtime only
The system SHALL burn one stored fuel unit after each configured duration of accumulated server runtime. Server downtime SHALL NOT advance burning. The system SHALL preserve enough state to resume the same burn schedule after server restart or world-object unload/reload.

#### Scenario: Campfire initial duration
- **WHEN** a campfire is constructed with five initial fuel units and a duration of 1,440 seconds per unit
- **THEN** it SHALL remain fueled for 7,200 seconds of accumulated server runtime unless it receives more fuel

#### Scenario: Server downtime does not burn fuel
- **WHEN** a burner is saved and the server is stopped for wall-clock time
- **THEN** its remaining fuel duration SHALL be unchanged when the server resumes

### Requirement: Empty burner performs its configured exhaustion outcome
When a burner exhausts its final fuel unit, the system SHALL apply its configured exhaustion outcome once, persist the result safely, and remove the original burner from world visibility and collision. A burner configured to drop an item SHALL create that item at the burner's former location as a normal dropped item.

#### Scenario: Campfire becomes ash
- **WHEN** a campfire's final fuel unit finishes burning
- **THEN** the campfire SHALL be removed and exactly one dropped `ash` item SHALL exist at its former location

#### Scenario: Expired unloaded burner is reconciled before exposure
- **WHEN** a world object whose persisted burner deadline has passed is loaded into an active chunk
- **THEN** its exhaustion outcome SHALL be reconciled before players can interact with the obsolete burner
