# Spec Delta

## ADDED Requirements

### Requirement: Burner appearance uses common object-state resources
When burner behavior initializes or changes a station-backed burner's shared `unlit` or `burning` state, the system SHALL set the object's client appearance resource to `{object}/unlit` or `{object}/burning`, respectively, and publish the existing appearance update when that resource changes. `{object}` SHALL be derived from the immutable object-definition key. Burner behavior and burner configuration MUST NOT contain a concrete object key, object-specific resource path, or per-object appearance field for this transition. Each object that uses these shared burner states SHALL register matching client resources under those conventional names.

#### Scenario: Unlit burner receives its conventional resource
- **WHEN** a newly constructed burner-backed object with definition key `campfire` is in the `unlit` state
- **THEN** its client appearance resource SHALL be `campfire/unlit`

#### Scenario: Ignition derives the burning resource without object-specific configuration
- **WHEN** a burner-backed object with definition key `test-hearth` transitions from `unlit` to `burning`
- **THEN** its client appearance resource SHALL become `test-hearth/burning` and no burner configuration value SHALL supply that resource path

## MODIFIED Requirements

### Requirement: Burner duration uses server runtime only
The system SHALL burn one stored fuel unit after each configured duration of accumulated server runtime while the burner is active. A burner with stored initial fuel that has not been activated SHALL preserve that fuel without scheduling or consuming it. Server downtime SHALL NOT advance active burning. The system SHALL preserve enough state to resume the same active or inactive burn state after server restart or world-object unload/reload.

#### Scenario: Campfire initial duration
- **WHEN** a campfire has five initial fuel units, a duration of 1,440 seconds per unit, and is successfully ignited
- **THEN** it SHALL remain fueled for 7,200 seconds of accumulated server runtime from ignition unless it receives more fuel

#### Scenario: Unignited initial fuel is preserved
- **WHEN** a newly constructed campfire has five initial fuel units and remains unlit
- **THEN** accumulated server runtime SHALL not reduce its fuel reserve or create an exhaustion outcome

#### Scenario: Server downtime does not burn fuel
- **WHEN** an active burner is saved and the server is stopped for wall-clock time
- **THEN** its remaining fuel duration SHALL be unchanged when the server resumes
