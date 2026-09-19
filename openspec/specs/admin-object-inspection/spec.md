# admin-object-inspection Specification

## Purpose

Lets administrators query a selected world object's live mutable state from the game chat without exposing its immutable definition template.

## Requirements

### Requirement: Administrator can arm one object inspection
The system MUST recognize `/info` as an administrator command with no arguments and arm exactly one object-inspection attempt for that administrator. It MUST tell the administrator to click an object, and it MUST replace any other pending administrator click action for that player.

#### Scenario: Inspection is armed
- **WHEN** an administrator sends `/info`
- **THEN** the system SHALL prompt that administrator to click an object and await one object-selection attempt

#### Scenario: Command arguments are rejected
- **WHEN** an administrator sends `/info` with one or more arguments
- **THEN** the system SHALL report the command usage and SHALL NOT arm inspection

### Requirement: One-shot object selection reports authoritative runtime state
While inspection is armed, the next primary click on a currently visible object MUST select that object's entity for inspection rather than performing the normal object interaction. The server MUST resolve the entity against the authoritative live world before reporting its state.

#### Scenario: Burner fuel is reported
- **WHEN** an administrator arms inspection and selects a live campfire with burner fuel remaining
- **THEN** the chat report SHALL include the campfire's current `burner` behavior state and its `fuel` value

#### Scenario: Target disappears before inspection
- **WHEN** an administrator selects an object that no longer exists in the authoritative world
- **THEN** the system SHALL report that the target is unavailable and SHALL NOT inspect another object

### Requirement: Inspection report excludes immutable template data
The inspection report MUST identify the selected entity and present its mutable object runtime state, including behavior-owned state and other current object parameters. It MUST NOT include fields read from the immutable object definition template solely for the report.

#### Scenario: Template configuration is omitted
- **WHEN** an administrator inspects an object configured by an object template
- **THEN** the report SHALL omit template configuration such as behavior definitions, fuel capacity, burn duration, appearance definitions, and immutable object metadata

#### Scenario: No mutable runtime state is present
- **WHEN** an administrator inspects a live object with no mutable object runtime state
- **THEN** the system SHALL report that no mutable runtime state is available without failing the command

### Requirement: Inspection selection is transient and isolated
The inspection attempt MUST be cleared after its first selection attempt, after an invalid non-object click, and when the player leaves the active world. It MUST be isolated per administrator.

#### Scenario: Ground click consumes the attempt
- **WHEN** an administrator has armed inspection and primary-clicks empty ground
- **THEN** the system SHALL report that an object is required and SHALL clear that administrator's pending inspection

#### Scenario: One administrator does not consume another's inspection
- **WHEN** two administrators each arm inspection and one selects an object
- **THEN** only the selecting administrator's inspection SHALL be cleared and reported
